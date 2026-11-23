package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

// pollSyncBangumi 同步 Bangumi 收藏到本地订阅
func (s *Scheduler) pollSyncBangumi(ctx context.Context) {
	if database.DB == nil {
		return
	}

	username := runtimeSetting("BGMTV_USERNAME")
	if cfg := s.runtimeConfig(); username == "" && cfg != nil {
		username = cfg.Metadata.BGMTV.Username
	}
	if username == "" || s.metadataProvider == nil || s.metadataProvider.Name() != "BGM.tv" {
		return
	}
	if !s.pollSyncBangumiRunning.CompareAndSwap(false, true) {
		log.Println("⏳ 上一轮 Bangumi 收藏同步尚未完成，跳过本次触发")
		return
	}
	defer s.pollSyncBangumiRunning.Store(false)

	log.Printf("🔄 开始同步 Bangumi 收藏 (用户: %s)...", username)

	animes, err := s.metadataProvider.GetCollections(ctx, username)
	if err != nil {
		log.Printf("❌ 获取 Bangumi 收藏失败: %v", err)
		return
	}

	addedCount := 0
	for _, anime := range animes {
		if ctx.Err() != nil {
			return
		}
		// 自动创建订阅
		subID, err := s.autoSubscribe(ctx, anime)
		if err != nil {
			log.Printf("⚠️ 自动同步番剧失败 [%s]: %v", anime.TitleCN, err)
			continue
		}
		if subID > 0 {
			addedCount++
		}
	}

	if addedCount > 0 {
		log.Printf("✅ Bangumi 同步完成：新增 %d 个订阅", addedCount)
	}
}

// autoSubscribe 根据元数据自动创建订阅记录
func (s *Scheduler) autoSubscribe(ctx context.Context, anime core.Anime) (uint, error) {
	if database.DB == nil {
		return 0, fmt.Errorf("数据库未就绪")
	}

	provider := anime.Provider
	if provider == "" && s.metadataProvider != nil {
		provider = s.metadataProvider.Name()
	}
	if provider == "" || anime.ID == "" {
		return 0, fmt.Errorf("缺少元数据提供者或作品 ID")
	}
	var subID uint
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		candidate := database.Subscription{
			TitleCN:          anime.TitleCN,
			TitleEN:          anime.TitleEN,
			TitleJP:          anime.TitleJP,
			MetadataID:       anime.ID,
			MetadataProvider: provider,
			TMDBID:           anime.TMDBID,
			IMDBID:           anime.IMDBID,
			Year:             anime.Year,
			Season:           anime.Season,
			AnimeType:        anime.Type,
			CoverURL:         anime.CoverURL,
			Description:      anime.Description,
			Enabled:          &[]bool{true}[0],
			SourceName:       provider,
			TotalEpisodes:    anime.TotalEps,
		}
		if err := tx.Exec("UPDATE subscriptions SET id = id WHERE 0").Error; err != nil {
			return err
		}
		found, err := database.SubscriptionMetadataExists(tx, candidate)
		if err != nil || found {
			return err
		}
		// 旧收藏同步曾把 BGM 标识写入共享字段，按作品名称与年份一并核实后补录。
		var old []database.Subscription
		work, _ := candidate.TMDBIdentity()
		legacyIDs := []string{anime.ID}
		if _, number, ok := strings.Cut(work, "/"); ok && number != anime.ID {
			legacyIDs = append(legacyIDs, number)
		}
		if err := tx.Where("bangumi_id IN ? AND (metadata_id IS NULL OR metadata_id = '')", legacyIDs).Find(&old).Error; err != nil {
			return err
		}
		for _, sub := range old {
			if sub.MetadataProvider != "" && !strings.EqualFold(provider, sub.MetadataProvider) {
				continue
			}
			if strings.EqualFold(provider, "TMDB") {
				existing, _ := sub.TMDBIdentity()
				kind, _, _ := strings.Cut(work, "/")
				legacyKind := core.TMDBMediaType(sub.AnimeType)
				if work == "" || existing != "" && existing != work || legacyKind != "" && legacyKind != kind {
					continue
				}
				if kind == "tv" && max(1, sub.Season) != max(1, anime.Season) {
					continue
				}
			}
			if !sameAnimeTitle(sub, anime) || sub.Year > 0 && anime.Year > 0 && sub.Year != anime.Year {
				continue
			}
			updates := map[string]any{"metadata_provider": provider, "metadata_id": anime.ID}
			if sub.TMDBID == "" && anime.TMDBID != "" {
				updates["tmdb_id"] = anime.TMDBID
			}
			if sub.AnimeType == "" && anime.Type != "" {
				updates["anime_type"] = anime.Type
			}
			if provider == "BGM.tv" && sub.MikanID == "" {
				updates["bangumi_id"] = ""
				if mikanFeedMatchesID(sub.RSSURL, sub.BangumiID) {
					updates["rss_url"] = ""
				}
			}
			return tx.Model(&sub).Updates(updates).Error
		}

		if err := tx.Create(&candidate).Error; err != nil {
			return err
		}
		subID = candidate.ID
		return nil
	})

	if err == nil && subID > 0 && s.bus != nil {
		log.Printf("🆕 [自动同步] 已添加订阅: %s (Bangumi ID: %s)", anime.TitleCN, anime.ID)
		// 触发一次补全扫描
		s.bus.Publish(core.Event{
			Type: core.EventSubscriptionAdded,
			Payload: map[string]interface{}{
				"subscription_id": subID,
				"title":           anime.TitleCN,
			},
			Time: time.Now(),
		})
	}

	return subID, err
}
