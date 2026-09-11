package downloader

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

const (
	// DefaultMinFreeDiskGB 默认最低磁盘剩余空间保护阈值（低于此值立即熔断阻断新增下载）
	DefaultMinFreeDiskGB = 5.0
)

// GetDiskFreeSpace 获取指定路径（或其父级挂载点）当前可用磁盘空间（字节）
func GetDiskFreeSpace(path string) (uint64, error) {
	ancestor := findExistingAncestor(path)
	return getFreeDiskSpace(ancestor)
}

// CheckDiskSpaceGuard 检查目标路径磁盘空间是否满足安全阈值
func CheckDiskSpaceGuard(path string) error {
	minGB := DefaultMinFreeDiskGB
	if database.DB != nil {
		var s database.Setting
		if err := database.DB.Where("key = ?", "DISK_MIN_FREE_GB").First(&s).Error; err == nil && s.Value != "" {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(s.Value), 64); err == nil {
				minGB = parsed
			}
		}
	}

	if minGB <= 0 {
		return nil // 用户显式设为 0 关闭急停熔断
	}

	minBytes := uint64(minGB * 1024 * 1024 * 1024)
	freeBytes, err := GetDiskFreeSpace(path)
	if err != nil {
		// 若无法获取磁盘空间，记录警告但不阻断
		log.Printf("⚠️ 无法获取路径 %s 磁盘空间: %v", path, err)
		return nil
	}

	if freeBytes < minBytes {
		freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
		return fmt.Errorf("🚨 [磁盘急停熔断] 目标路径剩余磁盘空间仅剩 %.2f GB (安全阈值: %.2f GB)，已自动阻断新增下载以防磁盘爆满", freeGB, minGB)
	}

	return nil
}

// findExistingAncestor 向上递归查找第一个真实存在的父目录
func findExistingAncestor(path string) string {
	cur := filepath.Clean(path)
	for {
		if _, err := os.Stat(cur); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur || parent == "" || parent == "." {
			wd, _ := os.Getwd()
			if wd != "" {
				return wd
			}
			return "."
		}
		cur = parent
	}
}
