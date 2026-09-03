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
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
)

// BGMTVProvider 实现 core.MetadataProvider 接口
// 使用 BGM.tv API 获取番剧元数据
type BGMTVProvider struct {
	httpClient    *http.Client
	userToken     string
	mirrorDomains []string // API 域名列表: api.bgm.tv, api.bangumi.tv, api.chii.in
	activeDomain  string
}

// NewBGMTVProvider 创建 BGM.tv 元数据提供者
func NewBGMTVProvider(userToken string, mirrorDomains []string) *BGMTVProvider {
	active := "api.bgm.tv"
	if len(mirrorDomains) > 0 {
		active = mirrorDomains[0]
	}
	return &BGMTVProvider{
		httpClient:    httpx.New(30 * time.Second),
		userToken:     userToken,
		mirrorDomains: mirrorDomains,
		activeDomain:  active,
	}
}

func (p *BGMTVProvider) Name() string { return "BGM.tv" }





// LatencyResult 镜像延迟结果
type LatencyResult struct {
	Domain    string `json:"domain"`
	LatencyMS int64  `json:"latency_ms"`
	OK        bool   `json:"ok"`
}

// TestLatency 测试所有 BGM 镜像延迟
func (p *BGMTVProvider) TestLatency(ctx context.Context) []LatencyResult {
	results := make([]LatencyResult, 0, len(p.mirrorDomains))
	for _, domain := range p.mirrorDomains {
		start := time.Now()
		fullURL := fmt.Sprintf("https://%s/v0/subjects/1", domain)
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
	path := fmt.Sprintf("/search/subject/%s?type=2&responseGroup=small",
		url.QueryEscape(title))

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("BGM 搜索失败: %w", err)
	}
	defer resp.Body.Close()

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
	path := fmt.Sprintf("/v0/subjects/%s", id)

	resp, err := p.tryMirrors(ctx, path)
	if err != nil {
		return core.Anime{}, fmt.Errorf("BGM 获取详情失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return core.Anime{}, fmt.Errorf("BGM: 条目未找到 (ID: %s)", id)
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
	}, nil
}

// GetEpisodes 获取番剧集数列表
// 注意：BGM.tv 没有多季概念，每季是独立 subject
func (p *BGMTVProvider) GetEpisodes(ctx context.Context, animeID string, season int) ([]core.Episode, error) {
	var allEpisodes []core.Episode
	offset := 0
	limit := 100

	for {
		path := fmt.Sprintf("/v0/episodes?subject_id=%s&type=0&limit=%d&offset=%d",
			animeID, limit, offset)

		resp, err := p.tryMirrors(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("BGM 获取剧集失败: %w", err)
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
				AnimeID: animeID,
				Season:  season,
				Number:  float32(ep.Ep),
				Title:   epTitle,
				AiredAt: airedAt,
			})
		}

		if offset+limit >= epResp.Total {
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

// GetCalendar 获取 Bangumi 每日放送时间表
func (p *BGMTVProvider) GetCalendar(ctx context.Context) ([]core.TorrentItem, error) {
	resp, err := p.tryMirrors(ctx, "/calendar")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var calendar []bgmCalendarItem
	if err := json.NewDecoder(resp.Body).Decode(&calendar); err != nil {
		return nil, err
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
				// 将星期信息暂存在 EpisodeURL 字段（Hack，稍后在 handler 转换）
				EpisodeURL: strconv.Itoa(day.Weekday.ID),
			})
		}
	}
	return results, nil
}

// GetCollections 获取用户的 Bangumi 收藏（想看/在看）
func (p *BGMTVProvider) GetCollections(ctx context.Context, username string) ([]core.Anime, error) {
	if username == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}

	// 状态码：1 = 想看, 3 = 在看
	statuses := []int{1, 3}
	var allAnime []core.Anime

	for _, status := range statuses {
		offset := 0
		limit := 30
		for {
			path := fmt.Sprintf("/v0/users/%s/collections?type=%d&limit=%d&offset=%d",
				username, status, limit, offset)

			resp, err := p.tryMirrors(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("获取收藏失败 (status=%d): %w", status, err)
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
			resp.Body.Close()

			if offset+limit >= collResp.Total || len(collResp.Data) == 0 {
				break
			}
			offset += limit
		}
	}

	return allAnime, nil
}


func (p *BGMTVProvider) tryMirrors(ctx context.Context, path string) (*http.Response, error) {
	domains := make([]string, 0, 1+len(p.mirrorDomains))
	if p.activeDomain != "" {
		domains = append(domains, p.activeDomain)
	}
	for _, d := range p.mirrorDomains {
		if d != p.activeDomain {
			domains = append(domains, d)
		}
	}

	var lastErr error
	for _, domain := range domains {
		fullURL := fmt.Sprintf("https://%s%s", domain, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Ani-Go/1.0")
		if p.userToken != "" {
			req.Header.Set("Authorization", "Bearer "+p.userToken)
		}

		resp, err := p.httpClient.Do(req)
		if err != nil {
			log.Printf("⚠️ BGM 镜像不可达 [%s]: %v", domain, err)
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
			return resp, nil
		}
		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			return nil, fmt.Errorf("BGM UserToken 无效 (401)")
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("所有 BGM 镜像均不可达: %w", lastErr)
}
