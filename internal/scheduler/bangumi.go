package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
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

	username := ""
	if s.cfg != nil {
		username = s.cfg.Metadata.BGMTV.Username
	}
	var setting database.Setting
	if err := database.DB.Where("key = ?", "BGMTV_USERNAME").First(&setting).Error; err == nil && setting.Value != "" {
		username = setting.Value
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
		// 检查是否已订阅
		var count int64
		database.DB.Model(&database.Subscription{}).Where("metadata_provider = ? AND metadata_id = ?", anime.Provider, anime.ID).Count(&count)
		if count > 0 {
			continue
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

	if anime.Provider == "" || anime.ID == "" {
		return 0, fmt.Errorf("番剧元数据缺少来源或编号")
	}
	sub := database.Subscription{
		TitleCN: anime.TitleCN, TitleEN: anime.TitleEN, TitleJP: anime.TitleJP,
		MetadataID: anime.ID, MetadataProvider: anime.Provider, TotalEpisodes: anime.TotalEps,
		Year: anime.Year, Season: anime.Season, CoverURL: anime.CoverURL, Description: anime.Description,
		SourceName: "Mikan",
	}
	if mikan := s.mikanSource(); mikan != nil {
		sub.BangumiID = resolveMikanID(ctx, mikan, sub)
		if sub.BangumiID != "" {
			sub.RSSURL, _ = mikan.ResolveFirstRSSURL(ctx, sub.BangumiID)
		}
	}
	var subID uint
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var existing database.Subscription
		query := tx.Where("metadata_provider = ? AND metadata_id = ?", anime.Provider, anime.ID)
		if sub.BangumiID != "" {
			query = query.Or("bangumi_id = ?", sub.BangumiID)
		}
		err := query.First(&existing).Error
		if err == nil {
			updates := map[string]interface{}{}
			if existing.MetadataID == "" {
				updates["metadata_id"] = anime.ID
				updates["metadata_provider"] = anime.Provider
			}
			if sub.BangumiID != "" && existing.BangumiID != sub.BangumiID {
				updates["bangumi_id"] = sub.BangumiID
				updates["rss_url"] = sub.RSSURL
			}
			if len(updates) > 0 {
				return tx.Model(&existing).Updates(updates).Error
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 旧版本把收藏的 subject ID 写入了资源 ID。两项身份同时吻合时才修正旧记录。
		if anime.Provider == "BGM.tv" {
			var legacy []database.Subscription
			if err := tx.Where("bangumi_id = ? AND (metadata_id = '' OR metadata_id IS NULL)", anime.ID).Find(&legacy).Error; err != nil {
				return err
			}
			for _, old := range legacy {
				if matchesAlias(old.TitleCN, aliases(sub)) && (old.Year == 0 || sub.Year == 0 || old.Year == sub.Year) {
					return tx.Model(&old).Updates(map[string]interface{}{"metadata_id": anime.ID, "metadata_provider": anime.Provider, "bangumi_id": sub.BangumiID, "rss_url": sub.RSSURL}).Error
				}
			}
		}
		if err := tx.Create(&sub).Error; err != nil {
			return err
		}
		subID = sub.ID
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
