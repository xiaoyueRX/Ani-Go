package backup

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

const dailyBackup = "0 0 0 * * *"

var scheduleParser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseSchedule 统一解析标准五字段、带秒六字段以及 cron 支持的别名。
func ParseSchedule(expr string) (cron.Schedule, error) {
	return scheduleParser.Parse(strings.TrimSpace(expr))
}

func (m *BackupManager) scheduleSettings() (string, int, error) {
	spec, keep := m.cfg.Scheduler.BackupCron, m.keepCount
	values := map[string]string{}
	if database.DB != nil {
		var rows []database.Setting
		if err := database.DB.Where("key IN ?", []string{"BACKUP_CRON", "BACKUP_KEEP_COUNT"}).Find(&rows).Error; err != nil {
			return "", 0, err
		}
		for _, row := range rows {
			values[row.Key] = row.Value
		}
	}
	for _, key := range []string{"BACKUP_CRON", "BACKUP_KEEP_COUNT"} {
		if raw := os.Getenv(key); strings.TrimSpace(raw) != "" {
			values[key] = raw
		}
	}
	if raw, ok := values["BACKUP_CRON"]; ok {
		spec = strings.TrimSpace(raw)
	}
	if raw, ok := values["BACKUP_KEEP_COUNT"]; ok {
		keep, _ = strconv.Atoi(strings.TrimSpace(raw))
	}
	if keep <= 0 {
		keep = 7
	}
	if spec == "" {
		spec = dailyBackup
	}
	return spec, keep, nil
}

// Start 在首次调用时注册备份任务。同一个管理器可停止后重新启动。
func (m *BackupManager) Start() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.cancel != nil {
		return nil
	}
	if err := m.protectExistingBackups(); err != nil {
		return err
	}
	spec, keep, err := m.scheduleSettings()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cron = cron.New()
	m.cancel = cancel
	m.setSchedule(ctx, spec, keep)
	m.cron.Start()
	return nil
}

// ReloadSchedule 在设置提交后重新读取运行中的定时任务和保留数量。
func (m *BackupManager) ReloadSchedule() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	spec, keep, err := m.scheduleSettings()
	if err != nil {
		return err
	}
	m.activeKeep.Store(int64(keep))
	if m.cancel == nil {
		return nil
	}
	if spec == m.cronSpec {
		return nil
	}
	m.setSchedule(m.scheduleCtx, spec, keep)
	return nil
}

func (m *BackupManager) setSchedule(ctx context.Context, spec string, keep int) {
	task, err := ParseSchedule(spec)
	if err != nil {
		log.Print("备份计划无效，使用每日零点")
		spec = dailyBackup
		task, _ = ParseSchedule(spec)
	}
	if m.entry != 0 {
		m.cron.Remove(m.entry)
	}
	m.activeKeep.Store(int64(keep))
	m.scheduleCtx = ctx
	m.cronSpec = spec
	job := cron.FuncJob(func() {
		run, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		if err := m.CreateBackup(run, m.includeEps); err != nil && run.Err() == nil {
			log.Printf("定时备份失败: %v", err)
		}
	})
	m.entry = m.cron.Schedule(task, cron.SkipIfStillRunning(cron.DiscardLogger)(job))
}

func (m *BackupManager) Stop() {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.cancel == nil {
		return
	}
	m.cancel()
	<-m.cron.Stop().Done()
	m.cancel = nil
	m.scheduleCtx = nil
	m.entry = 0
}
