package database

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// OrganizerQueuePredicate 与部分索引共用固定状态，让 SQLite 能识别队列条件。
const OrganizerQueuePredicate = "status IN ('downloaded', 'renaming')"

func Init(dbPath string) error {
	// 普通磁盘路径可能位于首次挂载的空卷内。
	if dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file:") {
		filename, _, _ := strings.Cut(dbPath, "?")
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return fmt.Errorf("创建数据库目录: %w", err)
		}
	}
	separator := "?"
	if strings.Contains(dbPath, "?") {
		separator = "&"
	}
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath+separator+"_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"), &gorm.Config{
		PrepareStmt: true, // 复用预编译语句缓存，降低 SQLite 解析开销与 CPU 峰值
		Logger:      logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return err
	}

	sqlDB, err := DB.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		existingEpisodes := tx.Migrator().HasTable(&Episode{})
		legacyEpisodeStatus := existingEpisodes && !tx.Migrator().HasColumn(&Episode{}, "ManualStatus")
		legacyMovieFeature := existingEpisodes && !tx.Migrator().HasColumn(&Episode{}, "MovieFeature")
		if tx.Migrator().HasIndex(&Episode{}, "idx_episodes_torrent_hash") {
			if err := tx.Migrator().DropIndex(&Episode{}, "idx_episodes_torrent_hash"); err != nil {
				return err
			}
		}
		if err := tx.AutoMigrate(
			&Subscription{},
			&Episode{},
			&DownloadRecord{},
			&Setting{},
			&User{},
			&NotificationLog{},
			&ParserCache{},
		); err != nil {
			return err
		}
		// 整理轮询也负责清理移动后保留的源文件，两次查询只需访问这些待办记录。
		if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_episode_organizer_work ON episodes(status, deleted_at)
			WHERE deleted_at IS NULL AND (` + OrganizerQueuePredicate + ` OR move_source_path <> '')`).Error; err != nil {
			return err
		}
		if err := migrateEpisodeLabels(tx, legacyEpisodeStatus); err != nil {
			return err
		}
		if legacyMovieFeature {
			return migrateMovieFeatures(tx)
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Printf("✅ 数据库初始化完成: %s", dbPath)
	return nil
}

// InitDefaultUser 只在空用户表中创建管理员，支持部署预置口令。
// 密码使用 Bcrypt 加盐哈希存储，绝不保存明文
func InitDefaultUser(hashFunc func(string) (string, error)) error {
	var count int64
	if err := DB.Model(&User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("查询用户数量失败: %w", err)
	}
	if count > 0 {
		return nil // 已有用户，跳过
	}

	password := os.Getenv("ANIGO_ADMIN_PASSWORD")
	generated := password == ""
	if generated {
		var secret [24]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return err
		}
		password = base64.RawURLEncoding.EncodeToString(secret[:])
	} else if len(password) < 6 || len(password) > 72 {
		return fmt.Errorf("ANIGO_ADMIN_PASSWORD 长度应为 6 到 72 字节")
	}
	hash, err := hashFunc(password)
	if err != nil {
		return err
	}

	user := User{
		Username:     "admin",
		PasswordHash: hash,
	}
	result := DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "username"}}, DoNothing: true}).Create(&user)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}

	if generated {
		// 仅向部署控制台显示一次，避免将口令写入 Web 可读取的应用日志。
		fmt.Fprintf(os.Stderr, "Ani-Go 管理员 admin 的首次登录密码：%s\n", password)
	}
	log.Println("已创建管理员账号 admin")
	return nil
}
