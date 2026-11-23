package downloader

import (
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// NewConfigured 供启动及运行时配置更新共用。
func NewConfigured(cfg config.DownloadersConfig) core.Downloader {
	switch strings.ToLower(cfg.Default) {
	case "transmission":
		return NewTransmission(cfg.Transmission.Host, cfg.Transmission.Username, cfg.Transmission.Password)
	case "aria2":
		return NewAria2(cfg.Aria2.Host, cfg.Aria2.Secret)
	default:
		return NewQBittorrent(cfg.QBittorrent.Host, cfg.QBittorrent.Username, cfg.QBittorrent.Password, cfg.QBittorrent.Category)
	}
}

// CanPause 与 CanResume 识别三个下载器的原生任务状态。
func CanPause(status string) bool {
	switch strings.ToLower(status) {
	case "active", "waiting", "downloading", "seeding", "queued", "uploading", "checking",
		"stalleddl", "stalledup", "queueddl", "queuedup", "forceddl", "forcedup",
		"metadl", "forcedmetadl", "checkingdl", "checkingup":
		return true
	}
	return false
}

func CanResume(status string) bool {
	switch strings.ToLower(status) {
	case "paused", "stopped", "pauseddl", "pausedup", "stoppeddl", "stoppedup":
		return true
	}
	return false
}
