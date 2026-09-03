package v2

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"text/template"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// --- 基础数据结构 ---

// NotificationJob 定义了一个通知任务
type NotificationJob struct {
	ID         string                 `json:"id"`
	Title      string                 `json:"title"`
	Content    string                 `json:"content"`      // 原始内容
	TemplateID string                 `json:"template_id"`  // 选用的模板
	Params     map[string]interface{} `json:"params"`       // 模板参数
	Target     string                 `json:"target"`       // 特定目标渠道，为空则全频道
	
	RetryLimit int                    `json:"retry_limit"`  // 最大重试次数
	RetryCount int                    `json:"retry_count"`  // 当前已重试次数
	LastError  error                  `json:"-"`
	NextRunAt  time.Time              `json:"-"`
}

// DLQStore 死信队列存储接口，允许持久化失败消息
type DLQStore interface {
	Store(job *NotificationJob) error
}

// DefaultDLQ 默认死信处理器（仅打日志）
type DefaultDLQ struct{}
func (d *DefaultDLQ) Store(job *NotificationJob) error {
	log.Printf("💀 [DLQ] 永久失败任务: ID=%s, Title=%s, Error=%v", job.ID, job.Title, job.LastError)
	return nil
}

// --- 核心调度器 ---

// AsyncCenter 高级异步通知中心
type AsyncCenter struct {
	notifiers map[string]core.Notifier
	templates map[string]*template.Template
	
	jobQueue  chan *NotificationJob
	retryChan chan *NotificationJob
	dlq       DLQStore
	
	workerCnt int
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	
	mu sync.RWMutex
}

// NewAsyncCenter 创建新的通知中心
func NewAsyncCenter(workerCount int, dlq DLQStore) *AsyncCenter {
	if dlq == nil {
		dlq = &DefaultDLQ{}
	}
	return &AsyncCenter{
		notifiers: make(map[string]core.Notifier),
		templates: make(map[string]*template.Template),
		jobQueue:  make(chan *NotificationJob, 1000),
		retryChan: make(chan *NotificationJob, 500),
		dlq:       dlq,
		workerCnt: workerCount,
	}
}

// RegisterNotifier 注册发送通道
func (ac *AsyncCenter) RegisterNotifier(n core.Notifier) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.notifiers[n.Name()] = n
}

// RegisterTemplate 注册消息模板
func (ac *AsyncCenter) RegisterTemplate(id, tplStr string) error {
	t, err := template.New(id).Parse(tplStr)
	if err != nil {
		return err
	}
	ac.mu.Lock()
	ac.templates[id] = t
	ac.mu.Unlock()
	return nil
}

// Start 启动后台处理引擎
func (ac *AsyncCenter) Start(ctx context.Context) {
	ac.ctx, ac.cancel = context.WithCancel(ctx)
	
	// 启动工作线程池
	for i := 0; i < ac.workerCnt; i++ {
		ac.wg.Add(1)
		go ac.worker(i)
	}
	
	// 启动重试调度
	go ac.retryLoop()
	
	log.Printf("📡 异步通知中心已就绪 (Workers: %d)", ac.workerCnt)
}

// Stop 优雅停机
func (ac *AsyncCenter) Stop() {
	ac.cancel()
	close(ac.jobQueue)
	ac.wg.Wait()
	log.Println("🔌 异步通知中心已安全关闭")
}

// Submit 提交通知任务
func (ac *AsyncCenter) Submit(job *NotificationJob) {
	if job.RetryLimit <= 0 {
		job.RetryLimit = 3 // 默认重试3次
	}
	
	select {
	case ac.jobQueue <- job:
	default:
		log.Printf("⚠️  通知中心队列溢出，丢弃任务: %s", job.Title)
	}
}

// --- 内部逻辑 ---

func (ac *AsyncCenter) worker(id int) {
	defer ac.wg.Done()
	for {
		select {
		case <-ac.ctx.Done():
			return
		case job, ok := <-ac.jobQueue:
			if !ok {
				return
			}
			ac.processJob(job)
		}
	}
}

func (ac *AsyncCenter) processJob(job *NotificationJob) {
	// 1. 处理模板渲染
	content := job.Content
	if job.TemplateID != "" {
		ac.mu.RLock()
		tpl, ok := ac.templates[job.TemplateID]
		ac.mu.RUnlock()
		if ok {
			var buf bytes.Buffer
			if err := tpl.Execute(&buf, job.Params); err == nil {
				content = buf.String()
			} else {
				log.Printf("❌ 模板执行失败: %v", err)
			}
		}
	}

	// 2. 路由分发
	ac.mu.RLock()
	var targets []core.Notifier
	if job.Target != "" {
		if n, ok := ac.notifiers[job.Target]; ok {
			targets = append(targets, n)
		}
	} else {
		for _, n := range ac.notifiers {
			targets = append(targets, n)
		}
	}
	ac.mu.RUnlock()

	// 3. 执行并行分发
	var swg sync.WaitGroup
	for _, n := range targets {
		swg.Add(1)
		go func(notifier core.Notifier) {
			defer swg.Done()
			
			// 设置单次发送的超时
			ctx, cancel := context.WithTimeout(ac.ctx, 15*time.Second)
			defer cancel()
			
			err := notifier.Send(ctx, job.Title, content)
			if err != nil {
				ac.handleFailure(notifier.Name(), job, err)
			}
		}(n)
	}
	swg.Wait()
}

func (ac *AsyncCenter) handleFailure(channel string, job *NotificationJob, err error) {
	log.Printf("❌ [%s] 发送失败: %v (重试 %d/%d)", channel, err, job.RetryCount, job.RetryLimit)
	
	if job.RetryCount >= job.RetryLimit {
		job.LastError = fmt.Errorf("[%s] %w", channel, err)
		_ = ac.dlq.Store(job)
		return
	}

	// 复制任务，专门为该失败渠道进行独立重试
	retryJob := *job
	retryJob.RetryCount++
	retryJob.Target = channel
	retryJob.LastError = err
	
	// 指数退避策略 + 抖动 (Jitter)
	backoff := int(1 << uint(retryJob.RetryCount))
	delay := time.Duration(backoff)*time.Second + time.Duration(rand.Intn(1000))*time.Millisecond
	retryJob.NextRunAt = time.Now().Add(delay)

	select {
	case ac.retryChan <- &retryJob:
	default:
		log.Printf("⚠️  通知重试队列已满，转移至死信队列: %s", retryJob.Title)
		_ = ac.dlq.Store(&retryJob)
	}
}

func (ac *AsyncCenter) retryLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var waitingJobs []*NotificationJob
	var mu sync.Mutex

	// 后台接收重试任务
	go func() {
		for {
			select {
			case <-ac.ctx.Done():
				return
			case j := <-ac.retryChan:
				mu.Lock()
				waitingJobs = append(waitingJobs, j)
				mu.Unlock()
			}
		}
	}()

	for {
		select {
		case <-ac.ctx.Done():
			return
		case <-ticker.C:
			mu.Lock()
			now := time.Now()
			var stillWaiting []*NotificationJob
			for _, j := range waitingJobs {
				if now.After(j.NextRunAt) {
					// 重新加入主队列
					select {
					case ac.jobQueue <- j:
					default:
						stillWaiting = append(stillWaiting, j)
					}
				} else {
					stillWaiting = append(stillWaiting, j)
				}
			}
			waitingJobs = stillWaiting
			mu.Unlock()
		}
	}
}
