package database

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateSubscriptionAcrossSQLiteConnections(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "subs.sqlite") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	var stores []*gorm.DB
	for range 2 {
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{PrepareStmt: true})
		if err != nil {
			t.Fatal(err)
		}
		conn, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		conn.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = conn.Close() })
		stores = append(stores, db)
	}
	if err := stores[0].AutoMigrate(&Subscription{}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 16)
	for i := range cap(errs) {
		go func() {
			<-start
			errs <- CreateSubscription(stores[i%2], &Subscription{TitleCN: "Across Connections", Season: 2})
		}()
	}
	close(start)
	created := 0
	for range cap(errs) {
		switch err := <-errs; {
		case err == nil:
			created++
		case errors.Is(err, ErrSubscriptionExists):
		default:
			t.Errorf("并发数据库请求失败: %v", err)
		}
	}
	var count int64
	if err := stores[1].Model(&Subscription{}).Count(&count).Error; err != nil || count != 1 || created != 1 {
		t.Fatalf("成功 %d 次、保存 %d 项，错误 %v", created, count, err)
	}
}

func TestCreateSubscriptionChecksExplicitTMDBKind(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kinds.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Subscription{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		title, source, id, kind string
		reject                  bool
	}{
		{"电影带剧集编号", "TMDB", "tv/456", "Film", true},
		{"剧集带电影编号", "tmdb", "MOVIE/456", "Miniseries", true},
		{"裸号电影", "TMDB", "21", "劇場版", false},
		{"未知分类", "TMDB", "movie/22", "OVA", false},
		{"其他来源", "BGM.tv", "movie/23", "TV", false},
	} {
		t.Run(tc.title, func(t *testing.T) {
			sub := Subscription{TitleCN: tc.title, MetadataProvider: tc.source, MetadataID: tc.id, AnimeType: tc.kind}
			err := CreateSubscription(db, &sub)
			var count int64
			if dbErr := db.Model(&Subscription{}).Where("title_cn = ?", tc.title).Count(&count).Error; dbErr != nil {
				t.Fatal(dbErr)
			}
			if tc.reject && (err == nil || sub.ID != 0 || count != 0) || !tc.reject && (err != nil || count != 1) {
				t.Fatalf("创建结果：错误 %v，编号 %d，记录数 %d", err, sub.ID, count)
			}
		})
	}
}
