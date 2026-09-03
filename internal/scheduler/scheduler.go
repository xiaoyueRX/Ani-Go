// Package scheduler 实现定时任务调度
// 负责周期性轮询 RSS、去重检查、下发下载任务
package scheduler

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/backup"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
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
	backupManager    *backup.BackupManager // 备份管理器
}

// New 创建调度器实例
func New(cfg *config.Config, source core.Source, dl core.Downloader, org core.Organizer, bus core.EventBus, md core.MetadataProvider, aic ai.Classifier) *Scheduler {
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
	return s
}

func (s *Scheduler) GetBackupManager() *backup.BackupManager {
	return s.backupManager
}

// jitteredTicker 创建带随机抖动（±20%）的定时器
// 优化点：固定间隔会让多个 ticker 在同一时刻集中触发，造成周期性 CPU 尖峰；
// 抖动把触发点打散，削平峰值且不改变轮询频率的期望值
func jitteredTicker(interval time.Duration) *time.Ticker {
	if interval <= 0 {
		interval = time.Minute
	}
	// rand 数值仅用于调度抖动，无安全要求，直接用全局源即可
	jitter := time.Duration(rand.Float64()*0.4*float64(interval)) - interval/5 // ±20%
	return time.NewTicker(interval + jitter)
}

// Start 启动调度器，运行所有定时任务
func (s *Scheduler) Start(ctx context.Context) {
	log.Println("⏰ 调度器已启动")

	// 启动定时备份
	if s.backupManager != nil {
		if err := s.backupManager.Start(); err != nil {
			log.Printf("⚠️ 启动定时备份失败: %v", err)
		}
		defer s.backupManager.Stop()
	}

	rssTicker := jitteredTicker(s.cfg.Scheduler.RSSInterval)
	defer rssTicker.Stop()

	orgTicker := jitteredTicker(s.cfg.Scheduler.OrganizerInterval)
	defer orgTicker.Stop()

	suppTicker := jitteredTicker(s.cfg.Scheduler.SupplementInterval)
	bgmTicker := jitteredTicker(s.cfg.Scheduler.SyncBangumiInterval)
	defer bgmTicker.Stop()
	defer suppTicker.Stop()

	// 种子自动清理定时器
	var seedCleanupTicker *time.Ticker
	var seedCleanupChan <-chan time.Time
	if s.cfg.Scheduler.SeedCleanupEnabled {
		seedCleanupTicker = jitteredTicker(s.cfg.Scheduler.SeedCleanupInterval)
		seedCleanupChan = seedCleanupTicker.C
		defer seedCleanupTicker.Stop()
		log.Printf("🧹 种子自动清理已启用：间隔=%v, 最小做种时间=%v, 最小比率=%.1f",
			s.cfg.Scheduler.SeedCleanupInterval, s.cfg.Scheduler.SeedCleanupMinSeedTime, s.cfg.Scheduler.SeedCleanupMinRatio)
	}

	// 下载状态扫描原为 10s 高频轮询（OPTIMIZE_TASK.md 优化点 1）：
	// 每次都会请求 qBittorrent API 并查询数据库，CPU 尖峰主要来源之一。
	// 改为 60s 基础间隔 + 每次触发后重新抖动，下载完成事件的感知延迟从 ~10s 变为 ~60s，
	// 对"整理文件"这一下游动作（本身 2 分钟一轮）无实际影响
	downloadInterval := 60 * time.Second
	downloadTicker := jitteredTicker(downloadInterval)
	defer downloadTicker.Stop()

	// 启动后立即执行一次 RSS 轮询
	go s.pollRSS(ctx)

	// 延迟 30 秒后执行首次补全扫描
	go func() {
		time.Sleep(30 * time.Second)
		s.pollSupplement(ctx)
	}()
	go s.pollSyncBangumi(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("⏰ 调度器已停止")
			return
		case <-rssTicker.C:
			// 每次触发后按基础间隔重新抖动，避免抖动量被反复累加或固化
			resetWithJitter(rssTicker, s.cfg.Scheduler.RSSInterval)
			go s.pollRSS(ctx)
		case <-orgTicker.C:
			resetWithJitter(orgTicker, s.cfg.Scheduler.OrganizerInterval)
			go s.pollOrganizer(ctx)
	case <-bgmTicker.C:
			resetWithJitter(bgmTicker, s.cfg.Scheduler.SyncBangumiInterval)
			go s.pollSyncBangumi(ctx)
		case <-suppTicker.C:
			resetWithJitter(suppTicker, s.cfg.Scheduler.SupplementInterval)
			go s.pollSupplement(ctx)
		case <-downloadTicker.C:
			resetWithJitter(downloadTicker, downloadInterval)
			go s.pollDownloads(ctx)
		case <-seedCleanupChan:
			if seedCleanupTicker != nil {
				resetWithJitter(seedCleanupTicker, s.cfg.Scheduler.SeedCleanupInterval)
				go s.pollSeedCleanup(ctx)
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
	jitter := time.Duration(rand.Float64()*0.4*float64(base)) - base/5
	ticker.Reset(base + jitter)
}

// pollRSS 执行单次 RSS 轮询
// RSSMode 为 "classic" 时启用自动建番剧（未匹配种子自动创建订阅）
// RSSMode 为 "personal" 时仅下载已匹配订阅的种子
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

			// 预先清理标题中的空格和特殊字符进行匹配
			itemTitleNorm := core.NormalizeTitle(item.Title)

			if sub.TitleCN != "" && strings.Contains(itemTitleNorm, core.NormalizeTitle(sub.TitleCN)) {
				currentMatchLen = len(sub.TitleCN)
			}
			if sub.TitleEN != "" && strings.Contains(itemTitleNorm, core.NormalizeTitle(sub.TitleEN)) {
				if len(sub.TitleEN) > currentMatchLen {
					currentMatchLen = len(sub.TitleEN)
				}
			}
			if sub.TitleJP != "" && strings.Contains(itemTitleNorm, core.NormalizeTitle(sub.TitleJP)) {
				if len(sub.TitleJP) > currentMatchLen {
					currentMatchLen = len(sub.TitleJP)
				}
			}

			if currentMatchLen > maxMatchLen {
				matchedSub = sub
				maxMatchLen = currentMatchLen
			}
		}

		// 个人 RSS 模式下的二次校验：确保标题解析出的关键部分确实匹配
		if matchedSub != nil && s.cfg.Mikan.RSSMode == core.RSSModePersonal {
			parsed := source.ParseMikanTitle(item.Title)
			// 如果解析出的标题与订阅标题差异过大，则拒绝匹配（防止误伤）
			if !strings.Contains(core.NormalizeTitle(parsed.Title), core.NormalizeTitle(matchedSub.TitleCN)) &&
				!strings.Contains(core.NormalizeTitle(matchedSub.TitleCN), core.NormalizeTitle(parsed.Title)) {
				log.Printf("⚠️  [Personal模式] 匹配校验未通过: item=%s, parsed=%s, matched=%s", item.Title, parsed.Title, matchedSub.TitleCN)
				matchedSub = nil
			}
		}

		// 如果匹配到了订阅，或者在 Classic 模式下成功自动创建了订阅，才下发下载任务
		var targetSubID uint
		if matchedSub != nil {
			targetSubID = matchedSub.ID
			savePath := s.cfg.Organizer.TVBasePath
			if matchedSub.CustomPath != "" {
				savePath = matchedSub.CustomPath
			}
			// 使用订阅的字幕组作为标签
			if matchedSub.SubgroupName != "" {
				item.GroupName = matchedSub.SubgroupName
			}

			// 下发下载任务
			if s.downloader != nil {
				if err := s.downloader.Add(ctx, item, savePath); err != nil {
					log.Printf("❌ 添加下载失败 [%s]: %v", item.Title, err)
					continue
				}
			}
			log.Printf("📥 匹配订阅下载: %s (ID=%d)", item.Title, targetSubID)
		} else {
			// 未匹配到现有订阅，检查 RSS 模式
			if s.cfg.Mikan.RSSMode == core.RSSModeClassic {
				autoSubID, err := autoCreateSubscription(ctx, s, item)
				if err != nil {
					log.Printf("⚠️ 自动创建订阅失败 [%s]: %v", item.Title, err)
					continue
				}
				targetSubID = autoSubID
				log.Printf("✅ 自动创建订阅 [%s]: ID=%d", item.Title, autoSubID)

				// 自动创建成功后，重新获取路径并下载
				savePath := s.cfg.Organizer.TVBasePath
				if s.downloader != nil {
					if err := s.downloader.Add(ctx, item, savePath); err != nil {
						log.Printf("❌ 添加自动下载失败 [%s]: %v", item.Title, err)
						continue
					}
				}
			} else {
				log.Printf("ℹ️  个人RSS模式: 跳过未匹配种子: %s", item.Title)
				continue
			}
		}

		// 记录到数据库并创建剧集记录
		recordDownload(item)
		season, epNum := parser.ExtractEpisode(item.Title)
		createEpisodeRecordWithParsed(targetSubID, item, season, epNum)
		newCount++

		newCount++

		// 更新订阅进度
		{
			var count int64
			database.DB.Model(&database.Episode{}).
				Where("subscription_id = ? AND status IN (?, ?) AND deleted_at IS NULL", targetSubID, "organized", "downloading").
				Count(&count)
			database.DB.Model(&database.Subscription{}).Where("id = ?", targetSubID).Update("current_episodes", int(count))
		}

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

	tasks, err := s.downloader.List(ctx)
	if err != nil {
		log.Printf("❌ 获取下载列表失败: %v", err)
		return
	}

	// 预先过滤出"已完成"的任务；没有完成任务时直接返回，连 DB 都不查
	var doneTasks []core.DownloadTask
	for _, task := range tasks {
		// 判断是否下载完成：completed(完全做种), stalledUP(做种中但没流量), uploading(正在做种上传)
		if task.Status == "completed" || task.Status == "stalledUP" || task.Status == "uploading" || task.Progress >= 1.0 {
			doneTasks = append(doneTasks, task)
		}
	}
	if len(doneTasks) == 0 {
		return
	}

	// 一次拉取全部 downloading 状态的 episodes，构建 hash / original_name 两级内存索引
	var downloadingEps []database.Episode
	if err := database.DB.Where("status = ?", "downloading").Find(&downloadingEps).Error; err != nil {
		log.Printf("❌ 批量查询下载中剧集失败: %v", err)
		return
	}
	hashIndex := make(map[string]int, len(downloadingEps))     // torrent_hash -> 下标
	nameIndex := make(map[string][]int, len(downloadingEps)) // original_name -> 下标列表（可能多条）
	for i, ep := range downloadingEps {
		if ep.TorrentHash != "" {
			hashIndex[ep.TorrentHash] = i
		}
		if ep.OriginalName != "" {
			nameIndex[ep.OriginalName] = append(nameIndex[ep.OriginalName], i)
		}
	}

	for _, task := range doneTasks {
		idx, ok := hashIndex[task.Hash]
		if !ok {
			// 回退：通过原始名称匹配，用于处理 RSS 等未提前获取到 Hash 的场景；
			// 命中后回写 hash，使后续轮询能走更快的 hash 索引
			candidates, byName := nameIndex[task.Name]
			if !byName {
				continue
			}
			matched := false
			for _, ci := range candidates {
				ep := &downloadingEps[ci]
				if ep.TorrentHash != "" {
					continue // 已绑定其他 hash 的记录不抢占
				}
				database.DB.Model(ep).Update("torrent_hash", task.Hash)
				ep.TorrentHash = task.Hash
				hashIndex[task.Hash] = ci
				idx = ci
				matched = true
				break
			}
			if !matched {
				continue
			}
		}

		ep := downloadingEps[idx]
		now := time.Now()
		database.DB.Model(&ep).Updates(map[string]interface{}{
			"status":               "downloaded",
			"download_finished_at": &now,
		})

		log.Printf("📥 下载完成: %s", task.Name)

		// 计算下载耗时（用于通知展示；无起始时间则留空）
		elapsed := ""
		if ep.DownloadStartedAt != nil {
			elapsed = now.Sub(*ep.DownloadStartedAt).Truncate(time.Second).String()
		}

		if s.bus != nil {
			s.bus.Publish(core.Event{
				Type: core.EventDownloadCompleted,
				Payload: map[string]any{
					"episode_id": ep.ID,
					"title":      ep.Title,
					"hash":       task.Hash, // 使用 task.Hash，确保即使刚更新也能传正确值
					"size":       task.Size,  // 文件大小（字节），通知格式化用
					"duration":   elapsed,    // 下载耗时，通知展示用
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
	// 查询数据库中的待整理 Episode 记录
	var episodes []database.Episode
	if err := database.DB.Where("status IN ? AND final_path = ?", []string{"downloaded", "downloading"}, "").Find(&episodes).Error; err != nil {
		log.Printf("❌ 查询待整理文件失败: %v", err)
		return
	}

	if len(episodes) == 0 {
		return
	}

	log.Printf("📂 发现 %d 个待整理的文件", len(episodes))

	successCount := 0
	failureCount := 0
	lastOrganizedPath := ""
	for _, ep := range episodes {
		// 1. 根据 Subscription 获取番剧元数据
		var sub database.Subscription
		if err := database.DB.First(&sub, ep.SubscriptionID).Error; err != nil {
			log.Printf("⚠️  整理跳过，找不到对应的订阅记录: %d", ep.SubscriptionID)
			failureCount++
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
			// 回退：三级匹配策略
			tasks, listErr := s.downloader.List(ctx)
			if listErr != nil {
				log.Printf("⚠️  获取种子列表失败: %v", listErr)
				failureCount++
				continue
			}
			var found bool

			// Level 1: hash 前缀匹配（ep.TorrentHash 存前缀，task.Hash 是完整 hash）
			if ep.TorrentHash != "" {
				for _, t := range tasks {
					if strings.HasPrefix(t.Hash, ep.TorrentHash) {
						task = t
						found = true
						log.Printf("✅ 整理匹配 Level1(hash前缀): ep.TorrentHash=%s -> task.Hash=%s task.Name=%s", ep.TorrentHash, t.Hash, t.Name)
						break
					}
				}
			}

			// Level 2: original_name 精确匹配 task.Name 或 ContentPath 文件名
			if !found && ep.OriginalName != "" {
				for _, t := range tasks {
					taskFileName := filepath.Base(t.ContentPath)
					if taskFileName == "" {
						taskFileName = t.Name
					}
					if t.Name == ep.OriginalName || taskFileName == ep.OriginalName {
						task = t
						found = true
						log.Printf("✅ 整理匹配 Level2(original_name): ep.OriginalName=%s -> task.Name=%s", ep.OriginalName, t.Name)
						break
					}
				}
			}

			// Level 3: title 模糊匹配（最后手段）
			if !found && ep.Title != "" {
				epTitleLower := strings.ToLower(ep.Title)
				for _, t := range tasks {
					taskNameLower := strings.ToLower(t.Name)
					if strings.Contains(taskNameLower, epTitleLower) || strings.Contains(epTitleLower, taskNameLower) {
						task = t
						found = true
						log.Printf("⚠️ 整理匹配 Level3(title模糊): ep.Title=%s -> task.Name=%s", ep.Title, t.Name)
						break
					}
				}
			}

			if !found {
				log.Printf("❌ 整理匹配失败: ep.OriginalName=%s ep.Title=%s ep.TorrentHash=%s (总任务数=%d)", ep.OriginalName, ep.Title, ep.TorrentHash, len(tasks))
				failureCount++
				continue
			}
		}

		// 这里处理任务的真实路径
		// 优先用 ContentPath（qB 直接下载到最终目录的情况），回退 SavePath+Name
		realPath := task.ContentPath
		if realPath == "" {
			realPath = filepath.Join(task.SavePath, task.Name)
		}

		// 检查文件是否真实存在（跳过 missingFiles / 还在下载的）
		if _, err := os.Stat(realPath); os.IsNotExist(err) {
			log.Printf("⏭️ 跳过整理：文件不存在 realPath=%s (task.Progress=%.2f state=%s)", realPath, task.Progress, task.Status)
			continue
		}

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
			failureCount++
			continue
		}

		// 4. 更新 Episode 记录
		now := time.Now()
		if err := database.DB.Model(&ep).Updates(map[string]interface{}{
			"status":       "organized",
			"final_path":   newPath,
			"organized_at": &now,
		}).Error; err != nil {
			log.Printf("❌ 更新整理状态失败 [ID=%d]: %v", ep.ID, err)
			failureCount++
			continue
		}
		{
			var count int64
			database.DB.Model(&database.Episode{}).
				Where("subscription_id = ? AND status IN (?, ?) AND deleted_at IS NULL", sub.ID, "organized", "downloading").
				Count(&count)
			database.DB.Model(&sub).Update("current_episodes", int(count))
		}

		var count int64
		database.DB.Model(&database.Episode{}).
			Where("subscription_id = ? AND status = ? AND deleted_at IS NULL", sub.ID, "organized").
			Count(&count)
		database.DB.Model(&sub).Update("current_episodes", int(count))
	}

	if s.bus != nil && (successCount > 0 || failureCount > 0) {
		s.bus.Publish(core.Event{
			Type: core.EventFileOrganized,
			Payload: map[string]any{
				"success":    successCount,
				"failed":     failureCount,
				"final_path": lastOrganizedPath,
			},
			Time: time.Now(),
		})
	}

	if successCount > 0 || failureCount > 0 {
		log.Printf("✅ 整理完成：成功 %d 个，失败 %d 个", successCount, failureCount)
	}
}

// pollSupplement 执行补全扫描：查找集数不完整的订阅，爬取历史种子补全
func (s *Scheduler) pollSupplement(ctx context.Context) {
	log.Println("🔍 开始补全扫描...")

	var subs []database.Subscription
	database.DB.Where("enabled = ?", true).Find(&subs)

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
	// 如果缺失 BangumiID，尝试通过标题自动找回
	if sub.BangumiID == "" {
		log.Printf("ℹ️  订阅 [%s] 缺失 BangumiID，尝试自动找回...", sub.TitleCN)
		var lastErr error
		if s.metadataProvider != nil {
			// 设置 10 秒超时，搜索需要更多时间
			searchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			results, err := s.metadataProvider.SearchAnime(searchCtx, sub.TitleCN)
			cancel()
			lastErr = err
			if err == nil && len(results) > 0 {
				best := bestMatch(results, sub.TitleCN)
				if best != nil {
					sub.BangumiID = best.ID
					database.DB.Model(&sub).Update("bangumi_id", best.ID)
					log.Printf("✅ 已通过标题找回 BangumiID: %s -> %s", sub.TitleCN, sub.BangumiID)
				}
			} else if err != nil {
				log.Printf("❌ 找回 BangumiID 搜索失败 [%s]: %v", sub.TitleCN, err)
			} else {
				log.Printf("ℹ️  找回 BangumiID 搜索无结果 [%s]", sub.TitleCN)
			}
		}

		// 如果找回后依然缺失，则无法补全
		if sub.BangumiID == "" {
			reason := "搜索无结果"
			if lastErr != nil {
				reason = lastErr.Error()
			}
			log.Printf("⚠️  订阅 %s 补全跳过：未设置且自动找回失败 (原因: %s)", sub.TitleCN, reason)
			return
		}
	}

	if s.metadataProvider != nil {
		// 设置 5 秒超时
		getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		anime, err := s.metadataProvider.GetAnime(getCtx, sub.BangumiID)
		cancel()
		if err == nil {
			updates := map[string]interface{}{}
			if sub.TotalEpisodes == 0 && anime.TotalEps > 0 {
				updates["total_episodes"] = anime.TotalEps
				sub.TotalEpisodes = anime.TotalEps
			}
			if sub.Year == 0 && anime.Year > 0 {
				updates["year"] = anime.Year
				sub.Year = anime.Year
			}
			if sub.AnimeType == "" && anime.Type != "" {
				updates["anime_type"] = anime.Type
				sub.AnimeType = anime.Type
			}
			if sub.Description == "" && anime.Description != "" {
				updates["description"] = anime.Description
				sub.Description = anime.Description
			}
			if sub.CoverURL == "" && anime.CoverURL != "" {
				updates["cover_url"] = anime.CoverURL
				sub.CoverURL = anime.CoverURL
			}
			if sub.MetadataProvider == "" {
				updates["metadata_provider"] = s.metadataProvider.Name()
				sub.MetadataProvider = s.metadataProvider.Name()
			}
			if sub.TMDBID == "" && anime.TMDBID != "" {
				updates["tmdb_id"] = anime.TMDBID
				sub.TMDBID = anime.TMDBID
			}
			if sub.IMDBID == "" && anime.IMDBID != "" {
				updates["imdb_id"] = anime.IMDBID
				sub.IMDBID = anime.IMDBID
			}

			if len(updates) > 0 {
				database.DB.Model(&sub).Updates(updates)
				log.Printf("✅ 已通过 %s 补全订阅 [%s] 的元数据: %+v", s.metadataProvider.Name(), sub.TitleCN, updates)
			}
		}
	} else if sub.BangumiID != "" {
		// 兜底：如果没配 metadataProvider，或者 BGM 获取失败，尝试从 Mikan 页面直接抓取
		if mikan, ok := s.source.(*source.MikanSource); ok {
			info, err := mikan.GetAnimeInfo(ctx, sub.BangumiID)
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

	newCount := 0
	for _, item := range items {
		if isDuplicate(item.URL) {
			continue
		}
		if item.InfoHash != "" && isEpisodeExists(item.InfoHash) {
			continue
		}

		// 集数级防重: 同一订阅下该集已有记录(合集/单集重复)则跳过,避免重复下载
		season, epNum := parser.ExtractEpisode(item.Title)
		if epNum > 0 && episodeNumberExists(sub.ID, season, epNum) {
			log.Printf("⏭️  补全跳过 [%s] S%02dE%02d: 该集已存在", sub.TitleCN, season, int(epNum))
			continue
		}

		savePath := sub.CustomPath
		if savePath == "" {
			savePath = s.cfg.Organizer.TVBasePath
		}

		// 使用订阅的字幕组作为标签（比标题解析更准确）
		if sub.SubgroupName != "" {
			item.GroupName = sub.SubgroupName
		}

		if err := s.downloader.Add(ctx, item, savePath); err != nil {
			log.Printf("❌ 添加补全下载失败 [%s]: %v", item.Title, err)
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
	{
		var count int64
		database.DB.Model(&database.Episode{}).
			Where("subscription_id = ? AND status IN (?, ?) AND deleted_at IS NULL", sub.ID, "organized", "downloading").
			Count(&count)
		database.DB.Model(&sub).Update("current_episodes", int(count))
	}

		newCount++
	}

	if newCount > 0 {
		log.Printf("✅ 补全 [%s]: 新增 %d 个下载", sub.TitleCN, newCount)
	}

	var count int64
	database.DB.Model(&database.Episode{}).
		Where("subscription_id = ? AND status = ? AND deleted_at IS NULL", sub.ID, "organized").
		Count(&count)
		database.DB.Model(&sub).Update("current_episodes", int(count))
		
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

		if sub.TotalEpisodes > 0 && int(count) >= sub.TotalEpisodes {
		// 如果本地集数已经齐备，确保 Completed 状态为 true
		if !sub.Completed {
			database.DB.Model(&sub).Update("completed", true)
		}
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
	} else if sub.Completed && int(count) < sub.TotalEpisodes {
		// 如果标记为 Completed 但集数不足（如用户删除了文件），则回退为未完结状态，允许后续轮询继续扫描
		database.DB.Model(&sub).Update("completed", false)
		log.Printf("ℹ️  订阅 [%s] 集数不足 (%d/%d)，已重置为未完结状态以允许继续补全", sub.TitleCN, int(count), sub.TotalEpisodes)
	}
}

// buildFilter 从订阅的 FilterJSON 构建 core.Filter
func buildFilter(sub database.Subscription) core.Filter {
	filter := core.Filter{
		PreferSubgroup: sub.SubgroupName,
	}

	// 解析 AllowedSubgroups (JSON 数组字符串)
	if sub.AllowedSubgroups != "" && sub.AllowedSubgroups != "[]" {
		var allowedSubgroups []string
		if err := json.Unmarshal([]byte(sub.AllowedSubgroups), &allowedSubgroups); err == nil {
			filter.AllowedSubgroups = allowedSubgroups
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
		GroupName:         item.GroupName,
		Resolution:        item.Resolution,
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
		GroupName:         item.GroupName,
		Resolution:        item.Resolution,
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

// episodeNumberExists 检查同一订阅下该季该集是否已有 Episode 记录（集数级防重）
func episodeNumberExists(subscriptionID uint, season int, number float32) bool {
	var count int64
	database.DB.Model(&database.Episode{}).
		Where("subscription_id = ? AND season = ? AND number = ? AND deleted_at IS NULL", subscriptionID, season, number).
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

// ============================================================
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

	// 4. 检查是否已订阅该 BangumiID
	var count int64
	database.DB.Model(&database.Subscription{}).Where("bangumi_id = ?", mikanBangumiID).Count(&count)
	if count > 0 {
		return 0, fmt.Errorf("BangumiID=%s 已订阅", mikanBangumiID)
	}

	// 5. 创建订阅（事务内）
	var subID uint
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// 事务内再次检查重复
		var existing database.Subscription
		result := tx.Where("bangumi_id = ?", mikanBangumiID).First(&existing)
		if result.RowsAffected > 0 {
			return fmt.Errorf("BangumiID=%s 已存在订阅", mikanBangumiID)
		}

		// 获取字幕组 RSS URL
		rssURL := ""
		mikanSrc, ok := s.source.(*source.MikanSource)
		if ok {
			if url, err := mikanSrc.ResolveFirstRSSURL(ctx, mikanBangumiID); err == nil {
				rssURL = url
			}
		}

		sub := database.Subscription{
			TitleCN:    cleanTitle,
			BangumiID:  mikanBangumiID,
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
	if s.metadataProvider != nil && s.mikanRSSURL != "" {
		go func(subID uint) {
			suppCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			var sub database.Subscription
			if database.DB.First(&sub, subID).Error == nil {
				s.supplementOne(suppCtx, sub)
			}
		}(subID)
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
	mikanSrc, ok := s.source.(*source.MikanSource)
	if !ok {
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
	httpClient := httpx.New(30 * time.Second)
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

	results, err := s.metadataProvider.SearchAnime(ctx, cleanTitle)
	if err != nil {
		return 0, fmt.Errorf("搜索番剧失败: %w", err)
	}
	if len(results) == 0 {
		return 0, fmt.Errorf("未找到匹配的番剧: %s", cleanTitle)
	}

	// 选最佳匹配
	best := bestMatch(results, cleanTitle)
	if best == nil {
		return 0, fmt.Errorf("无法确定最佳匹配: %s", cleanTitle)
	}

	// 检查是否已订阅
	var count int64
	database.DB.Model(&database.Subscription{}).Where("bangumi_id = ?", best.ID).Count(&count)
	if count > 0 {
		return 0, fmt.Errorf("BangumiID=%s 已订阅", best.ID)
	}

	// 创建订阅
	var subID uint
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		var existing database.Subscription
		result := tx.Where("bangumi_id = ?", best.ID).First(&existing)
		if result.RowsAffected > 0 {
			return fmt.Errorf("BangumiID=%s 已存在", best.ID)
		}

		sub := database.Subscription{
			TitleCN:          best.TitleCN,
			TitleEN:          best.TitleEN,
			TitleJP:          best.TitleJP,
			BangumiID:        best.ID,
			Year:             best.Year,
			Season:           best.Season,
			CoverURL:         best.CoverURL,
				Description:      best.Description,
				Enabled:          &[]bool{true}[0],
				SourceName:       "Mikan",
			MetadataID:       best.ID,
			MetadataProvider: s.metadataProvider.Name(),
			TotalEpisodes:    best.TotalEps,
		}

		// 补全 Mikan RSS URL
		mikanSrc, ok := s.source.(*source.MikanSource)
		if ok {
			if rssURL, err := mikanSrc.ResolveFirstRSSURL(ctx, best.ID); err == nil {
				sub.RSSURL = rssURL
			}
		}

		if err := tx.Create(&sub).Error; err != nil {
			return fmt.Errorf("创建订阅失败: %w", err)
		}
		subID = sub.ID
		log.Printf("✅ [自动订阅] 已通过标题搜索创建订阅 ID=%d: %s (BangumiID=%s)", subID, best.TitleCN, best.ID)
		return nil
	})
	return subID, err
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

	log.Println("🧹 开始种子自动清理扫描...")

	// 获取 qBittorrent 所有任务
	tasks, err := s.downloader.List(ctx)
	if err != nil {
		log.Printf("❌ 获取种子列表失败: %v", err)
		return
	}

	cleaned := 0
	minSeedTime := s.cfg.Scheduler.SeedCleanupMinSeedTime
	minRatio := s.cfg.Scheduler.SeedCleanupMinRatio

	// 预先获取所有非 organized 状态的剧集 hash，防止误删还没整理的种子
	var activeHashes []string
	database.DB.Model(&database.Episode{}).
		Where("status IN ? AND deleted_at IS NULL", []string{"downloading", "downloaded"}).
		Pluck("torrent_hash", &activeHashes)
	activeMap := make(map[string]bool)
	for _, h := range activeHashes {
		activeMap[h] = true
	}

	for _, task := range tasks {
		// 只处理已完成的任务
		if task.Progress < 1.0 {
			continue
		}

		// 保护：如果数据库中该种子还没被标记为已下载/已整理，严禁从下载器删除
		// 只有 status = 'organized' 或者不在 activeMap 中的（说明已整理或不归本系统管）才允许删
		if activeMap[task.Hash] {
			// 进一步检查：如果是 downloaded 状态（已完成下载但未整理），也暂时保留，直到整理器完成工作
			var ep database.Episode
			if err := database.DB.Where("torrent_hash = ?", task.Hash).First(&ep).Error; err == nil {
				if ep.Status != "organized" {
					continue 
				}
			}
		}

		// 检查做种时间（需要 qB 支持 seeding_time 字段，否则跳过时间检查）
		if minSeedTime > 0 {
			// TODO: 需要 qB API 返回 seeding_time 字段才能精确判断
			// 目前跳过时间检查，仅检查比率
		}

		// 检查做种比率
		if minRatio > 0 && task.Size > 0 {
			ratio := float64(task.Done) / float64(task.Size)
			if ratio < minRatio {
				continue
			}
		}

		// 删除种子（不删文件，因为已硬链接到媒体库）
		if err := s.downloader.Delete(ctx, task.Hash, false); err != nil {
			log.Printf("⚠️ 清理种子失败 [%s]: %v", task.Name, err)
			continue
		}

		log.Printf("🗑️ 已清理完成种子: %s (进度=%.0f%%, 比率=%.2f)", task.Name, task.Progress*100, float64(task.Done)/float64(task.Size))
		cleaned++
	}

	if cleaned > 0 {
		log.Printf("✅ 种子自动清理完成：清理 %d 个", cleaned)
	}
}

// episodeGroupExists 检查同一订阅下该季该集该组是否已有记录（多组共存核心逻辑）
func episodeGroupExists(subscriptionID uint, season int, number float32, groupName string) bool {
	var count int64
	database.DB.Model(&database.Episode{}).
		Where("subscription_id = ? AND season = ? AND number = ? AND group_name = ? AND deleted_at IS NULL",
			subscriptionID, season, number, groupName).
		Count(&count)
	return count > 0
}

