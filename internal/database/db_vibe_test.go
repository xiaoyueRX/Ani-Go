package database

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 使用已有的模型进行验证

func TestDatabaseVibe(t *testing.T) {
	// 1. 使用内存模式初始化连接 (Snapshot 模式，不产生物理文件)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接数据库失败喵: %v", err)
	}

	fmt.Println("🐾 [Vibe Check] 数据库内存连接成功！")

	// 2. 自动迁移模型 (使用 internal/database/models.go 中定义的模型)
	err = db.AutoMigrate(&User{}, &Subscription{}, &Episode{}, &DownloadRecord{}, &Setting{})
	if err != nil {
		t.Fatalf("模型迁移失败喵: %v", err)
	}
	fmt.Println("🐾 [Vibe Check] 核心模型 (User, Subscription, Episode 等) 迁移完成！")

	// 3. 写入测试数据
	testSub := Subscription{
		TitleCN: "关于我转生变成史莱姆这档事",
		BangumiID: "12345",
		Enabled: true,
	}
	db.Create(&testSub)
	fmt.Printf("🐾 [Vibe Check] 已存入番剧订阅: %s\n", testSub.TitleCN)

	// 4. 读取验证
	var result Subscription
	db.First(&result, "title_cn = ?", "关于我转生变成史莱姆这档事")
	
	if result.BangumiID == "12345" {
		fmt.Printf("🐾 [Vibe Check] 成功从数据库取回番剧，BangumiID 为: %s\n", result.BangumiID)
		fmt.Println("✨ 逻辑验证通过！主人，咱们现有的模型设计非常丝滑喵！")
	} else {
		t.Errorf("数据取回不匹配喵！")
	}
}
