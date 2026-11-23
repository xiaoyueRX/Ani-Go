package database

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

var ErrSubscriptionExists = errors.New("该番剧已在订阅列表中")

// CreateSubscription applies the same identity rules to manual, batch and MCP requests.
func CreateSubscription(db *gorm.DB, sub *Subscription) error {
	created := *sub
	err := db.Transaction(func(tx *gorm.DB) error {
		// A write first takes SQLite's writer lock. Concurrent requests then see
		// the previous committed subscription, even on separate connections.
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

// UpdateSubscription updates identity and derived RSS fields together. Existing
// duplicate legacy rows can still be edited when their identity is unchanged.
func UpdateSubscription(db *gorm.DB, id uint, changes map[string]interface{}) (Subscription, error) {
	var updated Subscription
	err := db.Transaction(func(tx *gorm.DB) error {
		// Start a write transaction before reading old identity fields. No rows
		// are touched by this statement, including rows with update triggers.
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
