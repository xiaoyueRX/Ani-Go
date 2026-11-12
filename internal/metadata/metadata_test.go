package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestTMDBProvider_MockEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/tv/85937/season/1"):
			w.Write([]byte(`{
				"episodes": [
					{"episode_number": 1, "name": "残酷", "air_date": "2019-04-06"}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/tv/85937/external_ids"):
			w.Write([]byte(`{"imdb_id": "tt9335498"}`))
		case strings.HasPrefix(r.URL.Path, "/movie/635302/external_ids"):
			w.Write([]byte(`{"imdb_id": "tt11032374"}`))
		case strings.HasPrefix(r.URL.Path, "/tv/85937"):
			// TV 详情
			w.Write([]byte(`{
				"id": 85937,
				"name": "鬼灭之刃",
				"original_name": "鬼滅の刃",
				"first_air_date": "2019-04-06",
				"number_of_seasons": 4,
				"number_of_episodes": 55,
				"type": "Animation",
				"overview": "炭治郎的灭鬼传奇",
				"external_ids": {"imdb_id": "tt9335498"}
			}`))
		case strings.HasPrefix(r.URL.Path, "/movie/635302"):
			// 电影详情（带 belongs_to_collection）
			w.Write([]byte(`{
				"id": 635302,
				"title": "鬼灭之刃 无限列车篇",
				"original_title": "劇場版「鬼滅の刃」無限列車編",
				"release_date": "2020-10-16",
				"overview": "无限列车任务",
				"belongs_to_collection": {
					"id": 726417,
					"name": "鬼灭之刃（系列）",
					"poster_path": "/collection_poster.jpg"
				},
				"external_ids": {"imdb_id": "tt11032374"}
			}`))
		case strings.HasPrefix(r.URL.Path, "/collection/726417"):
			// 合集详情
			w.Write([]byte(`{
				"id": 726417,
				"name": "鬼灭之刃（系列）",
				"overview": "鬼灭之刃动画合集",
				"parts": [
					{"id": 635302, "title": "鬼灭之刃 无限列车篇", "media_type": "movie"},
					{"id": 1056580, "title": "鬼灭之刃 上弦集结", "media_type": "movie"}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/search/tv"):
			w.Write([]byte(`{
				"page": 1,
				"results": [
					{"id": 85937, "name": "鬼灭之刃", "original_name": "鬼滅の刃", "first_air_date": "2019-04-06"}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/search/movie"):
			w.Write([]byte(`{
				"page": 1,
				"results": [
					{"id": 635302, "title": "鬼灭之刃 无限列车篇", "original_title": "劇場版「鬼滅の刃」無限列車編", "release_date": "2020-10-16"}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/search/collection"):
			w.Write([]byte(`{
				"page": 1,
				"results": [
					{"id": 726417, "name": "鬼灭之刃（系列）"}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/tv/85937/season/1"):
			w.Write([]byte(`{
				"episodes": [
					{"episode_number": 1, "name": "残酷", "air_date": "2019-04-06"}
				]
			}`))
		case strings.HasPrefix(r.URL.Path, "/tv/85937/external_ids"):
			w.Write([]byte(`{"imdb_id": "tt9335498"}`))
		case strings.HasPrefix(r.URL.Path, "/movie/635302/external_ids"):
			w.Write([]byte(`{"imdb_id": "tt11032374"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	provider := NewTMDBProvider("dummy_key", "zh-CN", []string{server.URL})

	// 1. 测试 TV 详情
	tvAnime, err := provider.GetAnime(ctx, "tv/85937")
	if err != nil {
		t.Fatalf("GetAnime TV 失败: %v", err)
	}
	if tvAnime.TitleCN != "鬼灭之刃" || tvAnime.Type != "Animation" || tvAnime.IMDBID != "tt9335498" {
		t.Errorf("TV 详情不匹配: %+v", tvAnime)
	}

	// 2. 测试 Movie 详情（解析 belongs_to_collection）
	movieAnime, err := provider.GetAnime(ctx, "movie/635302")
	if err != nil {
		t.Fatalf("GetAnime Movie 失败: %v", err)
	}
	if movieAnime.TitleCN != "鬼灭之刃 无限列车篇" || movieAnime.Type != "Movie" || movieAnime.SeriesID != "鬼灭之刃" {
		t.Errorf("Movie 详情或系列名称不匹配: %+v", movieAnime)
	}

	// 3. 测试 Collection 详情与包含条目
	coll, err := provider.GetCollection(ctx, 726417)
	if err != nil {
		t.Fatalf("GetCollection 失败: %v", err)
	}
	if coll.Name != "鬼灭之刃（系列）" || len(coll.Parts) != 2 {
		t.Errorf("合集数据不匹配: %+v", coll)
	}

	// 4. 测试 ResolveSeriesCollection (Movie)
	movieInfo, err := provider.ResolveSeriesCollection(ctx, "movie/635302", "鬼灭之刃 无限列车篇")
	if err != nil {
		t.Fatalf("ResolveSeriesCollection (Movie) 失败: %v", err)
	}
	if movieInfo.CollectionID != 726417 || movieInfo.SeriesID != "鬼灭之刃" || len(movieInfo.PartTMDBIDs) != 2 {
		t.Errorf("Movie 系列探测不符合期望: %+v", movieInfo)
	}

	// 5. 测试 ResolveSeriesCollection (TV 搜索合集反查匹配)
	tvInfo, err := provider.ResolveSeriesCollection(ctx, "tv/85937", "鬼灭之刃")
	if err != nil {
		t.Fatalf("ResolveSeriesCollection (TV) 失败: %v", err)
	}
	if tvInfo.CollectionID != 726417 || tvInfo.SeriesID != "鬼灭之刃" {
		t.Errorf("TV 探测合集不符合期望: %+v", tvInfo)
	}

	// 6. 测试剧集查询
	eps, err := provider.GetEpisodes(ctx, "85937", 1)
	if err != nil || len(eps) != 1 || eps[0].Title != "残酷" {
		t.Errorf("GetEpisodes 失败: %v, eps: %+v", err, eps)
	}

	// 7. 测试外部 ID 查询 (movie 与 tv)
	tvIMDB, err := provider.GetExternalIDs(ctx, "tv/85937")
	if err != nil || tvIMDB != "tt9335498" {
		t.Errorf("GetExternalIDs TV 失败: %v, got %s", err, tvIMDB)
	}
	movieIMDB, err := provider.GetExternalIDs(ctx, "movie/635302")
	if err != nil || movieIMDB != "tt11032374" {
		t.Errorf("GetExternalIDs Movie 失败: %v, got %s", err, movieIMDB)
	}
}

func TestMapper_MapBangumiToExternalWithSeries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/search/tv") {
			w.Write([]byte(`{
				"page": 1,
				"results": [{"id": 85937, "name": "鬼灭之刃", "first_air_date": "2019-04-06"}]
			}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/tv/85937/external_ids") {
			w.Write([]byte(`{"imdb_id": "tt9335498"}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/tv/85937") {
			w.Write([]byte(`{
				"id": 85937,
				"name": "鬼灭之刃",
				"type": "Animation"
			}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/search/collection") {
			w.Write([]byte(`{
				"page": 1,
				"results": [{"id": 726417, "name": "鬼灭之刃（系列）"}]
			}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/collection/726417") {
			w.Write([]byte(`{"id": 726417, "name": "鬼灭之刃（系列）"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewTMDBProvider("dummy", "zh-CN", []string{server.URL})
	mapper := NewMapper(provider)

	ctx := context.Background()
	tmdbID, imdbID, seriesID := mapper.MapBangumiToExternalWithSeries(ctx, "99999999", "鬼灭之刃", 2019)
	if tmdbID != "tv/85937" {
		t.Errorf("tmdbID 期望 tv/85937, 实际 %s", tmdbID)
	}
	if imdbID != "tt9335498" {
		t.Errorf("imdbID 期望 tt9335498, 实际 %s", imdbID)
	}
	if seriesID != "鬼灭之刃" {
		t.Errorf("seriesID 期望 鬼灭之刃, 实际 %s", seriesID)
	}
}

func TestNormalizeAndCleanHelpers(t *testing.T) {
	// ParseTMDBID
	mt1, id1 := ParseTMDBID("tv/12345")
	if mt1 != "tv" || id1 != "12345" {
		t.Errorf("ParseTMDBID tv 错误: %s, %s", mt1, id1)
	}
	mt2, id2 := ParseTMDBID("movie/67890")
	if mt2 != "movie" || id2 != "67890" {
		t.Errorf("ParseTMDBID movie 错误: %s, %s", mt2, id2)
	}
	mt3, id3 := ParseTMDBID("9999")
	if mt3 != "tv" || id3 != "9999" {
		t.Errorf("ParseTMDBID bare id 错误: %s, %s", mt3, id3)
	}

	// CleanCollectionName
	if s := CleanCollectionName("鬼灭之刃（系列）"); s != "鬼灭之刃" {
		t.Errorf("CleanCollectionName 失败: %s", s)
	}
	if s := CleanCollectionName("Fate Series Collection"); s != "Fate Series" {
		t.Errorf("CleanCollectionName 失败: %s", s)
	}

	// ExtractFranchiseTitle
	if s := ExtractFranchiseTitle("进击的巨人 The Final Season 完结篇 后篇"); s != "进击的巨人" {
		t.Errorf("ExtractFranchiseTitle 失败: %s", s)
	}
	if s := ExtractFranchiseTitle("剧场版 咒术回战 0"); s != "咒术回战 0" {
		t.Errorf("ExtractFranchiseTitle 失败: %s", s)
	}
	if s := ExtractFranchiseTitle("Re:从零开始的异世界生活 第二季 后半"); s != "Re:从零开始的异世界生活" {
		t.Errorf("ExtractFranchiseTitle 失败: %s", s)
	}
}
