package database

import "gorm.io/gorm"

// RecoverLegacyStatus 从旧版覆盖了调度阶段的手工标记恢复进度。
// 没有足够信息区分旧手工标记和真实阶段时保留原值。
func (ep *Episode) RecoverLegacyStatus() bool {
	switch ep.Status {
	case "pending", "downloading", "completed", "failed":
	default:
		return false
	}
	if ep.ManualStatus != "" && ep.Status != "completed" {
		return false
	}

	var stage string
	switch {
	case ep.FinalPath != "":
		stage = "organized"
	case ep.DownloadFinishedAt != nil || (ep.SourceFile != "" && ep.DownloadRoot != ""):
		stage = "downloaded"
	case ep.DownloadStartedAt != nil || (ep.Status == "completed" && (ep.TorrentHash != "" || ep.OriginKey != "")):
		stage = "downloading"
	case ep.Status == "completed":
		stage = "pending"
	default:
		return false
	}
	if ep.Status == stage {
		return false
	}
	if ep.ManualStatus == "" {
		ep.ManualStatus = ep.Status
	}
	ep.Status = stage
	return true
}

func migrateEpisodeLabels(tx *gorm.DB, legacyColumn bool) error {
	// 同一数据库被旧版写入或导入后，仍可能出现旧的 completed。
	statuses := []string{"completed"}
	if legacyColumn {
		statuses = append(statuses, "pending", "downloading", "failed")
	}
	var rows []Episode
	parents := make(map[uint]bool)
	err := tx.Unscoped().Where("status IN ?", statuses).FindInBatches(&rows, 200, func(_ *gorm.DB, _ int) error {
		for _, ep := range rows {
			if !ep.RecoverLegacyStatus() {
				continue
			}
			if err := tx.Unscoped().Model(&ep).UpdateColumns(map[string]any{"status": ep.Status, "manual_status": ep.ManualStatus}).Error; err != nil {
				return err
			}
			parents[ep.SubscriptionID] = true
		}
		return nil
	}).Error
	if err != nil {
		return err
	}
	for id := range parents {
		if _, err := SyncEpisodeProgress(tx.Unscoped(), id, true); err != nil {
			return err
		}
	}
	return nil
}
