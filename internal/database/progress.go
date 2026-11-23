package database

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 进度按订阅季的正片集号统计，已整理的同一集有多个字幕组时只计一次。
// 早期手动导入的无标题记录可能未写季号；保持其原有正片进度。
const regularEpisode = `episodes.number >= 1 AND episodes.number = CAST(episodes.number AS INTEGER)
	AND (episodes.season = COALESCE(NULLIF(subscriptions.season, 0), 1)
		OR (COALESCE(episodes.season, 0) = 0 AND COALESCE(episodes.title, '') = '' AND COALESCE(episodes.original_name, '') = ''))`

func finishedRegularEpisodes(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Model(&Episode{}).Where("episodes.status = ?", "organized").Where(regularEpisode)
}

// FinishedEpisodeCounts 供订阅列表、时间表与后台扫描读取同一套进度。
func FinishedEpisodeCounts(tx *gorm.DB, ids []uint) (map[uint]int, error) {
	result := make(map[uint]int, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var rows []struct {
		SubscriptionID uint
		Count          int
	}
	err := finishedRegularEpisodes(tx).Joins("JOIN subscriptions ON subscriptions.id = episodes.subscription_id").
		Where("episodes.subscription_id IN ?", ids).
		Select("episodes.subscription_id, COUNT(DISTINCT episodes.number) AS count").
		Group("episodes.subscription_id").Scan(&rows).Error
	for _, row := range rows {
		result[row.SubscriptionID] = row.Count
	}
	return result, err
}

// SyncEpisodeProgress 在更新语句内统计，避免并发的整理或删除被较早读到的计数覆盖。
func SyncEpisodeProgress(tx *gorm.DB, id uint, updateCompletion bool) (int, error) {
	count := finishedRegularEpisodes(tx).Where("episodes.subscription_id = subscriptions.id").Select("COUNT(DISTINCT episodes.number)")
	values := map[string]any{"current_episodes": count}
	if updateCompletion {
		values["completed"] = gorm.Expr("CASE WHEN total_episodes > 0 THEN (?) >= total_episodes ELSE completed END", count)
	}
	var sub Subscription
	err := tx.Model(&sub).Where("id = ?", id).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "current_episodes"}}}).UpdateColumns(values).Error
	return sub.CurrentEpisodes, err
}
