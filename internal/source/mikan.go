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
	"slices"
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
	isHexHash     = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	reFourDigits  = regexp.MustCompile(`(\d{4})`)
	reDigits      = regexp.MustCompile(`(\d+)`)
	searchCacheMu sync.Mutex
	searchCache   = newLRUCache(searchCacheMaxSize)
)

const (
	scheduleCacheMaxSize = 16
	scheduleCacheTTL     = 15 * time.Minute
	subgroupCacheMaxSize = 128
	subgroupCacheTTL     = 10 * time.Minute
)

type ttlCacheEntry[T any] struct {
	val       T
	expiresAt time.Time
}

type boundedTTLCache[T any] struct {
	maxSize int
	ttl     time.Duration
	keys    []string
	entries map[string]ttlCacheEntry[T]
}

func newBoundedTTLCache[T any](maxSize int, ttl time.Duration) *boundedTTLCache[T] {
	return &boundedTTLCache[T]{
		maxSize: maxSize,
		ttl:     ttl,
		keys:    make([]string, 0, maxSize),
		entries: make(map[string]ttlCacheEntry[T], maxSize),
	}
}

func (c *boundedTTLCache[T]) get(key string) (T, bool) {
	entry, ok := c.entries[key]
	if !ok {
		var zero T
		return zero, false
	}
	if time.Now().After(entry.expiresAt) {
		c.deleteKey(key)
		var zero T
		return zero, false
	}
	c.touch(key)
	return entry.val, true
}

func (c *boundedTTLCache[T]) set(key string, val T) {
	entry := ttlCacheEntry[T]{val: val, expiresAt: time.Now().Add(c.ttl)}
	if _, exists := c.entries[key]; exists {
		c.entries[key] = entry
		c.touch(key)
		return
	}
	for len(c.keys) >= c.maxSize {
		c.deleteKey(c.keys[0])
	}
	c.keys = append(c.keys, key)
	c.entries[key] = entry
}

func (c *boundedTTLCache[T]) deleteKey(key string) {
	delete(c.entries, key)
	for i, k := range c.keys {
		if k == key {
			c.keys = slices.Delete(c.keys, i, i+1)
			return
		}
	}
}

func (c *boundedTTLCache[T]) touch(key string) {
	for i, k := range c.keys {
		if k == key {
			c.keys = append(slices.Delete(c.keys, i, i+1), key)
			return
		}
	}
}

func cloneWeekDayItems(days []WeekDayItem) []WeekDayItem {
	out := make([]WeekDayItem, len(days))
	for i, d := range days {
		out[i] = WeekDayItem{
			DayOfWeek: d.DayOfWeek,
			Label:     d.Label,
			Items:     slices.Clone(d.Items),
		}
	}
	return out
}

// ============================================================
// MikanSource 实现 core.Source 接口
// ============================================================

type MikanSource struct {
	mu            sync.RWMutex
	httpClient    *http.Client
	mirrorClient  *http.Client
	domain        string
	proxyDomain   string
	mirrorDomains []string // 镜像域名列表，GFW 下自动回退
	cacheMu       sync.Mutex
	scheduleCache *boundedTTLCache[[]WeekDayItem]
	subgroupCache *boundedTTLCache[[]SubgroupInfo]
}

// NewMikanSource 创建新的 Mikan 资源源
func NewMikanSource(domain, proxyDomain string, mirrorDomains []string) *MikanSource {
	return &MikanSource{
		httpClient:    httpx.New(30 * time.Second),
		mirrorClient:  httpx.New(6 * time.Second),
		domain:        domain,
		proxyDomain:   proxyDomain,
		mirrorDomains: mirrorDomains,
		scheduleCache: newBoundedTTLCache[[]WeekDayItem](scheduleCacheMaxSize, scheduleCacheTTL),
		subgroupCache: newBoundedTTLCache[[]SubgroupInfo](subgroupCacheMaxSize, subgroupCacheTTL),
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
func (m *MikanSource) FetchRSS(ctx context.Context, feedURL string) ([]core.TorrentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
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

	var rss mikanRSS
	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		return nil, fmt.Errorf("解析 RSS XML 失败: %w", err)
	}

	items := make([]core.TorrentItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		pubAt, _ := parsePubDate(item.PubDate)
		info := ParseMikanTitle(item.Title)

		torrentURL := strings.TrimSpace(item.Enclosure.URL)
		magnetURL := ""
		for _, link := range []string{torrentURL, item.Link, item.GUID} {
			if strings.HasPrefix(link, "magnet:?") {
				magnetURL = link
				break
			}
		}
		if torrentURL == "" {
			if magnetURL != "" {
				torrentURL = magnetURL
			} else if u, e := url.Parse(item.Link); e == nil && strings.HasSuffix(strings.ToLower(u.Path), ".torrent") {
				torrentURL = item.Link
			}
		}
		if torrentURL == "" {
			continue
		}
		hash := extractInfoHash(magnetURL)
		if hash == "" {
			if detail, e := url.Parse(item.Link); e == nil && strings.HasPrefix(detail.Path, "/Home/Episode/") {
				id := strings.TrimPrefix(detail.Path, "/Home/Episode/")
				if isHexHash.MatchString(id) {
					hash = strings.ToLower(id)
				}
			}
		}
		items = append(items, core.TorrentItem{
			Title:       item.Title,
			URL:         torrentURL,
			MagnetURL:   magnetURL,
			InfoHash:    hash,
			GroupName:   info.Subgroup,
			Resolution:  info.Resolution,
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
	cacheKey := m.GetDomain() + "\x00" + title
	searchCacheMu.Lock()
	if cached, ok := searchCache.Get(cacheKey); ok {
		searchCacheMu.Unlock()
		return cached.items, nil
	}
	searchCacheMu.Unlock()

	var successfulPage bool
	encodedTitle := url.QueryEscape(title)
	path := "/Home/Search?searchstr=" + encodedTitle
	resp, err := m.tryMirrors(ctx, path)
	if err == nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			successfulPage = true
			items := parseMikanSearchHTML(string(body), resp.Request.URL.Host)
			if len(items) > 0 {
				searchCacheMu.Lock()
				searchCache.Set(cacheKey, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
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
				successfulPage = true
				items := parseMikanSearchHTML(string(body), resp.Request.URL.Host)
				if len(items) > 0 {
					searchCacheMu.Lock()
					searchCache.Set(cacheKey, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
					searchCacheMu.Unlock()
					return items, nil
				}
			}
		}
	}

	items, err := m.searchBySeason(ctx, title)
	if err != nil && successfulPage && ctx.Err() == nil {
		return nil, nil
	}
	if err == nil && len(items) > 0 {
		searchCacheMu.Lock()
		searchCache.Set(cacheKey, cacheEntry{items: items, expiresAt: time.Now().Add(cacheEntryTTL)})
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
	var lastErr error
	var fetched bool

	// 尝试当前季节和上一个季节
	for i := 0; i < 2; i++ {
		s := season - i
		y := year
		if s < 1 {
			s = 4
			y--
		}

		path := fmt.Sprintf("/Home/BangumiCoverFlowByDayOfWeek?year=%d&seasonStr=%s", y, url.QueryEscape(seasonToMikanStr(s)))
		resp, err := m.tryMirrors(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		fetched = true
		items := parseMikanSeasonHTML(string(body), resp.Request.URL.Host, title)
		allItems = append(allItems, items...)
	}

	if !fetched && lastErr != nil {
		return nil, lastErr
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
	if strings.TrimSpace(bangumiID) == "" {
		return nil, nil
	}
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

func seasonToMikanStr(season int) string {
	switch season {
	case 1:
		return "冬"
	case 2:
		return "春"
	case 3:
		return "夏"
	case 4:
		return "秋"
	default:
		return "冬"
	}
}

// FetchWeekSchedule 获取指定季度番剧按星期分组列表
// 如果未指定 year/season (即为0)，则依次尝试当前/上一季度，直到拿到数据为止
func (m *MikanSource) FetchWeekSchedule(ctx context.Context, year, season int) ([]WeekDayItem, error) {
	weekLabel := map[int]string{
		1: "星期一", 2: "星期二", 3: "星期三", 4: "星期四",
		5: "星期五", 6: "星期六", 7: "星期日",
	}

	if year > 0 && season > 0 {
		path := fmt.Sprintf("/Home/BangumiCoverFlowByDayOfWeek?year=%d&seasonStr=%s", year, url.QueryEscape(seasonToMikanStr(season)))
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

		path := fmt.Sprintf("/Home/BangumiCoverFlowByDayOfWeek?year=%d&seasonStr=%s", y, url.QueryEscape(seasonToMikanStr(s)))
		result, err := m.fetchPath(ctx, path, weekLabel)
		if err == nil && len(result) > 0 {
			return result, nil
		}
	}
	return nil, fmt.Errorf("failed to fetch schedule from Mikan")
}

func (m *MikanSource) getCachedSchedule(key string) ([]WeekDayItem, bool) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.scheduleCache == nil {
		return nil, false
	}
	days, ok := m.scheduleCache.get(key)
	if !ok {
		return nil, false
	}
	return cloneWeekDayItems(days), true
}

func (m *MikanSource) setCachedSchedule(key string, days []WeekDayItem) {
	if len(days) == 0 {
		return
	}
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.scheduleCache == nil {
		m.scheduleCache = newBoundedTTLCache[[]WeekDayItem](scheduleCacheMaxSize, scheduleCacheTTL)
	}
	m.scheduleCache.set(key, cloneWeekDayItems(days))
}

func (m *MikanSource) getCachedSubgroups(key string) ([]SubgroupInfo, bool) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.subgroupCache == nil {
		return nil, false
	}
	groups, ok := m.subgroupCache.get(key)
	if !ok {
		return nil, false
	}
	return slices.Clone(groups), true
}

func (m *MikanSource) setCachedSubgroups(key string, groups []SubgroupInfo) {
	if len(groups) == 0 {
		return
	}
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if m.subgroupCache == nil {
		m.subgroupCache = newBoundedTTLCache[[]SubgroupInfo](subgroupCacheMaxSize, subgroupCacheTTL)
	}
	m.subgroupCache.set(key, slices.Clone(groups))
}

func (m *MikanSource) fetchPath(ctx context.Context, path string, weekLabel map[int]string) ([]WeekDayItem, error) {
	domain := m.GetDomain()
	cacheKey := domain + "\x00" + path
	if cached, ok := m.getCachedSchedule(cacheKey); ok {
		return cached, nil
	}

	resp, err := m.tryMirrors(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	// 若页面中没有番剧数据（例如选中的是尚未排表或空的未来/历史季度），返回空列表而非报错 500
	if doc.Find(".sk-bangumi").Length() == 0 {
		return []WeekDayItem{}, nil
	}

	// 抓取完成后再取当前有效域名（tryMirrors 可能自动切换镜像）
	domain = m.GetDomain()
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
				AiredDate:  updateDate,
				InfoHash:   "", // Clear InfoHash as it is not a hash
				CoverURL:   cover,
			})
		})

		if len(items) > 0 {
			result = append(result, WeekDayItem{DayOfWeek: dow, Label: label, Items: items})
		}
	})
	m.setCachedSchedule(cacheKey, result)
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
	domain := m.GetDomain()
	cacheKey := domain + "\x00" + bangumiID
	if cached, ok := m.getCachedSubgroups(cacheKey); ok {
		return cached, nil
	}

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

	domain = m.GetDomain()
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
			rssURL = "https://" + domain + rssHref
		}

		groups = append(groups, SubgroupInfo{
			Name:   groupName,
			RSSURL: rssURL,
		})
	})

	m.setCachedSubgroups(cacheKey, groups)
	return slices.Clone(groups), nil
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
// MikanAnimeInfo 从 Mikan 页面直接抓取的元数据
type MikanAnimeInfo struct {
	SubjectID     string // 详情区链接的 Bangumi.tv 条目号
	TotalEpisodes int
	Year          int
	AnimeType     string
}

// GetAnimeInfo 从 Mikan 详情页通过 HTML 解析抓取基础元数据
func (m *MikanSource) GetAnimeInfo(ctx context.Context, bangumiID string) (*MikanAnimeInfo, error) {
	if bangumiID == "" {
		return nil, fmt.Errorf("bangumiID 不能为空")
	}

	detailURL := fmt.Sprintf("https://%s/Home/Bangumi/%s", m.GetDomain(), bangumiID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mikan 详情页返回状态码: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	info := &MikanAnimeInfo{}
	doc.Find(".bangumi-info a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		link, err := url.Parse(href)
		if err != nil || (link.Scheme != "" && link.Scheme != "https" && link.Scheme != "http") {
			return true
		}
		switch strings.ToLower(link.Hostname()) {
		case "bgm.tv", "bangumi.tv", "chii.in":
			id, ok := strings.CutPrefix(strings.TrimSuffix(link.Path, "/"), "/subject/")
			if n, err := strconv.ParseUint(id, 10, 64); ok && err == nil && n > 0 {
				info.SubjectID = id
				return false
			}
		}
		return true
	})
	// Mikan 详情页通常在 .bangumi-info 区域
	doc.Find(".bangumi-info p").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if strings.Contains(text, "放送开始") {
			// 提取年份，格式通常为 "放送开始： 2025-01-01"
			if m := reFourDigits.FindStringSubmatch(text); len(m) > 1 {
				info.Year, _ = strconv.Atoi(m[1])
			}
		} else if strings.Contains(text, "总集数") {
			// 提取总集数，格式为 "总集数： 12" 或 "总集数： 待定"
			if m := reDigits.FindStringSubmatch(text); len(m) > 1 {
				info.TotalEpisodes, _ = strconv.Atoi(m[1])
			}
		}
	})

	return info, nil
}
