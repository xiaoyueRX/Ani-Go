package notifier

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// Message 内部通知消息结构
type Message struct {
	ID         string                 `json:"id"`
	Title      string                 `json:"title"`
	Content    string                 `json:"content"`
	TemplateID string                 `json:"template_id"`
	Data       map[string]interface{} `json:"data"`
	Target     string                 `json:"target"` // 指定渠道，为空则全频道推送
	
	// 重试与元数据
	RetryCount int       `json:"retry_count"`
	MaxRetries int       `json:"max_retries"`
	NextRunAt  time.Time `json:"next_run_at"`
	CreatedAt  time.Time `json:"created_at"`
	LastError  string    `json:"last_error"`
}

// Dispatcher 核心异步分发器
type Dispatcher struct {
	notifiers []core.Notifier
	queue     chan *Message
	retryChan chan *Message
	dlq       chan *Message
	tm        *TemplateManager
	
	workerCount int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	
	// 性能统计
	mu      sync.RWMutex
	stats   map[string]int64
}

// NewDispatcher 创建高性能分发器
func NewDispatcher(tm *TemplateManager, notifiers ...core.Notifier) *Dispatcher {
	return &Dispatcher{
		notifiers:   notifiers,
		queue:       make(chan *Message, 5000), // 高容量主队列
		retryChan:   make(chan *Message, 1000), // 独立重试队列
		dlq:         make(chan *Message, 1000), // 死信队列
		tm:          tm,
		workerCount: 8,                         // 默认并发数
		stats:       make(map[string]int64),
	}
}

// Start 启动处理引擎
func (d *Dispatcher) Start(ctx context.Context) {
	d.ctx, d.cancel = context.WithCancel(ctx)
	
	// 启动 Worker 池
	for i := 0; i < d.workerCount; i++ {
		d.wg.Add(1)
		go d.worker(i)
	}
	
	// 启动重试调度器 (非阻塞重试的核心)
	go d.retryScheduler()
	
	// 启动死信队列处理器
	go d.handleDLQ()
	
	log.Printf("🚀 通知中心启动: Workers=%d, Providers=%v", d.workerCount, d.getProviderNames())
}

// Stop 优雅关闭
func (d *Dispatcher) Stop() {
	d.cancel()
	close(d.queue)
	d.wg.Wait()
	log.Println("🛑 通知中心已关闭")
}

// Publish 发布消息 (非阻塞)
func (d *Dispatcher) Publish(msg *Message) {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.MaxRetries == 0 {
		msg.MaxRetries = 3
	}
	
	select {
	case d.queue <- msg:
	case <-time.After(50 * time.Millisecond):
		log.Printf("⚠️  队列繁忙，丢弃消息: %s", msg.Title)
	}
}

// worker 处理协程
func (d *Dispatcher) worker(id int) {
	defer d.wg.Done()
	for {
		select {
		case <-d.ctx.Done():
			return
		case msg, ok := <-d.queue:
			if !ok {
				return
			}
			d.process(msg)
		}
	}
}

// process 执行渲染与并行分发
func (d *Dispatcher) process(msg *Message) {
	// 1. 模板渲染 (Lazy Rendering)
	if msg.Content == "" && msg.TemplateID != "" {
		rendered, err := d.tm.Render(msg.TemplateID, msg.Data)
		if err != nil {
			log.Printf("❌ 模板渲染失败 [%s]: %v", msg.TemplateID, err)
			return
		}
		msg.Content = rendered
	}

	// 2. 确定目标 Notifiers
	var targets []core.Notifier
	for _, n := range d.notifiers {
		if msg.Target == "" || msg.Target == n.Name() {
			targets = append(targets, n)
		}
	}

	// 3. 并行推送给各端
	var innerWg sync.WaitGroup
	for _, n := range targets {
		innerWg.Add(1)
		go func(notifier core.Notifier) {
			defer innerWg.Done()
			
			// 单端推送逻辑
			err := notifier.Send(d.ctx, msg.Title, msg.Content)
			if err != nil {
				d.handleFailure(notifier, msg, err)
			} else {
				d.incStat(notifier.Name() + ".success")
			}
		}(n)
	}
	innerWg.Wait()
}

// handleFailure 处理发送失败，实现指数退避重试逻辑
func (d *Dispatcher) handleFailure(n core.Notifier, msg *Message, err error) {
	log.Printf("⚠️  [%s] 发送失败: %v (重试 %d/%d)", n.Name(), err, msg.RetryCount, msg.MaxRetries)
	d.incStat(n.Name() + ".failure")

	if msg.RetryCount >= msg.MaxRetries {
		d.dlq <- msg
		return
	}

	// 创建一个克隆消息用于该渠道的独立重试 (避免全端重复推送)
	retryMsg := *msg
	retryMsg.RetryCount++
	retryMsg.Target = n.Name()
	retryMsg.LastError = err.Error()
	
	// 指数退避: 2^retry * 10s (10s, 20s, 40s...)
	delay := time.Duration(1<<uint(retryMsg.RetryCount)) * 10 * time.Second
	retryMsg.NextRunAt = time.Now().Add(delay)

	d.retryChan <- &retryMsg
}

// retryScheduler 负责等待并重新投递需要重试的消息
func (d *Dispatcher) retryScheduler() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var retryList []*Message
	var mu sync.Mutex

	// 接收协程
	go func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			case m := <-d.retryChan:
				mu.Lock()
				retryList = append(retryList, m)
				mu.Unlock()
			}
		}
	}()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			mu.Lock()
			now := time.Now()
			remaining := retryList[:0]
			for _, m := range retryList {
				if now.After(m.NextRunAt) {
					d.queue <- m // 重新进入主队列
				} else {
					remaining = append(remaining, m)
				}
			}
			retryList = remaining
			mu.Unlock()
		}
	}
}

// handleDLQ 死信队列持久化逻辑 (此处示例为日志输出，实际可接入 SQLite)
func (d *Dispatcher) handleDLQ() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case msg := <-d.dlq:
			// TODO: 写入数据库 internal/database
			log.Printf("💀 [DLQ] 永久失败消息: Title=%s, Target=%s, LastErr=%s", 
				msg.Title, msg.Target, msg.LastError)
		}
	}
}

func (d *Dispatcher) getProviderNames() []string {
	var names []string
	for _, n := range d.notifiers {
		names = append(names, n.Name())
	}
	return names
}

func (d *Dispatcher) incStat(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats[key]++
}
