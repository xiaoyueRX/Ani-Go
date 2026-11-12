// BGM.tv 元数据提供者
// 使用 BGM.tv API 获取番剧元数据，支持多域名镜像
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
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// BGMTVProvider 实现 core.MetadataProvider 接口
// 使用 BGM.tv API 获取番剧元数据
type BGMTVProvider struct {
	mu            sync.RWMutex // 保护认证令牌、镜像列表和当前活跃域名
	httpClient    *http.Client
	userToken     string
	mirrorDomains []string // API 域名列表: api.bgm.tv, api.bangumi.tv, api.chii.in
	activeDomain  string
	calMu         sync.RWMutex
	calCache      []core.TorrentItem
	calTime       time.Time
}

// NewBGMTVProvider 创建 BGM.tv 元数据提供者
func NewBGMTVProvider(userToken string, mirrorDomains []string) *BGMTVProvider {
	cleaned := cleanDomains(mirrorDomains)
	if len(cleaned) == 0 {
		cleaned = []string{"api.bgm.tv", "api.bangumi.tv", "api.chii.in"}
	}
	return &BGMTVProvider{
		httpClient:    httpx.New(30 * time.Second),
		userToken:     strings.TrimSpace(userToken),
		mirrorDomains: cleaned,
		activeDomain:  cleaned[0],
	}
}

func cleanDomains(domains []string) []string {
	out := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		d = strings.TrimRight(strings.TrimSpace(d), "/")
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

func (p *BGMTVProvider) Name() string { return "BGM.tv" }

// SetUserToken 更新认证令牌
func (p *BGMTVProvider) SetUserToken(token string) {
	p.mu.Lock()
	p.userToken = strings.TrimSpace(token)
	p.mu.Unlock()
}

// LatencyResult 镜像延迟结果
type LatencyResult struct {
	Domain    string `json:"domain"`
	LatencyMS int64  `json:"latency_ms"`
	OK        bool   `json:"ok"`
}

func buildMirrorURL(domain, path string) string {
	base := strings.TrimRight(strings.TrimSpace(domain), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	return base + path
}

// TestLatency 测试所有 BGM 镜像延迟
func (p *BGMTVProvider) TestLatency(ctx context.Context) []LatencyResult {
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.RLock()
	domains := append([]string(nil), p.mirrorDomains...)
	p.mu.RUnlock()

	results := make([]LatencyResult, 0, len(domains))
	for _, domain := range domains {
		start := time.Now()
		fullURL := buildMirrorURL(domain, "/v0/subjects/1")
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			results = append(results, LatencyResult{
				Domain:    domain,
				LatencyMS: 0,
				OK:        false,
			})
			continue
		}
		req.Header.Set("User-Agent", "Ani-Go/1.0")

		resp, err := p.httpClient.Do(req)
		latency := time.Since(start).Milliseconds()

		ok := err == nil && resp != nil && resp.StatusCode == http.StatusOK
		if resp != nil {
			resp.Body.Close()
		}

		results = append(results, LatencyResult{
			Domain:    domain,
			LatencyMS: latency,
			OK:        ok,
		})
	}
	return results
}

// ============================================================

type bgmCalendarItem struct {
	Weekday struct {
		ID int `json:"id"`
	} `json:"weekday"`
	Items []struct {
		ID int `json:"id"`
		Name string `json:"name"`
		NameCN string `json:"name_cn"`
		Images struct {
			Large string `json:"large"`
		} `json:"images"`
		AirDate string `json:"air_date"`
	} `json:"items"`
}

// BGM.tv JSON 响应结构体
// ============================================================

type bgmSearchResponse struct {
	Results int            `json:"results"`
	List    []bgmSearchItem `json:"list"`
}

type bgmSearchItem struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	NameCN  string `json:"name_cn"`
	Summary string `json:"summary"`
	Images  struct {
		Large string `json:"large"`
	} `json:"images"`
	AirDate string `json:"air_date"`
}

type bgmSubject struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	NameCN         string `json:"name_cn"`
	Summary        string `json:"summary"`
	Images         struct {
		Large string `json:"large"`
	} `json:"images"`
	TotalEpisodes int    `json:"total_episodes"`
	Eps           int    `json:"eps"`
	Date          string `json:"date"`
	Type          int    `json:"type"`
}

type bgmSubjectRelation struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	NameCN   string `json:"name_cn"`
	Relation string `json:"relation"`
}

type bgmEpisodeResponse struct {
	Total int         `json:"total"`
	Data  []bgmEpisode `json:"data"`
}

type bgmEpisode struct {
	ID       int    `json:"id"`
	Ep       int    `json:"ep"`
	Name     string `json:"name"`
	NameCN   string `json:"name_cn"`
	Airdate  string `json:"airdate"`
	Duration string `json:"duration"`
	Status   string `json:"status"`
}

// ============================================================
// 接口实现
// ============================================================

// SearchAnime 搜索番剧（使用旧版搜索接口，响应更简洁）
func (p *BGMTVProvider) SearchAnime(ctx context.Context, title string) ([]core.Anime, error) {
	if strings.TrimSpace(title) == "" {
		return []core.Anime{}, nil
	}
	path := fmt.Sprintf("/search/subject/%s?type=2&responseGroup=small",
		url.PathEscape(strings.TrimSpace(title)))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("BGM 搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BGM 搜索返回异常状态码: HTTP %d", resp.StatusCode)
	}

	var result bgmSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("BGM 响应解析失败: %w", err)
	}

	animes := make([]core.Anime, 0, len(result.List))
	for _, item := range result.List {
		year := 0
		if len(item.AirDate) >= 4 {
			year, _ = strconv.Atoi(item.AirDate[:4])
		}
		titleCN := item.NameCN
		if titleCN == "" {
			titleCN = item.Name
		}
		animes = append(animes, core.Anime{
			ID:          strconv.Itoa(item.ID),
			Provider:    "BGM.tv",
			TitleCN:     titleCN,
			TitleJP:     item.Name,
			Year:        year,
			Description: item.Summary,
			CoverURL:    item.Images.Large,
		})
	}
	return animes, nil
}

// GetAnime 获取番剧详情
func (p *BGMTVProvider) GetAnime(ctx context.Context, id string) (core.Anime, error) {
	cleanID := strings.TrimSpace(id)
	if cleanID == "" {
		return core.Anime{}, fmt.Errorf("BGM: 番剧 ID 不能为空")
	}
	escapedID := url.PathEscape(cleanID)
	path := fmt.Sprintf("/v0/subjects/%s", escapedID)

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return core.Anime{}, fmt.Errorf("BGM 获取详情失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return core.Anime{}, fmt.Errorf("BGM: 条目未找到 (ID: %s)", cleanID)
	}
	if resp.StatusCode != http.StatusOK {
		return core.Anime{}, fmt.Errorf("BGM 获取详情失败: HTTP %d", resp.StatusCode)
	}

	var subject bgmSubject
	if err := json.NewDecoder(resp.Body).Decode(&subject); err != nil {
		return core.Anime{}, fmt.Errorf("BGM 响应解析失败: %w", err)
	}

	year := 0
	if len(subject.Date) >= 4 {
		year, _ = strconv.Atoi(subject.Date[:4])
	}
	totalEps := subject.TotalEpisodes
	if totalEps == 0 {
		totalEps = subject.Eps
	}

	titleCN := subject.NameCN
	if titleCN == "" {
		titleCN = subject.Name
	}

	typeName := "TV"
	if subject.Type == 1 {
		typeName = "OVA"
	} else if subject.Type == 3 {
		typeName = "Movie"
	}

	seriesID := ""
	relPath := fmt.Sprintf("/v0/subjects/%s/subjects", escapedID)
	if relResp, err := p.tryMirrors(ctx, relPath); err == nil {
		if relResp.StatusCode == http.StatusOK {
			var relations []bgmSubjectRelation
			if json.NewDecoder(relResp.Body).Decode(&relations) == nil {
				for _, rel := range relations {
					if rel.Relation == "前传" || rel.Relation == "主线故事" {
						seriesID = strconv.Itoa(rel.ID)
						break
					}
				}
			}
		}
		relResp.Body.Close()
	}

	return core.Anime{
		ID:          strconv.Itoa(subject.ID),
		Provider:    "BGM.tv",
		TitleCN:     titleCN,
		TitleJP:     subject.Name,
		Year:        year,
		TotalEps:    totalEps,
		Type:        typeName,
		Description: subject.Summary,
		CoverURL:    subject.Images.Large,
		SeriesID:    seriesID,
	}, nil
}

// GetEpisodes 获取番剧集数列表
// 注意：BGM.tv 没有多季概念，每季是独立 subject
func (p *BGMTVProvider) GetEpisodes(ctx context.Context, animeID string, season int) ([]core.Episode, error) {
	cleanID := strings.TrimSpace(animeID)
	if cleanID == "" {
		return nil, fmt.Errorf("BGM: 番剧 ID 不能为空")
	}

	var allEpisodes []core.Episode
	offset := 0
	limit := 100

	for {
		path := fmt.Sprintf("/v0/episodes?subject_id=%s&type=0&limit=%d&offset=%d",
			url.QueryEscape(cleanID), limit, offset)

		resp, err := p.tryMirrors(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("BGM 获取剧集失败: %w", err)
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return nil, fmt.Errorf("BGM: 剧集条目未找到 (ID: %s)", cleanID)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("BGM 获取剧集失败: HTTP %d", resp.StatusCode)
		}

		var epResp bgmEpisodeResponse
		if err := json.NewDecoder(resp.Body).Decode(&epResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("BGM 响应解析失败: %w", err)
		}
		resp.Body.Close()

		for _, ep := range epResp.Data {
			airedAt := time.Time{}
			if ep.Airdate != "" {
				airedAt, _ = time.Parse("2006-01-02", ep.Airdate)
			}
			epTitle := ep.NameCN
			if epTitle == "" {
				epTitle = ep.Name
			}
			allEpisodes = append(allEpisodes, core.Episode{
				AnimeID: cleanID,
				Season:  season,
				Number:  float32(ep.Ep),
				Title:   epTitle,
				AiredAt: airedAt,
			})
		}

		if offset+limit >= epResp.Total || len(epResp.Data) == 0 {
			break
		}
		offset += limit
	}

	return allEpisodes, nil
}

// ============================================================
// 镜像重试
// ============================================================

// tryMirrors 依次尝试镜像域名发起 GET 请求，返回首个成功的响应

// GetCalendar 获取 Bangumi 每日放送时间表（带内存缓存与故障回退）
func (p *BGMTVProvider) GetCalendar(ctx context.Context) ([]core.TorrentItem, error) {
	p.calMu.RLock()
	if time.Since(p.calTime) < 1*time.Hour && len(p.calCache) > 0 {
		cached := make([]core.TorrentItem, len(p.calCache))
		copy(cached, p.calCache)
		p.calMu.RUnlock()
		return cached, nil
	}
	p.calMu.RUnlock()

	fallbackCache := func(cause error) ([]core.TorrentItem, error) {
		p.calMu.RLock()
		defer p.calMu.RUnlock()
		if len(p.calCache) > 0 {
			log.Printf("⚠️ 获取 Bangumi 每日放送失败 (%v)，回退至过往缓存数据", cause)
			cached := make([]core.TorrentItem, len(p.calCache))
			copy(cached, p.calCache)
			return cached, nil
		}
		return nil, cause
	}

	resp, err := p.tryMirrors(ctx, "/calendar")
	if err != nil {
		return fallbackCache(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallbackCache(fmt.Errorf("BGM 日历接口返回异常状态码: HTTP %d", resp.StatusCode))
	}

	var calendar []bgmCalendarItem
	if err := json.NewDecoder(resp.Body).Decode(&calendar); err != nil {
		return fallbackCache(err)
	}

	var results []core.TorrentItem
	for _, day := range calendar {
		for _, item := range day.Items {
			title := item.NameCN
			if title == "" {
				title = item.Name
			}
			results = append(results, core.TorrentItem{
				Title:      title,
				BangumiID:  strconv.Itoa(item.ID),
				CoverURL:   item.Images.Large,
				AiredDate:  item.AirDate,
				SourceName: "BGM.tv",
				// 将星期信息暂存在 EpisodeURL 字段（稍后在 handler 转换）
				EpisodeURL: strconv.Itoa(day.Weekday.ID),
			})
		}
	}

	p.calMu.Lock()
	p.calCache = make([]core.TorrentItem, len(results))
	copy(p.calCache, results)
	p.calTime = time.Now()
	p.calMu.Unlock()

	return results, nil
}

// GetCollections 获取用户的 Bangumi 收藏（想看/在看）
func (p *BGMTVProvider) GetCollections(ctx context.Context, username string) ([]core.Anime, error) {
	cleanUser := strings.TrimSpace(username)
	if cleanUser == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}
	escapedUser := url.PathEscape(cleanUser)

	// 状态码：1 = 想看, 3 = 在看
	statuses := []int{1, 3}
	var allAnime []core.Anime

	for _, status := range statuses {
		offset := 0
		limit := 30
		for {
			path := fmt.Sprintf("/v0/users/%s/collections?type=%d&limit=%d&offset=%d",
				escapedUser, status, limit, offset)

			resp, err := p.tryMirrors(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("获取收藏失败 (status=%d): %w", status, err)
			}

			if resp.StatusCode == http.StatusNotFound {
				resp.Body.Close()
				return nil, fmt.Errorf("BGM: 用户不存在或无权访问 (user=%s)", cleanUser)
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, fmt.Errorf("获取收藏失败 (status=%d): HTTP %d", status, resp.StatusCode)
			}

			var collResp struct {
				Total  int `json:"total"`
				Limit  int `json:"limit"`
				Offset int `json:"offset"`
				Data   []struct {
					SubjectID int `json:"subject_id"`
					Subject   struct {
						ID            int    `json:"id"`
						Name          string `json:"name"`
						NameCN        string `json:"name_cn"`
						Summary       string `json:"summary"`
						Images        struct {
							Large string `json:"large"`
						} `json:"images"`
						Date          string `json:"date"`
						Eps           int    `json:"eps"`
						TotalEpisodes int    `json:"total_episodes"`
					} `json:"subject"`
				} `json:"data"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&collResp); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("解析收藏失败: %w", err)
			}
			resp.Body.Close()

			for _, item := range collResp.Data {
				s := item.Subject
				year := 0
				if len(s.Date) >= 4 {
					year, _ = strconv.Atoi(s.Date[:4])
				}
				titleCN := s.NameCN
				if titleCN == "" {
					titleCN = s.Name
				}
				totalEps := s.TotalEpisodes
				if totalEps == 0 {
					totalEps = s.Eps
				}

				allAnime = append(allAnime, core.Anime{
					ID:          strconv.Itoa(s.ID),
					Provider:    "BGM.tv",
					TitleCN:     titleCN,
					TitleJP:     s.Name,
					Year:        year,
					TotalEps:    totalEps,
					Description: s.Summary,
					CoverURL:    s.Images.Large,
				})
			}

			if offset+limit >= collResp.Total || len(collResp.Data) == 0 {
				break
			}
			offset += limit
		}
	}

	return allAnime, nil
}

// tryMirrors 依次尝试镜像域名发起 GET 请求，返回首个成功或 404 的响应
func (p *BGMTVProvider) tryMirrors(ctx context.Context, path string) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.RLock()
	active := p.activeDomain
	mirrors := append([]string(nil), p.mirrorDomains...)
	userToken := p.userToken
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
		return nil, fmt.Errorf("未配置可用的 BGM 镜像域名")
	}

	var lastErr error
	for _, domain := range domains {
		fullURL := buildMirrorURL(domain, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Ani-Go/1.0")
		if userToken != "" {
			req.Header.Set("Authorization", "Bearer "+userToken)
		}

		resp, err := p.httpClient.Do(req)
		if err != nil {
			log.Printf("⚠️ BGM 镜像不可达 [%s]: %v", domain, err)
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
			if resp.StatusCode == http.StatusOK && domain != active {
				p.mu.Lock()
				p.activeDomain = domain
				p.mu.Unlock()
			}
			return resp, nil
		}
		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return nil, fmt.Errorf("BGM UserToken 无效 (401)")
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无可用响应")
	}
	return nil, fmt.Errorf("所有 BGM 镜像均不可达: %w", lastErr)
}
