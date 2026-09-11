package ai

import (
	"sync"
	"time"
)

// QuotaManager 管理 AI 每日调用限额与防爆熔断
type QuotaManager struct {
	mu           sync.RWMutex
	dailyLimit   int
	currentCount int
	currentDate  string // YYYY-MM-DD
}

var defaultQuotaManager = NewQuotaManager(30) // 默认每日限额 30 次

// NewQuotaManager 创建限额管理器
func NewQuotaManager(dailyLimit int) *QuotaManager {
	if dailyLimit <= 0 {
		dailyLimit = 30
	}
	return &QuotaManager{
		dailyLimit:  dailyLimit,
		currentDate: time.Now().Format("2006-01-02"),
	}
}

// SetDailyLimit 设置每日调用上限
func (qm *QuotaManager) SetDailyLimit(limit int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	if limit > 0 {
		qm.dailyLimit = limit
	}
}

// GetDailyLimit 获取每日调用上限
func (qm *QuotaManager) GetDailyLimit() int {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.dailyLimit
}

// GetCurrentCount 获取今日已调用次数
func (qm *QuotaManager) GetCurrentCount() int {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.checkDateReset()
	return qm.currentCount
}

// CheckAndIncrement 检查并自增配额计数；若超额则拒绝并返回 false
func (qm *QuotaManager) CheckAndIncrement() (int, bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.checkDateReset()

	if qm.currentCount >= qm.dailyLimit {
		return qm.currentCount, false
	}

	qm.currentCount++
	return qm.currentCount, true
}

func (qm *QuotaManager) checkDateReset() {
	today := time.Now().Format("2006-01-02")
	if qm.currentDate != today {
		qm.currentDate = today
		qm.currentCount = 0
	}
}

// GetDefaultQuotaManager 返回全局默认配额管理器
func GetDefaultQuotaManager() *QuotaManager {
	return defaultQuotaManager
}
