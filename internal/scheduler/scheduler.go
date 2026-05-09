// Package scheduler 实现定时任务调度
// 负责周期性轮询 RSS、去重检查、下发下载任务
package scheduler

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
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
	
	downloadTicker := time.NewTicker(10 * time.Second)
	defer downloadTicker.Stop()

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
		case <-downloadTicker.C:
			go s.pollDownloads(ctx)
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

	var subs []database.Subscription
	if err := database.DB.Where("enabled = ?", true).Find(&subs).Error; err != nil {
		log.Printf("❌ 获取订阅列表失败: %v", err)
		return
	}

	newCount := 0
	for _, item := range items {
		// 去重检查：通过 torrent URL 判断是否已下载
		if isDuplicate(item.URL) {
			continue
		}

		// 匹配订阅：寻找匹配长度最长的订阅标题（解决 Fate vs Fate/Zero 匹配歧义）
		var matchedSub *database.Subscription
		var maxMatchLen int
		for i := range subs {
			sub := &subs[i]
			currentMatchLen := 0
			
			if sub.TitleCN != "" && strings.Contains(strings.ToLower(item.Title), strings.ToLower(sub.TitleCN)) {
				currentMatchLen = len(sub.TitleCN)
			}
			if sub.TitleEN != "" && strings.Contains(strings.ToLower(item.Title), strings.ToLower(sub.TitleEN)) {
				if len(sub.TitleEN) > currentMatchLen {
					currentMatchLen = len(sub.TitleEN)
				}
			}
			if sub.TitleJP != "" && strings.Contains(strings.ToLower(item.Title), strings.ToLower(sub.TitleJP)) {
				if len(sub.TitleJP) > currentMatchLen {
					currentMatchLen = len(sub.TitleJP)
				}
			}

			if currentMatchLen > maxMatchLen {
				matchedSub = sub
				maxMatchLen = currentMatchLen
			}
		}

		savePath := s.cfg.Organizer.TVBasePath
		if matchedSub != nil && matchedSub.CustomPath != "" {
			savePath = matchedSub.CustomPath
		}

		// 下发下载任务
		if s.downloader != nil {
			if err := s.downloader.Add(ctx, item, savePath); err != nil {
				log.Printf("❌ 添加下载失败 [%s]: %v", item.Title, err)
				continue
			}
		} else {
			log.Printf("⚠️ 未配置下载器，跳过添加任务 [%s]", item.Title)
		}

		// 记录到数据库
		recordDownload(item)
		newCount++

		if matchedSub != nil {
			season, epNum := parser.ExtractEpisode(item.Title)
			createEpisodeRecordWithParsed(matchedSub.ID, item, season, epNum)
		} else {
			log.Printf("⚠️ 未找到匹配的订阅，跳过集数记录创建: %s", item.Title)
		}

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

// pollDownloads 查询下载状态并更新数据库
func (s *Scheduler) pollDownloads(ctx context.Context) {
	if s.downloader == nil {
		return
	}

	tasks, err := s.downloader.List(ctx)
	if err != nil {
		log.Printf("❌ 获取下载列表失败: %v", err)
		return
	}

	for _, task := range tasks {
		// 判断是否下载完成：completed(完全做种), stalledUP(做种中但没流量), uploading(正在做种上传)
		if task.Status == "completed" || task.Status == "stalledUP" || task.Status == "uploading" || task.Progress >= 1.0 {
			// 更新数据库状态
			var ep database.Episode
			err := database.DB.Where("torrent_hash = ? AND status = ?", task.Hash, "downloading").First(&ep).Error
			if err != nil {
				// 尝试通过原始名称匹配，用于处理 RSS 等未提前获取到 Hash 的场景
				if err2 := database.DB.Where("(torrent_hash = '' OR torrent_hash IS NULL) AND original_name = ? AND status = ?", task.Name, "downloading").First(&ep).Error; err2 == nil {
					database.DB.Model(&ep).Update("torrent_hash", task.Hash)
					err = nil
				}
			}

			if err == nil && ep.ID != 0 {
				now := time.Now()
				database.DB.Model(&ep).Updates(map[string]interface{}{
					"status":               "downloaded",
					"download_finished_at": &now,
				})

				log.Printf("📥 下载完成: %s", task.Name)

				if s.bus != nil {
					s.bus.Publish(core.Event{
						Type: core.EventDownloadCompleted,
						Payload: map[string]any{
							"episode_id": ep.ID,
							"title":      ep.Title,
							"hash":       task.Hash, // 使用 task.Hash，确保即使刚更新也能传正确值
						},
						Time: now,
					})
				}
			}
		}
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
		// 1. 根据 Subscription 获取番剧元数据
		var sub database.Subscription
		if err := database.DB.First(&sub, ep.SubscriptionID).Error; err != nil {
			log.Printf("⚠️  整理跳过，找不到对应的订阅记录: %d", ep.SubscriptionID)
			continue
		}

		anime := core.Anime{
			ID:       sub.BangumiID,
			Provider: "mikan",
			TitleCN:  sub.TitleCN,
			TitleEN:  sub.TitleEN,
			TitleJP:  sub.TitleJP,
			Year:     sub.Year,
			Season:   sub.Season,
			Type:     sub.AnimeType,
		}

		coreEp := core.Episode{
			AnimeID: anime.ID,
			Season:  ep.Season,
			Number:  ep.Number,
			Title:   ep.Title,
		}

		// 2. 从下载器获取真实保存路径
		task, err := s.downloader.GetStatus(ctx, ep.TorrentHash)
		if err != nil {
			log.Printf("⚠️  获取种子状态失败: %v", err)
			continue
		}

		// 这里处理任务的真实路径
		// 因为 task.SavePath 是基础保存目录，task.Name 是文件/文件夹名
		realPath := filepath.Join(task.SavePath, task.Name)
		
		// 如果是目录，我们要找到里面最大的视频文件作为真正要整理的文件
		info, err := os.Stat(realPath)
		if err == nil && info.IsDir() {
			realPath = findLargestVideoFile(realPath)
		}

		if realPath == "" {
			log.Printf("⚠️  无法在目录中找到有效的视频文件: %s", task.Name)
			continue
		}

		// 3. 应用路径模板并整理
		newPath, err := s.organizer.Organize(ctx, realPath, anime, coreEp)
		if err != nil {
			log.Printf("❌ 文件整理失败 [%s]: %v", ep.Title, err)
			continue
		}

		// 4. 更新 Episode 记录
		now := time.Now()
		database.DB.Model(&ep).Updates(map[string]interface{}{
			"status":       "organized",
			"final_path":   newPath,
			"organized_at": &now,
		})

		if s.bus != nil {
			s.bus.Publish(core.Event{
				Type: core.EventFileOrganized,
				Payload: map[string]any{
					"episode_id": ep.ID,
					"final_path": newPath,
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
		"enabled = ? AND completed = ? AND (total_episodes = 0 OR current_episodes < total_episodes)",
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

	if sub.TotalEpisodes == 0 && s.metadataProvider != nil {
		anime, err := s.metadataProvider.GetAnime(ctx, sub.BangumiID)
		if err == nil && anime.TotalEps > 0 {
			sub.TotalEpisodes = anime.TotalEps
			database.DB.Model(&sub).Update("total_episodes", anime.TotalEps)
			log.Printf("✅ 已更新订阅 [%s] 总集数: %d", sub.TitleCN, anime.TotalEps)
		}
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

	log.Printf("ℹ️  补全 [%s]: 获取到 %d 个历史种子", sub.TitleCN, len(items))

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
		
		// 使用新增的正则工具解析季数和集数
		season, epNum := parser.ExtractEpisode(item.Title)
		createEpisodeRecordWithParsed(sub.ID, item, season, epNum)
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

	if sub.TotalEpisodes > 0 && int(count) >= sub.TotalEpisodes {
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

// createEpisodeRecordWithParsed 使用解析好的季数和集数创建或更新 Episode 记录
func createEpisodeRecordWithParsed(subID uint, item core.TorrentItem, season int, number float32) {
	now := time.Now()
	hash := item.InfoHash
	if hash == "" {
		hash = fmt.Sprintf("url:%x", md5.Sum([]byte(item.URL)))
	}
	ep := database.Episode{
		SubscriptionID:    subID,
		Season:            season,
		Number:            number,
		Title:             item.Title,
		Status:            "downloading",
		TorrentHash:       hash,
		TorrentURL:        item.URL,
		OriginalName:      item.Title,
		FileSize:          item.Size,
		DownloadStartedAt: &now,
	}

	if item.InfoHash != "" {
		database.DB.Where("torrent_hash = ?", item.InfoHash).FirstOrCreate(&ep)
	} else if item.URL != "" {
		database.DB.Where("torrent_url = ?", item.URL).FirstOrCreate(&ep)
	} else {
		database.DB.Where("original_name = ?", item.Title).FirstOrCreate(&ep)
	}
}

// createEpisodeRecord 创建或更新 Episode 记录 (旧版本兼容)
func createEpisodeRecord(subID uint, item core.TorrentItem) {
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

	if item.InfoHash != "" {
		database.DB.Where("torrent_hash = ?", item.InfoHash).FirstOrCreate(&ep)
	} else if item.URL != "" {
		database.DB.Where("torrent_url = ?", item.URL).FirstOrCreate(&ep)
	} else {
		database.DB.Where("original_name = ?", item.Title).FirstOrCreate(&ep)
	}
}

// ============================================================
// 辅助函数
// ============================================================

// findLargestVideoFile 遍历目录找出最大的视频文件
func findLargestVideoFile(dirPath string) string {
	var largestFile string
	var maxSize int64
	videoExts := map[string]bool{".mp4": true, ".mkv": true, ".avi": true, ".rmvb": true}

	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if videoExts[ext] && info.Size() > maxSize {
			maxSize = info.Size()
			largestFile = path
		}
		return nil
	})

	return largestFile
}

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
	hash := item.InfoHash
	if hash == "" {
		hash = fmt.Sprintf("url:%x", md5.Sum([]byte(item.URL)))
	}
	rec := database.DownloadRecord{
		TorrentHash: hash,
		TorrentURL:  item.URL,
		SourceName:  item.SourceName,
		AddedAt:     time.Now(),
	}
	if result := database.DB.Create(&rec); result.Error != nil {
		log.Printf("⚠️  记录下载失败: %v", result.Error)
	}
}
