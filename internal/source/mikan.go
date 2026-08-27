// Package source 实现各资源站点的 Source 接口
// Mikan RSS 解析器负责解析 Mikan 个人 RSS 订阅源
package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
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
// 搜索缓存（LRU + TTL，限制容量防止内存无限增长）
// ============================================================

const (
	searchCacheMaxSize = 128           // 最大缓存条目数
	cacheEntryTTL      = 30 * time.Second
)

type cacheEntry struct {
	items     []core.TorrentItem
	expiresAt time.Time
}

// lruCache 简单 LRU 缓存（非线程安全，由调用方加锁）
type lruCache struct {
	maxSize int
	keys    []string                // 按访问顺序排列，尾部最新
	entries map[string]cacheEntry
}

func newLRUCache(maxSize int) *lruCache {
	return &lruCache{
		maxSize: maxSize,
		keys:    make([]string, 0, maxSize),
		entries: make(map[string]cacheEntry, maxSize),
	}
}

func (c *lruCache) Get(key string) (cacheEntry, bool) {
	entry, ok := c.entries[key]
	if !ok {
		return cacheEntry{}, false
	}
	if time.Now().After(entry.expiresAt) {
		c.delete(key)
		return cacheEntry{}, false
	}
	// 移到尾部（最近使用）
	c.touch(key)
	return entry, true
}

func (c *lruCache) Set(key string, entry cacheEntry) {
	if _, exists := c.entries[key]; exists {
		c.entries[key] = entry
		c.touch(key)
		return
	}
	// 淘汰最旧条目
	for len(c.keys) >= c.maxSize {
		oldest := c.keys[0]
		c.delete(oldest)
	}
	c.keys = append(c.keys, key)
	c.entries[key] = entry
}

func (c *lruCache) delete(key string) {
	delete(c.entries, key)
	for i, k := range c.keys {
		if k == key {
			c.keys = append(c.keys[:i], c.keys[i+1:]...)
			return
		}
	}
}

func (c *lruCache) touch(key string) {
	for i, k := range c.keys {
		if k == key {
			c.keys = append(c.keys[:i], c.keys[i+1:]...)
			c.keys = append(c.keys, key)
			return
		}
	}
}

var (
	searchCacheMu sync.Mutex
	searchCache   = newLRUCache(searchCacheMaxSize)
)

// ============================================================
// MikanSource 实现 core.Source 接口
// ============================================================

type MikanSource struct {
	mu            sync.RWMutex
	httpClient    *http.Client
	domain        string
	proxyDomain   string
	mirrorDomains []string // 镜像域名列表，GFW 下自动回退
}

// NewMikanSource 创建新的 Mikan 资源源
func NewMikanSource(domain, proxyDomain string, mirrorDomains []string) *MikanSource {
	return &MikanSource{
		httpClient:    httpx.New(30 * time.Second),
		domain:        domain,
		proxyDomain:   proxyDomain,
		mirrorDomains: mirrorDomains,
	}
}

func (m *MikanSource) Name() string { return "Mikan" }

// GetDomain 获取当前主域名
func (m *MikanSource) GetDomain() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.domain
}

// SetDomain 设置主域名（用于启动时自动切换到最快的镜像）
func (m *MikanSource) SetDomain(domain string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domain = domain
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
func cleanSearchTitle(title string) string {
	re := regexp.MustCompile(`(?i)\s+(第[一二三四五六七八九十\d]+[季期部篇]|S\d{1,2}|Season\s*\d+|Part\s*\d+|OVA|OAD|SP|特别篇|剧场版|合集)$`)
	return strings.TrimSpace(re.ReplaceAllString(title, ""))
}

func (m *MikanSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	searchCacheMu.Lock()
	if cached, ok := searchCache.Get(title); ok {
		searchCacheMu.Unlock()
		return cached.items, nil
	}
	searchCacheMu.Unlock()

	m.mu.RLock()
	domain := m.domain
	m.mu.RUnlock()

	encodedTitle := url.QueryEscape(title)
	path := "/Home/Search?searchstr=" + encodedTitle
	resp, err := m.tryMirrors(ctx, path)
	if err == nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			items := parseMikanSearchHTML(string(body), domain)
			if len(items) > 0 {
				searchCacheMu.Lock()
				searchCache.Set(title, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
				searchCacheMu.Unlock()
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
				items := parseMikanSearchHTML(string(body), domain)
				if len(items) > 0 {
					searchCacheMu.Lock()
					searchCache.Set(title, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
					searchCacheMu.Unlock()
					return items, nil
				}
			}
		}
	}

	items, err := m.searchBySeason(ctx, title)
	if err == nil && len(items) > 0 {
		searchCacheMu.Lock()
		searchCache.Set(title, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
		searchCacheMu.Unlock()
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

	m.mu.RLock()
	domain := m.domain
	m.mu.RUnlock()

	return parseMikanDetailHTML(string(body), filter, domain), nil
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
// 番剧周时间表相关
// ============================================================

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

// ============================================================
// 字幕组相关
// ============================================================

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