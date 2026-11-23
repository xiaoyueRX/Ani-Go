package scheduler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
)

func TestTMDBSupplementUsesSubscriptionSeason(t *testing.T) {
	db := pipelineDB(t)
	for _, tc := range []struct {
		name                  string
		detail                string
		season, initial, want int
	}{
		{"第二季", `"number_of_seasons":2,"seasons":[{"season_number":0,"episode_count":3},{"season_number":1,"episode_count":23},{"season_number":2,"episode_count":12}]`, 2, 0, 12},
		{"已有总数", `"number_of_seasons":2,"seasons":[{"season_number":2,"episode_count":12}]`, 2, 10, 10},
		{"未知季", `"number_of_seasons":2`, 2, 0, 0},
		{"仅其他季", `"number_of_seasons":2,"seasons":[{"season_number":1,"episode_count":23}]`, 2, 0, 0},
		{"独季作品", `"number_of_seasons":1`, 1, 0, 35},
		{"未核实的第二季", `"number_of_seasons":1`, 2, 0, 0},
		{"零集的季", `"number_of_seasons":1,"seasons":[{"season_number":1,"episode_count":0}]`, 1, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/tv/207" {
					t.Errorf("意外的 TMDB 请求 %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.Write([]byte(`{"id":207,"name":"样片","number_of_episodes":35,` + tc.detail + `}`))
			}))
			defer server.Close()
			provider := metadata.NewTMDBProvider("fixture", "zh-CN", []string{server.URL})
			anime, err := provider.GetAnime(t.Context(), "207")
			if err != nil || anime.TotalEps != 35 {
				t.Fatalf("全剧详情总数 %v，错误 %v", anime.TotalEps, err)
			}
			sub := database.Subscription{TitleCN: tc.name, Season: tc.season, TotalEpisodes: tc.initial, MetadataID: "207", MetadataProvider: "TMDB"}
			savePipelineRow(t, db, &sub)
			s := Scheduler{metadataProvider: provider, source: &subscriptionFeedFixture{}}
			if err := s.TriggerSupplement(t.Context(), sub.ID); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&sub, sub.ID).Error; err != nil || sub.TotalEpisodes != tc.want {
				t.Fatalf("订阅总数 %d，期望 %d，错误 %v", sub.TotalEpisodes, tc.want, err)
			}
		})
	}
}

func TestTMDBSupplementKeepsMovieIdentity(t *testing.T) {
	db := pipelineDB(t)
	for _, tc := range []struct {
		name, meta, tmdb, kind string
		paths                  []string
		wantTotal              int
	}{
		{"带类型的作品 ID", "movie/207", "", "", []string{"/movie/207"}, 1},
		{"裸作品 ID 配规范 TMDB ID", "207", "movie/207", "", []string{"/movie/207"}, 1},
		{"仅有规范 TMDB ID", "", "movie/207", "", []string{"/movie/207"}, 1},
		{"裸 ID 只有电影分类", "207", "", "剧场版", nil, 0},
		{"跨命名空间的冲突", "tv/207", "movie/207", "", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case "/movie/207":
					json.NewEncoder(w).Encode(map[string]any{"id": 207, "title": tc.name, "overview": "电影简介"})
				case "/search/tv":
					w.Write([]byte(`{"results":[]}`))
				case "/tv/207":
					w.Write([]byte(`{"id":207,"overview":"错误的同号剧集","number_of_seasons":1,"number_of_episodes":35}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			provider := metadata.NewTMDBProvider("fixture", "zh-CN", []string{server.URL})
			sub := database.Subscription{TitleCN: tc.name, Season: 1, MetadataProvider: "TMDB", MetadataID: tc.meta, TMDBID: tc.tmdb, AnimeType: tc.kind}
			savePipelineRow(t, db, &sub)
			s := Scheduler{metadataProvider: provider, source: &subscriptionFeedFixture{}}
			if err := s.TriggerSupplement(t.Context(), sub.ID); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&sub, sub.ID).Error; err != nil {
				t.Fatal(err)
			}
			s.resolveSubscriptionEpisode(t.Context(), core.TorrentItem{Title: "样片", PublishedAt: time.Now()}, sub)
			if !reflect.DeepEqual(paths, tc.paths) || sub.TotalEpisodes != tc.wantTotal || (tc.wantTotal == 1 && sub.Description != "电影简介") {
				t.Fatalf("请求 %v；订阅集数 %d，简介 %q", paths, sub.TotalEpisodes, sub.Description)
			}
		})
	}
}

func TestTMDBSupplementPreservesCountEditedDuringLookup(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "同步补全", Season: 2, MetadataID: "207", MetadataProvider: "TMDB"}
	savePipelineRow(t, db, &sub)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := db.Model(&database.Subscription{}).Where("id = ?", sub.ID).Update("total_episodes", 14).Error; err != nil {
			t.Error(err)
		}
		w.Write([]byte(`{"id":207,"seasons":[{"season_number":2,"episode_count":12}]}`))
	}))
	defer server.Close()
	s := Scheduler{metadataProvider: metadata.NewTMDBProvider("fixture", "zh-CN", []string{server.URL}), source: &subscriptionFeedFixture{}}
	if err := s.TriggerSupplement(t.Context(), sub.ID); err != nil {
		t.Fatal(err)
	}
	var saved database.Subscription
	if err := db.First(&saved, sub.ID).Error; err != nil || saved.TotalEpisodes != 14 {
		t.Fatalf("手工集数被补全覆盖：%d，错误 %v", saved.TotalEpisodes, err)
	}
}

func TestTMDBAutoSubscribeRetainsMovieIdentity(t *testing.T) {
	db := pipelineDB(t)
	movie := core.Anime{ID: "207", Provider: "TMDB", TMDBID: "movie/207", Type: "Movie", TitleCN: "电影样片", Year: 2026}
	s := Scheduler{}
	for _, legacy := range []bool{true, false} {
		var sub database.Subscription
		if legacy {
			sub = database.Subscription{TitleCN: movie.TitleCN, Year: movie.Year, BangumiID: movie.ID}
			savePipelineRow(t, db, &sub)
		} else {
			movie.ID, movie.TMDBID, movie.TitleCN = "208", "movie/208", "第二部电影"
		}
		id, err := s.autoSubscribe(t.Context(), movie)
		if err != nil {
			t.Fatal(err)
		}
		if legacy {
			id = sub.ID
		}
		if err = db.First(&sub, id).Error; err != nil || sub.TMDBID != movie.TMDBID || sub.AnimeType != "Movie" {
			t.Errorf("自动订阅丢失电影标识：tmdb=%q type=%q error=%v", sub.TMDBID, sub.AnimeType, err)
		}
	}
}

func TestSupplementKeepsSourceIDsEditedDuringTitleSearch(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		editMetadata bool
	}{
		{"标题命中时改元数据和蜜柑", `{"results":[{"id":101,"name":"海港回声"}]}`, true},
		{"标题未命中时改元数据和蜜柑", `{"results":[]}`, true},
		{"仅改蜜柑", `{"results":[]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "海港回声", Season: 2, MikanID: "351", BangumiID: "351"}
			savePipelineRow(t, db, &sub)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/search/tv" {
					t.Errorf("手工选定其他提供者后仍请求过时的元数据 %s", r.URL.Path)
					w.Write([]byte(`{}`))
					return
				}
				// 查询结果返回前，经用户编辑入口提交到同一数据库。
				changes := map[string]any{"mikan_id": "352", "bangumi_id": "352"}
				if tc.editMetadata {
					changes["metadata_provider"], changes["metadata_id"] = "BGM.tv", "207"
				}
				_, err := database.UpdateSubscription(db, sub.ID, changes)
				if err != nil {
					t.Error(err)
				}
				w.Write([]byte(tc.result))
			}))
			defer server.Close()
			feed := &subscriptionFeedFixture{}
			s := Scheduler{source: feed, metadataProvider: metadata.NewTMDBProvider("fixture", "zh-CN", []string{server.URL})}
			if err := s.TriggerSupplement(t.Context(), sub.ID); err != nil {
				t.Fatal(err)
			}
			var saved database.Subscription
			if err := db.First(&saved, sub.ID).Error; err != nil {
				t.Fatal(err)
			}
			if tc.editMetadata && (saved.MetadataProvider != "BGM.tv" || saved.MetadataID != "207" || saved.TMDBID != "") ||
				saved.MikanID != "352" || saved.BangumiID != "352" {
				t.Errorf("手工标识被覆盖：元数据 %s/%s，TMDB %s，蜜柑 %s/%s", saved.MetadataProvider, saved.MetadataID,
					saved.TMDBID, saved.MikanID, saved.BangumiID)
			}
			if feed.historyID != "" && feed.historyID != "352" {
				t.Errorf("本次补全使用了过时的蜜柑 ID %q", feed.historyID)
			}
		})
	}
}
