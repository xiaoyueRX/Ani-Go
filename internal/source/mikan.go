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
	Title      string  // 番剧名（不含字幕组、集数、分辨率）
	RawTitle   string  // 原始标题
	Subgroup   string  // 字幕组名称
	Episode    float32 // 集数（0 表示未识别）
	Season     int     // 季数（默认 1）
	InfoHash   string  // info hash（如有）
	Resolution string  // 分辨率，如 "1080p"
	Version    int     // 版本号（v2 → 2，0 表示无版本标识）
	IsSpecial  bool    // 是否为特别篇/OVA
	IsBatch    bool    // 是否为合集
}

// 用于解析 Mikan 标题的正则表达式集合
// 参考 ani-rss RenameUtil.REG_STR 的设计
var (
	// 提取【】[] 中的字幕组名称
	reSubgroup = regexp.MustCompile(`^[【\[［]([^】\]］]+)[】\]］]\s*`)

	// 集数模式（按优先级排列）
	reEpisodePatterns = []*regexp.Regexp{
		// SxxExx 格式优先匹配
		regexp.MustCompile(`[Ss](?:eason)?\s*(\d{1,2})\s*[Ee](?:p(?:isode)?)?\s*(\d{1,3})(?:\.5)?`),
		// "- 01"、" 01" 结尾（含可选 END/FIN/完标记）
		regexp.MustCompile(`[-\s](\d{1,3})(?:\.5)?(?:\s*\(\d+\))?(?:\s*(?:END|end|Fin|fin|完))?\s*(?:$|[\[【])`),
		// "Vol 5" 卷数
		regexp.MustCompile(`[Vv]ol\s*(\d{1,3})(?:\.5)?`),
		// "第01話/话/集"
		regexp.MustCompile(`第(\d{1,3})(?:\.5)?[話话集]`),
		// "EP01"、"E01"
		regexp.MustCompile(`[Ee][Pp]?\s*(\d{1,3})(?:\.5)?`),
		// "#01"
		regexp.MustCompile(`#(\d{1,3})(?:\.5)?`),
		// 【01】中文方括号集数
		regexp.MustCompile(`【(\d{1,3})(?:\.5)?】`),
		// [01] 英文方括号集数（含可选版本号和 END 标记）
		regexp.MustCompile(`\[(\d{1,3})(?:\.5)?(?:\s*\(\d+\))?(?:\s*[vV](\d))?(?:\s*(?:END|end|Fin|fin|完))?\]`),
	}

	// 分辨率（参考 ani-rss 的 getResolution 方法）
	reResolution = regexp.MustCompile(`(?i)(\d{3,4}p)`)

	// 季数（参考 ani-rss StringEnum.SEASON_REG）
	reSeasonYear = regexp.MustCompile(`[Ss](?:eason)?\s*(\d{1,2})`)
	reSeasonTitle = regexp.MustCompile(`第([\d一二三四五六七八九十]{1,3})季`)

	// 版本号 v2/v3
	reVersion = regexp.MustCompile(`(?i)[\s\[【][vV](\d)[\s\]】]?`)

	// 特别篇/OVA/SP
	reSpecial = regexp.MustCompile(`(?i)(OVA[Ds]?|SP|特别篇|特典|SP\s*\d|OAD|特別篇)`)

	// 合集/批量
	reBatch = regexp.MustCompile(`(?i)(合集|全(\d{1,3})集|Fin|END|完結)`)

	// 年份标记 (2024)
	reCleanTags = regexp.MustCompile(`(?i)\[?\s*(?:1080p|720p|2160p|4k|HEVC|AVC|AV1|H\.?264|H\.?265|x265|x264|AAC|FLAC|MKV|MP4|GB|BIG5|CH[TS]|简|繁|简繁|繁简|内[嵌封挂]|WebRip|BDRip|BD|Remux)\s*\]?\s*`)

	// 清理末尾纯数字
	reTrailingDigits = regexp.MustCompile(`\s+\d{1,3}\s*$`)
)

// ParseMikanTitle 从 Mikan 种子标题中提取结构化信息
// 参考 ani-rss RenameUtil.rename() 的解析逻辑
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

	// 检测特别篇/OVA
	if reSpecial.MatchString(title) {
		info.IsSpecial = true
	}

	// 检测合集
	if reBatch.MatchString(title) {
		info.IsBatch = true
	}

	// 提取分辨率
	if m := reResolution.FindStringSubmatch(title); m != nil {
		info.Resolution = m[1]
	}

	// 提取版本号 v2/v3
	if m := reVersion.FindStringSubmatch(title); m != nil {
		info.Version, _ = strconv.Atoi(m[1])
	}

	// 逐模式尝试提取集数
	for ri, re := range reEpisodePatterns {
		m := re.FindStringSubmatch(title)
		if m == nil {
			continue
		}

		// patterns[0] 是 SxxExx 格式（m[1]=Season, m[2]=Episode）
		if ri == 0 {
			if s, err := strconv.Atoi(m[1]); err == nil && s > 0 {
				info.Season = s
			}
			if ep, err := strconv.ParseFloat(m[2], 32); err == nil {
				info.Episode = float32(ep)
			}
		} else {
			// 其他格式：m[1] = 集数
			if ep, err := strconv.ParseFloat(m[1], 32); err == nil {
				info.Episode = float32(ep)
			}
		}

		// 检测 .5 集数
		if strings.Contains(m[0], ".5") {
			info.Episode += 0.5
		}

		// 检测版本号（后续捕获组可能含版本号，如 [01v2] 的 m[2]="2"）
		for i := 2; i < len(m); i++ {
			if v, err := strconv.Atoi(m[i]); err == nil && v >= 2 && v <= 9 && info.Version == 0 {
				info.Version = v
			}
		}

		title = re.ReplaceAllString(title, "")
		break
	}

	// 提取季数（从 "Season 2" 或 "第二季" 关键词）
	if m := reSeasonYear.FindStringSubmatch(title); m != nil {
		season, _ := strconv.Atoi(m[1])
		if season > info.Season {
			info.Season = season
		}
	}
	if m := reSeasonTitle.FindStringSubmatch(title); m != nil {
		info.Season = parseCNNumber(m[1])
	}

	// 提取年份（保留在标题中但记录下来）
	// 年份不单独存储字段，保留在 title 中供外部使用

	// 全面清理：去除分辨率、编码、字幕组等常见标签
	title = reCleanTags.ReplaceAllString(title, "")
	title = reResolution.ReplaceAllString(title, "")
	title = reVersion.ReplaceAllString(title, "")
	title = reSpecial.ReplaceAllString(title, "")
	title = reBatch.ReplaceAllString(title, "")

	// 清理杂项字符
	title = strings.TrimSpace(title)
	title = strings.TrimRight(title, "- _[(（/[]")
	title = strings.TrimSpace(title)

	// 移除末尾可能残留的纯数字
	if reTrailingDigits.MatchString(title) {
		title = reTrailingDigits.ReplaceAllString(title, "")
	}

	info.Title = strings.TrimSpace(title)
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
