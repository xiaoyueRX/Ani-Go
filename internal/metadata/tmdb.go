// Package metadata 实现各元数据提供者的 MetadataProvider 接口
// TMDB 提供者使用 TMDB API v3 获取番剧元数据
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// TMDBProvider 实现 core.MetadataProvider 接口
// 使用 TMDB API v3 获取番剧元数据
type TMDBProvider struct {
	httpClient    *http.Client
	apiKey        string
	language      string
	mirrorDomains []string // API 镜像基础 URL 列表
	baseURL       string   // 当前可用的基础 URL
}

// NewTMDBProvider 创建 TMDB 元数据提供者
func NewTMDBProvider(apiKey, language string, mirrorDomains []string) *TMDBProvider {
	baseURL := "https://api.themoviedb.org/3"
	if len(mirrorDomains) > 0 {
		baseURL = mirrorDomains[0]
	}
	return &TMDBProvider{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		apiKey:        apiKey,
		language:      language,
		mirrorDomains: mirrorDomains,
		baseURL:       baseURL,
	}
}

func (p *TMDBProvider) Name() string { return "TMDB" }

// ============================================================
// TMDB JSON 响应结构体
// ============================================================

type tmdbSearchResponse struct {
	Page    int               `json:"page"`
	Results []tmdbSearchResult `json:"results"`
}

type tmdbSearchResult struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	FirstAirDate string `json:"first_air_date"`
	PosterPath   string `json:"poster_path"`
	Overview     string `json:"overview"`
}

type tmdbTVDetail struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	OriginalName     string `json:"original_name"`
	FirstAirDate     string `json:"first_air_date"`
	PosterPath       string `json:"poster_path"`
	Overview         string `json:"overview"`
	NumberOfSeasons  int    `json:"number_of_seasons"`
	NumberOfEpisodes int    `json:"number_of_episodes"`
	Type             string `json:"type"`
}

type tmdbSeasonResponse struct {
	Episodes []tmdbEpisode `json:"episodes"`
}

type tmdbEpisode struct {
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	AirDate       string `json:"air_date"`
}

// ============================================================
// 接口实现
// ============================================================

// SearchAnime 搜索番剧
func (p *TMDBProvider) SearchAnime(ctx context.Context, title string) ([]core.Anime, error) {
	path := fmt.Sprintf("/search/tv?api_key=%s&language=%s&query=%s&page=1",
		p.apiKey, p.language, url.QueryEscape(title))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 搜索失败: %w", err)
	}
	defer resp.Body.Close()

	var result tmdbSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("TMDB 响应解析失败: %w", err)
	}

	animes := make([]core.Anime, 0, len(result.Results))
	for _, r := range result.Results {
		year := 0
		if len(r.FirstAirDate) >= 4 {
			year, _ = strconv.Atoi(r.FirstAirDate[:4])
		}
		coverURL := ""
		if r.PosterPath != "" {
			coverURL = "https://image.tmdb.org/t/p/w500" + r.PosterPath
		}
		animes = append(animes, core.Anime{
			ID:          strconv.Itoa(r.ID),
			Provider:    "TMDB",
			TitleCN:     r.Name,
			TitleJP:     r.OriginalName,
			Year:        year,
			Description: r.Overview,
			CoverURL:    coverURL,
		})
	}
	return animes, nil
}

// GetAnime 获取番剧详情
func (p *TMDBProvider) GetAnime(ctx context.Context, id string) (core.Anime, error) {
	path := fmt.Sprintf("/tv/%s?api_key=%s&language=%s", id, p.apiKey, p.language)

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return core.Anime{}, fmt.Errorf("TMDB 获取详情失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return core.Anime{}, fmt.Errorf("TMDB: 番剧未找到 (ID: %s)", id)
	}

	var detail tmdbTVDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return core.Anime{}, fmt.Errorf("TMDB 响应解析失败: %w", err)
	}

	year := 0
	if len(detail.FirstAirDate) >= 4 {
		year, _ = strconv.Atoi(detail.FirstAirDate[:4])
	}
	coverURL := ""
	if detail.PosterPath != "" {
		coverURL = "https://image.tmdb.org/t/p/w500" + detail.PosterPath
	}

	animeType := detail.Type
	if animeType == "" {
		animeType = "TV"
	}

	return core.Anime{
		ID:          strconv.Itoa(detail.ID),
		Provider:    "TMDB",
		TitleCN:     detail.Name,
		TitleJP:     detail.OriginalName,
		Year:        year,
		TotalEps:    detail.NumberOfEpisodes,
		Type:        animeType,
		Description: detail.Overview,
		CoverURL:    coverURL,
	}, nil
}

// GetEpisodes 获取指定季的集数列表
func (p *TMDBProvider) GetEpisodes(ctx context.Context, animeID string, season int) ([]core.Episode, error) {
	path := fmt.Sprintf("/tv/%s/season/%d?api_key=%s&language=%s",
		animeID, season, p.apiKey, p.language)

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 获取剧集失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("TMDB: 季未找到 (ID: %s, Season: %d)", animeID, season)
	}

	var seasonResp tmdbSeasonResponse
	if err := json.NewDecoder(resp.Body).Decode(&seasonResp); err != nil {
		return nil, fmt.Errorf("TMDB 响应解析失败: %w", err)
	}

	episodes := make([]core.Episode, 0, len(seasonResp.Episodes))
	for _, ep := range seasonResp.Episodes {
		airedAt := time.Time{}
		if ep.AirDate != "" {
			airedAt, _ = time.Parse("2006-01-02", ep.AirDate)
		}
		episodes = append(episodes, core.Episode{
			AnimeID: animeID,
			Season:  season,
			Number:  float32(ep.EpisodeNumber),
			Title:   ep.Name,
			AiredAt: airedAt,
		})
	}
	return episodes, nil
}

// ============================================================
// 镜像重试
// ============================================================

// tryMirrors 依次尝试镜像域名发起 GET 请求，返回首个成功的响应
func (p *TMDBProvider) tryMirrors(ctx context.Context, path string) (*http.Response, error) {
	domains := make([]string, 0, 1+len(p.mirrorDomains))
	if p.baseURL != "" {
		domains = append(domains, p.baseURL)
	}
	for _, d := range p.mirrorDomains {
		if d != p.baseURL {
			domains = append(domains, d)
		}
	}

	var lastErr error
	for _, baseURL := range domains {
		fullURL := baseURL + path
		if !strings.HasPrefix(fullURL, "http") {
			fullURL = "https://" + fullURL
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Ani-Go/1.0")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			log.Printf("⚠️ TMDB 镜像不可达 [%s]: %v", baseURL, err)
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
			return resp, nil
		}
		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return nil, fmt.Errorf("TMDB API Key 无效 (401)")
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("所有 TMDB 镜像均不可达: %w", lastErr)
}

