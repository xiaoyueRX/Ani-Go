package auth

import (
	"bytes"
	"path/filepath"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func secretDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.AutoMigrate(&database.Setting{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func TestPersistentSigningSecretSharesCommittedFirstKey(t *testing.T) {
	db := secretDB(t)
	var wg sync.WaitGroup
	problems := make(chan error, 6)
	for range 6 {
		wg.Go(func() { problems <- InitSecretFromDB(db) })
	}
	wg.Wait()
	close(problems)
	for err := range problems {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int64
	if err := db.Model(&database.Setting{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("密钥行数异常: %d %v", n, err)
	}
	token, err := GenerateToken("admin", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitSecret(); err != nil {
		t.Fatal(err)
	}
	if err := InitSecretFromDB(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateToken(token); err != nil {
		t.Fatal("重载同一库后无法验证已有登录")
	}
}
func TestPersistentSigningSecretPreservesLastKeyOnErrors(t *testing.T) {
	db := secretDB(t)
	if err := InitSecret(); err != nil {
		t.Fatal(err)
	}
	old := append([]byte(nil), GetSecretBytes()...)
	if err := db.Exec("CREATE TRIGGER reject_signing_key BEFORE INSERT ON settings BEGIN SELECT RAISE(ABORT, 'storage unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := InitSecretFromDB(db); err == nil || !bytes.Equal(old, GetSecretBytes()) {
		t.Fatal("数据库拒绝写入时发布了临时签名密钥")
	}
	if err := db.Exec("DROP TRIGGER reject_signing_key").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.Setting{Key: "jwt_secret", Value: "damaged"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := InitSecretFromDB(db); err == nil || !bytes.Equal(old, GetSecretBytes()) {
		t.Fatal("损坏的持久密钥被静默替换")
	}
	var row database.Setting
	if err := db.First(&row, "key = ?", "jwt_secret").Error; err != nil || row.Value != "damaged" {
		t.Fatal("写坏了原配置")
	}
}
