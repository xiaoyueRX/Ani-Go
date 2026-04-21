// Package database 负责数据库初始化和访问。
// 使用 GORM（Go 最流行的 ORM 库）+ SQLite（单文件数据库，无需安装）。
//
// 什么是 ORM？
//   直接写 SQL：SELECT * FROM subscriptions WHERE enabled = 1
//   用 ORM：   db.Where("enabled = ?", true).Find(&subs)
//   ORM 把 Go 结构体和数据库表自动对应起来，不用手写 SQL
package database

import (
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 是全局数据库连接，整个程序都用这一个
var DB *gorm.DB

// Init 初始化数据库连接，并自动创建/更新所有数据表
// dbPath 是 SQLite 文件的路径，如 "/data/autoani.db"
func Init(dbPath string) error {
	var err error

	// 打开 SQLite 数据库文件
	// 如果文件不存在，GORM 会自动创建
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn), // 只打印警告和错误
	})
	if err != nil {
		return err
	}

	// AutoMigrate：自动创建/修改数据表，让表结构和结构体定义保持一致
	// 新增字段时只需在结构体里加，不需要手写 ALTER TABLE
	err = DB.AutoMigrate(
		&Subscription{},
		&Episode{},
		&DownloadRecord{},
		&Setting{},
	)
	if err != nil {
		return err
	}

	log.Printf("✅ 数据库初始化完成: %s", dbPath)
	return nil
}
