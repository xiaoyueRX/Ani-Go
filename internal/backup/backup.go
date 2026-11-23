package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	customPluginsKey = "custom_plugins"
	pluginStatusKey  = "plugin_status"
)

var ErrInvalidArchive = errors.New("无效的 Ani-Go 备份")

// BackupData 备份数据结构
type BackupData struct {
	FormatVersion int                     `json:"format_version,omitempty"`
	SHA256        string                  `json:"sha256,omitempty"`
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
	cfg                   *config.Config
	cron                  *cron.Cron
	backupDir             string
	keepCount             int
	includeEps            bool
	lifecycleMu           sync.Mutex
	entry                 cron.EntryID
	cronSpec              string
	scheduleCtx           context.Context
	cancel                context.CancelFunc
	activeKeep            atomic.Int64
	createOnce            sync.Once
	createGate            chan struct{}
	builtinCredentialKeys atomic.Value // 保存只读的 map[string]bool
}

// SetBuiltinPanelManifests 记录当前程序中可接收授权的本地面板。115 使用独立的历史设置键。
func (m *BackupManager) SetBuiltinPanelManifests(manifests []plugin.PluginInfo) {
	var builtins []plugin.PluginInfo
	for _, p := range manifests {
		if p.IsBuiltIn && p.ID != plugin.Cloud115ID {
			builtins = append(builtins, p)
		}
	}
	m.builtinCredentialKeys.Store(plugin.PanelCredentialSettingKeys(builtins))
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

// CreateBackup 创建备份
func (m *BackupManager) CreateBackup(ctx context.Context, includeEpisodes bool) error {
	// 一次只发布一份快照。到期、停机或请求取消时可直接退出等待。
	m.createOnce.Do(func() { m.createGate = make(chan struct{}, 1) })
	select {
	case m.createGate <- struct{}{}:
		defer func() { <-m.createGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(m.backupDir, 0700); err != nil {
		return err
	}
	// 旧文件仍可读取；新名称用亚秒时间及随机后缀支持并发。
	now := time.Now()
	timestamp := now.Format("20060102_150405")
	suffix := "full"
	if !includeEpisodes {
		suffix = "config"
	}
	// 临时文件仅所有者可读写，完整落盘后发布最终名称。
	tmp, err := os.CreateTemp(m.backupDir, ".anigo-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	var counts snapshotCounts
	err = database.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		counts, err = m.writeSnapshot(tx, tmp, includeEpisodes, now)
		return err
	})
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	random := strings.TrimPrefix(filepath.Base(tmp.Name()), ".anigo-backup-")
	if err := ctx.Err(); err != nil {
		return err
	}
	fileName := fmt.Sprintf("anigo_backup_%s_%09d%s_%s.json", timestamp, now.Nanosecond(), random, suffix)
	if err := os.Rename(tmp.Name(), filepath.Join(m.backupDir, fileName)); err != nil {
		return err
	}

	m.cleanOldBackups(fileName)
	log.Printf("✅ 备份创建成功: %s (%d 个订阅, %d 个设置项%s)",
		fileName, counts.subscriptions, counts.settings,
		func() string {
			if includeEpisodes {
				return fmt.Sprintf(", %d 个剧集", counts.episodes)
			} else {
				return ""
			}
		}())

	return nil
}

// cleanOldBackups 清理旧备份文件
func (m *BackupManager) cleanOldBackups(newest string) {
	keep := int(m.activeKeep.Load())
	if keep == 0 {
		keep = m.keepCount
	}
	if keep <= 0 {
		return
	}
	files, err := m.ListBackups()
	if err != nil {
		log.Printf("读取备份清单失败: %v", err)
		return
	}
	for _, file := range files {
		// NAS 可能只有秒级修改时间；始终保留本次发布的快照。
		if file.Name == newest {
			continue
		}
		if keep > 1 {
			keep--
			continue
		}
		if err := m.DeleteBackup(file.Name); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("删除旧备份失败: %v", err)
		}
	}
}

// 兼容历史时间戳文件名和具有随机后缀的新文件名。
var backupName = regexp.MustCompile(`^anigo_backup_[0-9]{8}_[0-9]{6}(?:_[0-9]+)?_(?:full|config)\.json$`)

func (m *BackupManager) protectExistingBackups() error {
	files, err := m.ListBackups()
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.Chmod(filepath.Join(m.backupDir, file.Name), 0600); err != nil {
			return err
		}
	}
	return nil
}

// ListBackups 仅列出本程序生成的普通备份文件。
func (m *BackupManager) ListBackups() ([]BackupFileInfo, error) {
	entries, err := os.ReadDir(m.backupDir)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupFileInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	files := []BackupFileInfo{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !backupName.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files = append(files, BackupFileInfo{Name: entry.Name(), Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].ModTime.Equal(files[j].ModTime) {
			return files[i].Name > files[j].Name
		}
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

// 只保留清单所声明的 ID 与完整接收地址对应的授权。旧备份可能遗留已删除目标的凭据行。
func (m *BackupManager) filterBackupPanelCredentials(settings map[string]string) error {
	var keys, builtins map[string]bool
	if raw, ok := settings[customPluginsKey]; ok {
		var manifests []plugin.PluginInfo
		if json.Unmarshal([]byte(raw), &manifests) != nil || manifests == nil {
			return fmt.Errorf("%w：插件清单格式无效", ErrInvalidArchive)
		}
		for _, p := range manifests {
			if strings.TrimSpace(p.ID) == "" {
				return fmt.Errorf("%w：插件清单缺少 ID", ErrInvalidArchive)
			}
		}
		keys = plugin.PanelCredentialSettingKeys(manifests)
		builtins, _ = m.builtinCredentialKeys.Load().(map[string]bool)
	}
	for key := range settings {
		if strings.EqualFold(key, "jwt_secret") || (plugin.IsPanelCredentialSetting(key) && !keys[key] && !builtins[key]) {
			delete(settings, key)
		}
	}
	return nil
}

func decodePluginStatus(raw string) (map[string]*bool, error) {
	var states map[string]*bool
	if json.Unmarshal([]byte(raw), &states) != nil || states == nil {
		return nil, fmt.Errorf("%w：插件开关格式无效", ErrInvalidArchive)
	}
	for _, enabled := range states {
		if enabled == nil {
			return nil, fmt.Errorf("%w：插件开关必须为布尔值", ErrInvalidArchive)
		}
	}
	return states, nil
}

// 历史快照可能只记录内置开关；本机未随快照恢复的社区插件应保留当前选择。
func mergeLegacyPluginStatus(tx *gorm.DB, states map[string]*bool) (string, error) {
	for id := range states {
		if !plugin.IsOfficialPluginID(id) {
			delete(states, id)
		}
	}
	var current database.Setting
	result := tx.Where("key = ?", pluginStatusKey).Limit(1).Find(&current)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 0 {
		local, err := decodePluginStatus(current.Value)
		if err != nil {
			return "", err
		}
		for id, enabled := range local {
			if !plugin.IsOfficialPluginID(id) {
				states[id] = enabled
			}
		}
	}
	value, err := json.Marshal(states)
	return string(value), err
}

func (m *BackupManager) restoreBackupSettings(tx *gorm.DB, settings map[string]string) error {
	if err := m.filterBackupPanelCredentials(settings); err != nil {
		return err
	}
	_, full := settings[customPluginsKey]
	if raw, ok := settings[pluginStatusKey]; ok {
		states, err := decodePluginStatus(raw)
		if err != nil {
			return err
		}
		if !full {
			settings[pluginStatusKey], err = mergeLegacyPluginStatus(tx, states)
			if err != nil {
				return err
			}
		}
	} else if full {
		settings[pluginStatusKey] = "{}"
	}
	if full {
		var keys []string
		if err := tx.Model(&database.Setting{}).Pluck("key", &keys).Error; err != nil {
			return err
		}
		for _, key := range keys {
			if plugin.IsPanelCredentialSetting(key) || key == plugin.Cloud115TokenKey {
				if err := tx.Where("key = ?", key).Delete(&database.Setting{}).Error; err != nil {
					return err
				}
			}
		}
	}
	for key, value := range settings {
		setting := database.Setting{Key: key, Value: value}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&setting).Error; err != nil {
			return err
		}
	}
	return nil
}

// RestoreBackup 从备份恢复
func (m *BackupManager) RestoreBackup(ctx context.Context, fileName string) error {
	path := m.GetBackupPath(fileName)
	if path == "" {
		return os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := verifyArchive(data); err != nil {
		return err
	}
	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		return ErrInvalidArchive
	}
	if _, err := time.Parse(time.RFC3339, backup.Timestamp); err != nil || backup.Settings == nil {
		return ErrInvalidArchive
	}
	// SQL 参数可能包含私有 RSS 地址与凭据；恢复失败时仅对外返回不带数据值的诊断。
	db := database.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard})
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := m.restoreBackupSettings(tx, backup.Settings); err != nil {
			return err
		}
		ids := make(map[uint]uint, len(backup.Subscriptions))
		claimedSubs := make(map[uint]bool, len(backup.Subscriptions))
		for _, sub := range backup.Subscriptions {
			sourceID := sub.ID
			if _, duplicate := ids[sourceID]; duplicate || sourceID == 0 {
				return fmt.Errorf("%w：订阅编号无效（%d）", ErrInvalidArchive, sourceID)
			}
			model := tx.Unscoped().Model(&database.Subscription{})
			q := model.Session(&gorm.Session{}).Where("season = ? AND COALESCE(rss_url, '') = ? AND COALESCE(subgroup_name, '') = ?", sub.Season, sub.RSSURL, sub.SubgroupName)
			q = q.Where("COALESCE(source_name, '') = ? AND COALESCE(subgroup_id, '') = ?", sub.SourceName, sub.SubgroupID).
				Where("COALESCE(filter_json, '') = ? AND COALESCE(excluded_keywords, '') = ?", sub.FilterJSON, sub.ExcludedKeywords).
				Where("COALESCE(NULLIF(allowed_subgroups, ''), '[]') = COALESCE(NULLIF(?, ''), '[]')", sub.AllowedSubgroups)
			switch {
			case sub.MikanID != "":
				q = q.Where("mikan_id = ?", sub.MikanID)
			case sub.MetadataID != "" && sub.MetadataProvider != "":
				q = q.Where("LOWER(metadata_provider) = LOWER(?) AND metadata_id = ?", sub.MetadataProvider, sub.MetadataID)
			default:
				q = q.Where("title_cn = ? AND COALESCE(bangumi_id, '') = ? AND COALESCE(mikan_id, '') = ? AND COALESCE(metadata_id, '') = ? AND LOWER(COALESCE(metadata_provider, '')) = LOWER(?)", sub.TitleCN, sub.BangumiID, sub.MikanID, sub.MetadataID, sub.MetadataProvider)
			}
			id, err := matchArchiveRow(model, q, sourceID, sub.CreatedAt, claimedSubs)
			if err != nil {
				return err
			}
			err = saveArchiveRow(tx, &sub, &sub.Model, id, backup.FormatVersion == archiveFormat)
			if err != nil {
				return err
			}
			ids[sourceID] = sub.ID
			claimedSubs[sub.ID] = true
		}
		legacyProgress := make(map[uint]bool)
		legacyMovies := make(map[uint][]database.Episode)
		claimedEpisodes := make(map[uint]bool, len(backup.Episodes))
		episodeIDs := make(map[uint]bool, len(backup.Episodes))
		for _, ep := range backup.Episodes {
			if ep.ID == 0 || episodeIDs[ep.ID] {
				return fmt.Errorf("%w：剧集编号无效（%d）", ErrInvalidArchive, ep.ID)
			}
			episodeIDs[ep.ID] = true
			parent, ok := ids[ep.SubscriptionID]
			if !ok {
				return fmt.Errorf("%w：剧集缺少订阅（%d）", ErrInvalidArchive, ep.ID)
			}
			// 合集文件以任务与相对路径识别，用户确认的季集号之后可能变化。
			sourcePath := ep.SourceFile
			fileBound, err := restoreMemberPath(&ep, parent)
			if err != nil {
				return err
			}
			ep.SubscriptionID = parent
			if ep.RecoverLegacyStatus() {
				legacyProgress[parent] = true
			}
			id, err := matchArchiveEpisode(tx, ep, sourcePath, fileBound, claimedEpisodes)
			if err != nil {
				return err
			}
			if id != 0 {
				if err := preserveLocalFileEvidence(tx, id, &ep); err != nil {
					return err
				}
			}
			legacyMovie := ep.MovieFeature == nil && ep.Status == "organized" && ep.Number == 0 && ep.OrganizedAt != nil
			err = saveArchiveRow(tx, &ep, &ep.Model, id, backup.FormatVersion == archiveFormat)
			if err != nil {
				return err
			}
			claimedEpisodes[ep.ID] = true
			if legacyMovie {
				legacyMovies[parent] = append(legacyMovies[parent], ep)
			}
		}
		// 先恢复全部任务成员，避免把多文件备份中的第一条误当独立正片。
		for id, rows := range legacyMovies {
			var sub database.Subscription
			if err := tx.Unscoped().First(&sub, id).Error; err != nil {
				return err
			}
			if _, err := database.RecoverLegacyMovieFeatures(tx, sub, rows, true); err != nil {
				return err
			}
		}
		for _, id := range ids {
			if _, err := database.SyncEpisodeProgress(tx.Unscoped(), id, legacyProgress[id]); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil || errors.Is(err, ErrInvalidArchive) {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("恢复备份失败，请检查数据库后重试")
}

// UnrepresentableMemberError 表示快照中已核实的字面文件名在目标系统上有不同的路径含义。
type UnrepresentableMemberError struct{ EpisodeID uint }

func (e *UnrepresentableMemberError) Error() string {
	return fmt.Sprintf("剧集记录 %d 的源文件名含有字面反斜杠，Windows 无法表示该文件名，数据未恢复", e.EpisodeID)
}

func (*UnrepresentableMemberError) Unwrap() error { return ErrInvalidArchive }

// 只有快照的成员键能证明反斜杠是路径分隔符还是文件名的一部分。
func restoreMemberPath(ep *database.Episode, parent uint) (bool, error) {
	original := ep.SourceFile
	fileBound := ep.TorrentHash != "" && filepath.IsLocal(original) && filepath.Clean(original) != "."
	if ep.TorrentHash != "" && ep.ReservationKey != nil {
		portable := original
		if !ep.MatchesFileReservation(portable) && strings.Contains(original, `\`) && !strings.ContainsAny(original, "/:") {
			portable = strings.ReplaceAll(original, `\`, "/")
		}
		if portable != "." && fs.ValidPath(portable) && ep.MatchesFileReservation(portable) {
			local, err := filepath.Localize(portable)
			if err == nil {
				key := database.EpisodeFileReservationKey(parent, ep.TorrentHash, local)
				ep.SourceFile, ep.ReservationKey = local, &key
				return true, nil
			}
			if filepath.Separator == '\\' && strings.Contains(portable, `\`) {
				return false, &UnrepresentableMemberError{EpisodeID: ep.ID}
			}
		}
	}
	ep.ReservationKey = nil
	return fileBound, nil
}

func matchArchiveEpisode(tx *gorm.DB, ep database.Episode, original string, fileBound bool, claimed map[uint]bool) (uint, error) {
	model := tx.Unscoped().Model(&database.Episode{}).Where("subscription_id = ?", ep.SubscriptionID)
	hashCondition := "COALESCE(torrent_hash, '') = ?"
	if magnet.IsHexInfoHash(ep.TorrentHash) {
		hashCondition += " COLLATE NOCASE"
	}
	task := model.Session(&gorm.Session{}).Where(hashCondition, ep.TorrentHash)
	q := task.Session(&gorm.Session{}).Where("COALESCE(source_file, '') = ?", ep.SourceFile)
	if ep.ReservationKey != nil {
		// 成员身份不能被碰巧相同的主键、创建时间或另一成员的字面反斜杠覆盖。
		q = q.Where("reservation_key IS NULL OR reservation_key = ?", *ep.ReservationKey)
		if original != ep.SourceFile {
			// 旧版可能照抄了外系统拼写。缺少原键时还须核对创建时间、根目录和文件大小。
			q = task.Session(&gorm.Session{}).Where(`(source_file = ? AND (reservation_key IS NULL OR reservation_key = ?)) OR
				(source_file = ? AND (reservation_key = ? OR (reservation_key IS NULL AND ? AND created_at = ? AND COALESCE(download_root, '') = ? AND COALESCE(file_size, 0) = ?)))`,
				ep.SourceFile, *ep.ReservationKey, original, *ep.ReservationKey, !ep.CreatedAt.IsZero(), ep.CreatedAt, ep.DownloadRoot, ep.FileSize)
			var candidates []uint
			if err := q.Session(&gorm.Session{}).Limit(2).Pluck("id", &candidates).Error; err != nil {
				return 0, err
			}
			if len(candidates) > 1 {
				return 0, fmt.Errorf("%w：剧集记录 %d 同时匹配多条历史文件记录", ErrInvalidArchive, ep.ID)
			}
		}
		return matchArchiveRow(q, q, ep.ID, ep.CreatedAt, claimed)
	}
	if strings.Contains(original, `\`) {
		// 无法证明分隔符含义时保留归档拼写，不覆盖已经核实的本机成员。
		const noFileClaim = "reservation_key IS NULL OR reservation_key NOT LIKE '%/file/%'"
		model = model.Session(&gorm.Session{}).Where(noFileClaim)
		q = q.Where(noFileClaim)
	}
	if !fileBound {
		q = q.Where("season = ? AND number = ?", ep.Season, ep.Number)
	}
	if ep.TorrentHash == "" {
		q = q.Where("COALESCE(torrent_url, '') = ? AND COALESCE(original_name, '') = ?", ep.TorrentURL, ep.OriginalName)
	}
	return matchArchiveRow(model, q, ep.ID, ep.CreatedAt, claimed)
}

// 源文件核验时间与待清理路径只属于本机，恢复同一份文件时沿用本地记录。
func preserveLocalFileEvidence(tx *gorm.DB, id uint, ep *database.Episode) error {
	var local database.Episode
	if err := tx.Unscoped().First(&local, id).Error; err != nil {
		return err
	}
	if magnet.CanonicalTaskID(local.TorrentHash) != magnet.CanonicalTaskID(ep.TorrentHash) ||
		magnet.CanonicalTaskID(local.OriginKey) != magnet.CanonicalTaskID(ep.OriginKey) ||
		local.SourceFile != ep.SourceFile || local.DownloadRoot != ep.DownloadRoot || local.FileSize != ep.FileSize {
		return nil
	}
	if ep.TorrentHash == "" && ep.OriginKey == "" && (ep.TorrentURL != local.TorrentURL || ep.OriginalName != local.OriginalName) {
		return nil
	}
	if ep.SourceFile != "" && ep.DownloadRoot != "" {
		ep.SourceModTime = local.SourceModTime
	}
	if ep.Status == "organized" && local.Status == "organized" && ep.FinalPath != "" && ep.FinalPath == local.FinalPath {
		ep.MoveSourcePath = local.MoveSourcePath
		ep.MoveSourceModTime = local.MoveSourceModTime
		if ep.MovieFeature == nil && local.MovieFeature != nil {
			ep.MovieFeature = local.MovieFeature
		}
	}
	return nil
}

// 同库按主键和创建时间原地恢复。跨库按来源匹配，原创建时间可区分重复条目。
func matchArchiveRow(model, identity *gorm.DB, sourceID uint, created time.Time, claimed map[uint]bool) (uint, error) {
	if !created.IsZero() && !claimed[sourceID] {
		var count int64
		if err := model.Session(&gorm.Session{}).Where("id = ? AND created_at = ?", sourceID, created).Count(&count).Error; err != nil {
			return 0, err
		}
		if count > 0 {
			return sourceID, nil
		}
	}
	var candidates []struct {
		ID        uint
		CreatedAt time.Time
	}
	if err := identity.Select("id, created_at").Find(&candidates).Error; err != nil {
		return 0, err
	}
	var match, exact uint
	var available, exactCount int
	for _, candidate := range candidates {
		if claimed[candidate.ID] {
			continue
		}
		available++
		match = candidate.ID
		if !created.IsZero() && candidate.CreatedAt.Equal(created) {
			exactCount++
			exact = candidate.ID
		}
	}
	if exactCount == 1 {
		return exact, nil
	}
	if available > 1 {
		return 0, fmt.Errorf("%w：备份记录匹配不唯一（%d）", ErrInvalidArchive, sourceID)
	}
	return match, nil
}

// 新格式区分实际零值和历史版本中尚不存在的字段。必要时纠正 ORM 插入默认值。
func saveArchiveRow[T comparable](tx *gorm.DB, row *T, model *gorm.Model, matched uint, exact bool) error {
	if matched != 0 {
		model.ID = matched
		return tx.Unscoped().Session(&gorm.Session{SkipHooks: true}).Save(row).Error
	}
	id, err := unusedBackupID(tx, row, model.ID)
	if err != nil {
		return err
	}
	model.ID = id
	original := *row
	if err := tx.Create(row).Error; err != nil {
		return err
	}
	if !exact {
		return nil
	}
	inserted := *row
	assigned := *model
	*row = original
	model.ID = assigned.ID
	if model.CreatedAt.IsZero() {
		model.CreatedAt = assigned.CreatedAt
	}
	if model.UpdatedAt.IsZero() {
		model.UpdatedAt = assigned.UpdatedAt
	}
	if inserted == *row {
		return nil
	}
	return tx.Unscoped().Model(row).Select("*").UpdateColumns(row).Error
}

// unusedBackupID 在同一事务中检查旧主键，跨库冲突时让 SQLite 分配新主键。
func unusedBackupID(tx *gorm.DB, model any, id uint) (uint, error) {
	if id == 0 {
		return 0, nil
	}
	var count int64
	if err := tx.Unscoped().Model(model).Where("id = ?", id).Count(&count).Error; err != nil {
		return 0, err
	}
	if count != 0 {
		return 0, nil
	}
	return id, nil
}

// DeleteBackup 删除指定备份文件。
func (m *BackupManager) DeleteBackup(fileName string) error {
	path := m.GetBackupPath(fileName)
	if path == "" {
		return os.ErrNotExist
	}
	return os.Remove(path)
}

// GetBackupPath 仅返回已存在的普通备份文件。
func (m *BackupManager) GetBackupPath(fileName string) string {
	if !backupName.MatchString(fileName) {
		return ""
	}
	path := filepath.Join(m.backupDir, fileName)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return path
}
