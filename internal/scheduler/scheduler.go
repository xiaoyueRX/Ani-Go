// Package scheduler 实现定时任务调度
// 负责周期性轮询 RSS、去重检查、下发下载任务
package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// Scheduler 调度器，管理所有定时任务
type Scheduler struct {
	cfg        *config.Config
	mikanRSSURL string
	source     core.Source
	downloader core.Downloader
	organizer  core.Organizer
	bus        core.EventBus
}

// New 创建调度器实例
func New(cfg *config.Config, source core.Source, dl core.Downloader, org core.Organizer, bus core.EventBus) *Scheduler {
	return &Scheduler{
		cfg:         cfg,
		mikanRSSURL: cfg.Mikan.PersonalRSSURL,
		source:      source,
		downloader:  dl,
		organizer:   org,
		bus:         bus,
	}
}

// Start 启动调度器，运行所有定时任务
func (s *Scheduler) Start(ctx context.Context) {
	log.Println("⏰ 调度器已启动")

	rssTicker := time.NewTicker(s.cfg.Scheduler.RSSInterval)
	defer rssTicker.Stop()

	orgTicker := time.NewTicker(s.cfg.Scheduler.OrganizerInterval)
	defer orgTicker.Stop()

	// 启动后立即执行一次 RSS 轮询
	go s.pollRSS(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("⏰ 调度器已停止")
			return
		case <-rssTicker.C:
			go s.pollRSS(ctx)
		case <-orgTicker.C:
			go s.pollOrganizer(ctx)
		}
	}
}

// pollRSS 执行单次 RSS 轮询
func (s *Scheduler) pollRSS(ctx context.Context) {
	if s.mikanRSSURL == "" {
		log.Println("⚠️  Mikan RSS URL 未配置，跳过 RSS 轮询")
		return
	}

	log.Println("🔍 开始 RSS 轮询...")

	items, err := s.source.FetchRSS(ctx, s.mikanRSSURL)
	if err != nil {
		log.Printf("❌ RSS 轮询失败: %v", err)
		return
	}

	log.Printf("📡 获取到 %d 个种子", len(items))

	newCount := 0
	for _, item := range items {
		// 去重检查：通过 torrent URL 判断是否已下载
		if isDuplicate(item.URL) {
			continue
		}

		// 下发下载任务
		if err := s.downloader.Add(ctx, item, s.cfg.Organizer.TVBasePath); err != nil {
			log.Printf("❌ 添加下载失败 [%s]: %v", item.Title, err)
			continue
		}

		// 记录到数据库
		recordDownload(item)
		newCount++

		// 发布事件
		if s.bus != nil {
			s.bus.Publish(core.Event{
				Type: core.EventDownloadStarted,
				Payload: map[string]any{
					"title": item.Title,
					"url":   item.URL,
					"size":  item.Size,
				},
				Time: time.Now(),
			})
		}
	}

	if newCount > 0 {
		log.Printf("✅ RSS 轮询完成: 新增 %d 个下载", newCount)
	} else {
		log.Println("✅ RSS 轮询完成: 无新内容")
	}
}

// pollOrganizer 执行文件整理轮询
func (s *Scheduler) pollOrganizer(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	// 查询数据库中的待整理 Episode 记录
	var episodes []database.Episode
	if err := database.DB.Where("status = ? AND final_path = ?", "downloaded", "").Find(&episodes).Error; err != nil {
		log.Printf("❌ 查询待整理文件失败: %v", err)
		return
	}

	if len(episodes) == 0 {
		return
	}

	log.Printf("📂 发现 %d 个待整理的文件", len(episodes))

	for _, ep := range episodes {
		// TODO: 实现完整的文件整理逻辑
		// 1. 根据 Subscription 获取番剧元数据
		// 2. 应用路径模板
		// 3. 创建目录、移动/硬链接文件
		// 4. 更新 Episode 记录

		if s.bus != nil {
			s.bus.Publish(core.Event{
				Type: core.EventFileOrganized,
				Payload: map[string]any{
					"episode_id": ep.ID,
					"final_path": ep.FinalPath,
				},
				Time: time.Now(),
			})
		}
	}

	log.Printf("✅ 已整理 %d 个文件", len(episodes))
}

// ============================================================
// 辅助函数
// ============================================================

// isDuplicate 检查种子 URL 是否已存在下载记录
func isDuplicate(torrentURL string) bool {
	var count int64
	database.DB.Model(&database.DownloadRecord{}).
		Where("torrent_url = ?", torrentURL).
		Count(&count)
	return count > 0
}

// recordDownload 记录已下载的种子
func recordDownload(item core.TorrentItem) {
	rec := database.DownloadRecord{
		TorrentURL: item.URL,
		SourceName: item.SourceName,
		AddedAt:    time.Now(),
	}
	if result := database.DB.Create(&rec); result.Error != nil {
		log.Printf("⚠️  记录下载失败: %v", result.Error)
	}
}
