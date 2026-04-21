package database

import (
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init(dbPath string) error {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return err
	}

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
