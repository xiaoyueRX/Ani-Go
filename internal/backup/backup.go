package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
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
			return
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

	backup := BackupData{
		Timestamp: time.Now().Format(time.RFC3339),
		Version:   "v0.5.0",
		Settings:  make(map[string]string),
	}
	// 所有表从同一个 SQLite 快照读取，避免备份中的剧集引用快照外的订阅。
	if err := database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var settings []database.Setting
		if err := tx.Find(&settings).Error; err != nil {
			return err
		}
		for _, setting := range settings {
			backup.Settings[setting.Key] = setting.Value
		}
		if err := tx.Find(&backup.Subscriptions).Error; err != nil {
			return err
		}
		if includeEpisodes {
			return tx.Find(&backup.Episodes).Error
		}
		return nil
	}); err != nil {
		return fmt.Errorf("读取备份快照失败: %w", err)
	}

	// 生成文件名：anigo_backup_20260830_full.json
	timestamp := time.Now().Format("20060102_150405.000000000")
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

	tmp, err := os.CreateTemp(m.backupDir, ".anigo-backup-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filePath); err != nil {
		return fmt.Errorf("保存备份文件失败: %w", err)
	}
	log.Printf("✅ 备份创建成功: %s (%d 个订阅, %d 个设置项, %d 个剧集)", fileName, len(backup.Subscriptions), len(backup.Settings), len(backup.Episodes))

	return nil
}

// 兼容历史日期和秒级文件名，仅轮转本程序生成的备份。
var backupNamePattern = regexp.MustCompile(`^anigo_backup_[0-9]{8}(_[0-9]{6}(\.[0-9]{9})?)?_(config|full)\.json$`)

func (m *BackupManager) cleanOldBackups() {
	files, err := m.ListBackups()
	if err != nil {
		log.Printf("读取备份目录失败: %v", err)
		return
	}
	if len(files) <= m.keepCount {
		return
	}
	for _, file := range files[m.keepCount:] {
		if err := os.Remove(filepath.Join(m.backupDir, file.Name)); err != nil {
			log.Printf("删除旧备份 %s 失败: %v", file.Name, err)
		}
	}
}

// ListBackups 列出所有备份文件
func (m *BackupManager) ListBackups() ([]BackupFileInfo, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.MkdirAll(m.backupDir, 0755)
			return []BackupFileInfo{}, nil
		}
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}

	var files []BackupFileInfo
	for _, entry := range entries {
		if entry.Type().IsRegular() && backupNamePattern.MatchString(entry.Name()) {
			info, err := entry.Info()
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
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

	return database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range backup.Settings {
			if err := tx.Save(&database.Setting{Key: key, Value: value}).Error; err != nil {
				return fmt.Errorf("恢复设置 %s: %w", key, err)
			}
		}
		ids := make(map[uint]uint, len(backup.Subscriptions))
		used := make(map[uint]bool, len(backup.Subscriptions))
		for _, sub := range backup.Subscriptions {
			oldID := sub.ID
			if oldID == 0 || ids[oldID] != 0 {
				return fmt.Errorf("备份中订阅 ID 无效或重复: %d", oldID)
			}
			id, err := restoreSubscription(tx, sub, used)
			if err != nil {
				return fmt.Errorf("恢复订阅 %d: %w", oldID, err)
			}
			ids[oldID], used[id] = id, true
		}
		for _, ep := range backup.Episodes {
			parentID, ok := ids[ep.SubscriptionID]
			if !ok {
				return fmt.Errorf("剧集 %d 引用了缺失的备份订阅 %d", ep.ID, ep.SubscriptionID)
			}
			ep.SubscriptionID = parentID
			if err := restoreEpisode(tx, ep); err != nil {
				return fmt.Errorf("恢复剧集 %d: %w", ep.ID, err)
			}
		}
		return nil
	})
}

// 本地主键只用于在身份一致的候选中消歧，不能单独用来覆盖订阅。
func restoreSubscription(tx *gorm.DB, sub database.Subscription, used map[uint]bool) (uint, error) {
	if sub.AllowedSubgroups == "" {
		sub.AllowedSubgroups = "[]"
	}
	query := tx.Unscoped().Where("COALESCE(bangumi_id, '') = ? AND season = ?", sub.BangumiID, sub.Season).
		Where("COALESCE(source_name, '') = ? AND COALESCE(subgroup_id, '') = ? AND COALESCE(subgroup_name, '') = ?", sub.SourceName, sub.SubgroupID, sub.SubgroupName).
		Where("COALESCE(rss_url, '') = ? AND COALESCE(filter_json, '') = ? AND COALESCE(excluded_keywords, '') = ?", sub.RSSURL, sub.FilterJSON, sub.ExcludedKeywords).
		Where("COALESCE(NULLIF(allowed_subgroups, ''), '[]') = ?", sub.AllowedSubgroups)
	if sub.MetadataID != "" {
		query = query.Where("metadata_id = ? AND metadata_provider = ?", sub.MetadataID, sub.MetadataProvider)
	} else if sub.BangumiID == "" {
		query = query.Where("title_cn = ? AND COALESCE(title_jp, '') = ? AND COALESCE(title_en, '') = ? AND COALESCE(year, 0) = ?", sub.TitleCN, sub.TitleJP, sub.TitleEN, sub.Year)
	}
	var candidates []database.Subscription
	if err := query.Find(&candidates).Error; err != nil {
		return 0, err
	}
	var chosen uint
	matches := 0
	for _, candidate := range candidates {
		if used[candidate.ID] {
			continue
		}
		chosen = candidate.ID
		matches++
		if chosen == sub.ID {
			matches = 1
			break
		}
	}
	if matches > 1 {
		return 0, fmt.Errorf("订阅 %q 匹配多个本地记录", sub.TitleCN)
	}
	sub.ID = chosen
	if err := tx.Unscoped().Save(&sub).Error; err != nil {
		return 0, err
	}
	return sub.ID, nil
}

func restoreEpisode(tx *gorm.DB, ep database.Episode) error {
	query := tx.Unscoped()
	if ep.TorrentHash == "" {
		query = query.Where("COALESCE(torrent_hash, '') = '' AND subscription_id = ? AND season = ? AND number = ? AND COALESCE(original_name, '') = ?", ep.SubscriptionID, ep.Season, ep.Number, ep.OriginalName)
	} else {
		query = query.Where("torrent_hash = ?", ep.TorrentHash)
	}
	var existing []database.Episode
	if err := query.Limit(2).Find(&existing).Error; err != nil {
		return err
	}
	ep.ID = 0
	if len(existing) > 0 {
		if len(existing) != 1 || existing[0].SubscriptionID != ep.SubscriptionID {
			return fmt.Errorf("剧集记录归属冲突")
		}
		ep.ID = existing[0].ID
	}
	if ep.Status == "completed" {
		ep.ManualStatus = "completed"
		ep.Status = "downloading"
		if ep.FinalPath != "" {
			ep.Status = "organized"
		}
	}
	write := tx.Unscoped()
	if ep.TorrentHash == "" {
		// NULL 哈希可在不同剧集上共存；空字符串会触发全局唯一索引。
		write = write.Omit("TorrentHash")
	}
	return write.Save(&ep).Error
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
