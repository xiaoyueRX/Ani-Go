// Package metadata 实现元数据提供者与外部平台映射
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// Mapper 提供 Bangumi ID 到 TMDB/IMDB 及统一系列的自动映射
type Mapper struct {
	httpClient *http.Client
	tmdb       *TMDBProvider
}

// NewMapper 创建外部元数据映射器
func NewMapper(tmdb *TMDBProvider) *Mapper {
	return &Mapper{
		httpClient: httpx.New(15 * time.Second),
		tmdb:       tmdb,
	}
}

// MapBangumiToExternal 将 Bangumi ID 映射为 TMDB/IMDB ID
func (m *Mapper) MapBangumiToExternal(ctx context.Context, bgmID string, title string, year int) (tmdbID, imdbID string) {
	// 1. 优先查社区在线映射表 (API: arm.moe)
	tmdbID, imdbID = m.queryARM(ctx, bgmID)
	if tmdbID != "" {
		return tmdbID, imdbID
	}

	// 2. 兜底：使用 TMDB TV 搜索
	cleanTitle := strings.TrimSpace(title)
	if m.tmdb != nil && cleanTitle != "" {
		results, err := m.tmdb.SearchAnime(ctx, cleanTitle)
		if err == nil && len(results) > 0 {
			best := results[0]
			for _, r := range results {
				if r.Year == year {
					best = r
					break
				}
			}
			tmdbID = "tv/" + best.ID
			imdb, _ := m.tmdb.GetExternalIDs(ctx, tmdbID)
			imdbID = imdb
			return tmdbID, imdbID
		}

		// 3. 剧场版/Movie 搜索兜底
		movieResults, err := m.tmdb.SearchMovies(ctx, cleanTitle)
		if err == nil && len(movieResults) > 0 {
			bestMovie := movieResults[0]
			for _, r := range movieResults {
				if r.Year == year {
					bestMovie = r
					break
				}
			}
			tmdbID = "movie/" + bestMovie.ID
			imdb, _ := m.tmdb.GetExternalIDs(ctx, tmdbID)
			imdbID = imdb
			return tmdbID, imdbID
		}
	}

	return tmdbID, imdbID
}

// MapBangumiToExternalWithSeries 在映射 TMDB/IMDB 的同时，解析其统一系列 ID
func (m *Mapper) MapBangumiToExternalWithSeries(ctx context.Context, bgmID string, title string, year int) (tmdbID, imdbID, seriesID string) {
	tmdbID, imdbID = m.MapBangumiToExternal(ctx, bgmID, title, year)
	cleanTitle := strings.TrimSpace(title)
	if m.tmdb != nil && (tmdbID != "" || cleanTitle != "") {
		if info, err := m.tmdb.ResolveSeriesCollection(ctx, tmdbID, cleanTitle); err == nil && info.SeriesID != "" {
			seriesID = info.SeriesID
		}
	}
	if seriesID == "" && cleanTitle != "" {
		seriesID = ExtractFranchiseTitle(cleanTitle)
	}
	return tmdbID, imdbID, seriesID
}

func (m *Mapper) queryARM(ctx context.Context, bgmID string) (tmdbID, imdbID string) {
	cleanID := strings.TrimSpace(bgmID)
	if cleanID == "" {
		return "", ""
	}
	armURL := fmt.Sprintf("https://api.arm.moe/api/v1/relations/bgm/%s", url.PathEscape(cleanID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, armURL, nil)
	if err != nil {
		return "", ""
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", ""
	}

	var result struct {
		TMDBID int    `json:"tmdb_id"`
		IMDBID string `json:"imdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if result.TMDBID > 0 {
			tmdbID = fmt.Sprintf("tv/%d", result.TMDBID)
		}
		imdbID = result.IMDBID
	}
	return tmdbID, imdbID
}
