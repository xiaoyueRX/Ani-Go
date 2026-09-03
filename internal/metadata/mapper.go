package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

type Mapper struct {
	httpClient *http.Client
	tmdb       *TMDBProvider
}

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

	// 2. 兜底：使用 TMDB 搜索
	if m.tmdb != nil && title != "" {
		results, err := m.tmdb.SearchAnime(ctx, title)
		if err == nil && len(results) > 0 {
			// 简单的年份匹配
			best := results[0]
			for _, r := range results {
				if r.Year == year {
					best = r
					break
				}
			}
			tmdbID = "tv/" + best.ID // 假设是 TV，TMDBProvider 搜的是 TV
			// 补全 IMDB ID
			imdb, _ := m.tmdb.GetExternalIDs(ctx, tmdbID)
			imdbID = imdb
		}
	}

	return tmdbID, imdbID
}

func (m *Mapper) queryARM(ctx context.Context, bgmID string) (tmdbID, imdbID string) {
	url := fmt.Sprintf("https://api.arm.moe/api/v1/relations/bgm/%s", bgmID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
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
