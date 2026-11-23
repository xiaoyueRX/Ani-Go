package migrate

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestMigrationInputsAndRetries(t *testing.T) {
	dir := t.TempDir()
	open := func(path string) *gorm.DB {
		db, e := gorm.Open(sqlite.Open(path), &gorm.Config{})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close() })
		return db
	}
	orig := database.DB
	database.DB = open(filepath.Join(dir, "target.db"))
	defer func() { database.DB = orig }()
	if err := database.DB.AutoMigrate(&database.Subscription{}, &database.Episode{}, &database.DownloadRecord{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "auto.db")
	source := open(path)
	if err := source.AutoMigrate(&abAnime{}, &abEpisode{}); err != nil {
		t.Fatal(err)
	}
	// 旧数据库的 bangumi_id 可能来自蜜柑，不可只凭相同数字认领。
	other := database.Subscription{TitleCN: "其他来源", BangumiID: "111", Season: 1}
	if err := database.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	a := abAnime{Title: "样例", BgmID: "111", Enable: 1}
	source.Create(&a)
	source.Create(&abEpisode{AnimeID: a.ID, EpisodeNumber: 1, TorrentURL: "https://example.org/t", TorrentHash: "hash"})
	for i := 0; i < 2; i++ {
		st, e := MigrateFromPath(path)
		if e != nil {
			t.Fatal(e)
		}
		if st.Subscriptions != 1-i || st.Episodes != 1-i || st.Downloads != 1-i {
			t.Errorf("重复导入计数 %d: %+v", i, st)
		}
	}
	raw := []byte(`[{"id":"example","url":"https://example.org/rss","title":"另一部","season":2,"enable":false,"currentEpisodeNumber":3,"totalEpisodeNumber":12}]`)
	jsonPath := filepath.Join(dir, "ani.v2.json")
	if err := os.WriteFile(jsonPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	st, err := MigrateFromPath(jsonPath)
	if err != nil || st.Subscriptions != 1 {
		t.Fatalf("订阅清单导入失败: %+v %v", st, err)
	}
	zPath := filepath.Join(dir, "backup.zip")
	f, e := os.Create(zPath)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	entry, e := w.Create("ani.v2.json")
	if e != nil {
		t.Fatal(e)
	}
	entry.Write(raw)
	w.Close()
	f.Close()
	st, err = MigrateFromPath(zPath)
	if err != nil || st.Subscriptions != 0 {
		t.Fatalf("原生备份未按同一订阅匹配: %+v %v", st, err)
	}
	var sub database.Subscription
	if err := database.DB.Where("title_cn = ?", "另一部").First(&sub).Error; err != nil || sub.Enabled == nil || *sub.Enabled || sub.CurrentEpisodes != 3 || sub.TotalEpisodes != 12 {
		t.Fatalf("订阅属性未保留: %+v %v", sub, err)
	}
}
