package database

import (
	"errors"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"

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

// TMDBIdentity 返回用于比较的规范作品标识和可查询的原始标识。
// 历史电影如只有裸号和本地分类，可查重，但缺少可用于远端补全的带类型标识。
func (s Subscription) TMDBIdentity() (canonical, lookup string) {
	id, aux := strings.TrimSpace(s.MetadataID), strings.TrimSpace(s.TMDBID)
	if !strings.EqualFold(s.MetadataProvider, "TMDB") && (s.MetadataProvider != "" || id != "") {
		return "", ""
	}
	kind, number := core.ParseTMDBID(id)
	if aux != "" {
		auxKind, auxNumber := core.ParseTMDBID(aux)
		if id != "" && (number != auxNumber || strings.Contains(id, "/") && strings.Contains(aux, "/") && kind != auxKind) {
			return "", ""
		}
		if id == "" || strings.Contains(aux, "/") {
			id, kind, number = aux, auxKind, auxNumber
		}
	}
	if number == "" {
		return "", ""
	}
	if core.TMDBMediaType(s.AnimeType) == "movie" && kind != "movie" {
		if strings.Contains(id, "/") {
			return "", ""
		}
		return "movie/" + number, ""
	}
	return kind + "/" + number, id
}

// SubscriptionMetadataExists 用已存的来源标识查重。电影没有季，电视剧按订阅季区分。
func SubscriptionMetadataExists(tx *gorm.DB, sub Subscription) (bool, error) {
	query := tx.Model(&Subscription{}).Where("id <> ?", sub.ID)
	if !strings.EqualFold(sub.MetadataProvider, "TMDB") && !(sub.MetadataProvider == "" && sub.MetadataID == "" && sub.TMDBID != "") {
		if sub.MetadataID == "" || sub.MetadataProvider == "" {
			return false, nil
		}
		var found []uint
		err := query.Where("metadata_id = ? AND LOWER(metadata_provider) = LOWER(?)", sub.MetadataID, sub.MetadataProvider).Limit(1).Pluck("id", &found).Error
		return len(found) > 0, err
	}
	canonical, _ := sub.TMDBIdentity()
	kind, _, ok := strings.Cut(canonical, "/")
	if !ok {
		return false, nil
	}
	if kind == "tv" {
		query = query.Where("COALESCE(NULLIF(season, 0), 1) = ?", max(1, sub.Season))
	}
	// 由同一个解析器核对候选，历史 ID 的空白与类型前缀处理与远端查询保持一致。
	var rows []Subscription
	err := query.Select("metadata_provider", "metadata_id", "tmdb_id", "anime_type").
		Where("LOWER(metadata_provider) = 'tmdb' OR (COALESCE(metadata_provider, '') = '' AND COALESCE(metadata_id, '') = '' AND tmdb_id <> '')").
		Find(&rows).Error
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if existing, _ := row.TMDBIdentity(); existing == canonical {
			return true, nil
		}
	}
	return false, nil
}

func checkSubscriptionIdentity(tx *gorm.DB, created Subscription) error {
	found, err := SubscriptionMetadataExists(tx, created)
	if err != nil {
		return err
	}
	if found {
		return ErrSubscriptionExists
	}
	identity := tx.Where("title_cn = ? AND COALESCE(NULLIF(season, 0), 1) = ?", created.TitleCN, max(1, created.Season))
	if created.MikanID != "" {
		identity = identity.Or("mikan_id = ?", created.MikanID)
	}
	legacy := created.MikanID == "" && created.MetadataID == "" && created.BangumiID != ""
	if legacy {
		identity = identity.Or("bangumi_id = ?", created.BangumiID)
	}
	var rows []Subscription
	if err := tx.Where("id <> ?", created.ID).Where(identity).Select("metadata_provider", "metadata_id", "tmdb_id", "anime_type", "mikan_id", "bangumi_id").Find(&rows).Error; err != nil {
		return err
	}
	work, _ := created.TMDBIdentity()
	for _, row := range rows {
		if created.MikanID != "" && row.MikanID == created.MikanID || legacy && row.BangumiID == created.BangumiID {
			return ErrSubscriptionExists
		}
		other, _ := row.TMDBIdentity()
		if work == "" || other == "" || strings.HasPrefix(work, "movie/") == strings.HasPrefix(other, "movie/") {
			return ErrSubscriptionExists
		}
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
		season, setSeason := changes["season"].(int)
		total, setTotal := changes["total_episodes"].(int)
		kind, setKind := changes["anime_type"].(string)
		changedMovie := setKind && prior.TotalEpisodes == 1 && movieType(kind) != movieType(prior.AnimeType)
		if changedMovie || (setSeason && max(1, season) != max(1, prior.Season)) || (setTotal && total != prior.TotalEpisodes) {
			_, explicitCompletion := changes["completed"]
			if _, err := SyncEpisodeProgress(tx, id, !explicitCompletion); err != nil {
				return err
			}
		}
		if err := tx.First(&updated, id).Error; err != nil {
			return err
		}
		updatedWork, _ := updated.TMDBIdentity()
		priorWork, _ := prior.TMDBIdentity()
		if updated.TitleCN == prior.TitleCN && max(1, updated.Season) == max(1, prior.Season) &&
			updated.MikanID == prior.MikanID && updated.BangumiID == prior.BangumiID &&
			updated.MetadataID == prior.MetadataID && strings.EqualFold(updated.MetadataProvider, prior.MetadataProvider) &&
			updatedWork == priorWork {
			return nil
		}
		return checkSubscriptionIdentity(tx, updated)
	})
	return updated, err
}
