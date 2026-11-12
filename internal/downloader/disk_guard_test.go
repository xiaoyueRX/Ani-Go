package downloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestGetDiskFreeSpace(t *testing.T) {
	tmpDir := t.TempDir()
	free, err := GetDiskFreeSpace(tmpDir)
	if err != nil {
		t.Fatalf("GetDiskFreeSpace 失败: %v", err)
	}
	if free == 0 {
		t.Errorf("GetDiskFreeSpace 返回 0 字节空间")
	}

	// 测试深层不存在的子路径向上查找
	subPath := filepath.Join(tmpDir, "level1", "level2", "file.txt")
	freeSub, err := GetDiskFreeSpace(subPath)
	if err != nil {
		t.Fatalf("深层不存在路径 GetDiskFreeSpace 失败: %v", err)
	}
	if freeSub != free {
		t.Logf("子路径磁盘空间 %d vs 父路径 %d", freeSub, free)
	}
}

func TestCheckDiskSpaceGuard(t *testing.T) {
	// 初始化内存 SQLite 测试环境
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("初始化内存数据库失败: %v", err)
	}
	_ = db.AutoMigrate(&database.Setting{})
	origDB := database.DB
	database.DB = db
	defer func() { database.DB = origDB }()

	tmpDir := t.TempDir()

	// 1. 正常阈值（5GB），测试机通常有超过 5GB 空间，应返回 nil
	if err := CheckDiskSpaceGuard(tmpDir); err != nil {
		t.Logf("默认 5GB 阈值触发（若测试机剩余空间不足 5GB 则正常）: %v", err)
	}

	// 2. 将阈值设置为巨大数值（999999 GB），必然触发急停熔断
	database.DB.Create(&database.Setting{
		Key:   "DISK_MIN_FREE_GB",
		Value: "999999",
	})

	err = CheckDiskSpaceGuard(tmpDir)
	if err == nil {
		t.Fatalf("设置超大阈值 999999GB 时应返回急停熔断错误，但实际返回 nil")
	}

	// 3. 将阈值设置为 0（关闭急停熔断）
	database.DB.Model(&database.Setting{}).Where("key = ?", "DISK_MIN_FREE_GB").Update("value", "0")
	if err := CheckDiskSpaceGuard(tmpDir); err != nil {
		t.Fatalf("设置为 0 时应关闭熔断，但返回错误: %v", err)
	}

	// 4. 将阈值设置为负数（如 -1），同样关闭急停熔断
	database.DB.Model(&database.Setting{}).Where("key = ?", "DISK_MIN_FREE_GB").Update("value", "-1.5")
	if err := CheckDiskSpaceGuard(tmpDir); err != nil {
		t.Fatalf("设置为负数时应关闭熔断，但返回错误: %v", err)
	}

	// 5. 空路径不应误触发熔断
	if err := CheckDiskSpaceGuard(""); err != nil {
		t.Fatalf("空路径应静默返回 nil，但返回: %v", err)
	}

	// 6. 环境变量优先于数据库配置
	database.DB.Model(&database.Setting{}).Where("key = ?", "DISK_MIN_FREE_GB").Update("value", "0") // DB 为 0
	os.Setenv("DISK_MIN_FREE_GB", "999999")                                                           // ENV 为超大
	defer os.Unsetenv("DISK_MIN_FREE_GB")
	if err := CheckDiskSpaceGuard(tmpDir); err == nil {
		t.Fatalf("环境变量 DISK_MIN_FREE_GB=999999 应覆盖数据库配置并触发熔断")
	}
}

func TestFindExistingAncestor(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "a", "b", "c")
	ancestor := findExistingAncestor(path)
	if ancestor != tmpDir {
		t.Errorf("期望现有祖先为 %s, 实际 %s", tmpDir, ancestor)
	}

	// 根路径自身存在
	cwd, _ := os.Getwd()
	if findExistingAncestor(cwd) != cwd {
		t.Errorf("当前工作目录自身应为其祖先")
	}
}
