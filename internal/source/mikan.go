// Package source 实现各资源站点的 Source 接口
// Mikan RSS 解析器负责解析 Mikan 个人 RSS 订阅源
package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// ============================================================
// Mikan RSS XML 结构体（RSS 2.0 标准格式）
// ============================================================

type mikanRSS struct {
	XMLName xml.Name   `xml:"rss"`
	Channel mikanChannel `xml:"channel"`
}

type mikanChannel struct {
	Title string      `xml:"title"`
	Link  string      `xml:"link"`
	Items []mikanItem `xml:"item"`
}

type mikanItem struct {
	Title       string       `xml:"title"`
	Link        string       `xml:"link"`
	GUID        string       `xml:"guid"`
	PubDate     string       `xml:"pubDate"`
	Description string       `xml:"description"`
	Enclosure   mikanEnclosure `xml:"enclosure"`
}

type mikanEnclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length int64  `xml:"length,attr"`
}

// ============================================================
// MikanSource 实现 core.Source 接口
// ============================================================

type MikanSource struct {
	httpClient  *http.Client
	domain      string
	proxyDomain string
}

// NewMikanSource 创建新的 Mikan 资源源
func NewMikanSource(domain, proxyDomain string) *MikanSource {
	return &MikanSource{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		domain:     domain,
		proxyDomain: proxyDomain,
	}
}

func (m *MikanSource) Name() string { return "Mikan" }

// FetchRSS 解析 Mikan 个人 RSS 订阅源
func (m *MikanSource) FetchRSS(ctx context.Context, url string) ([]core.TorrentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 RSS 请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Ani-Go/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取 RSS 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RSS 请求返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 RSS 响应失败: %w", err)
	}

	var rss mikanRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("解析 RSS XML 失败: %w", err)
	}

	items := make([]core.TorrentItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		pubAt, _ := parsePubDate(item.PubDate)
		info := ParseMikanTitle(item.Title)

		items = append(items, core.TorrentItem{
			Title:       item.Title,
			URL:         item.Enclosure.URL,
			MagnetURL:   "",
			InfoHash:    info.InfoHash,
			Size:        item.Enclosure.Length,
			PublishedAt: pubAt,
			SourceName:  "Mikan",
		})
	}

	return items, nil
}

// SearchAnime 在 Mikan 上搜索番剧
func (m *MikanSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	// Mikan 搜索页面的 RSS 格式
	searchURL := fmt.Sprintf("https://%s/Home/Search?searchstr=%s", m.domain, title)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建搜索请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Ani-Go/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索请求返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应失败: %w", err)
	}

	return parseMikanSearchHTML(string(body), m.domain), nil
}

// FetchHistory 爬取 Mikan 番剧详情页获取全量历史种子
func (m *MikanSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	// Mikan 番剧详情页
	detailURL := fmt.Sprintf("https://%s/Home/Bangumi/%s", m.domain, bangumiID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建详情页请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Ani-Go/1.0")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取详情页失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("详情页返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取详情页响应失败: %w", err)
	}

	return parseMikanDetailHTML(string(body), filter, m.domain), nil
}

func (m *MikanSource) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+m.domain, nil)
	if err != nil {
		return false
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ============================================================
// 标题解析器 — 从 Mikan 种子标题中提取元数据
// ============================================================

// TitleInfo 保存从种子标题中解析出的元数据
type TitleInfo struct {
	Title    string // 番剧名（不含字幕组、集数、分辨率）
	RawTitle string // 原始标题
	Subgroup string // 字幕组名称
	Episode  float32 // 集数（0 表示未识别）
	Season   int     // 季数（默认 1）
	InfoHash string // info hash（如有）
	Resolution string // 分辨率，如 "1080p"
}

// 用于解析 Mikan 标题的正则表式集合
var (
	// 提取【】[] 中的字幕组名称
	reSubgroup = regexp.MustCompile(`^[【\[［]([^】\]］]+)[】\]］]\s*`)
	// 集数模式
	reEpisodePatterns = []*regexp.Regexp{
		regexp.MustCompile(`[-\s](\d{1,3})(?:\s*\.\s*5)?(?:\s*(?:END|end|Fin|fin))?$`),                         // " - 01" 或 " 01"
		regexp.MustCompile(`第(\d{1,3})(?:\.5)?[話话集]`),                                                          // "第01話"
		regexp.MustCompile(`[Ee][Pp]?(\d{1,3})(?:\.5)?`),                                                         // "E01" 或 "EP01"
		regexp.MustCompile(`#(\d{1,3})(?:\.5)?`),                                                                  // "#01"
		regexp.MustCompile(`[Ss](?:eason)?\s*(\d{1,2})\s*[Ee](?:p(?:isode)?)?\s*(\d{1,3})`),                      // "S01E03"
	}
	// 分辨率
	reResolution = regexp.MustCompile(`(?i)(\d{3,4}p)`)
	// 季数（从标题中提取，支持下中文数字和阿拉伯数字）
	reSeasonTitle      = regexp.MustCompile(`第([\d一二三四五六七八九十]{1,3})季`)
	// 清理末尾的纯数字（误匹配的集数）
	reTrailingDigits = regexp.MustCompile(`\s+\d{1,3}$`)
)

// ParseMikanTitle 从 Mikan 种子标题中提取结构化信息
func ParseMikanTitle(rawTitle string) TitleInfo {
	info := TitleInfo{
		RawTitle: rawTitle,
		Season:   1,
	}

	title := strings.TrimSpace(rawTitle)

	// 提取字幕组
	if m := reSubgroup.FindStringSubmatch(title); m != nil {
		info.Subgroup = strings.TrimSpace(m[1])
		title = strings.TrimSpace(reSubgroup.ReplaceAllString(title, ""))
	}

	// 提取分辨率
	if m := reResolution.FindStringSubmatch(title); m != nil {
		info.Resolution = m[1]
	}

	// 提取 SxxExx 式
	if m := reEpisodePatterns[4].FindStringSubmatch(title); m != nil {
		season, _ := strconv.Atoi(m[1])
		ep, _ := strconv.ParseFloat(m[2], 32)
		info.Season = season
		info.Episode = float32(ep)
		title = reEpisodePatterns[4].ReplaceAllString(title, "")
	}

	// 如果还没找到集数，尝试其他模式
	if info.Episode == 0 {
		for _, re := range reEpisodePatterns[:4] {
			if m := re.FindStringSubmatch(title); m != nil {
				ep, _ := strconv.ParseFloat(m[1], 32)
				info.Episode = float32(ep)
				// 检测 .5 集数（如 EP12.5）
				if strings.Contains(m[0], ".5") {
					info.Episode += 0.5
				}
				title = re.ReplaceAllString(title, "")
				break
			}
		}
	}

	// 提取季数（从标题如 "第二季"）
	if m := reSeasonTitle.FindStringSubmatch(title); m != nil {
		info.Season = parseCNNumber(m[1])
		title = reSeasonTitle.ReplaceAllString(title, "")
	}

	// 清理杂项标记
	title = reResolution.ReplaceAllString(title, "")
	title = strings.TrimSpace(title)
	title = strings.TrimRight(title, "- _[(（/")
	title = strings.TrimSpace(title)

	// 移除末尾可能残留的纯数字
	if reTrailingDigits.MatchString(title) {
		title = reTrailingDigits.ReplaceAllString(title, "")
	}

	info.Title = title
	return info
}

// ============================================================
// 辅助函数
// ============================================================

// parseCNNumber 将中文数字字符串转为阿拉伯数字
func parseCNNumber(s string) int {
	// 先尝试直接解析阿拉伯数字
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	cnMap := map[rune]int{
		'一': 1, '二': 2, '三': 3, '四': 4, '五': 5,
		'六': 6, '七': 7, '八': 8, '九': 9, '十': 10,
	}
	// 处理 "十一" ~ "十九"
	if len([]rune(s)) == 2 {
		runes := []rune(s)
		if runes[0] == '十' {
			return 10 + cnMap[runes[1]]
		}
		if runes[1] == '十' {
			return cnMap[runes[0]] * 10
		}
	}
	// 处理 "二十" ~ "九十九"
	if len([]rune(s)) == 3 {
		runes := []rune(s)
		if runes[1] == '十' {
			return cnMap[runes[0]]*10 + cnMap[runes[2]]
		}
	}
	// 单个中文数字
	if n, ok := cnMap[[]rune(s)[0]]; ok {
		return n
	}
	return 1
}

// parsePubDate 解析 RSS 中的 pubDate 字段
func parsePubDate(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	// 尝试多种日期格式
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Now(), nil
}

// parseMikanSearchHTML 从 Mikan 搜索结果 HTML 中提取种子列表（占位实现）
func parseMikanSearchHTML(_html, _domain string) []core.TorrentItem {
	// TODO: 使用 goquery 解析 HTML 搜索结果
	return nil
}

// parseMikanDetailHTML 从 Mikan 番剧详情页 HTML 中提取全量种子（占位实现）
func parseMikanDetailHTML(_html string, _filter core.Filter, _domain string) []core.TorrentItem {
	// TODO: 使用 goquery 解析 HTML 详情页
	return nil
}

// BuildMikanRSSURL 构建 Mikan 个人 RSS 完整 URL
func BuildMikanRSSURL(tokenURL string) string {
	return strings.TrimSpace(tokenURL)
}
