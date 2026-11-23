// Package scheduler 实现定时任务调度
// 负责周期性轮询 RSS、去重检查、下发下载任务
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/backup"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

// Scheduler 调度器，管理所有定时任务
type Scheduler struct {
	runMu            sync.Mutex
	run              *schedulerRun
	scans            map[uint]*supplementRun
	defaults         *config.Config
	cfg              *config.Config
	mikanRSSURL      string
	source           core.Source
	downloader       core.Downloader
	organizer        core.Organizer
	bus              core.EventBus
	metadataProvider core.MetadataProvider // 元数据提供者（可选）
	aiClient         ai.Classifier         // AI 分类器（可选）
	backupManager    *backup.BackupManager // 备份管理器

	pollRSSRunning         atomic.Bool
	pollOrganizerRunning   atomic.Bool
	pollDownloadsRunning   atomic.Bool
	pollSupplementRunning  atomic.Bool
	pollSyncBangumiRunning atomic.Bool
	pollSeedCleanupRunning atomic.Bool
}

// New 创建调度器实例
func New(cfg *config.Config, source core.Source, dl core.Downloader, org core.Organizer, bus core.EventBus, md core.MetadataProvider, aic ai.Classifier, startupDefaults ...*config.Config) *Scheduler {
	s := &Scheduler{
		cfg:              cfg,
		mikanRSSURL:      cfg.Mikan.PersonalRSSURL,
		source:           source,
		downloader:       dl,
		organizer:        org,
		bus:              bus,
		metadataProvider: md,
		aiClient:         aic,
		backupManager:    backup.NewBackupManager(cfg),
	}
	if len(startupDefaults) > 0 && startupDefaults[0] != nil {
		s.defaults = startupDefaults[0]
		s.mikanRSSURL = s.defaults.Mikan.PersonalRSSURL
	}
	return s
}

func (s *Scheduler) runtimeConfig() *config.Config {
	if s.defaults != nil {
		return s.defaults
	}
	return s.cfg
}

// 与启动时相同的优先级。执行中的任务只读取已经提交的设置。
func runtimeSetting(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	if database.DB == nil {
		return ""
	}
	var raw string
	database.DB.Model(&database.Setting{}).Select("value").Where("key = ?", key).Scan(&raw)
	return strings.TrimSpace(raw)
}

func (s *Scheduler) GetBackupManager() *backup.BackupManager {
	return s.backupManager
}

// jitteredTicker 创建带随机抖动（±20%）的定时器
// 优化点：固定间隔会让多个 ticker 在同一时刻集中触发，造成周期性 CPU 尖峰；
// 抖动把触发点打散，削平峰值且不改变轮询频率的期望值
func jitteredTicker(interval time.Duration) *time.Ticker {
	return time.NewTicker(jitteredInterval(interval))
}

func jitteredInterval(interval time.Duration) time.Duration {
	if interval <= 0 {
		interval = time.Minute
	}
	// rand 数值仅用于调度抖动，无安全要求，直接用全局源即可
	jitter := time.Duration(rand.Float64()*0.4*float64(interval)) - interval/5 // ±20%
	delay := interval + jitter
	if delay <= 0 {
		return interval
	}
	return delay
}

func (s *Scheduler) getInterval(key string, defaultVal time.Duration) time.Duration {
	return s.durationSetting(key, defaultVal, time.Nanosecond)
}

func (s *Scheduler) durationSetting(key string, defaultVal, minimum time.Duration) time.Duration {
	if d, err := time.ParseDuration(runtimeSetting(key)); err == nil && d >= minimum {
		return d
	}
	return defaultVal
}

func (s *Scheduler) getMikanRSSURL() string {
	if raw := runtimeSetting("MIKAN_RSS_URL"); raw != "" {
		return raw
	}
	return s.mikanRSSURL
}

func (s *Scheduler) getTVBasePath() string {
	if raw := runtimeSetting("TV_BASE_PATH"); raw != "" {
		return raw
	}
	if cfg := s.runtimeConfig(); cfg != nil {
		return cfg.Organizer.TVBasePath
	}
	return ""
}

func (s *Scheduler) getRSSMode() string {
	if raw := runtimeSetting("MIKAN_RSS_MODE"); raw != "" {
		return raw
	}
	if cfg := s.runtimeConfig(); cfg != nil {
		return cfg.Mikan.RSSMode
	}
	return core.RSSModePersonal
}

func (s *Scheduler) isSeedCleanupEnabled() bool {
	if raw := runtimeSetting("SEED_CLEANUP_ENABLED"); raw != "" {
		return strings.EqualFold(raw, "true") || raw == "1"
	}
	if cfg := s.runtimeConfig(); cfg != nil {
		return cfg.Scheduler.SeedCleanupEnabled
	}
	return false
}

// Start 启动调度器，运行所有定时任务
func (s *Scheduler) Start(ctx context.Context) { <-s.StartAsync(ctx) }

func (s *Scheduler) runTimers(ctx context.Context, settingsChanged <-chan struct{}) {
	log.Println("⏰ 调度器已启动")
	if err := s.repairLegacyEpisodes(ctx); err != nil {
		log.Printf("修复既有剧集标识失败: %v", err)
	}

	// 启动定时备份
	if s.backupManager != nil {
		if err := s.backupManager.Start(); err != nil {
			log.Printf("⚠️ 启动定时备份失败: %v", err)
		}
		defer s.backupManager.Stop()
	}

	schedule := s.runtimeConfig().Scheduler
	rssTicker := s.scheduleTicker("RSS_INTERVAL", schedule.RSSInterval)
	defer rssTicker.Stop()

	orgTicker := s.scheduleTicker("ORGANIZER_INTERVAL", schedule.OrganizerInterval)
	defer orgTicker.Stop()

	suppTicker := s.scheduleTicker("SUPPLEMENT_INTERVAL", schedule.SupplementInterval)
	bgmTicker := s.scheduleTicker("BGMTV_SYNC_INTERVAL", schedule.SyncBangumiInterval)
	defer bgmTicker.Stop()
	defer suppTicker.Stop()

	// 种子自动清理定时器（常驻检查，动态响应设置变化）
	seedCleanupTicker := s.scheduleTicker("SEED_CLEANUP_INTERVAL", schedule.SeedCleanupInterval)
	defer seedCleanupTicker.Stop()
	if s.isSeedCleanupEnabled() {
		log.Printf("🧹 种子自动清理已启用：间隔=%v, 最小做种时间=%v, 最小比率=%.1f",
			seedCleanupTicker.interval, schedule.SeedCleanupMinSeedTime, schedule.SeedCleanupMinRatio)
	}

	// 下载状态扫描原为 10s 高频轮询（OPTIMIZE_TASK.md 优化点 1）：
	// 每次都会请求 qBittorrent API 并查询数据库，CPU 尖峰主要来源之一。
	// 改为 60s 基础间隔 + 每次触发后重新抖动，下载完成事件的感知延迟从 ~10s 变为 ~60s，
	// 对"整理文件"这一下游动作（本身 2 分钟一轮）无实际影响
	downloadInterval := 60 * time.Second
	downloadTicker := jitteredTicker(downloadInterval)
	defer downloadTicker.Stop()

	// 启动后立即执行一次 RSS 轮询
	s.launch(ctx, s.pollRSS)

	// 延迟 30 秒后执行首次补全扫描
	s.launch(ctx, func(ctx context.Context) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		s.pollSupplement(ctx)
	})
	s.launch(ctx, s.pollSyncBangumi)

	for {
		select {
		case <-ctx.Done():
			log.Println("⏰ 调度器已停止")
			return
		case <-settingsChanged:
			for _, ticker := range []*scheduleTicker{rssTicker, orgTicker, bgmTicker, suppTicker, seedCleanupTicker} {
				ticker.refresh(s, false)
			}
		case <-rssTicker.C:
			// 每次触发后按基础间隔重新抖动，避免抖动量被反复累加或固化
			rssTicker.refresh(s, true)
			s.launch(ctx, s.pollRSS)
		case <-orgTicker.C:
			orgTicker.refresh(s, true)
			s.launch(ctx, s.pollOrganizer)
		case <-bgmTicker.C:
			bgmTicker.refresh(s, true)
			s.launch(ctx, s.pollSyncBangumi)
		case <-suppTicker.C:
			suppTicker.refresh(s, true)
			s.launch(ctx, s.pollSupplement)
		case <-downloadTicker.C:
			resetWithJitter(downloadTicker, downloadInterval)
			s.launch(ctx, s.pollDownloads)
		case <-seedCleanupTicker.C:
			seedCleanupTicker.refresh(s, true)
			if s.isSeedCleanupEnabled() {
				s.launch(ctx, s.pollSeedCleanup)
			}
		}
	}
}

// resetWithJitter 停止旧 ticker 并以新的抖动间隔重启
// 注意：Stop 后 channel 中可能残留一个未消费的 tick，这里主动排空防止紧跟着立即重复触发
func resetWithJitter(ticker *time.Ticker, base time.Duration) {
	ticker.Stop()
	select {
	case <-ticker.C:
	default:
	}
	ticker.Reset(jitteredInterval(base))
}

// pollRSS 执行单次 RSS 轮询
// RSSMode 为 "classic" 时启用自动建番剧（未匹配种子自动创建订阅）
// RSSMode 为 "personal" 时仅下载已匹配订阅的种子
func (s *Scheduler) pollRSS(ctx context.Context) {
	if !s.pollRSSRunning.CompareAndSwap(false, true) {
		log.Println("⏳ 上一轮 RSS 轮询尚未完成，跳过本次触发")
		return
	}
	defer s.pollRSSRunning.Store(false)

	log.Println("🔍 开始 RSS 轮询...")

	var subs []database.Subscription
	if err := database.DB.Where("enabled = ?", true).Find(&subs).Error; err != nil {
		log.Printf("❌ 获取订阅列表失败: %v", err)
		return
	}
	globalURL := strings.TrimSpace(s.getMikanRSSURL())
	mode := s.getRSSMode()
	feeds := make(map[string][]database.Subscription)
	var urls []string
	for _, sub := range subs {
		url := strings.TrimSpace(sub.RSSURL)
		if url == "" {
			continue
		}
		if _, seen := feeds[url]; !seen {
			urls = append(urls, url)
		}
		feeds[url] = append(feeds[url], sub)
	}
	if globalURL != "" {
		if _, seen := feeds[globalURL]; !seen {
			urls = append(urls, globalURL)
		}
		feeds[globalURL] = subs
	}
	for _, url := range urls {
		if ctx.Err() != nil {
			return
		}
		items, err := s.source.FetchRSS(ctx, url)
		if err != nil {
			log.Printf("RSS 抓取失败: %v", err)
			continue
		}
		s.processRSSFeed(ctx, items, feeds[url], mode, url == globalURL)
	}
}

func (s *Scheduler) processRSSFeed(ctx context.Context, items []core.TorrentItem, subs []database.Subscription, mode string, global bool) {
	// 批量去重查询：一次性拉取批次内已有 URL 到内存集合，消除循环 N 次 DB 查询 (OPTIMIZE_TASK.md 第 2 条)
	existingURLs := make(map[string]bool, len(items))
	if len(items) > 0 {
		urls := make([]string, 0, len(items))
		for _, it := range items {
			if it.URL != "" {
				urls = append(urls, it.URL)
			}
		}
		if len(urls) > 0 {
			var records []string
			database.DB.Model(&database.DownloadRecord{}).Where("torrent_url IN ?", urls).Pluck("torrent_url", &records)
			for _, u := range records {
				existingURLs[u] = true
			}
		}
	}

	newCount := 0
	for _, item := range items {
		// 去重检查：通过内存集合判断是否已下载
		if existingURLs[item.URL] {
			continue
		}

		var matchedSub *database.Subscription
		if !global && len(subs) == 1 && mikanFeedMatchesID(subs[0].RSSURL, subs[0].MikanID) {
			matchedSub = &subs[0]
		} else {
			var ambiguous bool
			matchedSub, ambiguous = matchFeedSubscription(item, subs)
			if ambiguous {
				continue
			}
		}
		// 如果匹配到了订阅，或者在 Classic 模式下成功自动创建了订阅，才下发下载任务
		var targetSubID uint
		var targetSeason int
		var targetEpNum float32

		if matchedSub != nil {
			targetSubID = matchedSub.ID

			// 校验订阅过滤器（包含词、排除词、分辨率、允许字幕组）
			subFilter := buildFilter(*matchedSub)
			if !matchSubscriptionFilter(item, subFilter) {
				log.Printf("⏭️ [过滤器拦截] 种子 [%s] 未满足订阅 [%s] 过滤规则", item.Title, matchedSub.TitleCN)
				continue
			}

			targetSeason, targetEpNum = s.resolveSubscriptionEpisode(ctx, item, *matchedSub)
			added, err := s.enqueue(ctx, *matchedSub, item, targetSeason, targetEpNum)
			if err != nil {
				log.Printf("添加下载失败 [%s]: %v", item.Title, err)
				continue
			}
			if !added {
				continue
			}
			log.Printf("📥 匹配订阅下载: %s (ID=%d, S%02dE%.1f)", item.Title, targetSubID, targetSeason, targetEpNum)
		} else {
			// 未匹配到现有订阅，检查 RSS 模式
			if global && mode == core.RSSModeClassic {
				autoSubID, err := autoCreateSubscription(ctx, s, item)
				if err != nil {
					log.Printf("⚠️ 自动创建订阅失败 [%s]: %v", item.Title, err)
					continue
				}
				targetSubID = autoSubID
				log.Printf("✅ 自动创建订阅 [%s]: ID=%d", item.Title, autoSubID)

				var sub database.Subscription
				if err := database.DB.First(&sub, autoSubID).Error; err != nil {
					continue
				}
				if !matchSubscriptionFilter(item, buildFilter(sub)) {
					continue
				}
				targetSeason, targetEpNum = s.resolveSubscriptionEpisode(ctx, item, sub)
				added, err := s.enqueue(ctx, sub, item, targetSeason, targetEpNum)
				if err != nil {
					log.Printf("添加自动下载失败 [%s]: %v", item.Title, err)
					continue
				}
				if !added {
					continue
				}

			} else {
				log.Printf("ℹ️  个人RSS模式: 跳过未匹配种子: %s", item.Title)
				continue
			}
		}

		// 记录到数据库并创建剧集记录
		if item.URL != "" {
			existingURLs[item.URL] = true
		}
		newCount++

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
// 优化点（OPTIMIZE_TASK.md 第 2 条）：原先对每个完成的 qB 任务逐条 First 查询，
// 日志中大量 "record not found" 即来自这里；改为一次性批量拉取所有
// downloading 状态的 episodes 建内存索引，再与 qB 任务列表匹配，
// 将每轮 N+1 次查询降为 1 次，消除无谓的 DB 往返与日志噪音
func (s *Scheduler) pollDownloads(ctx context.Context) {
	if s.downloader == nil {
		return
	}
	if !s.pollDownloadsRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.pollDownloadsRunning.Store(false)

	tasks, err := s.downloader.List(ctx)
	if err != nil {
		log.Printf("❌ 获取下载列表失败: %v", err)
		return
	}

	var episodes []database.Episode
	if err := database.DB.Where("status IN ? OR ((torrent_hash = '' OR torrent_hash LIKE 'url:%') AND status IN ?)", []string{"downloading", "submitting", "retrying", "downloaded"}, []string{"organized"}).Find(&episodes).Error; err != nil {
		log.Printf("查询待核对的剧集失败: %v", err)
		return
	}
	index := newDownloadIndex(tasks, episodes)
	for _, ep := range episodes {
		task, ok := index.match(ep)
		if !ok {
			if (ep.Status == "submitting" || ep.Status == "retrying") && time.Since(ep.UpdatedAt) > 5*time.Minute {
				if err := s.retryEpisode(ctx, ep); err != nil {
					log.Printf("重试剧集 %d 失败: %v", ep.ID, err)
				}
			}
			continue
		}
		updates := map[string]any{}
		if ep.TorrentHash != task.Hash {
			updates["torrent_hash"] = task.Hash
		}
		active := ep.Status == "downloading" || ep.Status == "submitting" || ep.Status == "retrying"
		awaitingFile := (active || ep.Status == "downloaded") && ep.SourceFile == "" && ep.FinalPath == ""
		if awaitingFile && len(task.FollowedBy) == 0 && task.Size > 0 && task.Size != ep.FileSize {
			// RSS 可能只给出种子文件大小；绑定到合集成员之后则要保留该文件自己的大小。
			updates["file_size"] = task.Size
		}
		now := time.Now()
		done := active && task.IsComplete()
		if ep.Status == "downloaded" && ep.TorrentHash != task.Hash && !task.IsComplete() {
			updates["status"] = "downloading"
			updates["download_finished_at"] = nil
		}
		if done {
			updates["status"] = "downloaded"
			updates["download_finished_at"] = now
		} else if active && ep.Status != "downloading" {
			updates["status"] = "downloading"
		}
		if len(updates) == 0 {
			continue
		}
		r := ep.OrganizationSnapshot(database.DB.WithContext(ctx)).Updates(updates)
		if r.Error != nil {
			log.Printf("更新剧集下载状态失败: %v", r.Error)
			continue
		}
		if !done || r.RowsAffected != 1 {
			continue
		}

		log.Printf("📥 下载完成: %s", task.Name)

		// 计算下载耗时（用于通知展示；无起始时间则留空）
		elapsed := ""
		if ep.DownloadStartedAt != nil {
			elapsed = now.Sub(*ep.DownloadStartedAt).Truncate(time.Second).String()
		}

		var animeTitle string
		if ep.SubscriptionID > 0 {
			var sub database.Subscription
			if err := database.DB.Select("title_cn").First(&sub, ep.SubscriptionID).Error; err == nil && sub.TitleCN != "" {
				animeTitle = sub.TitleCN
			}
		}

		if s.bus != nil {
			s.bus.Publish(core.Event{
				Type: core.EventDownloadCompleted,
				Payload: map[string]any{
					"episode_id":  ep.ID,
					"title":       ep.Title,
					"anime_title": animeTitle,
					"episode":     ep.Number,
					"hash":        task.Hash, // 使用 task.Hash，确保即使刚更新也能传正确值
					"size":        task.Size, // 文件大小（字节），通知格式化用
					"duration":    elapsed,   // 下载耗时，通知展示用
				},
				Time: now,
			})
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
	if !s.pollOrganizerRunning.CompareAndSwap(false, true) {
		log.Println("⏳ 上一轮文件整理尚未完成，跳过本次触发")
		return
	}
	defer s.pollOrganizerRunning.Store(false)
	defer finishPendingMoves(ctx)

	// 查询数据库中的待整理 Episode 记录
	var episodes []database.Episode
	if err := database.DB.Where("status IN ?", []string{"downloaded", "renaming"}).Order("CASE WHEN status = 'renaming' THEN 0 ELSE 1 END").Find(&episodes).Error; err != nil {
		log.Printf("❌ 查询待整理文件失败: %v", err)
		return
	}

	if len(episodes) == 0 {
		return
	}

	log.Printf("📂 发现 %d 个待整理的文件", len(episodes))

	// 批量预查相关订阅，避免循环逐条查询 DB (N+1)
	subIDs := make([]uint, 0, len(episodes))
	for _, ep := range episodes {
		if ep.SubscriptionID > 0 {
			subIDs = append(subIDs, ep.SubscriptionID)
		}
	}
	subMap := make(map[uint]database.Subscription, len(subIDs))
	if len(subIDs) > 0 {
		var subs []database.Subscription
		database.DB.Where("id IN ?", subIDs).Find(&subs)
		for _, s := range subs {
			subMap[s.ID] = s
		}
	}

	successCount := 0
	failureCount := 0
	lastOrganizedPath := ""
	lastOrganizedAnimeTitle := ""
	var lastOrganizedEpNum float32
	for i := 0; i < len(episodes); i++ {
		ep := episodes[i]
		// 1. 根据 Subscription 获取番剧元数据
		sub, ok := subMap[ep.SubscriptionID]
		if !ok {
			log.Printf("⚠️  整理跳过，找不到对应的订阅记录: %d", ep.SubscriptionID)
			failureCount++
			continue
		}

		anime := organizedAnime(sub)

		coreEp := core.Episode{
			AnimeID: anime.ID,
			Season:  ep.Season,
			Number:  ep.Number,
			Title:   ep.Title,
		}

		// 只根据已绑定的任务标识整理，避免同名文件或正在校验的文件进入媒体库。
		realPath := ep.FinalPath
		dl := s.downloader
		if dynamic, ok := dl.(*downloader.DynamicDownloader); ok {
			dl = dynamic.GetCurrent()
		}
		if ep.Status == "renaming" {
			if unknownLegacyEpisode(ep, sub) {
				if err := s.recoverLegacyEpisode(ctx, ep, sub); err != nil {
					s.reportOrganizationError(ctx, ep, err)
				}
				continue
			}
			if !existingVideo(realPath) {
				continue
			}
			target, safe := s.legacyRenameDestination(ctx, ep, sub, ep.Season)
			if !safe {
				continue
			}
			var other int64
			if err := database.DB.Unscoped().Model(&database.Episode{}).Where("id <> ? AND final_path IN ?", ep.ID, []string{realPath, target}).Count(&other).Error; err != nil || other > 0 {
				continue
			}
		} else {
			var members []database.Episode
			var err error
			realPath, members, err = s.organizationSource(ctx, ep, sub, dl)
			if err != nil {
				s.reportOrganizationError(ctx, ep, err)
				failureCount++
				continue
			}
			episodes = append(episodes, members...)
			if realPath == "" {
				continue
			}
		}

		// 3. 应用路径模板并整理
		newPath, err := s.commitOrganization(ctx, ep, realPath, anime, coreEp)
		if err != nil {
			s.reportOrganizationError(ctx, ep, err)
			failureCount++
			continue
		}
		successCount++
		lastOrganizedPath = newPath
		lastOrganizedAnimeTitle = anime.TitleCN
		if lastOrganizedAnimeTitle == "" {
			lastOrganizedAnimeTitle = anime.TitleJP
		}
		if lastOrganizedAnimeTitle == "" {
			lastOrganizedAnimeTitle = anime.TitleEN
		}
		lastOrganizedEpNum = coreEp.Number

		if _, err := database.SyncEpisodeProgress(database.DB.WithContext(ctx), sub.ID, sub.Enabled == nil || *sub.Enabled); err != nil {
			log.Printf("更新订阅 %d 整理进度失败: %v", sub.ID, err)
		}
	}

	if s.bus != nil && (successCount > 0 || failureCount > 0) {
		s.bus.Publish(core.Event{
			Type: core.EventFileOrganized,
			Payload: map[string]any{
				"success":     successCount,
				"failed":      failureCount,
				"final_path":  lastOrganizedPath,
				"anime_title": lastOrganizedAnimeTitle,
				"episode":     lastOrganizedEpNum,
			},
			Time: time.Now(),
		})
	}

	if successCount > 0 || failureCount > 0 {
		if successCount == 0 {
			log.Printf("❌ 整理全部失败：共 %d 个文件失败", failureCount)
		} else if failureCount > 0 {
			log.Printf("⚠️  整理部分完成：成功 %d 个，失败 %d 个", successCount, failureCount)
		} else {
			log.Printf("✅ 整理完成：成功 %d 个", successCount)
		}
	}
}

// pollSupplement 执行补全扫描：查找集数不完整的订阅，爬取历史种子补全
func (s *Scheduler) pollSupplement(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	if !s.pollSupplementRunning.CompareAndSwap(false, true) {
		log.Println("⏳ 上一轮追番补全检查尚未完成，跳过本次触发")
		return
	}
	defer s.pollSupplementRunning.Store(false)

	log.Println("🔍 开始补全扫描...")

	var subs []database.Subscription
	db := database.DB.WithContext(ctx)
	if err := db.Where("enabled = ?", true).Find(&subs).Error; err != nil {
		log.Printf("读取待补全订阅失败: %v", err)
		return
	}
	ids := make([]uint, 0, len(subs))
	for _, sub := range subs {
		ids = append(ids, sub.ID)
	}
	counts, err := database.FinishedEpisodeCounts(db, ids)
	if err != nil {
		log.Printf("读取补全进度失败: %v", err)
		return
	}
	for i := range subs {
		sub := &subs[i]
		count := counts[sub.ID]
		if count != sub.CurrentEpisodes || (sub.TotalEpisodes > 0 && sub.Completed != (count >= sub.TotalEpisodes)) {
			current, err := database.SyncEpisodeProgress(db, sub.ID, true)
			if err != nil {
				log.Printf("修复订阅进度失败: %v", err)
				return
			}
			sub.CurrentEpisodes = current
			if sub.TotalEpisodes > 0 {
				sub.Completed = current >= sub.TotalEpisodes
			}
		}
	}

	if len(subs) == 0 {
		log.Println("✅ 补全扫描完成: 无需补全的订阅")
		return
	}

	// 过滤：(未完结 AND 集数不足) OR (已完结但本地 organized 集数少于总集数)
	var targets []database.Subscription
	for _, sub := range subs {
		// 情况 1: 未完结且集数不足
		if !sub.Completed && (sub.TotalEpisodes == 0 || sub.CurrentEpisodes < sub.TotalEpisodes) {
			targets = append(targets, sub)
			continue
		}
		// 情况 2: 虽然标记为已完结，但本地 organized 的集数依然少于总集数（可能用户手动删除了文件或之前下载不全）
		if sub.Completed && sub.TotalEpisodes > 0 && sub.CurrentEpisodes < sub.TotalEpisodes {
			log.Printf("ℹ️  订阅 [%s] 虽然已完结，但本地集数 (%d/%d) 不全，加入补全扫描", sub.TitleCN, sub.CurrentEpisodes, sub.TotalEpisodes)
			targets = append(targets, sub)
		}
	}

	if len(targets) == 0 {
		log.Println("✅ 补全扫描完成: 所有订阅集数已齐备")
		return
	}

	log.Printf("📋 发现 %d 个需要补全的订阅", len(targets))

	for _, sub := range targets {
		if ctx.Err() != nil {
			return
		}
		s.supplementOne(ctx, sub)
	}

	log.Println("✅ 补全扫描完成")
}

// TriggerSupplement 对单个订阅执行补全扫描（供 API 调用）
func (s *Scheduler) TriggerSupplement(ctx context.Context, subID uint) error {
	ctx, finish, err := s.beginWork(ctx)
	if err != nil {
		return err
	}
	defer finish()
	var sub database.Subscription
	if err := database.DB.WithContext(ctx).First(&sub, subID).Error; err != nil {
		return err
	}
	if sub.Enabled != nil && !*sub.Enabled {
		return fmt.Errorf("订阅 %d 已停用", subID)
	}
	return s.supplementOne(ctx, sub)
}

// supplementOne 对单个订阅执行补全逻辑
type supplementRun struct {
	done chan struct{}
	err  error
}

func (s *Scheduler) supplementOne(ctx context.Context, sub database.Subscription) error {
	s.runMu.Lock()
	if previous := s.scans[sub.ID]; previous != nil {
		s.runMu.Unlock()
		select {
		case <-previous.done:
			return previous.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.scans == nil {
		s.scans = make(map[uint]*supplementRun)
	}
	run := &supplementRun{done: make(chan struct{})}
	s.scans[sub.ID] = run
	s.runMu.Unlock()
	defer func() { s.runMu.Lock(); delete(s.scans, sub.ID); close(run.done); s.runMu.Unlock() }()
	run.err = s.scanSubscription(ctx, sub)
	return run.err
}

func (s *Scheduler) scanSubscription(ctx context.Context, sub database.Subscription) (scanErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	// 多季同名时，仅凭片名无法判断未标季号的资源或数据源的搜索结果属于哪一季。
	// 有明确的站点 ID 时，仍可按该 ID 匹配。
	var peer database.Subscription
	matches := database.DB.WithContext(ctx).Where("enabled = ? AND title_cn = ? AND id <> ? AND COALESCE(NULLIF(season, 0), 1) <> ?", true, sub.TitleCN, sub.ID, max(1, sub.Season)).Select("id").Limit(1).Find(&peer)
	if matches.Error != nil {
		return matches.Error
	}
	multipleSeasons := matches.RowsAffected != 0
	if !multipleSeasons {
		current, err := s.resolveSubscriptionIDs(ctx, &sub)
		if err != nil || !current {
			return err
		}
	}
	if id := s.subscriptionMetadataID(sub); id != "" {
		getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var anime core.Anime
		var err error
		if md, ok := s.metadataProvider.(core.SeasonMetadataProvider); ok {
			anime, err = md.GetAnimeForSeason(getCtx, id, max(1, sub.Season))
		} else {
			anime, err = s.metadataProvider.GetAnime(getCtx, id)
		}
		cancel()
		if err == nil {
			updates := map[string]interface{}{}
			if sub.MetadataID == "" {
				updates["metadata_id"] = id
			}
			if sub.TotalEpisodes == 0 && anime.TotalEps > 0 {
				updates["total_episodes"] = gorm.Expr("COALESCE(NULLIF(total_episodes, 0), ?)", anime.TotalEps)
			}
			if sub.Year == 0 && anime.Year > 0 {
				updates["year"] = anime.Year
			}
			if sub.AnimeType == "" && anime.Type != "" {
				updates["anime_type"] = anime.Type
			}
			if sub.Description == "" && anime.Description != "" {
				updates["description"] = anime.Description
			}
			if sub.CoverURL == "" && anime.CoverURL != "" {
				updates["cover_url"] = anime.CoverURL
			}
			if sub.MetadataProvider == "" {
				updates["metadata_provider"] = s.metadataProvider.Name()
			}
			if sub.TMDBID == "" && anime.TMDBID != "" {
				updates["tmdb_id"] = anime.TMDBID
			}
			if sub.IMDBID == "" && anime.IMDBID != "" {
				updates["imdb_id"] = anime.IMDBID
			}

			if len(updates) > 0 {
				saved := database.DB.WithContext(ctx).Model(&sub).
					Where("COALESCE(NULLIF(season, 0), 1) = ? AND COALESCE(metadata_id, '') = ? AND COALESCE(metadata_provider, '') = ? AND COALESCE(tmdb_id, '') = ?",
						max(1, sub.Season), sub.MetadataID, sub.MetadataProvider, sub.TMDBID).
					Clauses(clause.Returning{}).Updates(updates)
				if saved.Error != nil || saved.RowsAffected == 0 {
					return saved.Error
				}
				log.Printf("✅ 已通过 %s 补全订阅 [%s] 的元数据", s.metadataProvider.Name(), sub.TitleCN)
			}
		}
	} else if sub.MikanID != "" {
		// 兜底：如果没配 metadataProvider，或者 BGM 获取失败，尝试从 Mikan 页面直接抓取
		if mikan := source.MikanFrom(s.source); mikan != nil {
			info, err := mikan.GetAnimeInfo(ctx, sub.MikanID)
			if err == nil {
				updates := map[string]interface{}{}
				if sub.TotalEpisodes == 0 && info.TotalEpisodes > 0 {
					updates["total_episodes"] = info.TotalEpisodes
				}
				if sub.Year == 0 && info.Year > 0 {
					updates["year"] = info.Year
				}
				if len(updates) > 0 {
					database.DB.Model(&sub).Updates(updates)
					log.Printf("✅ 已通过 Mikan 详情页补全订阅 [%s] 的元数据: %+v", sub.TitleCN, updates)
				}
			}
		}
	}

	filter := buildFilter(sub)

	newCount := 0
	currentEpisodes := sub.CurrentEpisodes
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
		defer func() {
			data := map[string]any{
				"subscription_id": sub.ID, "title": sub.TitleCN, "new_count": newCount,
				"current_episodes": currentEpisodes, "total_episodes": sub.TotalEpisodes,
			}
			if scanErr != nil {
				data["error"] = scanErr.Error()
			}
			s.bus.Publish(core.Event{Type: core.EventSupplementCompleted, Time: time.Now(), Payload: data})
		}()
	}

	items, err := s.source.FetchHistory(ctx, sub.MikanID, filter)
	if err != nil {
		scanErr = err
		log.Printf("❌ 获取历史种子失败 [%s]: %v", sub.TitleCN, err)
		return
	}

	log.Printf("ℹ️  补全 [%s]: 获取到 %d 个历史种子", sub.TitleCN, len(items))

	if s.bus != nil {
		s.bus.Publish(core.Event{
			Type: "supplement.progress",
			Payload: map[string]any{
				"subscription_id": sub.ID,
				"title":           sub.TitleCN,
				"message":         fmt.Sprintf("获取到 %d 个种子，准备筛选...", len(items)),
			},
			Time: time.Now(),
		})
	}

	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if multipleSeasons {
			if strings.EqualFold(item.SourceName, "Mikan") && sub.MikanID != "" {
				if item.BangumiID != "" && item.BangumiID != sub.MikanID {
					continue
				}
			} else {
				episode := parser.ExtractEpisodeDetailed(item.Title)
				if !episode.ExplicitSeason || episode.Season != max(1, sub.Season) {
					continue
				}
			}
		}
		if !matchesHistoryRelease(item, sub, filter) {
			continue
		}

		if item.URL != "" && isDuplicate(item.URL) {
			continue
		}
		if item.InfoHash != "" && isEpisodeExists(item.InfoHash) {
			continue
		}

		season, epNum := s.resolveSubscriptionEpisode(ctx, item, sub)
		added, err := s.enqueue(ctx, sub, item, season, epNum)
		if err != nil {
			scanErr = err
			log.Printf("添加补全下载失败 [%s]: %v", item.Title, err)
			continue
		}
		if !added {
			continue
		}

		if s.bus != nil {
			s.bus.Publish(core.Event{
				Type: "supplement.progress",
				Payload: map[string]any{
					"subscription_id": sub.ID,
					"title":           sub.TitleCN,
					"message":         fmt.Sprintf("已添加下载: %s", item.Title),
				},
				Time: time.Now(),
			})
		}
		newCount++
	}

	if newCount > 0 {
		log.Printf("✅ 补全 [%s]: 新增 %d 个下载", sub.TitleCN, newCount)
	}

	currentEpisodes, err = database.SyncEpisodeProgress(database.DB.WithContext(ctx), sub.ID, true)
	if err != nil {
		scanErr = err
		log.Printf("更新订阅 %d 补全进度失败: %v", sub.ID, err)
	}
	return scanErr
}

// organizedAnime 保留整理路径所用标识，同时携带本地已有的刮削信息。
func organizedAnime(sub database.Subscription) core.Anime {
	id, provider := sub.MetadataID, sub.MetadataProvider
	if id == "" || provider == "" {
		id, provider = sub.MikanID, "mikan"
		if id == "" {
			id = sub.BangumiID
		}
	}
	return core.Anime{
		ID: id, Provider: provider, TitleCN: sub.TitleCN, TitleJP: sub.TitleJP, TitleEN: sub.TitleEN,
		Year: sub.Year, Season: sub.Season, Type: sub.AnimeType, TotalEps: sub.TotalEpisodes,
		Description: sub.Description, CoverURL: sub.CoverURL, SeriesID: sub.SeriesID,
		TMDBID: sub.TMDBID, IMDBID: sub.IMDBID,
	}
}

// publishDownloadFailure 将下载器拒绝添加任务的结果送到通知与 Webhook。
func (s *Scheduler) publishDownloadFailure(item core.TorrentItem, subID uint, err error) {
	if s.bus != nil {
		s.bus.Publish(core.Event{Type: core.EventDownloadFailed, Time: time.Now(), Payload: map[string]any{
			"subscription_id": subID, "title": item.Title, "error": err.Error(),
		}})
	}
}

// buildFilter 从订阅的 FilterJSON 构建 core.Filter
func buildFilter(sub database.Subscription) core.Filter {
	filter := core.Filter{
		PreferSubgroup: sub.SubgroupName,
		Titles:         []string{sub.TitleCN, sub.TitleEN, sub.TitleJP},
	}

	// 解析 AllowedSubgroups (JSON 数组字符串)
	if sub.AllowedSubgroups != "" && sub.AllowedSubgroups != "[]" {
		var allowedSubgroups []string
		if err := json.Unmarshal([]byte(sub.AllowedSubgroups), &allowedSubgroups); err == nil {
			filter.AllowedSubgroups = allowedSubgroups
		}
	}
	// 兼容老版本将时间表多选保存为一个逗号分隔名称的记录。
	if groups := strings.Split(filter.PreferSubgroup, ","); len(groups) > 1 {
		filter.PreferSubgroup = ""
		if len(filter.AllowedSubgroups) == 0 {
			for _, group := range groups {
				if group = strings.TrimSpace(group); group != "" {
					filter.AllowedSubgroups = append(filter.AllowedSubgroups, group)
				}
			}
		}
	}

	// 解析 ExcludedKeywords
	if sub.ExcludedKeywords != "" {
		var excludedKeywords []string
		if err := json.Unmarshal([]byte(sub.ExcludedKeywords), &excludedKeywords); err == nil {
			filter.ExcludeKeywords = excludedKeywords
		}
	}

	if sub.FilterJSON != "" {
		var stored struct {
			IncludeKeywords []string `json:"include_keywords"`
			ExcludeKeywords []string `json:"exclude_keywords"`
			Resolution      string   `json:"resolution"`
		}
		if err := json.Unmarshal([]byte(sub.FilterJSON), &stored); err == nil {
			filter.IncludeKeywords = stored.IncludeKeywords
			// Merge exclude keywords from FilterJSON
			if len(stored.ExcludeKeywords) > 0 {
				filter.ExcludeKeywords = append(filter.ExcludeKeywords, stored.ExcludeKeywords...)
			}
			if stored.Resolution != "" {
				filter.Resolution = stored.Resolution
			}
		}
	}
	return filter
}

// matchSubscriptionFilter 校验种子条目是否匹配订阅的过滤规则（包含词、排除词、分辨率、允许字幕组）
func matchSubscriptionFilter(item core.TorrentItem, filter core.Filter) bool {
	title := item.Title
	titleLower := strings.ToLower(title)

	// 1. 排除关键词：一旦命中任意一个排除词，立即拒绝
	for _, kw := range filter.ExcludeKeywords {
		trimmed := strings.TrimSpace(kw)
		if trimmed != "" && strings.Contains(titleLower, strings.ToLower(trimmed)) {
			return false
		}
	}

	// 2. 包含关键词：若配置了包含词列表，则必须至少包含其中一个
	if len(filter.IncludeKeywords) > 0 {
		matched := false
		for _, kw := range filter.IncludeKeywords {
			trimmed := strings.TrimSpace(kw)
			if trimmed != "" && strings.Contains(titleLower, strings.ToLower(trimmed)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 3. 分辨率过滤
	if filter.Resolution != "" {
		res := strings.ToLower(strings.TrimSpace(filter.Resolution))
		if !strings.Contains(titleLower, res) && !strings.EqualFold(item.Resolution, res) {
			return false
		}
	}

	// 指定首选字幕组时，历史与实时来源使用同一筛选条件。
	if preferred := strings.TrimSpace(filter.PreferSubgroup); preferred != "" && !strings.Contains(titleLower, strings.ToLower(preferred)) && !strings.EqualFold(item.GroupName, preferred) {
		return false
	}

	// 4. 允许的字幕组列表
	if len(filter.AllowedSubgroups) > 0 {
		matched := false
		for _, allowed := range filter.AllowedSubgroups {
			trimmed := strings.TrimSpace(allowed)
			if trimmed != "" && (strings.Contains(title, trimmed) || strings.EqualFold(item.GroupName, trimmed)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// isDuplicate 检查种子 URL 是否已存在下载记录
func isDuplicate(torrentURL string) bool {
	if database.DB == nil {
		return false
	}
	var count int64
	database.DB.Model(&database.DownloadRecord{}).
		Where("torrent_url = ?", torrentURL).
		Count(&count)
	return count > 0
}

// isEpisodeExists 检查通过 InfoHash 是否已存在 Episode 记录
func isEpisodeExists(infoHash string) bool {
	if database.DB == nil {
		return false
	}
	var count int64
	database.DB.Model(&database.Episode{}).
		Where("torrent_hash = ?", infoHash).
		Count(&count)
	return count > 0
}

// 自动订阅：从 Mikan RSS 未匹配的种子自动创建订阅
// ============================================================

// autoCreateSubscription 自动创建订阅
// 从种子详情页爬取 Mikan BangumiID，然后创建 Subscription 记录
func autoCreateSubscription(ctx context.Context, s *Scheduler, item core.TorrentItem) (uint, error) {
	// 1. 提取 CleanTitle
	parsed := source.ParseMikanTitle(item.Title)
	cleanTitle := parsed.Title
	if cleanTitle == "" {
		cleanTitle = strings.TrimSpace(item.Title)
	}

	// 2. 尝试从种子详情页爬取 Mikan BangumiID 和封面图
	mikanBangumiID, mikanCover := extractMikanMetadata(ctx, s, item)

	// 3. 如果没有详情页 URL 或爬取失败，回退到标题搜索
	if mikanBangumiID == "" {
		log.Printf("ℹ️  [自动订阅] 尝试通过标题搜索 BangumiID: %s", cleanTitle)
		return createSubscriptionFromTitle(ctx, s, item, cleanTitle)
	}

	// 同一批次中新建的订阅也可能再次命中蜜柑条目。
	var prior database.Subscription
	if result := database.DB.Where("mikan_id = ?", mikanBangumiID).Limit(1).Find(&prior); result.Error != nil {
		return 0, result.Error
	} else if prior.ID != 0 {
		if prior.Enabled != nil && !*prior.Enabled {
			return 0, fmt.Errorf("订阅已停用")
		}
		return prior.ID, nil
	}

	// 5. 创建订阅（事务内）
	rssURL := ""
	if mikan := source.MikanFrom(s.source); mikan != nil {
		rssURL, _ = mikan.ResolveFirstRSSURL(ctx, mikanBangumiID)
	}
	var subID uint
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// 事务内再次检查重复
		var existing database.Subscription
		result := tx.Where("mikan_id = ?", mikanBangumiID).First(&existing)
		if result.RowsAffected > 0 {
			if existing.Enabled != nil && !*existing.Enabled {
				return fmt.Errorf("订阅已停用")
			}
			subID = existing.ID
			return nil
		}

		sub := database.Subscription{
			TitleCN:    cleanTitle,
			BangumiID:  mikanBangumiID,
			MikanID:    mikanBangumiID,
			RSSURL:     rssURL,
			CoverURL:   mikanCover,
			Enabled:    &[]bool{true}[0],
			SourceName: "Mikan",
		}

		if err := tx.Create(&sub).Error; err != nil {
			return fmt.Errorf("创建订阅失败: %w", err)
		}
		subID = sub.ID
		log.Printf("✅ [自动订阅] 已创建订阅 ID=%d: %s (BangumiID=%s)", subID, cleanTitle, mikanBangumiID)
		return nil
	})
	if err != nil {
		return 0, err
	}

	// 6. 触发补全扫描（非事务，可失败）
	if s.metadataProvider != nil {
		s.launch(ctx, func(ctx context.Context) {
			suppCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			var sub database.Subscription
			if database.DB.First(&sub, subID).Error == nil {
				s.supplementOne(suppCtx, sub)
			}
		})
	}

	return subID, nil
}

// extractMikanMetadata 从 Mikan 种子详情页提取 BangumiID 和封面图
// 详情页 URL 在 TorrentItem.EpisodeURL 中，格式如 https://mikanime.tv/Home/Episode/<hash>
// 页面中包含: <button data-bangumiid="3899" ...> 或 <a href="/Home/Bangumi/3899">
// 封面图从 <div class="bangumi-poster" style="background-image:url('/path/to/poster')"> 提取
func extractMikanMetadata(ctx context.Context, s *Scheduler, item core.TorrentItem) (string, string) {
	if item.EpisodeURL == "" {
		return "", ""
	}

	// 使用 MikanSource 的抓取能力（镜像回退等）
	mikanSrc := source.MikanFrom(s.source)
	if mikanSrc == nil {
		return "", ""
	}

	// 解析详情页 URL 的路径
	u, err := url.Parse(item.EpisodeURL)
	if err != nil {
		return "", ""
	}

	// 用 MikanSource 的域名构造完整 URL
	domain := mikanSrc.GetDomain()
	detailURL := fmt.Sprintf("https://%s%s", domain, u.Path)

	// 自己发起 HTTP 请求抓取详情页（复用全局 HTTP 连接池，避免每请求新建 Client）
	httpClient := httpx.Default
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("⚠️  [自动订阅] 抓取详情页失败: %v", err)
		return "", ""
	}
	defer resp.Body.Close()

	// 限制详情页读取上限 10MB（正常页面远小于此值），防止异常响应撑爆内存
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", ""
	}

	// 优先从 data-bangumiid 属性提取（最精确）
	var bangumiID string
	doc.Find("button.js-subscribe_bangumi_page").Each(func(_ int, sel *goquery.Selection) {
		if id, exists := sel.Attr("data-bangumiid"); exists && id != "" {
			bangumiID = id
		}
	})

	// 回退：从 /Home/Bangumi/<id> 链接中提取
	if bangumiID == "" {
		re := regexp.MustCompile(`/Home/Bangumi/(\d+)`)
		doc.Find("a[href*='/Home/Bangumi/']").Each(func(_ int, sel *goquery.Selection) {
			if href, exists := sel.Attr("href"); exists {
				if m := re.FindStringSubmatch(href); m != nil {
					bangumiID = m[1]
				}
			}
		})
	}

	// 提取海报图
	var coverURL string
	doc.Find(".bangumi-poster").Each(func(_ int, sel *goquery.Selection) {
		style, exists := sel.Attr("style")
		if exists && strings.Contains(style, "background-image") {
			// 提取 background-image:url('/path') 中的路径
			re := regexp.MustCompile(`url\(['"]?([^'"]+)['"]?\)`)
			if m := re.FindStringSubmatch(style); m != nil {
				path := m[1]
				if strings.HasPrefix(path, "/") {
					coverURL = "https://" + domain + path
				} else {
					coverURL = path
				}
			}
		}
	})

	return bangumiID, coverURL
}

// createSubscriptionFromTitle 当无法从详情页获取 BangumiID 时，通过标题搜索回退创建
func createSubscriptionFromTitle(ctx context.Context, s *Scheduler, item core.TorrentItem, cleanTitle string) (uint, error) {
	if s.metadataProvider == nil || cleanTitle == "" {
		return 0, fmt.Errorf("无法识别番剧: %s", cleanTitle)
	}

	provider := s.metadataProvider.Name()
	season := 0
	if strings.EqualFold(provider, "TMDB") {
		detail := parser.ExtractEpisodeDetailed(item.Title)
		if detail.ExplicitSeason && detail.Season > 0 {
			season = detail.Season
			// TMDB 按整部作品搜索；资源标题中明确指定的季号用于建立订阅。
			cleanTitle = strings.TrimSpace(releaseSeason.ReplaceAllString(cleanTitle, ""))
			if cleanTitle == "" {
				return 0, fmt.Errorf("无法识别番剧: %s", item.Title)
			}
		}
	}
	results, err := s.metadataProvider.SearchAnime(ctx, cleanTitle)
	if err != nil {
		return 0, fmt.Errorf("搜索番剧失败: %w", err)
	}
	if len(results) == 0 {
		return 0, fmt.Errorf("未找到匹配的番剧: %s", cleanTitle)
	}

	for _, anime := range results {
		sub := database.Subscription{TitleCN: cleanTitle}
		if sameAnimeTitle(sub, anime) {
			anime.Provider = provider
			if season > 0 {
				anime.Season = season
			}
			return s.autoSubscribe(ctx, anime)
		}
	}
	return 0, fmt.Errorf("无法核实番剧: %s", cleanTitle)
}

// bestMatch 从 Bangumi 搜索结果中选最佳匹配
func bestMatch(results []core.Anime, cleanTitle string) *core.Anime {
	if len(results) == 0 {
		return nil
	}

	cleanLower := strings.ToLower(cleanTitle)

	// 1. TitleCN 完全匹配优先
	for i := range results {
		if strings.ToLower(results[i].TitleCN) == cleanLower ||
			strings.ToLower(results[i].TitleEN) == cleanLower {
			return &results[i]
		}
	}

	// 2. CleanTitle 包含在 TitleCN 中且长度最长
	var best *core.Anime
	var maxLen int
	for i := range results {
		title := strings.ToLower(results[i].TitleCN)
		if strings.Contains(title, cleanLower) || strings.Contains(cleanLower, title) {
			if len(title) > maxLen {
				maxLen = len(title)
				best = &results[i]
			}
		}
	}

	if best != nil {
		return best
	}

	// 3. 直接返回第一个结果（兜底）
	return &results[0]
}

// pollSeedCleanup 清理已做种满足条件的种子（仅删任务记录，不删文件）
func (s *Scheduler) pollSeedCleanup(ctx context.Context) {
	if s.downloader == nil {
		return
	}
	if !s.pollSeedCleanupRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.pollSeedCleanupRunning.Store(false)

	log.Println("🧹 开始种子自动清理扫描...")

	var defaultMinSeedTime time.Duration = 48 * time.Hour
	var defaultMinRatio float64 = 1.0
	if cfg := s.runtimeConfig(); cfg != nil {
		defaultMinSeedTime = cfg.Scheduler.SeedCleanupMinSeedTime
		defaultMinRatio = cfg.Scheduler.SeedCleanupMinRatio
	}
	minSeedTime := s.durationSetting("SEED_CLEANUP_MIN_SEED_TIME", defaultMinSeedTime, 0)
	minRatio := defaultMinRatio
	if value, err := strconv.ParseFloat(runtimeSetting("SEED_CLEANUP_MIN_RATIO"), 64); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
		minRatio = value
	}

	canDelete := func(task core.DownloadTask) bool {
		// 严密保护：如果数据库未就绪或未在本地剧集库找到对应记录，坚决拒绝清理（防止误伤用户私人种子）
		if database.DB == nil {
			return false
		}
		cleanHash := strings.TrimSpace(task.Hash)
		if cleanHash == "" {
			return false
		}

		var eps []database.Episode
		if err := database.DB.Where("(LOWER(torrent_hash) = LOWER(?) OR torrent_hash = ?) AND deleted_at IS NULL", cleanHash, cleanHash).Find(&eps).Error; err != nil || len(eps) == 0 {
			// 未在本地剧集库中找到对应记录（非本系统管理或尚未关联），为安全起见绝不删除未受管种子
			return false
		}

		// 检查关联的所有剧集：必须严格满足状态为 organized，且 FinalPath 非空且 OrganizedAt != nil
		for _, ep := range eps {
			if ep.Status != "organized" || strings.TrimSpace(ep.FinalPath) == "" || ep.OrganizedAt == nil {
				return false
			}
		}
		return true
	}

	cleaned, err := downloader.CleanSatisfiedSeeds(ctx, s.downloader, minSeedTime, minRatio, canDelete)
	if err != nil {
		log.Printf("❌ 种子自动清理执行失败: %v", err)
		return
	}

	if cleaned > 0 {
		log.Printf("✅ 种子自动清理完成：清理 %d 个", cleaned)
	}
}

// episodeGroupExists 检查同一订阅下该季该集该组是否已有记录（多组共存核心逻辑）
func episodeGroupExists(subscriptionID uint, season int, number float32, groupName string) bool {
	if database.DB == nil {
		return false
	}
	var count int64
	database.DB.Model(&database.Episode{}).
		Where("subscription_id = ? AND season = ? AND number = ? AND group_name = ? AND deleted_at IS NULL",
			subscriptionID, season, number, groupName).
		Count(&count)
	return count > 0
}
