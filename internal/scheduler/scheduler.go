// Package scheduler 实现定时任务调度
// 负责周期性轮询 RSS、去重检查、下发下载任务
package scheduler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// Scheduler 调度器，管理所有定时任务
type Scheduler struct {
	cfg              *config.Config
	mikanRSSURL      string
	source           core.Source
	downloader       core.Downloader
	organizer        core.Organizer
	bus              core.EventBus
	metadataProvider core.MetadataProvider // 元数据提供者（可选）
	aiClient         ai.Classifier         // AI 分类器（可选）
}

// New 创建调度器实例
func New(cfg *config.Config, source core.Source, dl core.Downloader, org core.Organizer, bus core.EventBus, md core.MetadataProvider, aic ai.Classifier) *Scheduler {
	return &Scheduler{
		cfg:              cfg,
		mikanRSSURL:      cfg.Mikan.PersonalRSSURL,
		source:           source,
		downloader:       dl,
		organizer:        org,
		bus:              bus,
		metadataProvider: md,
		aiClient:         aic,
	}
}

// Start 启动调度器，运行所有定时任务
func (s *Scheduler) Start(ctx context.Context) {
	log.Println("⏰ 调度器已启动")

	rssTicker := time.NewTicker(s.cfg.Scheduler.RSSInterval)
	defer rssTicker.Stop()

	orgTicker := time.NewTicker(s.cfg.Scheduler.OrganizerInterval)
	defer orgTicker.Stop()

	suppTicker := time.NewTicker(s.cfg.Scheduler.SupplementInterval)
	defer suppTicker.Stop()

	// 启动后立即执行一次 RSS 轮询
	go s.pollRSS(ctx)

	// 延迟 30 秒后执行首次补全扫描
	go func() {
		time.Sleep(30 * time.Second)
		s.pollSupplement(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("⏰ 调度器已停止")
			return
		case <-rssTicker.C:
			go s.pollRSS(ctx)
		case <-orgTicker.C:
			go s.pollOrganizer(ctx)
		case <-suppTicker.C:
			go s.pollSupplement(ctx)
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

// pollSupplement 执行补全扫描：查找集数不完整的订阅，爬取历史种子补全
func (s *Scheduler) pollSupplement(ctx context.Context) {
	log.Println("🔍 开始补全扫描...")

	var subs []database.Subscription
	database.DB.Where(
		"enabled = ? AND completed = ? AND total_episodes > 0 AND current_episodes < total_episodes",
		true, false,
	).Find(&subs)

	if len(subs) == 0 {
		log.Println("✅ 补全扫描完成: 无需补全的订阅")
		return
	}

	log.Printf("📋 发现 %d 个需要补全的订阅", len(subs))

	for _, sub := range subs {
		s.supplementOne(ctx, sub)
	}

	log.Println("✅ 补全扫描完成")
}

// TriggerSupplement 对单个订阅执行补全扫描（供 API 调用）
func (s *Scheduler) TriggerSupplement(ctx context.Context, subID uint) error {
	var sub database.Subscription
	if err := database.DB.First(&sub, subID).Error; err != nil {
		return err
	}
	s.supplementOne(ctx, sub)
	return nil
}

// supplementOne 对单个订阅执行补全逻辑
func (s *Scheduler) supplementOne(ctx context.Context, sub database.Subscription) {
	if sub.BangumiID == "" {
		log.Printf("⚠️  订阅 %s 未设置 BangumiID，跳过补全", sub.TitleCN)
		return
	}

	filter := buildFilter(sub)

	if s.bus != nil {
		s.bus.Publish(core.Event{
			Type: core.EventSupplementTriggered,
			Payload: map[string]any{
				"subscription_id": sub.ID,
				"bangumi_id":      sub.BangumiID,
				"title":           sub.TitleCN,
			},
			Time: time.Now(),
		})
	}

	items, err := s.source.FetchHistory(ctx, sub.BangumiID, filter)
	if err != nil {
		log.Printf("❌ 获取历史种子失败 [%s]: %v", sub.TitleCN, err)
		return
	}

	newCount := 0
	for _, item := range items {
		if isDuplicate(item.URL) {
			continue
		}
		if item.InfoHash != "" && isEpisodeExists(item.InfoHash) {
			continue
		}

		savePath := sub.CustomPath
		if savePath == "" {
			savePath = s.cfg.Organizer.TVBasePath
		}

		if err := s.downloader.Add(ctx, item, savePath); err != nil {
			log.Printf("❌ 添加补全下载失败 [%s]: %v", item.Title, err)
			continue
		}

		recordDownload(item)
		createEpisodeRecord(sub.ID, item)
		newCount++
	}

	if newCount > 0 {
		log.Printf("✅ 补全 [%s]: 新增 %d 个下载", sub.TitleCN, newCount)
	}

	var count int64
	database.DB.Model(&database.Episode{}).
		Where("subscription_id = ? AND status IN ?", sub.ID, []string{"downloaded", "downloading"}).
		Count(&count)
	database.DB.Model(&sub).Update("current_episodes", count)

	if int(count) >= sub.TotalEpisodes {
		database.DB.Model(&sub).Update("completed", true)
		if s.bus != nil {
			s.bus.Publish(core.Event{
				Type: core.EventSupplementCompleted,
				Payload: map[string]any{
					"subscription_id": sub.ID,
					"title":           sub.TitleCN,
				},
				Time: time.Now(),
			})
		}
	}
}

// buildFilter 从订阅的 FilterJSON 构建 core.Filter
func buildFilter(sub database.Subscription) core.Filter {
	filter := core.Filter{
		PreferSubgroup: sub.SubgroupName,
	}
	if sub.FilterJSON != "" {
		var stored struct {
			IncludeKeywords []string `json:"include_keywords"`
			ExcludeKeywords []string `json:"exclude_keywords"`
			Resolution      string   `json:"resolution"`
		}
		if err := json.Unmarshal([]byte(sub.FilterJSON), &stored); err == nil {
			filter.IncludeKeywords = stored.IncludeKeywords
			filter.ExcludeKeywords = stored.ExcludeKeywords
			if stored.Resolution != "" {
				filter.Resolution = stored.Resolution
			}
		}
	}
	return filter
}

// createEpisodeRecord 创建或更新 Episode 记录
func createEpisodeRecord(subID uint, item core.TorrentItem) {
	if item.InfoHash == "" {
		return
	}
	now := time.Now()
	ep := database.Episode{
		SubscriptionID:    subID,
		Season:            1,
		Number:            0,
		Title:             item.Title,
		Status:            "downloading",
		TorrentHash:       item.InfoHash,
		TorrentURL:        item.URL,
		OriginalName:      item.Title,
		FileSize:          item.Size,
		DownloadStartedAt: &now,
	}
	database.DB.Where("torrent_hash = ?", item.InfoHash).FirstOrCreate(&ep)
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

// isEpisodeExists 检查通过 InfoHash 是否已存在 Episode 记录
func isEpisodeExists(infoHash string) bool {
	var count int64
	database.DB.Model(&database.Episode{}).
		Where("torrent_hash = ?", infoHash).
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
