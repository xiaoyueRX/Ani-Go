package downloader

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// LifecycleFilter 判断种子是否安全可删的回调函数
type LifecycleFilter func(task core.DownloadTask) bool

// CleanSatisfiedSeeds 清理满足做种时长或分享率条件的已完成任务（仅删任务，不删物理文件）
func CleanSatisfiedSeeds(ctx context.Context, dl core.Downloader, minSeedTime time.Duration, minRatio float64, canDelete LifecycleFilter) (int, error) {
	if dl == nil {
		return 0, fmt.Errorf("downloader not available")
	}

	tasks, err := dl.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list tasks failed: %w", err)
	}

	cleaned := 0
	minSeconds := int64(minSeedTime.Seconds())

	for _, task := range tasks {
		// 1. 仅检查已 100% 下载完成的种子
		if task.Progress < 1.0 {
			continue
		}

		// 2. 自定义安全检查（必须满足外部业务约束，如媒体库已整理入库）
		if canDelete != nil && !canDelete(task) {
			continue
		}

		// 3. 检查做种时长限制
		if minSeedTime > 0 {
			if task.SeedingTime > 0 && task.SeedingTime < minSeconds {
				continue
			}
		}

		// 4. 检查分享率限制
		if minRatio > 0 {
			if task.Ratio < minRatio {
				continue
			}
		}

		// 5. 执行安全删除：deleteFiles = false（保留已下载与硬链接的文件）
		if err := dl.Delete(ctx, task.Hash, false); err != nil {
			log.Printf("⚠️ [做种生命周期] 清理种子失败 [%s]: %v", task.Name, err)
			continue
		}

		log.Printf("🗑️ [做种生命周期] 已安全清理种子: %s (进度=%.0f%%, 分享率=%.2f, 做种时长=%ds)",
			task.Name, task.Progress*100, task.Ratio, task.SeedingTime)
		cleaned++
	}

	return cleaned, nil
}
