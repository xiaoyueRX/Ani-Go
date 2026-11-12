package downloader

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// LifecycleFilter 判断种子是否安全可删的回调函数
type LifecycleFilter func(task core.DownloadTask) bool

// CleanSatisfiedSeeds 清理满足做种时长与分享率条件的已完成任务（仅删任务记录，严禁删除物理文件）
func CleanSatisfiedSeeds(ctx context.Context, dl core.Downloader, minSeedTime time.Duration, minRatio float64, canDelete LifecycleFilter) (int, error) {
	if dl == nil {
		return 0, fmt.Errorf("downloader not available")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// 必须至少配置一个正向清理阈值，防止误配置为 0/负数时批量清空所有已完成任务
	if minSeedTime <= 0 && minRatio <= 0 {
		log.Println("⚠️ [做种生命周期] minSeedTime 与 minRatio 均未启用（<= 0），跳过自动清理")
		return 0, nil
	}

	tasks, err := dl.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list tasks failed: %w", err)
	}

	cleaned := 0

	for _, task := range tasks {
		if ctx.Err() != nil {
			return cleaned, ctx.Err()
		}

		// 0. 必须具备有效 Hash 标识，避免向下载器发送空目标删除请求
		if strings.TrimSpace(task.Hash) == "" {
			continue
		}

		// 1. 仅检查已 100% 下载完成且处于安全做种/完成状态的种子
		if !isTaskReadyForSeedCleanup(task) {
			continue
		}

		// 2. 自定义安全检查（必须满足外部业务约束，如媒体库已整理入库）
		if canDelete != nil && !canDelete(task) {
			continue
		}

		// 3. 检查做种时长限制（含 SeedingTime == 0 的刚完成任务，不得绕过最小做种时长）
		if minSeedTime > 0 {
			if time.Duration(task.SeedingTime)*time.Second < minSeedTime {
				continue
			}
		}

		// 4. 检查分享率限制
		if minRatio > 0 {
			if task.Ratio < minRatio {
				continue
			}
		}

		// 5. 执行安全删除：强制传入 deleteFiles = false（保留已下载与硬链接的媒体文件）
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

// isTaskReadyForSeedCleanup 判断下载任务是否处于可进行做种生命周期清理的完成状态
// 兼容 qBittorrent / Transmission / Aria2 的状态枚举，并放行单测中未填写 Status 的任务
func isTaskReadyForSeedCleanup(task core.DownloadTask) bool {
	if task.Progress < 1.0 {
		return false
	}

	status := strings.ToLower(strings.TrimSpace(task.Status))
	if status == "" {
		return true
	}

	switch status {
	case "uploading", "stalledup", "pausedup", "stoppedup", "queuedup", "forcedup",
		"seeding", "stopped", "completed", "complete", "active", "paused":
		return true
	default:
		// 显式排除 downloading / checking / moving / error / missingfiles / unknown 等中间或异常状态
		return false
	}
}
