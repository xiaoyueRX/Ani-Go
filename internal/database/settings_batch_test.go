package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLoadSettingsMap(t *testing.T) {
	if got := LoadSettingsMap(nil); len(got) != 0 {
		t.Fatalf("nil db 应返回空 map，实际 %+v", got)
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Setting{}); err != nil {
		t.Fatal(err)
	}

	if got := LoadSettingsMap(db); len(got) != 0 {
		t.Fatalf("空表应返回空 map，实际 %+v", got)
	}

	_ = db.Create(&Setting{Key: "A", Value: "1"}).Error
	_ = db.Create(&Setting{Key: "B", Value: "2"}).Error

	got := LoadSettingsMap(db)
	if len(got) != 2 || got["A"] != "1" || got["B"] != "2" {
		t.Fatalf("批量读取设置不匹配: %+v", got)
	}
}
