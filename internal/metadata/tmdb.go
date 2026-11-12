// Package metadata 实现各元数据提供者的 MetadataProvider 接口
// TMDB 提供者使用 TMDB API v3 获取番剧元数据及系列合集归属
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// TMDBProvider 实现 core.MetadataProvider 接口
// 使用 TMDB API v3 获取番剧、剧场版及系列合集元数据
type TMDBProvider struct {
	mu            sync.RWMutex // 保护 API 配置与当前可用镜像 URL
	httpClient    *http.Client
	apiKey        string
	language      string
	mirrorDomains []string // API 镜像基础 URL 列表
	baseURL       string   // 当前可用的基础 URL
}

// NewTMDBProvider 创建 TMDB 元数据提供者
func NewTMDBProvider(apiKey, language string, mirrorDomains []string) *TMDBProvider {
	cleaned := cleanDomains(mirrorDomains)
	if len(cleaned) == 0 {
		cleaned = []string{"https://api.themoviedb.org/3"}
	}
	return &TMDBProvider{
		httpClient:    httpx.New(30 * time.Second),
		apiKey:        strings.TrimSpace(apiKey),
		language:      strings.TrimSpace(language),
		mirrorDomains: cleaned,
		baseURL:       cleaned[0],
	}
}

func (p *TMDBProvider) Name() string { return "TMDB" }

// ============================================================
// TMDB JSON 响应与合集解析结构体
// ============================================================

type tmdbSearchResponse struct {
	Page    int                `json:"page"`
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

type tmdbMovieSearchResponse struct {
	Page    int                     `json:"page"`
	Results []tmdbMovieSearchResult `json:"results"`
}

type tmdbMovieSearchResult struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	PosterPath    string `json:"poster_path"`
	Overview      string `json:"overview"`
}

type tmdbCollectionRef struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	PosterPath string `json:"poster_path"`
}

type tmdbTVDetail struct {
	ID                  int                `json:"id"`
	Name                string             `json:"name"`
	OriginalName        string             `json:"original_name"`
	FirstAirDate        string             `json:"first_air_date"`
	PosterPath          string             `json:"poster_path"`
	Overview            string             `json:"overview"`
	NumberOfSeasons     int                `json:"number_of_seasons"`
	NumberOfEpisodes    int                `json:"number_of_episodes"`
	Type                string             `json:"type"`
	BelongsToCollection *tmdbCollectionRef `json:"belongs_to_collection"`
	ExternalIDs         struct {
		IMDBID string `json:"imdb_id"`
	} `json:"external_ids"`
}

type tmdbMovieDetail struct {
	ID                  int                `json:"id"`
	Title               string             `json:"title"`
	OriginalTitle       string             `json:"original_title"`
	ReleaseDate         string             `json:"release_date"`
	PosterPath          string             `json:"poster_path"`
	Overview            string             `json:"overview"`
	BelongsToCollection *tmdbCollectionRef `json:"belongs_to_collection"`
	ExternalIDs         struct {
		IMDBID string `json:"imdb_id"`
	} `json:"external_ids"`
}

type tmdbSeasonResponse struct {
	Episodes []tmdbEpisode `json:"episodes"`
}

type tmdbEpisode struct {
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	AirDate       string `json:"air_date"`
}

// TMDBCollection TMDB 合集（系列）详情
type TMDBCollection struct {
	ID           int                  `json:"id"`
	Name         string               `json:"name"`
	Overview     string               `json:"overview"`
	PosterPath   string               `json:"poster_path"`
	BackdropPath string               `json:"backdrop_path"`
	Parts        []TMDBCollectionPart `json:"parts"`
}

// TMDBCollectionPart TMDB 合集中的单部作品条目
type TMDBCollectionPart struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	MediaType     string `json:"media_type"`
	ReleaseDate   string `json:"release_date"`
	PosterPath    string `json:"poster_path"`
}

type tmdbCollectionSearchResponse struct {
	Page    int              `json:"page"`
	Results []TMDBCollection `json:"results"`
}

// SeriesCollectionInfo 番剧系列/合集归属解析结果
type SeriesCollectionInfo struct {
	TMDBID         string   `json:"tmdb_id,omitempty"`         // 规范化后的 TMDB 标识（如 tv/123 或 movie/456）
	MediaType      string   `json:"media_type,omitempty"`      // "tv" 或 "movie"
	CollectionID   int      `json:"collection_id,omitempty"`   // TMDB Collection ID（剧场版或合集匹配命中时非 0）
	CollectionName string   `json:"collection_name,omitempty"` // TMDB 合集原始名称
	SeriesID       string   `json:"series_id,omitempty"`       // 推荐用于目录聚合的统一 SeriesID
	FranchiseKey   string   `json:"franchise_key,omitempty"`   // 归一化系列键，用于跨 TV/Movie 关联
	Source         string   `json:"source,omitempty"`          // 命中来源：movie_collection / tv_collection / collection_search / tv_detail / normalized_title
	PartTMDBIDs    []string `json:"part_tmdb_ids,omitempty"`   // 同合集下所有成员的规范化 TMDB ID
}

// ============================================================
// TMDB ID 与系列名称规范化辅助函数
// ============================================================

// ParseTMDBID 解析 TMDB ID 的媒体类型和纯数字/字符串标识
// 支持 "tv/123"、"movie/456" 或裸 ID "123"（默认归为 tv）
func ParseTMDBID(raw string) (mediaType string, cleanID string) {
	s := strings.Trim(strings.TrimSpace(raw), "/")
	if s == "" {
		return "", ""
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "movie/") {
		id := strings.Trim(s[len("movie/"):], "/")
		if id == "" {
			return "", ""
		}
		return "movie", id
	}
	if strings.HasPrefix(lower, "tv/") {
		id := strings.Trim(s[len("tv/"):], "/")
		if id == "" {
			return "", ""
		}
		return "tv", id
	}
	return "tv", s
}

// NormalizeTMDBID 将 TMDB ID 规范化为 "tv/{id}" 或 "movie/{id}" 形式
func NormalizeTMDBID(raw string) string {
	mediaType, cleanID := ParseTMDBID(raw)
	if cleanID == "" {
		return ""
	}
	return mediaType + "/" + cleanID
}

var (
	reCollectionSuffix = regexp.MustCompile(`(?i)\s*(?:[-–—]?\s*(?:\(collection\)|collection|（系列）|\(系列\)|系列|（合集）|\(合集\)|合集))+$`)
	reLeadingPrefix    = regexp.MustCompile(`^(?:剧场版|劇場版|映画)\s*[:：\-]?\s*`)
	reSeasonSuffix     = regexp.MustCompile(`(?i)(?:\s*[:：\-]?\s*(?:第[一二三四五六七八九十百0-9]+[季期部话話]|第[一二三四五六七八九十0-9]+クール|season\s*\d+|\d+(?:st|nd|rd|th)\s+season|(?:the\s+)?final\s+season|最终季|最終季|最终章|最終章|完结篇|完結篇|part\s*\d+|cour\s*\d+|(?:the\s+)?movie|剧场版|劇場版|ova|oad|special|sp|\bs\d{1,2}|\b(?:ii|iii|iv|v)|\s+[\p{Han}\w]+[篇章]))+$`)
)

// CleanCollectionName 清理 TMDB 合集名称中的通用“Collection/系列/合集”后缀
func CleanCollectionName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	cleaned := strings.TrimSpace(reCollectionSuffix.ReplaceAllString(s, ""))
	cleaned = strings.TrimRight(cleaned, ":：-–—/ ")
	if cleaned == "" {
		return s
	}
	return cleaned
}

// ExtractFranchiseTitle 从番剧或剧场版标题中提取核心系列主标题（剥离季号、篇幅、剧场版及合集后缀）
func ExtractFranchiseTitle(title string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		return ""
	}
	s = CleanCollectionName(s)

	// 仅在剥离“剧场版”前缀后仍有主标题时才生效
	if stripped := strings.TrimSpace(reLeadingPrefix.ReplaceAllString(s, "")); stripped != "" {
		s = stripped
	}

	// 迭代剥离尾部季号、篇幅后缀（例如“最终季 完结篇 后篇”）
	for i := 0; i < 4; i++ {
		// 处理“前篇 / 后篇 / 上篇 / 下篇 / 前半 / 后半”等独立尾部标识
		for _, tail := range []string{"前篇", "后篇", "後篇", "上篇", "下篇", "前編", "後編", "前半", "后半", "後半"} {
			if strings.HasSuffix(s, tail) && len(s) > len(tail) {
				candidate := strings.TrimSpace(strings.TrimRight(strings.TrimSuffix(s, tail), ":：-–—/ "))
				if candidate != "" {
					s = candidate
				}
			}
		}

		next := strings.TrimSpace(reSeasonSuffix.ReplaceAllString(s, ""))
		next = strings.TrimRight(next, ":：-–—/ ")
		if next == "" || next == s {
			break
		}
		s = next
	}

	return strings.TrimSpace(s)
}

// NormalizeFranchiseName 生成用于系列聚类的归一化匹配键
func NormalizeFranchiseName(title string) string {
	base := ExtractFranchiseTitle(title)
	if base == "" {
		return ""
	}
	return core.NormalizeTitle(base)
}

// ============================================================
// 接口实现与扩展方法
// ============================================================

// SearchAnime 搜索 TV 番剧
func (p *TMDBProvider) SearchAnime(ctx context.Context, title string) ([]core.Anime, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return []core.Anime{}, nil
	}

	apiKey, lang := p.credentials()
	path := fmt.Sprintf("/search/tv?api_key=%s&language=%s&query=%s&page=1",
		url.QueryEscape(apiKey), url.QueryEscape(lang), url.QueryEscape(cleanTitle))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB 搜索返回异常状态码: HTTP %d", resp.StatusCode)
	}

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
			Type:        "TV",
			Description: r.Overview,
			CoverURL:    coverURL,
			TMDBID:      strconv.Itoa(r.ID),
		})
	}
	return animes, nil
}

// SearchMovies 搜索 TMDB 剧场版/电影条目
func (p *TMDBProvider) SearchMovies(ctx context.Context, title string) ([]core.Anime, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return []core.Anime{}, nil
	}

	apiKey, lang := p.credentials()
	path := fmt.Sprintf("/search/movie?api_key=%s&language=%s&query=%s&page=1",
		url.QueryEscape(apiKey), url.QueryEscape(lang), url.QueryEscape(cleanTitle))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 电影搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB 电影搜索返回异常状态码: HTTP %d", resp.StatusCode)
	}

	var result tmdbMovieSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("TMDB 电影响应解析失败: %w", err)
	}

	animes := make([]core.Anime, 0, len(result.Results))
	for _, r := range result.Results {
		year := 0
		if len(r.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(r.ReleaseDate[:4])
		}
		coverURL := ""
		if r.PosterPath != "" {
			coverURL = "https://image.tmdb.org/t/p/w500" + r.PosterPath
		}
		idStr := strconv.Itoa(r.ID)
		animes = append(animes, core.Anime{
			ID:          idStr,
			Provider:    "TMDB",
			TitleCN:     r.Title,
			TitleJP:     r.OriginalTitle,
			Year:        year,
			Type:        "Movie",
			Description: r.Overview,
			CoverURL:    coverURL,
			TMDBID:      "movie/" + idStr,
		})
	}
	return animes, nil
}

// SearchCollections 按系列名搜索 TMDB 合集
func (p *TMDBProvider) SearchCollections(ctx context.Context, query string) ([]TMDBCollection, error) {
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return []TMDBCollection{}, nil
	}

	apiKey, lang := p.credentials()
	path := fmt.Sprintf("/search/collection?api_key=%s&language=%s&query=%s&page=1",
		url.QueryEscape(apiKey), url.QueryEscape(lang), url.QueryEscape(cleanQuery))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 合集搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB 合集搜索返回异常状态码: HTTP %d", resp.StatusCode)
	}

	var result tmdbCollectionSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("TMDB 合集搜索响应解析失败: %w", err)
	}
	return result.Results, nil
}

// GetCollection 获取指定 TMDB 合集详情及其包含的全部作品列表
func (p *TMDBProvider) GetCollection(ctx context.Context, collectionID int) (*TMDBCollection, error) {
	if collectionID <= 0 {
		return nil, fmt.Errorf("TMDB: 无效的合集 ID (%d)", collectionID)
	}

	apiKey, lang := p.credentials()
	path := fmt.Sprintf("/collection/%d?api_key=%s&language=%s",
		collectionID, url.QueryEscape(apiKey), url.QueryEscape(lang))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 获取合集失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("TMDB: 合集未找到 (ID: %d)", collectionID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB 获取合集返回异常状态码: HTTP %d", resp.StatusCode)
	}

	var coll TMDBCollection
	if err := json.NewDecoder(resp.Body).Decode(&coll); err != nil {
		return nil, fmt.Errorf("TMDB 合集响应解析失败: %w", err)
	}
	return &coll, nil
}

// GetAnime 获取番剧或剧场版详情（同时支持 "tv/{id}"、"movie/{id}" 与裸 ID）
func (p *TMDBProvider) GetAnime(ctx context.Context, id string) (core.Anime, error) {
	mediaType, cleanID := ParseTMDBID(id)
	if cleanID == "" {
		return core.Anime{}, fmt.Errorf("TMDB: 番剧 ID 不能为空")
	}

	if mediaType == "movie" {
		detail, err := p.getMovieDetail(ctx, cleanID)
		if err != nil {
			return core.Anime{}, err
		}
		year := 0
		if len(detail.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(detail.ReleaseDate[:4])
		}
		coverURL := ""
		if detail.PosterPath != "" {
			coverURL = "https://image.tmdb.org/t/p/w500" + detail.PosterPath
		}
		seriesID := ""
		if detail.BelongsToCollection != nil && detail.BelongsToCollection.Name != "" {
			seriesID = CleanCollectionName(detail.BelongsToCollection.Name)
		}
		idStr := strconv.Itoa(detail.ID)
		return core.Anime{
			ID:          idStr,
			Provider:    "TMDB",
			TitleCN:     detail.Title,
			TitleJP:     detail.OriginalTitle,
			Year:        year,
			TotalEps:    1,
			Type:        "Movie",
			Description: detail.Overview,
			CoverURL:    coverURL,
			SeriesID:    seriesID,
			TMDBID:      "movie/" + idStr,
			IMDBID:      detail.ExternalIDs.IMDBID,
		}, nil
	}

	detail, err := p.getTVDetail(ctx, cleanID)
	if err != nil {
		return core.Anime{}, err
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

	seriesID := ""
	if detail.BelongsToCollection != nil && detail.BelongsToCollection.Name != "" {
		seriesID = CleanCollectionName(detail.BelongsToCollection.Name)
	}

	idStr := strconv.Itoa(detail.ID)
	return core.Anime{
		ID:          idStr,
		Provider:    "TMDB",
		TitleCN:     detail.Name,
		TitleJP:     detail.OriginalName,
		Year:        year,
		TotalEps:    detail.NumberOfEpisodes,
		Type:        animeType,
		Description: detail.Overview,
		CoverURL:    coverURL,
		SeriesID:    seriesID,
		TMDBID:      idStr,
		IMDBID:      detail.ExternalIDs.IMDBID,
	}, nil
}

func (p *TMDBProvider) getTVDetail(ctx context.Context, cleanID string) (*tmdbTVDetail, error) {
	apiKey, lang := p.credentials()
	path := fmt.Sprintf("/tv/%s?api_key=%s&language=%s&append_to_response=external_ids",
		url.PathEscape(cleanID), url.QueryEscape(apiKey), url.QueryEscape(lang))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 获取 TV 详情失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("TMDB: 番剧未找到 (ID: tv/%s)", cleanID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB 获取 TV 详情返回异常状态码: HTTP %d", resp.StatusCode)
	}

	var detail tmdbTVDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("TMDB TV 响应解析失败: %w", err)
	}
	return &detail, nil
}

func (p *TMDBProvider) getMovieDetail(ctx context.Context, cleanID string) (*tmdbMovieDetail, error) {
	apiKey, lang := p.credentials()
	path := fmt.Sprintf("/movie/%s?api_key=%s&language=%s&append_to_response=external_ids",
		url.PathEscape(cleanID), url.QueryEscape(apiKey), url.QueryEscape(lang))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 获取电影详情失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("TMDB: 电影未找到 (ID: movie/%s)", cleanID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB 获取电影详情返回异常状态码: HTTP %d", resp.StatusCode)
	}

	var detail tmdbMovieDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("TMDB 电影响应解析失败: %w", err)
	}
	return &detail, nil
}

// ResolveSeriesCollection 解析给定 TMDB 条目或标题所属的 TMDB Collection 与系列聚合信息
// 说明：TMDB /tv/{id} 通常不返回 belongs_to_collection，本方法会综合电影 belongs_to_collection、
// /collection/{id} 成员列表、合集检索匹配及归一化系列标题，打通 TV 季与剧场版/OVA 的同系列归属。
func (p *TMDBProvider) ResolveSeriesCollection(ctx context.Context, tmdbID string, title string) (SeriesCollectionInfo, error) {
	mediaType, cleanID := ParseTMDBID(tmdbID)
	info := SeriesCollectionInfo{
		MediaType: mediaType,
	}
	if cleanID != "" {
		info.TMDBID = mediaType + "/" + cleanID
	}

	// 1. 电影条目：优先读取 /movie/{id} 的 belongs_to_collection 与 /collection/{id}
	if mediaType == "movie" && cleanID != "" {
		detail, err := p.getMovieDetail(ctx, cleanID)
		if err != nil {
			return info, err
		}
		candidateTitle := detail.Title
		if candidateTitle == "" {
			candidateTitle = title
		}
		if detail.BelongsToCollection != nil && detail.BelongsToCollection.ID > 0 {
			info.CollectionID = detail.BelongsToCollection.ID
			info.CollectionName = detail.BelongsToCollection.Name
			info.SeriesID = CleanCollectionName(detail.BelongsToCollection.Name)
			if info.SeriesID == "" {
				info.SeriesID = ExtractFranchiseTitle(candidateTitle)
			}
			info.FranchiseKey = NormalizeFranchiseName(info.SeriesID)
			info.Source = "movie_collection"
			if coll, err := p.GetCollection(ctx, detail.BelongsToCollection.ID); err == nil && coll != nil {
				if coll.Name != "" {
					info.CollectionName = coll.Name
					if cleaned := CleanCollectionName(coll.Name); cleaned != "" {
						info.SeriesID = cleaned
						info.FranchiseKey = NormalizeFranchiseName(cleaned)
					}
				}
				info.PartTMDBIDs = extractCollectionPartIDs(coll)
			}
			return info, nil
		}
		// 电影未挂靠合集时，尝试使用规范化主标题继续检索同名合集
		if candidateTitle != "" {
			p.enrichByCollectionSearch(ctx, candidateTitle, &info)
			if info.SeriesID == "" {
				info.SeriesID = ExtractFranchiseTitle(candidateTitle)
				info.FranchiseKey = NormalizeFranchiseName(candidateTitle)
				info.Source = "normalized_title"
			}
		}
		return info, nil
	}

	// 2. TV 条目：读取 /tv/{id} 详情，若无 belongs_to_collection 则用系列主标题反查 TMDB 合集
	if cleanID != "" {
		detail, err := p.getTVDetail(ctx, cleanID)
		if err != nil {
			return info, err
		}
		candidateTitle := detail.Name
		if candidateTitle == "" {
			candidateTitle = title
		}
		if candidateTitle == "" {
			candidateTitle = detail.OriginalName
		}

		if detail.BelongsToCollection != nil && detail.BelongsToCollection.ID > 0 {
			info.CollectionID = detail.BelongsToCollection.ID
			info.CollectionName = detail.BelongsToCollection.Name
			info.SeriesID = CleanCollectionName(detail.BelongsToCollection.Name)
			info.FranchiseKey = NormalizeFranchiseName(info.SeriesID)
			info.Source = "tv_collection"
			if coll, err := p.GetCollection(ctx, detail.BelongsToCollection.ID); err == nil && coll != nil {
				info.PartTMDBIDs = extractCollectionPartIDs(coll)
			}
			return info, nil
		}

		if candidateTitle != "" {
			p.enrichByCollectionSearch(ctx, candidateTitle, &info)
			if info.CollectionID > 0 {
				return info, nil
			}
			info.SeriesID = ExtractFranchiseTitle(candidateTitle)
			info.FranchiseKey = NormalizeFranchiseName(candidateTitle)
			info.Source = "tv_detail"
		}
		return info, nil
	}

	// 3. 无 TMDB ID：仅凭标题进行合集搜索与系列标题归一化
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle != "" {
		p.enrichByCollectionSearch(ctx, cleanTitle, &info)
		if info.SeriesID == "" {
			info.SeriesID = ExtractFranchiseTitle(cleanTitle)
			info.FranchiseKey = NormalizeFranchiseName(cleanTitle)
			info.Source = "normalized_title"
		}
	}
	return info, nil
}

func (p *TMDBProvider) enrichByCollectionSearch(ctx context.Context, rawTitle string, info *SeriesCollectionInfo) {
	queryTitle := ExtractFranchiseTitle(rawTitle)
	targetKey := NormalizeFranchiseName(rawTitle)
	if queryTitle == "" || targetKey == "" {
		return
	}
	collections, err := p.SearchCollections(ctx, queryTitle)
	if err != nil || len(collections) == 0 {
		return
	}
	for _, c := range collections {
		if NormalizeFranchiseName(c.Name) == targetKey {
			info.CollectionID = c.ID
			info.CollectionName = c.Name
			info.SeriesID = CleanCollectionName(c.Name)
			if info.SeriesID == "" {
				info.SeriesID = queryTitle
			}
			info.FranchiseKey = targetKey
			info.Source = "collection_search"
			if coll, err := p.GetCollection(ctx, c.ID); err == nil && coll != nil {
				info.PartTMDBIDs = extractCollectionPartIDs(coll)
			}
			return
		}
	}
}

func extractCollectionPartIDs(coll *TMDBCollection) []string {
	if coll == nil || len(coll.Parts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(coll.Parts))
	for _, part := range coll.Parts {
		if part.ID <= 0 {
			continue
		}
		mt := strings.ToLower(strings.TrimSpace(part.MediaType))
		if mt != "tv" {
			mt = "movie"
		}
		ids = append(ids, fmt.Sprintf("%s/%d", mt, part.ID))
	}
	return ids
}

// GetEpisodes 获取指定季的集数列表
func (p *TMDBProvider) GetEpisodes(ctx context.Context, animeID string, season int) ([]core.Episode, error) {
	mediaType, cleanID := ParseTMDBID(animeID)
	if cleanID == "" {
		return nil, fmt.Errorf("TMDB: 番剧 ID 不能为空")
	}
	if mediaType == "movie" {
		return []core.Episode{}, nil
	}

	apiKey, lang := p.credentials()
	path := fmt.Sprintf("/tv/%s/season/%d?api_key=%s&language=%s",
		url.PathEscape(cleanID), season, url.QueryEscape(apiKey), url.QueryEscape(lang))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("TMDB 获取剧集失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("TMDB: 季未找到 (ID: %s, Season: %d)", cleanID, season)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB 获取剧集返回异常状态码: HTTP %d", resp.StatusCode)
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
			AnimeID: cleanID,
			Season:  season,
			Number:  float32(ep.EpisodeNumber),
			Title:   ep.Name,
			AiredAt: airedAt,
		})
	}
	return episodes, nil
}

// GetExternalIDs 获取关联的外部 ID (如 IMDB)，同时支持 tv/{id}、movie/{id} 与裸 ID
func (p *TMDBProvider) GetExternalIDs(ctx context.Context, tmdbID string) (imdbID string, err error) {
	mediaType, cleanID := ParseTMDBID(tmdbID)
	if cleanID == "" {
		return "", fmt.Errorf("TMDB: ID 不能为空")
	}

	apiKey, _ := p.credentials()
	path := fmt.Sprintf("/%s/%s/external_ids?api_key=%s",
		mediaType, url.PathEscape(cleanID), url.QueryEscape(apiKey))
	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("TMDB: 外部 ID 未找到 (%s/%s)", mediaType, cleanID)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TMDB 获取外部 ID 返回异常状态码: HTTP %d", resp.StatusCode)
	}

	var result struct {
		IMDBID string `json:"imdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.IMDBID, nil
}

// GetCollections TMDB 暂不支持同步个人收藏
func (p *TMDBProvider) GetCollections(ctx context.Context, username string) ([]core.Anime, error) {
	return nil, fmt.Errorf("TMDB provider does not support GetCollections")
}

// ============================================================
// 镜像重试与并发安全工具
// ============================================================

func (p *TMDBProvider) credentials() (apiKey, language string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.apiKey, p.language
}

// tryMirrors 依次尝试镜像域名发起 GET 请求，返回首个成功或 404 的响应
func (p *TMDBProvider) tryMirrors(ctx context.Context, path string) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.RLock()
	active := p.baseURL
	mirrors := append([]string(nil), p.mirrorDomains...)
	p.mu.RUnlock()

	domains := make([]string, 0, 1+len(mirrors))
	if active != "" {
		domains = append(domains, active)
	}
	for _, d := range mirrors {
		if d != active {
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("未配置可用的 TMDB 镜像域名")
	}

	var lastErr error
	for _, baseURL := range domains {
		fullURL := buildMirrorURL(baseURL, path)
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
			if resp.StatusCode == http.StatusOK && baseURL != active {
				p.mu.Lock()
				p.baseURL = baseURL
				p.mu.Unlock()
			}
			return resp, nil
		}
		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return nil, fmt.Errorf("TMDB API Key 无效 (401)")
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无可用响应")
	}
	return nil, fmt.Errorf("所有 TMDB 镜像均不可达: %w", lastErr)
}
