package database

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

var ErrSubscriptionExists = errors.New("该番剧已在订阅列表中")

// CreateSubscription 对手动、批量和 MCP 订阅请求使用同一套查重规则。
func CreateSubscription(db *gorm.DB, sub *Subscription) error {
	created := *sub
	err := db.Transaction(func(tx *gorm.DB) error {
		// 先写入以取得 SQLite 写锁，再检查重复订阅。
		// 即使使用不同连接，并发请求也能看到前一个事务已提交的订阅。
		if err := tx.Create(&created).Error; err != nil {
			return err
		}

		return checkSubscriptionIdentity(tx, created)
	})
	if err == nil {
		*sub = created
	}
	return err
}

func checkSubscriptionIdentity(tx *gorm.DB, created Subscription) error {
	identity := tx.Where("title_cn = ? AND COALESCE(NULLIF(season, 0), 1) = ?", created.TitleCN, max(1, created.Season))
	if created.MikanID != "" {
		identity = identity.Or("mikan_id = ?", created.MikanID)
	}
	if created.MetadataID != "" && created.MetadataProvider != "" {
		identity = identity.Or("metadata_id = ? AND LOWER(metadata_provider) = LOWER(?)", created.MetadataID, created.MetadataProvider)
	}
	if created.MikanID == "" && created.MetadataID == "" && created.BangumiID != "" {
		identity = identity.Or("bangumi_id = ?", created.BangumiID)
	}

	var other Subscription
	result := tx.Where("id <> ?", created.ID).Where(identity).Select("id").Limit(1).Find(&other)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 0 {
		return ErrSubscriptionExists
	}
	return nil
}

// UpdateSubscription 同时更新订阅标识和由此派生的 RSS 字段。
// 只要标识不变，仍可编辑历史重复记录。
func UpdateSubscription(db *gorm.DB, id uint, changes map[string]interface{}) (Subscription, error) {
	var updated Subscription
	err := db.Transaction(func(tx *gorm.DB) error {
		// 先开启写事务，再读取原有订阅标识。
		// 这条语句不匹配任何记录，因此行级更新触发器不会执行。
		if err := tx.Exec("UPDATE subscriptions SET id = id WHERE 0").Error; err != nil {
			return err
		}
		var prior Subscription
		if err := tx.First(&prior, id).Error; err != nil {
			return err
		}
		if legacy, ok := changes["bangumi_id"].(string); ok && legacy != prior.BangumiID {
			if _, explicit := changes["mikan_id"]; !explicit {
				changes["mikan_id"] = ""
			}
			changes["rss_url"] = ""
		}
		if mikan, ok := changes["mikan_id"].(string); ok && mikan != prior.MikanID {
			changes["rss_url"] = ""
		}
		if err := tx.Model(&Subscription{}).Where("id = ?", id).Updates(changes).Error; err != nil {
			return err
		}
		if err := tx.First(&updated, id).Error; err != nil {
			return err
		}
		if updated.TitleCN == prior.TitleCN && max(1, updated.Season) == max(1, prior.Season) &&
			updated.MikanID == prior.MikanID && updated.BangumiID == prior.BangumiID &&
			updated.MetadataID == prior.MetadataID && strings.EqualFold(updated.MetadataProvider, prior.MetadataProvider) {
			return nil
		}
		return checkSubscriptionIdentity(tx, updated)
	})
	return updated, err
}
