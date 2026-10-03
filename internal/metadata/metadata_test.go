package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBGMTVProvider_GetAnimeRelations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/subjects/100" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"id": 100,
				"name": "Shingeki no Kyojin",
				"name_cn": "进击的巨人",
				"date": "2013-04-06",
				"total_episodes": 25,
				"type": 2
			}`))
			return
		}
		if r.URL.Path == "/v0/subjects/100/subjects" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{
					"id": 90,
					"name": "Prequel Anime",
					"name_cn": "前传",
					"relation": "前传"
				}
			]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewBGMTVProvider("", []string{server.URL})
	anime, err := provider.GetAnime(context.Background(), "100")
	if err != nil {
		t.Fatalf("GetAnime 失败: %v", err)
	}
	if anime.TitleCN != "进击的巨人" {
		t.Fatalf("TitleCN 不符合: %s", anime.TitleCN)
	}
}



func TestBGMTVProvider_GetAnimeRelations_FindSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/subjects/200" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"id": 200,
				"name": "Shingeki no Kyojin Season 2",
				"name_cn": "进击的巨人 第二季",
				"date": "2017-04-01",
				"total_episodes": 12,
				"type": 2
			}`))
			return
		}
		if r.URL.Path == "/v0/subjects/200/subjects" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{
					"id": 100,
					"name": "Shingeki no Kyojin",
					"name_cn": "进击的巨人",
					"relation": "前传"
				}
			]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewBGMTVProvider("", []string{server.URL})
	anime, err := provider.GetAnime(context.Background(), "200")
	if err != nil {
		t.Fatalf("GetAnime 失败: %v", err)
	}
	if anime.SeriesID != "100" {
		t.Fatalf("SeriesID 期望 100, 实际得到: %s", anime.SeriesID)
	}
}
