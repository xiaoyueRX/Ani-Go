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
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		return err
	}

	err = DB.AutoMigrate(
		&Subscription{},
		&Episode{},
		&DownloadRecord{},
		&Setting{},
		&User{},
	)
	if err != nil {
		return err
	}

	log.Printf("✅ 数据库初始化完成: %s", dbPath)
	return nil
}

// InitDefaultUser 首次启动时自动创建默认管理员 admin/admin
// 密码使用 Bcrypt 加盐哈希存储，绝不保存明文
func InitDefaultUser(hashFunc func(string) (string, error)) error {
	var count int64
	DB.Model(&User{}).Count(&count)
	if count > 0 {
		return nil // 已有用户，跳过
	}

	hash, err := hashFunc("admin")
	if err != nil {
		return err
	}

	user := User{
		Username:     "admin",
		PasswordHash: hash,
	}
	if err := DB.Create(&user).Error; err != nil {
		return err
	}

	log.Println("🔑 已创建默认管理员账号: admin / admin")
	log.Println("⚠️  请尽快登录并修改密码！")
	return nil
}
