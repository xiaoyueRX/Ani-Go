package v2

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
)

// ============================================================
// 通知消息结构
// ============================================================
type NotifyMessage struct {
	ID        string
	Title     string
	Content   string
	EventType string // 事件类型：download.started, download.completed, file.organized, episode.missing, error
	Target    string // 指定渠道，为空则全渠道推送
	Priority  int    // 优先级：0=普通，1=高，2=紧急

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
	notifiers []Notifier
	bus       *event.Bus
	queue     chan *NotifyMessage
	retryChan chan *NotifyMessage
	dlq       chan *NotifyMessage // 死信队列
	logQueue  chan *database.NotificationLog // 日志持久化队列

	workerCount int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc

	mu          sync.RWMutex
	enabled     bool
	running     bool
	eventSubs   []eventSubRef
	stats       map[string]int64
	reloader    func() []Notifier

	// 事件路由规则：事件类型 -> 目标渠道列表（空=全渠道）
	routeRules map[string][]string
}

type eventSubRef struct {
	evType string
	subID  core.SubscriptionID
}

func NewNotifyManager(notifiers []Notifier, bus *event.Bus) *NotifyManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &NotifyManager{
		notifiers:   notifiers,
		bus:         bus,
		queue:       make(chan *NotifyMessage, 5000),
		retryChan:   make(chan *NotifyMessage, 1000),
		dlq:         make(chan *NotifyMessage, 1000),
		logQueue:    make(chan *database.NotificationLog, 1000),
		workerCount: 4,
		ctx:         ctx,
		cancel:      cancel,
		enabled:     true,
		stats:       make(map[string]int64),
		routeRules: map[string][]string{
			"download.started":   {}, // 全渠道
			"download.completed": {}, // 全渠道
			"file.organized":     {}, // 全渠道
			"episode.missing":    {}, // 全渠道
			"error":              {}, // 全渠道
		},
	}
}

// SetReloader 设置通知器加载函数
func (m *NotifyManager) SetReloader(reloader func() []Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloader = reloader
}

// Reload 重新加载全渠道通知器
func (m *NotifyManager) Reload() {
	m.mu.RLock()
	reloader := m.reloader
	m.mu.RUnlock()
	if reloader != nil {
		m.ReloadNotifiers(reloader())
	}
}

// SetEnabled 设置通知中心启停状态（完全控制底层协程生命周期与事件监听）
func (m *NotifyManager) SetEnabled(enabled bool) {
	if enabled {
		m.Start()
	} else {
		m.Stop()
	}
}

// IsEnabled 获取通知中心启停状态
func (m *NotifyManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled && m.running
}

// Start 启动通知管理器（保证幂等与协程安全）
func (m *NotifyManager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.queue = make(chan *NotifyMessage, 5000)
	m.retryChan = make(chan *NotifyMessage, 1000)
	m.dlq = make(chan *NotifyMessage, 1000)
	m.logQueue = make(chan *database.NotificationLog, 1000)
	m.running = true
	m.enabled = true
	m.mu.Unlock()

	m.wg.Add(4)
	go m.worker()
	go m.retryScheduler()
	go m.handleDLQ()
	go m.handleLogPersist()

	m.subscribeEvents()
	log.Printf("🚀 通知中心启动: Workers=%d, Providers=%v", m.workerCount, m.getProviderNames())
}

// Stop 优雅关闭通知管理器并回收所有后台协程
func (m *NotifyManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.enabled = false
	if m.cancel != nil {
		m.cancel()
	}
	m.unsubscribeEvents()
	close(m.queue)
	close(m.logQueue)
	m.mu.Unlock()

	m.wg.Wait()
	log.Println("🛑 通知中心已关闭（0 协程/0 网络消耗）")
}

// subscribeEvents 订阅核心业务事件
func (m *NotifyManager) subscribeEvents() {
	if m.bus == nil {
		log.Println("⚠️ EventBus 未初始化，通知功能不可用")
		return
	}
	m.unsubscribeEvents()

	sub := func(evType string, handler core.EventHandler) {
		id := m.bus.Subscribe(evType, handler)
		m.eventSubs = append(m.eventSubs, eventSubRef{evType: evType, subID: id})
	}

	// 下载开始
	sub(core.EventDownloadStarted, func(e core.Event) {
		title := "📥 开始下载"
		msg := fmt.Sprintf("%v", e.Payload["title"])
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "download.started",
			Priority:   0,
			MaxRetries: 3,
		})
	})

	// 下载完成
	sub(core.EventDownloadCompleted, func(e core.Event) {
		title := "✅ 下载完成"
		data := e.Payload

		var sizeStr string
		if s, ok := data["size"].(int64); ok {
			sizeStr = formatBytes(s)
		} else if s, ok := data["size"].(int); ok {
			sizeStr = formatBytes(int64(s))
		} else if s, ok := data["size"].(float64); ok {
			sizeStr = formatBytes(int64(s))
		} else {
			sizeStr = "未知"
		}

		animeTitle, _ := data["anime_title"].(string)
		if animeTitle == "" {
			animeTitle = fmt.Sprintf("%v", data["title"])
		}
		epVal := data["episode"]
		var epText string
		if epVal != nil {
			epText = fmt.Sprintf(" 第 %v 集", epVal)
		}

		msg := fmt.Sprintf("番剧: 《%s》%s\n文件大小: %s\n下载耗时: %s",
			animeTitle, epText, sizeStr, data["duration"])
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "download.completed",
			Priority:   1,
			MaxRetries: 3,
		})
	})

	// 文件整理结果（每个整理批次发送一次）
	sub(core.EventFileOrganized, func(e core.Event) {
		m.Publish(organizedNotification(e.Payload))
	})

	// 补全缺失集数
	sub(core.EventSupplementCompleted, func(e core.Event) {
		data := e.Payload
		if data == nil {
			return
		}
		newCount, _ := data["new_count"].(int)
		if newCount <= 0 {
			// 没有实际新增的补全任务，不打扰用户
			return
		}
		title := "📥 历史剧集补全"
		msg := fmt.Sprintf("番剧: %v\n已自动识别并添加 %d 个缺失集数下载任务", data["title"], newCount)
		m.Publish(&NotifyMessage{
			Title:      title,
			Content:    msg,
			EventType:  "episode.missing",
			Priority:   1,
			MaxRetries: 3,
		})
	})

	// 错误事件（通用）
	sub(core.EventDownloadFailed, func(e core.Event) {
		title := "🚨 系统错误"
		msg := fmt.Sprintf("%v", e.Payload["message"])
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

// unsubscribeEvents 注销所有事件监听
func (m *NotifyManager) unsubscribeEvents() {
	if m.bus == nil {
		return
	}
	for _, s := range m.eventSubs {
		m.bus.Unsubscribe(s.evType, s.subID)
	}
	m.eventSubs = nil
}

func organizedNotification(data map[string]interface{}) *NotifyMessage {
	if data == nil {
		return nil
	}
	success := intValue(data["success"])
	failed := intValue(data["failed"])
	if success == 0 && failed == 0 {
		return nil
	}

	msg := &NotifyMessage{
		EventType:  "file.organized",
		MaxRetries: 3,
	}

	animeTitle, _ := data["anime_title"].(string)
	epVal := data["episode"]

	if success == 0 && failed > 0 {
		msg.Title = "🚨 文件整理失败"
		if animeTitle != "" {
			msg.Content = fmt.Sprintf("番剧: 《%s》\n失败: %d 个 (全部整理失败)", animeTitle, failed)
		} else {
			msg.Content = fmt.Sprintf("失败: %d 个 (全部整理失败)", failed)
		}
		msg.Priority = 2
		return msg
	}

	if failed > 0 {
		msg.Title = "📁 文件整理结果"
		if animeTitle != "" {
			msg.Content = fmt.Sprintf("番剧: 《%s》\n成功: %d 个, 失败: %d 个\n最新路径: %s", animeTitle, success, failed, data["final_path"])
		} else {
			msg.Content = fmt.Sprintf("成功: %d 个, 失败: %d 个\n最新路径: %s", success, failed, data["final_path"])
		}
		msg.Priority = 1
		return msg
	}

	msg.Title = "📁 媒体已整理入库"
	if success == 1 && animeTitle != "" {
		if epVal != nil && fmt.Sprintf("%v", epVal) != "0" {
			msg.Content = fmt.Sprintf("番剧: 《%s》 第 %v 集已成功归档入库！\n最终路径: %s", animeTitle, epVal, data["final_path"])
		} else {
			msg.Content = fmt.Sprintf("番剧: 《%s》 已成功归档入库！\n最终路径: %s", animeTitle, data["final_path"])
		}
	} else {
		msg.Content = fmt.Sprintf("成功: %d 个剧集文件已整理入库\n最新路径: %s", success, data["final_path"])
	}
	msg.Priority = 1
	return msg
}

func intValue(value interface{}) int {
	number, ok := value.(int)
	if !ok {
		return 0
	}
	return number
}

// Publish 发布通知消息（非阻塞）
func (m *NotifyManager) Publish(msg *NotifyMessage) {
	if msg == nil || !m.IsEnabled() {
		return
	}

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
	if !m.IsEnabled() {
		return fmt.Errorf("消息通知推送插件当前处于停用状态，请先在插件管理中启用")
	}

	var targets []Notifier
	targetName := strings.ToLower(strings.TrimSpace(channel))
	m.mu.RLock()
	for _, n := range m.notifiers {
		curName := strings.ToLower(n.Name())
		if channel == "" || curName == targetName ||
			strings.Contains(curName, targetName) ||
			strings.Contains(targetName, curName) ||
			(targetName == "qq" && strings.Contains(curName, "qq")) ||
			(targetName == "smtp" && (curName == "email" || strings.Contains(curName, "smtp"))) ||
			(targetName == "serverchan" && strings.Contains(curName, "serverchan")) ||
			(targetName == "wecom" && (curName == "企业微信" || strings.Contains(curName, "wecom"))) ||
			(targetName == "dingtalk" && (curName == "钉钉" || strings.Contains(curName, "ding"))) ||
			(targetName == "feishu" && (curName == "飞书" || strings.Contains(curName, "feishu"))) {
			targets = append(targets, n)
		}
	}
	m.mu.RUnlock()

	if len(targets) == 0 {
		return fmt.Errorf("未找到通知渠道: %s (请检查该渠道是否已填写配置并保存)", channel)
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
	if msg == nil {
		return
	}

	// 确定目标 Notifiers
	m.mu.RLock()
	var targets []Notifier
	for _, n := range m.notifiers {
		if msg.Target == "" || msg.Target == n.Name() {
			targets = append(targets, n)
		}
	}
	m.mu.RUnlock()

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
				m.recordLog(msg.EventType, notifier.Name(), msg.Title, msg.Content, "success", "", msg.RetryCount)
			}
		}(n)
	}
	innerWg.Wait()
}

// handleFailure 处理发送失败，指数退避重试
func (m *NotifyManager) handleFailure(n Notifier, msg *NotifyMessage, err error) {
	log.Printf("⚠️ [%s] 发送失败: %v (重试 %d/%d)", n.Name(), err, msg.RetryCount, msg.MaxRetries)
	m.incStat(n.Name() + ".failure")
	m.recordLog(msg.EventType, n.Name(), msg.Title, msg.Content, "failed", err.Error(), msg.RetryCount)

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
			m.recordLog(msg.EventType, msg.Target, msg.Title, msg.Content, "dlq", msg.LastError, msg.RetryCount)
		}
	}
}

// handleLogPersist 异步持久化通知记录协程
func (m *NotifyManager) handleLogPersist() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case entry, ok := <-m.logQueue:
			if !ok {
				return
			}
			if entry != nil && database.DB != nil {
				if err := database.DB.Create(entry).Error; err != nil {
					log.Printf("⚠️ 持久化通知记录失败: %v", err)
				}
			}
		}
	}
}

// recordLog 记录通知投递流水
func (m *NotifyManager) recordLog(eventType, channel, title, content, status, errorMsg string, retryCount int) {
	entry := &database.NotificationLog{
		EventType:  eventType,
		Channel:    channel,
		Title:      title,
		Content:    content,
		Status:     status,
		ErrorMsg:   errorMsg,
		RetryCount: retryCount,
	}
	select {
	case m.logQueue <- entry:
	default:
		go func() {
			if database.DB != nil {
				_ = database.DB.Create(entry).Error
			}
		}()
	}
}

// RecordManualLog 记录手动测试等外部通知事件
func (m *NotifyManager) RecordManualLog(eventType, channel, title, content, status, errorMsg string) {
	m.recordLog(eventType, channel, title, content, status, errorMsg, 0)
}

// ============================================================
// 日志查询与统计方法
// ============================================================

// NotificationLogFilter 日志筛选参数
type NotificationLogFilter struct {
	Channel   string
	Status    string
	EventType string
	Page      int
	PageSize  int
}

// NotificationStats 统计数据
type NotificationStats struct {
	Total        int64                     `json:"total"`
	Success      int64                     `json:"success"`
	Failed       int64                     `json:"failed"`
	DLQ          int64                     `json:"dlq"`
	ChannelStats map[string]ChannelStatDTO `json:"channel_stats"`
}

type ChannelStatDTO struct {
	Success int64 `json:"success"`
	Failed  int64 `json:"failed"`
}

// GetNotificationLogs 分页查询通知日志
func GetNotificationLogs(filter NotificationLogFilter) (int64, []database.NotificationLog, error) {
	if database.DB == nil {
		return 0, nil, fmt.Errorf("数据库未初始化")
	}
	query := database.DB.Model(&database.NotificationLog{})
	if filter.Channel != "" && filter.Channel != "all" {
		query = query.Where("channel = ?", filter.Channel)
	}
	if filter.Status != "" && filter.Status != "all" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.EventType != "" && filter.EventType != "all" {
		query = query.Where("event_type = ?", filter.EventType)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, nil, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var logs []database.NotificationLog
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return 0, nil, err
	}
	return total, logs, nil
}

// ClearNotificationLogs 清空通知日志（days <= 0 则全部清空，否则清空 N 天前）
func ClearNotificationLogs(days int) error {
	if database.DB == nil {
		return fmt.Errorf("数据库未初始化")
	}
	if days <= 0 {
		return database.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&database.NotificationLog{}).Error
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	return database.DB.Where("created_at < ?", cutoff).Unscoped().Delete(&database.NotificationLog{}).Error
}

// GetNotificationStats 获取通知统计数据
func GetNotificationStats() (NotificationStats, error) {
	stats := NotificationStats{
		ChannelStats: make(map[string]ChannelStatDTO),
	}
	if database.DB == nil {
		return stats, nil
	}

	database.DB.Model(&database.NotificationLog{}).Count(&stats.Total)
	database.DB.Model(&database.NotificationLog{}).Where("status = ?", "success").Count(&stats.Success)
	database.DB.Model(&database.NotificationLog{}).Where("status = ?", "failed").Count(&stats.Failed)
	database.DB.Model(&database.NotificationLog{}).Where("status = ?", "dlq").Count(&stats.DLQ)

	type row struct {
		Channel string
		Status  string
		Count   int64
	}
	var rows []row
	database.DB.Model(&database.NotificationLog{}).
		Select("channel, status, count(*) as count").
		Group("channel, status").
		Find(&rows)

	for _, r := range rows {
		cs := stats.ChannelStats[r.Channel]
		if r.Status == "success" {
			cs.Success += r.Count
		} else {
			cs.Failed += r.Count
		}
		stats.ChannelStats[r.Channel] = cs
	}

	return stats, nil
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

func formatInt64(v interface{}) string {
	if v == nil { return "0" }
	switch val := v.(type) {
	case int64: return formatBytes(val)
	case int: return formatBytes(int64(val))
	case float64: return formatBytes(int64(val))
	default: return "0"
	}
}
