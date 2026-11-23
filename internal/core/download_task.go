package core

import "strings"

// IsComplete 统一判断所选文件是否已完成，排除元数据任务、校验及异常状态。
func (task DownloadTask) IsComplete() bool {
	if len(task.FollowedBy) > 0 {
		return false
	}
	if task.RemainingBytes != nil {
		if *task.RemainingBytes != 0 {
			return false
		}
	} else if !(task.Progress >= 1) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(task.Status)) {
	case "", "completed", "complete", "uploading", "stalledup", "pausedup", "stoppedup", "queuedup", "forcedup", "seeding", "stopped", "active", "paused":
		return true
	default:
		return false
	}
}
