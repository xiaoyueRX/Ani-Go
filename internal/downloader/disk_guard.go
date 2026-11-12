package downloader

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

const (
	// DefaultMinFreeDiskGB 默认最低磁盘剩余空间保护阈值（低于此值立即熔断阻断新增下载）
	DefaultMinFreeDiskGB = 5.0

	bytesPerGB = 1024 * 1024 * 1024
)

// GetDiskFreeSpace 获取指定路径（或其父级挂载点）当前可用磁盘空间（字节）
func GetDiskFreeSpace(path string) (uint64, error) {
	ancestor := findExistingAncestor(path)
	return getFreeDiskSpace(ancestor)
}

// resolveMinFreeDiskGB 按「环境变量 > 数据库 settings 表 > 内置默认值」解析磁盘安全阈值
func resolveMinFreeDiskGB() float64 {
	minGB := DefaultMinFreeDiskGB

	if database.DB != nil {
		var s database.Setting
		if err := database.DB.Where("key = ?", "DISK_MIN_FREE_GB").First(&s).Error; err == nil && s.Value != "" {
			if parsed, ok := parseValidDiskThreshold(s.Value); ok {
				minGB = parsed
			}
		}
	}

	if envVal := os.Getenv("DISK_MIN_FREE_GB"); strings.TrimSpace(envVal) != "" {
		if parsed, ok := parseValidDiskThreshold(envVal); ok {
			minGB = parsed
		}
	}

	return minGB
}

func parseValidDiskThreshold(raw string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

// CheckDiskSpaceGuard 检查目标路径磁盘空间是否满足安全阈值
func CheckDiskSpaceGuard(path string) error {
	minGB := resolveMinFreeDiskGB()

	if minGB <= 0 {
		return nil // 用户显式设为 <= 0 关闭急停熔断
	}

	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil // 未指定下载路径时，由下载器自身默认机制处理，不实施本地路径熔断
	}

	freeBytes, err := GetDiskFreeSpace(trimmed)
	if err != nil {
		// 若无法获取磁盘空间，记录警告但不阻断
		log.Printf("⚠️ 无法获取路径 %s 磁盘空间: %v", trimmed, err)
		return nil
	}

	freeGB := float64(freeBytes) / float64(bytesPerGB)
	if freeGB < minGB {
		return fmt.Errorf("🚨 [磁盘急停熔断] 目标路径剩余磁盘空间仅剩 %.2f GB (安全阈值: %.2f GB)，已自动阻断新增下载以防磁盘爆满", freeGB, minGB)
	}

	return nil
}

// findExistingAncestor 向上递归查找第一个真实存在的父目录
func findExistingAncestor(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		trimmed = "."
	}
	cur := filepath.Clean(trimmed)
	for {
		if info, err := os.Stat(cur); err == nil {
			// 若目标是已存在的普通文件，取其所在目录以确保 Windows GetDiskFreeSpaceEx 正常调用
			if !info.IsDir() {
				dir := filepath.Dir(cur)
				if dir != "" {
					return dir
				}
			}
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur || parent == "" || parent == "." {
			// 若为绝对路径且已追溯至根挂载点仍不存在（如 Windows 下未挂载盘符），保留根路径以便上层感知真实错误
			if filepath.IsAbs(cur) {
				return cur
			}
			wd, _ := os.Getwd()
			if wd != "" {
				return wd
			}
			return "."
		}
		cur = parent
	}
}
