// Package source 提供番剧资源站接口实现
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// YucWikiSource 从 yuc.wiki 获取新番时间表
type YucWikiSource struct {
	httpClient *http.Client
	baseURL    string
	cache      sync.Map
}

// yucCacheItem 内存缓存项
type yucCacheItem struct {
	timestamp time.Time
	data      any
}

func NewYucWikiSource() *YucWikiSource {
	return &YucWikiSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    "https://yuc.wiki",
	}
}

func (y *YucWikiSource) Name() string { return "YucWiki" }

// seasonPath 根据指定年份和季度返回 yuc.wiki 的季度页面路径
// season: 1=1月, 2=4月, 3=7月, 4=10月
func seasonPath(year, season int) string {
	if year <= 0 || season <= 0 {
		now := time.Now()
		year = now.Year()
		m := now.Month()
		switch {
		case m >= 1 && m <= 3:
			season = 1
		case m >= 4 && m <= 6:
			season = 2
		case m >= 7 && m <= 9:
			season = 3
		default:
			season = 4
		}
	}

	var month string
	switch season {
	case 1:
		month = "01"
	case 2:
		month = "04"
	case 3:
		month = "07"
	case 4:
		month = "10"
	default:
		month = "01" // 默认一月
	}

	return fmt.Sprintf("/%d%s/", year, month)
}

// SPTypedGroup SP/OVA/剧场版按月和类型分组
type SPTypedGroup struct {
	Month string             `json:"month"` // "2026年3月"
	Type  string             `json:"type"`  // "剧场版"/"OVA"/"SP"
	Items []core.TorrentItem `json:"items"`
}

// FetchSPItems 获取 SP/OVA/剧场版列表
func (y *YucWikiSource) FetchSPItems(ctx context.Context) ([]SPTypedGroup, error) {
	cacheKey := "sp_items"
	if val, ok := y.cache.Load(cacheKey); ok {
		if item, ok := val.(yucCacheItem); ok {
			if time.Since(item.timestamp) < 6*time.Hour {
				return item.data.([]SPTypedGroup), nil
			}
		}
	}

	url := y.baseURL + "/sp/"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := y.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var groups []SPTypedGroup
	monthRe := regexp.MustCompile(`(\d{4}年\d{1,2}月)`)

	type itemWithType struct {
		item core.TorrentItem
		tp   string
	}

	doc.Find("details").Each(func(i int, det *goquery.Selection) {
		summary := det.Find("summary").Text()
		match := monthRe.FindStringSubmatch(summary)
		if len(match) < 2 {
			return
		}
		month := match[1]

		// 按类型归类本月条目
		typeMap := make(map[string][]core.TorrentItem)
		
		det.Find("div[style='float:left']").Each(func(j int, s *goquery.Selection) {
			titleTd := s.Find("td.sp_title")
			if titleTd.Length() == 0 {
				return
			}

			// 提取类型
			tp := "其他"
			if class, ok := s.Find("td[class^='type-']").Attr("class"); ok {
				switch {
				case strings.Contains(class, "type-m"):
					tp = "剧场版"
				case strings.Contains(class, "type-ova") || strings.Contains(class, "type-o"):
					tp = "OVA"
				case strings.Contains(class, "type-sp") || strings.Contains(class, "type-websp") || 
					strings.Contains(class, "type-tvsp") || strings.Contains(class, "type-s"):
					tp = "SP"
				}
			}

			// 处理标题中的 <br>
			titleTd.Find("br").ReplaceWithHtml(" ")
			title := strings.TrimSpace(titleTd.Text())
			title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")


			// 获取日期
			releaseTd := s.Find("td.sp_release")
			airedDate := strings.TrimSpace(releaseTd.Text())

			// 获取海报
			img := s.Find("img").First()
			cover, _ := img.Attr("data-src")
			if cover == "" {
				cover, _ = img.Attr("src")
			}

			// 转换为 https
			if cover != "" {
				if strings.HasPrefix(cover, "http://") {
					cover = "https://" + cover[7:]
				} else if strings.HasPrefix(cover, "//") {
					cover = "https:" + cover
				}
			}

			if title != "" {
				item := core.TorrentItem{
					Title:      title,
					SourceName: "YucWiki",
					AiredDate:  airedDate,
					CoverURL:   cover,
				}
				typeMap[tp] = append(typeMap[tp], item)
			}
		})

		// 按照 剧场版 -> OVA -> SP -> 其他 的顺序加入组
		typeOrder := []string{"剧场版", "OVA", "SP", "其他"}
		for _, tp := range typeOrder {
			if items, ok := typeMap[tp]; ok && len(items) > 0 {
				groups = append(groups, SPTypedGroup{
					Month: month,
					Type:  tp,
					Items: items,
				})
			}
		}
	})

	y.cache.Store(cacheKey, yucCacheItem{
		timestamp: time.Now(),
		data:      groups,
	})

	return groups, nil
}

// FetchWeekSchedule 获取按星期分组的新番时间表
func (y *YucWikiSource) FetchWeekSchedule(ctx context.Context, year, season int) ([]WeekDayItem, error) {
	cacheKey := fmt.Sprintf("schedule:%d:%d", year, season)
	if val, ok := y.cache.Load(cacheKey); ok {
		if item, ok := val.(yucCacheItem); ok {
			if time.Since(item.timestamp) < 6*time.Hour {
				return item.data.([]WeekDayItem), nil
			}
		}
	}

	url := y.baseURL + seasonPath(year, season)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := y.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result, err := parseYucWiki(string(body))
	if err != nil {
		return nil, err
	}

	y.cache.Store(cacheKey, yucCacheItem{
		timestamp: time.Now(),
		data:      result,
	})

	return result, nil
}

func parseYucWiki(html string) ([]WeekDayItem, error) {
	weekLabel := map[int]string{
		1: "星期一", 2: "星期二", 3: "星期三", 4: "星期四",
		5: "星期五", 6: "星期六", 7: "星期日",
	}

	// 解析 html 获取封面图映射
	coverMap := make(map[string]string)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err == nil {
		doc.Find(".date_title, .date_title_, .date_title__").Each(func(i int, s *goquery.Selection) {
			// 将 <br> 替换为空格，并去除多余空白
			s.Find("br").ReplaceWithHtml(" ")
			title := strings.TrimSpace(s.Text())
			// yucwiki 的标题可能有换行，清洗一下
			title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
			
			parentDiv := s.ParentsFiltered("div").Parent()
			img := parentDiv.Find("img").First()
			src, _ := img.Attr("data-src")
			if src == "" {
				src, _ = img.Attr("src")
			}
			if src != "" {
				// 转换为 https 并处理 bilibili 链接
				if strings.HasPrefix(src, "http://") {
					src = "https://" + src[7:]
				} else if strings.HasPrefix(src, "//") {
					src = "https:" + src
				}
			}
			if title != "" && src != "" {
				coverMap[title] = src
			}
		})
	}

	// 去除 HTML 标签得到纯文本
	cleanText := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")
	cleanText = regexp.MustCompile(`\s+`).ReplaceAllString(cleanText, " ")

	weekdays := []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}

	var result []WeekDayItem

	for i, wd := range weekdays {
		// 找到该星期的区间
		wdPattern := wd + " ("
		wdStart := strings.Index(cleanText, wdPattern)
		if wdStart < 0 {
			continue
		}

		// 找到下一个星期或结尾
		wdEnd := len(cleanText)
		for j := i + 1; j < len(weekdays); j++ {
			nextPattern := weekdays[j] + " ("
			nextIdx := strings.Index(cleanText[wdStart+len(wdPattern):], nextPattern)
			if nextIdx >= 0 {
				candidate := wdStart + len(wdPattern) + nextIdx
				if candidate < wdEnd {
					wdEnd = candidate
				}
			}
		}

		section := cleanText[wdStart:wdEnd]

		// 提取番剧条目: 时间~日期~标题 OR 完结 (全xx话) 标题
		entryRe := regexp.MustCompile(`(?:(\d+:\d+)~\s*([\d/]+)~|完结\s*(?:\(全\d+话\))?)\s*(.+?)(?:\s+(?:环大陆|港台|大陆|网络|完结)\s*|\s+\d+:\d+~|$)`)
		matches := entryRe.FindAllStringSubmatch(section, -1)

		var items []core.TorrentItem
		for _, m := range matches {
			title := strings.TrimSpace(m[3])
			if title == "" {
				continue
			}

			// 匹配封面图，清理标题空白
			cleanTitle := regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
			cover := coverMap[cleanTitle]

			items = append(items, core.TorrentItem{
				Title:      title,
				SourceName: "YucWiki",
				AiredTime:  strings.TrimSpace(m[1]),
				AiredDate:  strings.TrimSpace(m[2]),
				CoverURL:   cover,
			})
		}

		if len(items) > 0 {
			result = append(result, WeekDayItem{
				DayOfWeek: i + 1,
				Label:     weekLabel[i+1],
				Items:     items,
			})
		}
	}

	return result, nil
}
