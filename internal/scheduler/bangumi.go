package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
	"gorm.io/gorm"
)

// pollSyncBangumi 同步 Bangumi 收藏到本地订阅
func (s *Scheduler) pollSyncBangumi(ctx context.Context) {
	username := s.cfg.Metadata.BGMTV.Username
	if username == "" || s.metadataProvider == nil || s.metadataProvider.Name() != "BGM.tv" {
		return
	}

	log.Printf("🔄 开始同步 Bangumi 收藏 (用户: %s)...", username)

	animes, err := s.metadataProvider.GetCollections(ctx, username)
	if err != nil {
		log.Printf("❌ 获取 Bangumi 收藏失败: %v", err)
		return
	}

	addedCount := 0
	for _, anime := range animes {
		// 检查是否已订阅
		var count int64
		database.DB.Model(&database.Subscription{}).Where("bangumi_id = ?", anime.ID).Count(&count)
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
	var subID uint
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// 再次检查防止并发冲突
		var existing database.Subscription
		if err := tx.Where("bangumi_id = ?", anime.ID).First(&existing).Error; err == nil {
			return nil // 已存在
		}

		sub := database.Subscription{
			TitleCN:       anime.TitleCN,
			TitleEN:       anime.TitleEN,
			TitleJP:       anime.TitleJP,
			BangumiID:     anime.ID,
			Year:          anime.Year,
			Season:        anime.Season,
			CoverURL:      anime.CoverURL,
			Description:   anime.Description,
			Enabled:       &[]bool{true}[0],
			SourceName:    "Mikan",
			TotalEpisodes: anime.TotalEps,
		}

		// 尝试获取 Mikan RSS URL
		mikanSrc, ok := s.source.(*source.MikanSource)
		if !ok {
			// 如果是 MultiSource，尝试获取其中的 MikanSource
			if multi, ok := s.source.(*source.MultiSource); ok {
				for _, src := range multi.Sources() {
					if m, ok := src.(*source.MikanSource); ok {
						mikanSrc = m
						break
					}
				}
			}
		}

		if mikanSrc != nil {
			if rssURL, err := mikanSrc.ResolveFirstRSSURL(ctx, anime.ID); err == nil {
				sub.RSSURL = rssURL
			}
		}

		if err := tx.Create(&sub).Error; err != nil {
			return err
		}
		subID = sub.ID
		return nil
	})

	if err == nil && subID > 0 {
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
