// Package source 实现各资源站点的 Source 接口
// Mikan RSS 解析器负责解析 Mikan 个人 RSS 订阅源
package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
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

// 全局搜索缓存（跨实例共享，30s TTL）
var (
	searchCache   sync.Map
	cacheEntryTTL = 30 * time.Second
)

type cacheEntry struct {
	items     []core.TorrentItem
	expiresAt time.Time
}

type MikanSource struct {
	httpClient    *http.Client
	domain        string
	proxyDomain   string
	mirrorDomains []string // 镜像域名列表，GFW 下自动回退
}

// NewMikanSource 创建新的 Mikan 资源源
func NewMikanSource(domain, proxyDomain string, mirrorDomains []string) *MikanSource {
	return &MikanSource{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		domain:        domain,
		proxyDomain:   proxyDomain,
		mirrorDomains: mirrorDomains,
	}
}

func (m *MikanSource) Name() string { return "Mikan" }

// GetDomain 获取当前主域名
func (m *MikanSource) GetDomain() string { return m.domain }

// SetDomain 设置主域名（用于启动时自动切换到最快的镜像）
func (m *MikanSource) SetDomain(domain string) { m.domain = domain }

// MirrorLatency 镜像延迟测试结果
type MirrorLatency struct {
	Domain  string `json:"domain"`
	Latency int64  `json:"latency_ms"` // 毫秒
	OK      bool   `json:"ok"`
}

// TestLatency 并发测试所有镜像延迟，返回结果（不改变内部状态）
func (m *MikanSource) TestLatency(ctx context.Context) []MirrorLatency {
	domains := make([]string, 0, 2+len(m.mirrorDomains))
	if m.proxyDomain != "" {
		domains = append(domains, m.proxyDomain)
	}
	domains = append(domains, m.domain)
	domains = append(domains, m.mirrorDomains...)

	// 去重
	seen := make(map[string]bool)
	unique := make([]string, 0, len(domains))
	for _, d := range domains {
		if !seen[d] {
			seen[d] = true
			unique = append(unique, d)
		}
	}

	results := make([]MirrorLatency, len(unique))
	var wg sync.WaitGroup
	for i, domain := range unique {
		wg.Add(1)
		go func(idx int, d string) {
			defer wg.Done()
			start := time.Now()
			url := fmt.Sprintf("https://%s/", d)
			req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
			if err != nil {
				results[idx] = MirrorLatency{Domain: d, Latency: 99999, OK: false}
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0")
			resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
			elapsed := time.Since(start).Milliseconds()
			if err != nil {
				results[idx] = MirrorLatency{Domain: d, Latency: elapsed, OK: false}
				return
			}
			resp.Body.Close()
			results[idx] = MirrorLatency{Domain: d, Latency: elapsed, OK: true}
		}(i, domain)
	}
	wg.Wait()
	return results
}

// BestDomain 从延迟结果中选择最快的域名
func BestDomain(results []MirrorLatency, fallback string) string {
	best := fallback
	bestLatency := int64(99999)
	for _, r := range results {
		if r.OK && r.Latency < bestLatency {
			bestLatency = r.Latency
			best = r.Domain
		}
	}
	return best
}

// tryMirrors 依次尝试通过代理域名、主域名、镜像域名发起 HTTP GET 请求
// 在 GFW 环境下主域名可能不可达，自动回退到镜像域名
func (m *MikanSource) tryMirrors(ctx context.Context, path string) (*http.Response, error) {
	domains := make([]string, 0, 2+len(m.mirrorDomains))
	if m.proxyDomain != "" {
		domains = append(domains, m.proxyDomain)
	}
	domains = append(domains, m.domain)
	domains = append(domains, m.mirrorDomains...)

	var lastErr error
	for _, domain := range domains {
		url := fmt.Sprintf("https://%s%s", domain, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

		resp, err := m.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("镜像 %s 返回状态码: %d", domain, resp.StatusCode)
	}
	return nil, fmt.Errorf("所有镜像均不可达: %w", lastErr)
}

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
			EpisodeURL:  item.Link,
		})
	}

	return items, nil
}

// SearchAnime 在 Mikan 上搜索番剧
// 优先使用文本搜索，如果需要登录则回退到季节搜索+本地过滤
// cleanSearchTitle 剥离季数/集数后缀
func cleanSearchTitle(title string) string {
	re := regexp.MustCompile(`(?i)\s+(第[一二三四五六七八九十\d]+[季期部篇]|S\d{1,2}|Season\s*\d+|Part\s*\d+|OVA|OAD|SP|特别篇|剧场版|合集)$`)
	return strings.TrimSpace(re.ReplaceAllString(title, ""))
}

func (m *MikanSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	if cached, ok := searchCache.Load(title); ok {
		entry := cached.(cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.items, nil
		}
		searchCache.Delete(title)
	}

	encodedTitle := url.QueryEscape(title)
	path := "/Home/Search?searchstr=" + encodedTitle
	resp, err := m.tryMirrors(ctx, path)
	if err == nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			items := parseMikanSearchHTML(string(body), m.domain)
			if len(items) > 0 {
				searchCache.Store(title, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
				return items, nil
			}
		}
	}

	// 完整标题无结果，剥离季数/集数后缀模糊搜索
	cleaned := cleanSearchTitle(title)
	if cleaned != title {
		encodedCleaned := url.QueryEscape(cleaned)
		path = "/Home/Search?searchstr=" + encodedCleaned
		resp, err = m.tryMirrors(ctx, path)
		if err == nil {
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err == nil {
				items := parseMikanSearchHTML(string(body), m.domain)
				if len(items) > 0 {
					searchCache.Store(title, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
					return items, nil
				}
			}
		}
	}

	items, err := m.searchBySeason(ctx, title)
	if err == nil && len(items) > 0 {
		searchCache.Store(title, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
	}
	return items, err
}

// searchBySeason 通过季节列表搜索番剧（不需要登录）
func (m *MikanSource) searchBySeason(ctx context.Context, title string) ([]core.TorrentItem, error) {
	// 获取当前年份和季节
	now := time.Now()
	year := now.Year()
	season := getSeason(now.Month())

	var allItems []core.TorrentItem

	// 尝试当前季节和上一个季节
	for i := 0; i < 2; i++ {
		s := season - i
		y := year
		if s < 1 {
			s = 4
			y--
		}

		path := fmt.Sprintf("/Home/BangumiCoverFlowByDayOfWeek?year=%d&seasonStr=%d", y, s)
		resp, err := m.tryMirrors(ctx, path)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		items := parseMikanSeasonHTML(string(body), m.domain, title)
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

// getSeason 根据月份返回季节（1-4）
func getSeason(month time.Month) int {
	switch {
	case month >= 1 && month <= 3:
		return 1 // 冬季
	case month >= 4 && month <= 6:
		return 2 // 春季
	case month >= 7 && month <= 9:
		return 3 // 夏季
	default:
		return 4 // 秋季
	}
}

// FetchHistory 爬取 Mikan 番剧详情页获取全量历史种子
func (m *MikanSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	resp, err := m.tryMirrors(ctx, "/Home/Bangumi/"+bangumiID)
	if err != nil {
		return nil, fmt.Errorf("获取详情页失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取详情页响应失败: %w", err)
	}

	return parseMikanDetailHTML(string(body), filter, m.domain), nil
}

func (m *MikanSource) IsAvailable(ctx context.Context) bool {
	resp, err := m.tryMirrors(ctx, "/")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
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

// 用户自定义正则模式（线程安全）
var (
	customRegexPatterns []*regexp.Regexp
	customRegexMu       sync.RWMutex
)

// SetCustomRegexPatterns 设置用户自定义正则模式
// 每个模式应包含一个捕获组提取集数（可选两个：Season + Episode 类似 SxxExx）
func SetCustomRegexPatterns(patterns []string) error {
	customRegexMu.Lock()
	defer customRegexMu.Unlock()

	customRegexPatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("自定义正则编译失败: %s → %w", p, err)
		}
		customRegexPatterns = append(customRegexPatterns, re)
	}
	if len(customRegexPatterns) > 0 {
		log.Printf("✅ 已加载 %d 条自定义正则解析规则", len(customRegexPatterns))
	}
	return nil
}

// GetCustomRegexPatterns 返回当前自定义正则模式（用于展示）
func GetCustomRegexPatterns() []string {
	customRegexMu.RLock()
	defer customRegexMu.RUnlock()

	result := make([]string, len(customRegexPatterns))
	for i, re := range customRegexPatterns {
		result[i] = re.String()
	}
	return result
}

// LoadCustomPatternsFromSettings 从数据库 settings 表加载自定义正则
// 格式：custom_regex_0, custom_regex_1, ...
func LoadCustomPatternsFromSettings(getSetting func(key string) (string, bool)) {
	var patterns []string
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("custom_regex_%d", i)
		val, ok := getSetting(key)
		if !ok || strings.TrimSpace(val) == "" {
			break
		}
		patterns = append(patterns, val)
	}
	if err := SetCustomRegexPatterns(patterns); err != nil {
		log.Printf("⚠️  加载自定义正则失败: %v", err)
	}
}

// ParseMikanTitle 从 Mikan 种子标题中提取结构化信息
// 参考 ani-rss RenameUtil.rename() 的解析逻辑
// 优先尝试用户自定义正则，再回退到内置 8 种模式
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

	// 先尝试用户自定义正则（优先级高于内置模式）
	customRegexMu.RLock()
	customPats := make([]*regexp.Regexp, len(customRegexPatterns))
	copy(customPats, customRegexPatterns)
	customRegexMu.RUnlock()

	var matched bool
	for i, re := range customPats {
		m := re.FindStringSubmatch(title)
		if m == nil {
			continue
		}
		// 捕获组 >= 3：(1=Season, 2=Episode)；否则 1=Episode
		if len(m) >= 3 {
			if s, err := strconv.Atoi(m[1]); err == nil && s > 0 {
				info.Season = s
			}
			if ep, err := strconv.ParseFloat(m[2], 32); err == nil {
				info.Episode = float32(ep)
			}
		} else {
			if ep, err := strconv.ParseFloat(m[1], 32); err == nil {
				info.Episode = float32(ep)
			}
		}
		if strings.Contains(m[0], ".5") {
			info.Episode += 0.5
		}
		title = re.ReplaceAllString(title, "")
		log.Printf("🔧 自定义正则命中 [%d]: %s → S%dE%.1f", i, m[0], info.Season, info.Episode)
		matched = true
		break
	}

	// 逐模式尝试提取集数（内置 8 种模式）
	if !matched {
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

		// 检测 .5 集数 (仅当当前集数为整数时才加 0.5，避免从自定义正则或模式匹配中双重计算)
		if strings.Contains(m[0], ".5") && info.Episode == float32(int(info.Episode)) {
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

// parseMikanSearchHTML 从 Mikan 搜索结果 HTML 中提取种子列表
// 使用 goquery 解析 HTML，参考 ani-rss MikanService.java 的 CSS 选择器
func parseMikanSearchHTML(html, domain string) []core.TorrentItem {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Printf("⚠️ Mikan 搜索页 HTML 解析失败: %v", err)
		return nil
	}

	var items []core.TorrentItem
	seen := make(map[string]bool)

	// 方法1：从推荐番剧列表提取（.an-ul 结构）
	doc.Find(".an-ul li").Each(func(_ int, sel *goquery.Selection) {
		a := sel.Find("a").First()
		title := strings.TrimSpace(a.Text())
		href, ok := a.Attr("href")
		if title == "" || !ok {
			return
		}

		bangumiID := ""
		if strings.HasPrefix(href, "/Home/Bangumi/") {
			bangumiID = strings.TrimPrefix(href, "/Home/Bangumi/")
		}
		key := href
		if seen[key] {
			return
		}
		seen[key] = true

		// 提取封面图
		cover := ""
		coverEl := sel.Find("span[data-src]")
		if coverEl.Length() > 0 {
			src, exists := coverEl.Attr("data-src")
			if exists && src != "" {
				if strings.HasPrefix(src, "/") {
					cover = "https://" + domain + src
				} else {
					cover = src
				}
			}
		}

		items = append(items, core.TorrentItem{
			Title:      title,
			URL:        "https://" + domain + href,
			SourceName: "Mikan",
			BangumiID:  bangumiID,
			CoverURL:   cover,
		})
	})

	// 方法2：从搜索结果中提取 Bangumi 链接（直接查找 /Home/Bangumi/ 模式）
	doc.Find("a[href*=\"/Home/Bangumi/\"]").Each(func(_ int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Text())
		href, ok := sel.Attr("href")
		if title == "" || !ok {
			return
		}

		bangumiID := strings.TrimPrefix(href, "/Home/Bangumi/")
		key := href
		if seen[key] {
			return
		}
		seen[key] = true

		items = append(items, core.TorrentItem{
			Title:      title,
			URL:        "https://" + domain + href,
			SourceName: "Mikan",
			BangumiID:  bangumiID,
		})
	})

	return items
}

// FetchWeekSchedule 获取指定季度番剧按星期分组列表
// 如果未指定 year/season (即为0)，则依次尝试当前/上一季度，直到拿到数据为止
func (m *MikanSource) FetchWeekSchedule(ctx context.Context, year, season int) ([]WeekDayItem, error) {
	weekLabel := map[int]string{
		1: "星期一", 2: "星期二", 3: "星期三", 4: "星期四",
		5: "星期五", 6: "星期六", 7: "星期日",
	}

	if year > 0 && season > 0 {
		path := fmt.Sprintf("/Home/BangumiCoverFlowByDayOfWeek?year=%d&seasonStr=%d", year, season)
		return m.fetchPath(ctx, path, weekLabel)
	}

	now := time.Now()
	year = now.Year()
	season = getSeason(now.Month())

	// 最多尝试 3 个季度（当前、上季、上上季）
	for i := 0; i < 3; i++ {
		s := season - i
		y := year
		for s < 1 {
			s += 4
			y--
		}

		path := fmt.Sprintf("/Home/BangumiCoverFlowByDayOfWeek?year=%d&seasonStr=%d", y, s)
		result, err := m.fetchPath(ctx, path, weekLabel)
		if err == nil && len(result) > 0 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("failed to fetch schedule from Mikan")
}

func (m *MikanSource) fetchPath(ctx context.Context, path string, weekLabel map[int]string) ([]WeekDayItem, error) {
	resp, err := m.tryMirrors(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) < 2000 {
		return nil, fmt.Errorf("body too short")
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	var result []WeekDayItem
	doc.Find(".sk-bangumi").Each(func(_ int, sel *goquery.Selection) {
		dowStr, exists := sel.Attr("data-dayofweek")
		if !exists {
			return
		}
		dow, _ := strconv.Atoi(dowStr)
		label := weekLabel[dow]
		if label == "" {
			label = dowStr
		}

		var items []core.TorrentItem
		sel.Find("a.an-text").Each(func(_ int, a *goquery.Selection) {
			title := strings.TrimSpace(a.Text())
			href, _ := a.Attr("href")
			if title == "" || href == "" {
				return
			}
			bangumiID := ""
			if strings.HasPrefix(href, "/Home/Bangumi/") {
				bangumiID = strings.TrimPrefix(href, "/Home/Bangumi/")
			}

			// 提取更新日期
			parent := a.ParentsFiltered(".an-info-group")
			updateDate := ""
			if parent.Length() > 0 {
				updateDate = strings.TrimSpace(parent.Find(".date-text").Text())
			}

			// 提取封面图（从同级的 b-lazy span 的 data-src 属性）
			cover := ""
			listItem := a.ParentsFiltered("li").First()
			if listItem.Length() > 0 {
				src, exists := listItem.Find(".b-lazy").Attr("data-src")
				if exists && src != "" {
					if strings.HasPrefix(src, "/") {
						cover = "https://" + m.domain + src
					} else {
						cover = src
					}
				}
			}

			items = append(items, core.TorrentItem{
				Title:      title,
				URL:        "https://" + m.domain + href,
				BangumiID:  bangumiID,
				SourceName: "Mikan",
				AiredDate:  updateDate,
				InfoHash:   "", // Clear InfoHash as it is not a hash
				CoverURL:   cover,
			})
		})

		if len(items) > 0 {
			result = append(result, WeekDayItem{DayOfWeek: dow, Label: label, Items: items})
		}
	})
	return result, nil
}


// parseMikanSeasonHTML 从 Mikan 季度列表 HTML 中提取番剧（不需要登录）
// 使用 goquery 解析 HTML，参考 mikanime.tv 的实际 HTML 结构
func parseMikanSeasonHTML(html, domain, searchText string) []core.TorrentItem {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Printf("⚠️ Mikan 季度页 HTML 解析失败: %v", err)
		return nil
	}

	searchLower := strings.ToLower(searchText)
	var items []core.TorrentItem

	// 查找所有番剧链接（.an-text 类）
	doc.Find("a.an-text").Each(func(_ int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Text())
		href, _ := sel.Attr("href")
		if title == "" || href == "" {
			return
		}

		// 本地过滤：标题包含搜索关键词
		if searchText != "" && !strings.Contains(strings.ToLower(title), searchLower) {
			return
		}

		// 提取 Bangumi ID
		bangumiID := ""
		if strings.HasPrefix(href, "/Home/Bangumi/") {
			bangumiID = strings.TrimPrefix(href, "/Home/Bangumi/")
		}

		cover := ""
		listItem := sel.ParentsFiltered("li").First()
		if listItem.Length() > 0 {
			src, exists := listItem.Find(".b-lazy, span[data-src]").Attr("data-src")
			if exists && src != "" {
				if strings.HasPrefix(src, "/") {
					cover = "https://" + domain + src
				} else {
					cover = src
				}
			}
		}

		items = append(items, core.TorrentItem{
			Title:      title,
			URL:        "https://" + domain + href,
			BangumiID:  bangumiID,
			SourceName: "Mikan",
			CoverURL:   cover,
		})
	})
	return items
}

// parseMikanDetailHTML 从 Mikan 番剧详情页 HTML 中提取全量种子
// 参考 ani-rss MikanService.java 的 CSS 选择器实现
func parseMikanDetailHTML(html string, filter core.Filter, domain string) []core.TorrentItem {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Printf("⚠️ Mikan 详情页 HTML 解析失败: %v", err)
		return nil
	}

	var items []core.TorrentItem

	// 遍历每个字幕组区块 (.leftbar-item)
	doc.Find(".leftbar-item").Each(func(_ int, leftbar *goquery.Selection) {
		groupName := strings.TrimSpace(leftbar.Find("a.subgroup-name").Text())
		if groupName == "" {
			return
		}

		// 字幕组过滤：如果指定了首选字幕组且不匹配则跳过
		if filter.PreferSubgroup != "" && !strings.Contains(groupName, filter.PreferSubgroup) {
			return
		}

		// 获取字幕组锚点 ID，定位相邻的种子表格
		anchor := leftbar.Find("a[name]").First()
		anchorID, _ := anchor.Attr("name")
		if anchorID == "" {
			// 尝试用 data-anchor 属性
			anchorID, _ = leftbar.Find("a.subgroup-name").Attr("data-anchor")
			if anchorID == "" {
				return
			}
			log.Printf("🔎 尝试解析字幕组: %s, data-anchor: %s", groupName, anchorID)
			selector := anchorID
			if !strings.HasPrefix(selector, "#") {
				selector = "a[name=\"" + anchorID + "\"]"
			}
			// 找到对应的表格区域
			doc.Find(selector).Each(func(_ int, namedAnchor *goquery.Selection) {
				table := namedAnchor.NextAllFiltered(".episode-table").First().Find("table").First()
				if table.Length() == 0 {
					table = namedAnchor.NextAllFiltered("table").First()
				}
				if table.Length() == 0 {
					log.Printf("⚠️ 找不到 table (anchorID=%s)", anchorID)
					return
				}
				log.Printf("✅ 找到 table (anchorID=%s)", anchorID)
				extractTorrentTable(table, groupName, domain, filter, &items)
			})
			return
		}

		log.Printf("🔎 尝试解析字幕组: %s, anchor: %s", groupName, anchorID)
		table := anchor.NextAllFiltered("table").First()
		if table.Length() == 0 {
			log.Printf("⚠️ 找不到 table (anchorID=%s)", anchorID)
			return
		}
		log.Printf("✅ 找到 table (anchorID=%s)", anchorID)
		extractTorrentTable(table, groupName, domain, filter, &items)
	})

	return items
}

// extractTorrentTable 从字幕组对应的种子表格中提取种子条目
func extractTorrentTable(table *goquery.Selection, groupName, domain string, filter core.Filter, items *[]core.TorrentItem) {
	table.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
		// 提取磁力链接
		magnetLink, _ := tr.Find("a[data-clipboard-text]").Attr("data-clipboard-text")

		// 提取种子标题（第一个 a 标签的文本）
		title := strings.TrimSpace(tr.Find("a").First().Text())
		if title == "" {
			log.Printf("⚠️ 标题为空，跳过 (HTML: %s)", tr.Text())
			return
		}
		
		// 关键词过滤
		if len(filter.IncludeKeywords) > 0 {
			matched := false
			for _, kw := range filter.IncludeKeywords {
				if strings.Contains(title, kw) {
					matched = true
					break
				}
			}
			if !matched {
				log.Printf("⚠️ 标题 [%s] 不匹配包含关键词: %v", title, filter.IncludeKeywords)
				return
			}
		}
		for _, kw := range filter.ExcludeKeywords {
			if strings.Contains(title, kw) {
				log.Printf("⚠️ 标题 [%s] 匹配排除关键词: %s", title, kw)
				return
			}
		}

		// 提取种子下载链接
		torrentURL := ""
		tr.Find("a").Each(func(_ int, a *goquery.Selection) {
			href, _ := a.Attr("href")
			if strings.Contains(href, ".torrent") {
				if !strings.HasPrefix(href, "http") {
					href = "https://" + domain + href
				}
				torrentURL = href
			}
		})

		// 提取文件大小（第三个 td）
		sizeText := strings.TrimSpace(tr.Find("td").Eq(2).Text())

		// 提取日期（第四个 td）
		dateText := strings.TrimSpace(tr.Find("td").Eq(3).Text())

		pubAt, _ := parsePubDate(dateText)

		// 用现有标题解析器提取结构化信息
		info := ParseMikanTitle(title)
		if info.Subgroup == "" {
			info.Subgroup = groupName
		}

		*items = append(*items, core.TorrentItem{
			Title:       title,
			URL:         torrentURL,
			MagnetURL:   magnetLink,
			InfoHash:    extractInfoHash(magnetLink),
			Size:        parseSize(sizeText),
			PublishedAt: pubAt,
			SourceName:  "Mikan",
			GroupName:   info.Subgroup,
		})
	})
}

// parseSize 解析文件大小字符串（如 "1.2 GB", "500 MB"）为字节数
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`([\d.]+)\s*(GB|MB|KB|B|gb|mb|kb|b)`)
	matches := re.FindStringSubmatch(s)
	if len(matches) != 3 {
		return 0
	}
	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(matches[2]) {
	case "GB":
		return int64(val * 1024 * 1024 * 1024)
	case "MB":
		return int64(val * 1024 * 1024)
	case "KB":
		return int64(val * 1024)
	default:
		return int64(val)
	}
}

// extractInfoHash 从磁力链接中提取 40 位十六进制 BT InfoHash
func extractInfoHash(magnetURL string) string {
	re := regexp.MustCompile(`btih:([0-9a-fA-F]{40})`)
	matches := re.FindStringSubmatch(magnetURL)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// SubgroupInfo Mikan 番剧页面的字幕组信息
type SubgroupInfo struct {
	Name   string `json:"name"`
	RSSURL string `json:"rss_url"`
}

// FetchSubgroups 获取 Mikan 番剧详情页的所有字幕组列表及其 RSS URL
func (m *MikanSource) FetchSubgroups(ctx context.Context, bangumiID string) ([]SubgroupInfo, error) {
	path := "/Home/Bangumi/" + bangumiID
	resp, err := m.tryMirrors(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("获取 Mikan 详情页失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 Mikan 详情页失败: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("解析 Mikan 详情页 HTML 失败: %w", err)
	}

	var groups []SubgroupInfo
	doc.Find(".leftbar-item").Each(func(_ int, leftbar *goquery.Selection) {
		groupName := strings.TrimSpace(leftbar.Find("a.subgroup-name").Text())
		if groupName == "" {
			return
		}

		// 通过 data-anchor 定位字幕组锚点（如 #202）
		anchorID, exists := leftbar.Find("a.subgroup-name").Attr("data-anchor")
		if !exists || anchorID == "" {
			return
		}

		// 在文档中找到锚点对应的区块，再找 .mikan-rss
		anchorSection := doc.Find(anchorID)
		if anchorSection.Length() == 0 {
			return
		}

		rssHref, exists := anchorSection.Find(".mikan-rss").Attr("href")
		if !exists {
			return
		}

		rssURL := rssHref
		if !strings.HasPrefix(rssURL, "http") {
			rssURL = "https://" + m.domain + rssHref
		}

		groups = append(groups, SubgroupInfo{
			Name:   groupName,
			RSSURL: rssURL,
		})
	})

	return groups, nil
}

// ResolveFirstRSSURL 从 BangumiID 获取 Mikan 详情页并提取第一个可用字幕组的 RSS URL
func (m *MikanSource) ResolveFirstRSSURL(ctx context.Context, bangumiID string) (string, error) {
	groups, err := m.FetchSubgroups(ctx, bangumiID)
	if err != nil {
		return "", err
	}
	if len(groups) == 0 {
		return "", fmt.Errorf("未找到任何字幕组")
	}
	return groups[0].RSSURL, nil
}

// BuildMikanRSSURL 构建 Mikan 个人 RSS 完整 URL
func BuildMikanRSSURL(tokenURL string) string {
	return strings.TrimSpace(tokenURL)
}
