package database

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 升级真实入库过的五个独立单文件。同时验证只有库路径或手工完成状态不足以认定正片。
func TestUpgradeSingleMovieFeatureProgress(t *testing.T) {
	previous := DB
	t.Cleanup(func() { DB = previous })
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Subscription{}, &Episode{}); err != nil {
		t.Fatal(err)
	}
	makeMovie := func(name, kind string, total int) Subscription {
		s := Subscription{TitleCN: name, AnimeType: kind, TotalEpisodes: total}
		if err := db.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	film := makeMovie("机动战士高达 逆袭的夏亚", "movie", 1)
	other := makeMovie("Another Film", "劇場版", 1)
	tv := makeMovie("True Series", "TV", 1)
	collection := makeMovie("Movie Set", "film", 2)
	future := makeMovie("Untitled Film", "movie", 0)
	confirmed := makeMovie("Confirmed Feature", "Film", 1)
	stamp := time.Now()
	epFor := func(s Subscription, n int, name string) Episode {
		hash := fmt.Sprintf("%040x", n)
		key := EpisodeFileReservationKey(s.ID, hash, name)
		return Episode{SubscriptionID: s.ID, Season: 1, Status: "organized", OriginalName: name[:len(name)-4], Title: name[:len(name)-4],
			TorrentHash: hash, ReservationKey: &key, DownloadRoot: "/downloads", SourceFile: name, SourceModTime: stamp.UnixNano(), FileSize: 57023,
			FinalPath: "/library/" + name, OrganizedAt: &stamp}
	}
	put := func(e Episode) {
		if err := db.Create(&e).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 5; i++ {
		ep := epFor(film, i, fmt.Sprintf("[Grp%d] %s [Movie] [1080p].mkv", i, film.TitleCN))
		ep.GroupName = fmt.Sprintf("Grp%d", i)
		put(ep)
	}
	put(epFor(tv, 6, "True Series.mkv"))
	put(epFor(collection, 7, "Movie Set.mkv"))
	put(epFor(future, 8, "Untitled Film.mkv"))
	put(epFor(other, 9, "Another Film - Trailer.mkv"))
	put(epFor(other, 10, "selected.mkv"))
	put(epFor(other, 13, "Another Film.srt"))
	manual := epFor(other, 11, "Another Film.mkv")
	manual.Status = "completed"
	manual.OrganizedAt = nil
	put(manual)
	pack := epFor(other, 12, "Another Film (1080p).mkv")
	put(pack)
	pack = epFor(other, 12, "Disc2.mkv")
	pack.Status = EpisodeNeedsReview
	pack.ReviewKind = ReviewMovieSelection
	put(pack)
	chosen := epFor(confirmed, 14, "Disc/main.mkv")
	chosen.ConfirmedIdentity = chosen.ConfirmationSignature(ReviewMovieSelection)
	put(chosen)
	put(epFor(confirmed, 14, "Disc/extra.mkv"))
	stale := epFor(other, 15, "Disc/main.mkv")
	stale.ConfirmedIdentity = stale.ConfirmationSignature(ReviewMovieSelection)
	stale.SourceModTime++
	put(stale)
	if db.Migrator().HasColumn(&Episode{}, "movie_feature") {
		if err := db.Exec("ALTER TABLE episodes DROP COLUMN movie_feature").Error; err != nil {
			t.Fatal(err)
		}
	}
	conn, _ := db.DB()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	conn, _ = DB.DB()
	t.Cleanup(func() { conn.Close() })
	ids := []uint{film.ID, other.ID, tv.ID, collection.ID, future.ID, confirmed.ID}
	counts, err := FinishedEpisodeCounts(DB, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		want := 0
		if id == film.ID || id == confirmed.ID {
			want = 1
		}
		var saved Subscription
		if err := DB.First(&saved, id).Error; err != nil {
			t.Fatal(err)
		}
		if counts[id] != want || saved.CurrentEpisodes != want || saved.Completed != (want == 1) || saved.Enabled == nil || !*saved.Enabled {
			t.Errorf("subscription %s: listed %d, stored %d, completed %t, enabled %v, want %d", saved.TitleCN, counts[id], saved.CurrentEpisodes, saved.Completed, saved.Enabled, want)
		}
	}
	// 类型会改变无集号电影的进度，存储计数应随编辑立即重算；显式完结开关仍由用户决定。
	for _, tc := range []struct {
		kind string
		want int
	}{{"tv", 0}, {"film", 1}} {
		saved, err := UpdateSubscription(DB, film.ID, map[string]any{"anime_type": tc.kind, "completed": false})
		if err != nil || saved.CurrentEpisodes != tc.want || saved.Completed {
			t.Errorf("type %s, persisted %d completed %v; want %d and false: %v", tc.kind, saved.CurrentEpisodes, saved.Completed, tc.want, err)
		}
	}
	// 新旧正片编号都代表该片，不能让数值变成 2。
	putNumbered := Episode{SubscriptionID: film.ID, Number: 1, Season: 1, Status: "organized"}
	if err := DB.Create(&putNumbered).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := SyncEpisodeProgress(DB, film.ID, false); err != nil || count != 1 {
		t.Errorf("mixed formats count %d: %v", count, err)
	}
}
