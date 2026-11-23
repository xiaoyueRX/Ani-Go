package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/backup"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"gorm.io/gorm"
)

func validatePollingIntervals(settings map[string]string) error {
	for _, key := range []string{"RSS_INTERVAL", "ORGANIZER_INTERVAL", "SUPPLEMENT_INTERVAL", "BGMTV_SYNC_INTERVAL", "SEED_CLEANUP_INTERVAL"} {
		raw, ok := settings[key]
		if !ok {
			continue
		}
		value := strings.TrimSpace(raw)
		if value != "" {
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 {
				return fmt.Errorf("%s 需要大于零的有效时长，例如 15m 或 1h", key)
			}
		}
		settings[key] = value
	}
	if raw := strings.TrimSpace(settings["BACKUP_CRON"]); raw != "" {
		if _, err := backup.ParseSchedule(raw); err != nil {
			return fmt.Errorf("BACKUP_CRON 需要有效的定时表达式")
		}
	}
	if raw := strings.TrimSpace(settings["BACKUP_KEEP_COUNT"]); raw != "" {
		if count, err := strconv.Atoi(raw); err != nil || count <= 0 {
			return fmt.Errorf("BACKUP_KEEP_COUNT 需要正整数")
		}
	}
	return nil
}

func sensitiveSetting(key string) bool {
	for _, part := range []string{"PASS", "SECRET", "KEY", "TOKEN", "WEBHOOK"} {
		if strings.Contains(strings.ToUpper(key), part) {
			return true
		}
	}
	return false
}

// 插件管理器独占这些状态和凭据，普通设置响应与修改请求都应跳过。
func internalPluginSetting(key string) bool {
	return strings.EqualFold(key, "custom_plugins") || strings.EqualFold(key, "plugin_status") || plugin.IsPanelCredentialSetting(key)
}

// 启动、热重载与连接测试均采用环境变量优先于数据库的顺序。
func configuredDownloaders(values map[string]string) config.DownloadersConfig {
	cfg := config.Load()
	cfg.MergeFromSettings(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	return cfg.Downloaders
}

// 只比较活动下载器，修改其他下载器的草稿配置无需替换运行中的实例。
func effectiveDownloaders(values map[string]string) config.DownloadersConfig {
	active := configuredDownloaders(values)
	switch strings.ToLower(active.Default) {
	case "transmission":
		active.QBittorrent = config.QBittorrentConfig{}
		active.Aria2 = config.Aria2Config{}
	case "aria2":
		active.QBittorrent = config.QBittorrentConfig{}
		active.Transmission = config.TransmissionConfig{}
	default:
		active.Transmission = config.TransmissionConfig{}
		active.Aria2 = config.Aria2Config{}
	}
	return active
}

func settingValues(tx *gorm.DB) (map[string]string, error) {
	var settings []database.Setting
	if err := tx.Find(&settings).Error; err != nil {
		return nil, err
	}
	values := make(map[string]string, len(settings))
	for _, setting := range settings {
		values[setting.Key] = setting.Value
	}
	return values, nil
}

func applySettings(tx *gorm.DB, updates map[string]string) (config.DownloadersConfig, bool, error) {
	values, err := settingValues(tx)
	if err != nil {
		return config.DownloadersConfig{}, false, err
	}
	before := effectiveDownloaders(values)
	for key, value := range updates {
		if internalPluginSetting(key) {
			continue
		}
		switch strings.ToUpper(key) {
		case "JWT_SECRET", "TOKEN_SECRET", "MCP_TOKEN", "MCP_TOKEN_CONFIGURED", plugin.Cloud115TokenKey:
			continue
		}
		if value == "" && sensitiveSetting(key) && values[key] != "" {
			continue
		}
		if err := tx.Save(&database.Setting{Key: key, Value: value}).Error; err != nil {
			return before, false, err
		}
		values[key] = value
		if key == "USER_AVATAR_URL" {
			if err := tx.Model(&database.User{}).Where("1 = 1").Update("avatar_url", value).Error; err != nil {
				return before, false, err
			}
		}
	}
	after := effectiveDownloaders(values)
	return after, before != after, nil
}
