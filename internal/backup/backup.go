package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// BackupData 备份数据结构
type BackupData struct {
	Timestamp     string                  `json:"timestamp"`
	Version       string                  `json:"version"`
	Settings      map[string]string       `json:"settings"`
	Subscriptions []database.Subscription `json:"subscriptions"`
	Episodes      []database.Episode      `json:"episodes,omitempty"` // 可选：是否包含剧集记录
}

// BackupConfig 备份配置
type BackupConfig struct {
	Path       string // 备份目录
	Cron       string // cron 表达式
	KeepCount  int    // 保留份数
	IncludeEps bool   // 是否包含剧集记录
}

// BackupManager 备份管理器
type BackupManager struct {
	cfg        *config.Config
	cron       *cron.Cron
	backupDir  string
	keepCount  int
	includeEps bool
}

// NewBackupManager 创建备份管理器
func NewBackupManager(cfg *config.Config) *BackupManager {
	backupDir := cfg.Scheduler.BackupPath
	if backupDir == "" {
		backupDir = "./data/backups"
	}

	keepCount := cfg.Scheduler.BackupKeepCount
	if keepCount <= 0 {
		keepCount = 7
	}

	return &BackupManager{
		cfg:        cfg,
		cron:       cron.New(cron.WithSeconds()),
		backupDir:  backupDir,
		keepCount:  keepCount,
		includeEps: false, // 默认不包含剧集
	}
}

// Start 启动定时备份
func (m *BackupManager) Start() error {
	cronExpr := m.cfg.Scheduler.BackupCron
	if cronExpr == "" {
		cronExpr = "0 0 * * *" // 默认每日凌晨
	}

	// cron 表达式验证（简单检查）
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(cronExpr); err != nil {
		// 尝试标准 5 字段格式
		if _, err2 := cron.ParseStandard(cronExpr); err2 != nil {
			log.Printf("⚠️ 备份 cron 表达式无效: %s, 使用默认值", cronExpr)
			cronExpr = "0 0 * * *"
		} else {
			// 如果是标准 5 字段，给 m.cron (带秒的) 用，需要在前面补 0
			cronExpr = "0 " + cronExpr
		}
	}

	_, err := m.cron.AddFunc(cronExpr, func() {
		log.Println("⏰ 触发定时备份任务...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := m.CreateBackup(ctx, m.includeEps); err != nil {
			log.Printf("❌ 定时备份失败: %v", err)
		}
		m.cleanOldBackups()
	})
	if err != nil {
		return fmt.Errorf("添加定时任务失败: %w", err)
	}

	m.cron.Start()
	log.Printf("✅ 定时备份已启动: cron=%s, 目录=%s, 保留=%d份", cronExpr, m.backupDir, m.keepCount)
	return nil
}

// Stop 停止定时备份
func (m *BackupManager) Stop() {
	m.cron.Stop()
	log.Println("⏰ 定时备份已停止")
}

// CreateBackup 创建备份
func (m *BackupManager) CreateBackup(ctx context.Context, includeEpisodes bool) error {
	// 确保备份目录存在
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return fmt.Errorf("创建备份目录失败: %w", err)
	}

	// 获取所有设置
	var settings []database.Setting
	if err := database.DB.Find(&settings).Error; err != nil {
		return fmt.Errorf("读取设置失败: %w", err)
	}
	settingsMap := make(map[string]string, len(settings))
	for _, s := range settings {
		settingsMap[s.Key] = s.Value
	}

	// 获取所有订阅
	var subs []database.Subscription
	if err := database.DB.Find(&subs).Error; err != nil {
		return fmt.Errorf("读取订阅失败: %w", err)
	}

	// 获取剧集（可选）
	var episodes []database.Episode
	if includeEpisodes {
		if err := database.DB.Find(&episodes).Error; err != nil {
			return fmt.Errorf("读取剧集失败: %w", err)
		}
	}

	// 构建备份数据
	backup := BackupData{
		Timestamp:     time.Now().Format(time.RFC3339),
		Version:       "v0.5.0",
		Settings:      settingsMap,
		Subscriptions: subs,
		Episodes:      episodes,
	}

	// 生成文件名：anigo_backup_20260830_full.json
	timestamp := time.Now().Format("20060102_150405")
	suffix := "full"
	if !includeEpisodes {
		suffix = "config"
	}
	fileName := fmt.Sprintf("anigo_backup_%s_%s.json", timestamp, suffix)
	filePath := filepath.Join(m.backupDir, fileName)

	// 写入文件
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化备份数据失败: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}

	log.Printf("✅ 备份创建成功: %s (%d 个订阅, %d 个设置项%s)",
		fileName, len(subs), len(settingsMap),
		func() string {
			if includeEpisodes {
				return fmt.Sprintf(", %d 个剧集", len(episodes))
			} else {
				return ""
			}
		}())

	return nil
}

// cleanOldBackups 清理旧备份文件
func (m *BackupManager) cleanOldBackups() {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		log.Printf("⚠️ 读取备份目录失败: %v", err)
		return
	}

	var backupFiles []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			backupFiles = append(backupFiles, entry)
		}
	}

	// 按修改时间排序（最新在前）
	sort.Slice(backupFiles, func(i, j int) bool {
		infoI, _ := backupFiles[i].Info()
		infoJ, _ := backupFiles[j].Info()
		return infoI.ModTime().After(infoJ.ModTime())
	})

	// 删除超出保留数量的文件
	if len(backupFiles) > m.keepCount {
		for _, file := range backupFiles[m.keepCount:] {
			filePath := filepath.Join(m.backupDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				log.Printf("⚠️ 删除旧备份失败 [%s]: %v", file.Name(), err)
			} else {
				log.Printf("🗑️ 已清理旧备份: %s", file.Name())
			}
		}
	}
}

// ListBackups 列出所有备份文件
func (m *BackupManager) ListBackups() ([]BackupFileInfo, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}

	var files []BackupFileInfo
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			info, _ := entry.Info()
			files = append(files, BackupFileInfo{
				Name:    entry.Name(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}
	}

	// 按修改时间排序（最新在前）
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	return files, nil
}

// BackupFileInfo 备份文件信息
type BackupFileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// RestoreBackup 从备份恢复
func (m *BackupManager) RestoreBackup(ctx context.Context, fileName string) error {
	cleanName := filepath.Base(fileName)
	if cleanName != fileName || cleanName == "." || cleanName == "/" || cleanName == "\\" {
		return fmt.Errorf("非法的文件名: %s", fileName)
	}
	filePath := filepath.Join(m.backupDir, cleanName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取备份文件失败: %w", err)
	}

	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		return fmt.Errorf("解析备份数据失败: %w", err)
	}

	// 恢复设置
	for key, value := range backup.Settings {
		setting := database.Setting{Key: key, Value: value}
		database.DB.Where("key = ?", key).Assign(setting).FirstOrCreate(&setting)
	}
	log.Printf("✅ 已恢复 %d 项设置", len(backup.Settings))

	// 恢复订阅
	for _, sub := range backup.Subscriptions {
		var existing database.Subscription
		// 清除 ID 以便 GORM 自动处理主键或匹配
		subID := sub.ID
		sub.ID = 0
		if err := database.DB.Where("bangumi_id = ?", sub.BangumiID).First(&existing).Error; err == nil {
			// 已存在，更新（使用 Select("*") 确保零值字段如 Enabled:false 也能被覆盖）
			database.DB.Model(&existing).Select("*").Updates(sub)
		} else {
			// 新建
			sub.ID = subID
			database.DB.Create(&sub)
		}
	}
	log.Printf("✅ 已恢复 %d 个订阅", len(backup.Subscriptions))

	// 恢复剧集（如果有）
	if len(backup.Episodes) > 0 {
		for _, ep := range backup.Episodes {
			var existing database.Episode
			// 同样处理剧集 ID
			epID := ep.ID
			ep.ID = 0
			if err := database.DB.Where("torrent_hash = ?", ep.TorrentHash).First(&existing).Error; err == nil {
				database.DB.Model(&existing).Select("*").Updates(ep)
			} else {
				ep.ID = epID
				database.DB.Create(&ep)
			}
		}
		log.Printf("✅ 已恢复 %d 个剧集记录", len(backup.Episodes))
	}

	return nil
}

// DeleteBackup 删除指定备份文件
func (m *BackupManager) DeleteBackup(fileName string) error {
	cleanName := filepath.Base(fileName)
	if cleanName != fileName || cleanName == "." || cleanName == "/" || cleanName == "\\" {
		return fmt.Errorf("非法的文件名: %s", fileName)
	}
	filePath := filepath.Join(m.backupDir, cleanName)
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("删除备份文件失败: %w", err)
	}
	log.Printf("🗑️ 已删除备份: %s", cleanName)
	return nil
}

// GetBackupPath 获取备份文件完整路径
func (m *BackupManager) GetBackupPath(fileName string) string {
	cleanName := filepath.Base(fileName)
	if cleanName != fileName || cleanName == "." || cleanName == "/" || cleanName == "\\" {
		return ""
	}
	filePath := filepath.Join(m.backupDir, cleanName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ""
	}
	return filePath
}
