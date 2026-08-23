package v2

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
)

// ============================================================
// 通知消息结构
// ============================================================
type NotifyMessage struct {
	ID         string
	Title      string
	Content    string
	EventType  string // 事件类型：download.started, download.completed, file.organized, episode.missing, error
	Target     string // 指定渠道，为空则全渠道推送
	Priority   int    // 优先级：0=普通，1=高，2=紧急

	RetryCount int
	MaxRetries int
	NextRunAt  time.Time
	CreatedAt  time.Time
	LastError  string
}

// ============================================================
// 通知管理器：异步分发 + 重试 + 事件订阅
// ============================================================
type NotifyManager struct {
	notifiers   []Notifier
	bus         *event.Bus
	queue       chan *NotifyMessage
	retryChan   chan *NotifyMessage
	dlq         chan *NotifyMessage // 死信队列

	workerCount int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc

	mu    sync.RWMutex
	stats map[string]int64

	// 事件路由规则：事件类型 -> 目标渠道列表（空=全渠道）
	routeRules map[string][]string
}

func NewNotifyManager(notifiers []Notifier, bus *event.Bus) *NotifyManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &NotifyManager{
		notifiers:   notifiers,
		bus:         bus,
		queue:       make(chan *NotifyMessage, 5000),
		retryChan:   make(chan *NotifyMessage, 1000),
		dlq:         make(chan *NotifyMessage, 1000),
		workerCount: 4,
		ctx:         ctx,
		cancel:      cancel,
		stats:       make(map[string]int64),
		routeRules: map[string][]string{
			"download.started":    {},     // 全渠道
			"download.completed":  {},     // 全渠道
			"file.organized":      {},     // 全渠道
			"episode.missing":     {},     // 全渠道
			"error":               {},     // 全渠道
		},
	}
}

// Start 启动通知管理器
func (m *NotifyManager) Start() {
	m.wg.Add(1)
	go m.worker()

	// 重试调度器
	m.wg.Add(1)
	go m.retryScheduler()

	// 死信队列处理
	m.wg.Add(1)
	go m.handleDLQ()

	// 订阅事件总线
	m.subscribeEvents()

	log.Printf("🚀 通知中心启动: Workers=%d, Providers=%v", m.workerCount, m.getProviderNames())
}

// Stop 优雅关闭
func (m *NotifyManager) Stop() {
	m.cancel()
	close(m.queue)
	m.wg.Wait()
	log.Println("🛑 通知中心已关闭")
}

// subscribeEvents 订阅核心业务事件
func (m *NotifyManager) subscribeEvents() {
	if m.bus == nil {
		log.Println("⚠️ EventBus 未初始化，通知功能不可用")
		return
	}

	// 下载开始
	m.bus.Subscribe(core.EventDownloadStarted, func(e core.Event) {
		title := "📥 开始下载"
		msg := e.Payload["title"].(string)
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "download.started",
			Priority:   0,
			MaxRetries: 3,
		})
	})

	// 下载完成
	m.bus.Subscribe(core.EventDownloadCompleted, func(e core.Event) {
		title := "✅ 下载完成"
		data := e.Payload
		msg := fmt.Sprintf("标题: %s\n大小: %s\n耗时: %s",
			data["title"], formatBytes(data["size"].(int64)), data["duration"])
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "download.completed",
			Priority:   1,
			MaxRetries: 3,
		})
	})

	// 文件整理完成
	m.bus.Subscribe(core.EventFileOrganized, func(e core.Event) {
		title := "📁 文件已整理"
		data := e.Payload
		msg := fmt.Sprintf("最终路径: %s", data["final_path"])
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "file.organized",
			Priority:   1,
			MaxRetries: 3,
		})
	})

	// 补全缺失集数
	m.bus.Subscribe(core.EventSupplementCompleted, func(e core.Event) {
		title := "⚠️ 发现缺失集数"
		data := e.Payload
		msg := fmt.Sprintf("番剧: %s\n缺失: S%02dE%02d",
			data["title"], data["season"], data["episode"])
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "episode.missing",
			Priority:   1,
			MaxRetries: 3,
		})
	})

	// 错误事件（通用）
	m.bus.Subscribe(core.EventDownloadFailed, func(e core.Event) {
		title := "🚨 系统错误"
		msg := e.Payload["message"].(string)
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "error",
			Priority:   2,
			MaxRetries: 5,
		})
	})

	log.Println("📡 已订阅事件: download.started, download.completed, file.organized, episode.missing, error")
}

// Publish 发布通知消息（非阻塞）
func (m *NotifyManager) Publish(msg *NotifyMessage) {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.MaxRetries == 0 {
		msg.MaxRetries = 3
	}

	// 根据事件类型确定目标渠道
	if targets, ok := m.routeRules[msg.EventType]; ok && len(targets) > 0 {
		msg.Target = targets[0] // 简化：取第一个，实际可扩展为多目标
	}

	select {
	case m.queue <- msg:
	case <-time.After(50 * time.Millisecond):
		log.Printf("⚠️ 通知队列繁忙，丢弃消息: %s", msg.Title)
	}
}

// SendTest 发送测试消息（同步，用于 WebUI 测试按钮）
func (m *NotifyManager) SendTest(ctx context.Context, channel, title, message string) error {
	var targets []Notifier
	for _, n := range m.notifiers {
		if channel == "" || channel == n.Name() {
			targets = append(targets, n)
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("未找到通知渠道: %s", channel)
	}

	var errs []string
	var wg sync.WaitGroup
	for _, n := range targets {
		wg.Add(1)
		go func(notifier Notifier) {
			defer wg.Done()
			if err := notifier.Send(ctx, title, message); err != nil {
				errs = append(errs, fmt.Sprintf("[%s] %v", notifier.Name(), err))
			}
		}(n)
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// worker 处理协程
func (m *NotifyManager) worker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case msg, ok := <-m.queue:
			if !ok {
				return
			}
			m.process(msg)
		}
	}
}

// process 执行分发
func (m *NotifyManager) process(msg *NotifyMessage) {
	// 确定目标 Notifiers
	var targets []Notifier
	for _, n := range m.notifiers {
		if msg.Target == "" || msg.Target == n.Name() {
			targets = append(targets, n)
		}
	}

	if len(targets) == 0 {
		log.Printf("⚠️ 无可用通知渠道: %s", msg.Target)
		return
	}

	// 并行推送
	var innerWg sync.WaitGroup
	for _, n := range targets {
		innerWg.Add(1)
		go func(notifier Notifier) {
			defer innerWg.Done()
			err := notifier.Send(m.ctx, msg.Title, msg.Content)
			if err != nil {
				m.handleFailure(notifier, msg, err)
			} else {
				m.incStat(notifier.Name() + ".success")
			}
		}(n)
	}
	innerWg.Wait()
}

// handleFailure 处理发送失败，指数退避重试
func (m *NotifyManager) handleFailure(n Notifier, msg *NotifyMessage, err error) {
	log.Printf("⚠️ [%s] 发送失败: %v (重试 %d/%d)", n.Name(), err, msg.RetryCount, msg.MaxRetries)
	m.incStat(n.Name() + ".failure")

	if msg.RetryCount >= msg.MaxRetries {
		m.dlq <- msg
		return
	}

	retryMsg := *msg
	retryMsg.RetryCount++
	retryMsg.Target = n.Name() // 只重试失败的渠道
	retryMsg.LastError = err.Error()
	delay := time.Duration(1<<uint(retryMsg.RetryCount)) * 10 * time.Second
	retryMsg.NextRunAt = time.Now().Add(delay)

	m.retryChan <- &retryMsg
}

// retryScheduler 重试调度器
func (m *NotifyManager) retryScheduler() {
	defer m.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var retryList []*NotifyMessage
	var mu sync.Mutex

	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return
			case msg := <-m.retryChan:
				mu.Lock()
				retryList = append(retryList, msg)
				mu.Unlock()
			}
		}
	}()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			mu.Lock()
			now := time.Now()
			remaining := retryList[:0]
			for _, msg := range retryList {
				if now.After(msg.NextRunAt) {
					m.queue <- msg
				} else {
					remaining = append(remaining, msg)
				}
			}
			retryList = remaining
			mu.Unlock()
		}
	}
}

// handleDLQ 死信队列
func (m *NotifyManager) handleDLQ() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case msg := <-m.dlq:
			log.Printf("💀 [DLQ] 永久失败: Title=%s, Target=%s, Err=%s", msg.Title, msg.Target, msg.LastError)
			// TODO: 持久化到数据库
		}
	}
}

// GetStats 获取统计信息
func (m *NotifyManager) GetStats() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := make(map[string]int64)
	for k, v := range m.stats {
		stats[k] = v
	}
	return stats
}

// ReloadNotifiers 热重载通知器（配置变更时调用）
func (m *NotifyManager) ReloadNotifiers(newNotifiers []Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers = newNotifiers
	log.Printf("🔄 通知器已热重载: %v", m.getProviderNames())
}

// Notifiers 返回当前通知器列表（供外部检查）
func (m *NotifyManager) Notifiers() []Notifier {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Notifier, len(m.notifiers))
	copy(result, m.notifiers)
	return result
}

func (m *NotifyManager) getProviderNames() []string {
	var names []string
	for _, n := range m.notifiers {
		names = append(names, n.Name())
	}
	return names
}

func (m *NotifyManager) incStat(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats[key]++
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}