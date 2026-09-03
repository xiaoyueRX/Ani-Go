package source

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type YucWikiSource struct {
	httpClient *http.Client
	baseURL    string
	cache      sync.Map
}

type yucCacheItem struct {
	timestamp time.Time
	data      interface{}
}

func NewYucWikiSource() *YucWikiSource {
	// YucWiki 证书常年过期，必须用 Insecure
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &YucWikiSource{
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   20 * time.Second,
		},
		baseURL: "https://yuc.wiki",
	}
}

func (y *YucWikiSource) Name() string { return "YucWiki" }

func (y *YucWikiSource) FetchRSS(ctx context.Context, url string) ([]core.TorrentItem, error) {
	return nil, fmt.Errorf("YucWiki 不支持 RSS 抓取")
}

func (y *YucWikiSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	return nil, fmt.Errorf("YucWiki 不支持番剧搜索")
}

func (y *YucWikiSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	return nil, fmt.Errorf("YucWiki 不支持历史记录抓取")
}

func (y *YucWikiSource) IsAvailable(ctx context.Context) bool {
	return true
}

func (y *YucWikiSource) seasonPath(year, season int) string {
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
		month = "01"
	}

	// 历史年份适配：实测 2024 年及以后都是目录格式
	if year < 2024 {
		return fmt.Sprintf("/%d%s.htm", year, month)
	}
	return fmt.Sprintf("/%d%s/", year, month)
}

func (y *YucWikiSource) FetchWeekSchedule(ctx context.Context, year, season int) ([]WeekDayItem, error) {
	cacheKey := fmt.Sprintf("schedule:%d:%d", year, season)
	if val, ok := y.cache.Load(cacheKey); ok {
		if item, ok := val.(yucCacheItem); ok {
			if time.Since(item.timestamp) < 6*time.Hour {
				return item.data.([]WeekDayItem), nil
			}
		}
	}

	url := y.baseURL + y.seasonPath(year, season)

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

	result, err := y.parseYucWiki(string(body))
	if err != nil {
		return nil, err
	}

	y.cache.Store(cacheKey, yucCacheItem{
		timestamp: time.Now(),
		data:      result,
	})

	return result, nil
}

func (y *YucWikiSource) parseYucWiki(html string) ([]WeekDayItem, error) {
	weekLabel := map[int]string{
		1: "星期一", 2: "星期二", 3: "星期三", 4: "星期四",
		5: "星期五", 6: "星期六", 7: "星期日",
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	// 1. 建立封面图映射
	type meta struct {
		src        string
		totalEps   int
		isFinished bool
	}
	coverMap := make(map[string]meta)
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("data-src")
		if src == "" {
			src, _ = s.Attr("src")
		}
		if src == "" {
			return
		}

		// yuc.wiki 结构：<div class="div_date"><img src="..."></div><div><table>...<td class="date_title">标题</td>
		var title string
		if divDate := s.Closest(".div_date"); divDate.Length() > 0 {
			// 排除那些“版权方/播放平台”的 logo 图（这类图通常被包在 td 里，且没有 div_date 父节点）
			// 但 yuc.wiki 的番剧封面是在 div.div_date 里的直接子节点
			titleNode := divDate.Next().Find("td.date_title, td.date_title_, td.date_title__").First()
			tClone := titleNode.Clone()
			tClone.Find("br").ReplaceWithHtml(" ")
			title = strings.TrimSpace(tClone.Text())
			title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
		}

		// 3. 提取总集数信息 (yuc.wiki 结构：td.date_title 后的 td.date_title_)
		var totalEps int
		var isFinished bool
		if divDate := s.Closest(".div_date"); divDate.Length() > 0 {
			if titleNode := divDate.Next().Find("td.date_title, td.date_title_, td.date_title__"); titleNode.Length() > 0 {
				titleNode.Each(func(idx int, sel *goquery.Selection) {
					txt := sel.Text()
					if strings.Contains(txt, "全") && strings.Contains(txt, "话") {
						re := regexp.MustCompile(`全(\d+)话`)
						if matches := re.FindStringSubmatch(txt); len(matches) > 1 {
							totalEps, _ = strconv.Atoi(matches[1])
							isFinished = true
						}
					}
				})
			}
		}

		// 增加硬核过滤：yuc.wiki 的巴哈姆特/B站版权占位图 md5 或 特征 URL
		if strings.Contains(src, "d06ad4b201012067db7d59c9dedfadeb6da75cf3.jpg") || 
		   strings.Contains(src, "dec51811ba1216627f761e1a730be1f3512995925.jpg") {
			return
		}

			if title != "" {
				if strings.HasPrefix(src, "//") {
					src = "https:" + src
				} else if strings.HasPrefix(src, "http://") {
					src = "https" + src[4:]
				}
				// 移除 URL 中的尺寸限制后缀，获取原图
				if idx := strings.Index(src, "?"); idx != -1 {
					src = src[:idx]
				}
				coverMap[title] = meta{
					src:        src,
					totalEps:   totalEps,
					isFinished: isFinished,
				}
			}
	})

	// 2. 按星期分组抓取番剧标题
	weekdaysData := make(map[int][]string)
	doc.Find("td.date2").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		var day int
		switch {
		case strings.Contains(text, "一"): day = 1
		case strings.Contains(text, "二"): day = 2
		case strings.Contains(text, "三"): day = 3
		case strings.Contains(text, "四"): day = 4
		case strings.Contains(text, "五"): day = 5
		case strings.Contains(text, "六"): day = 6
		case strings.Contains(text, "日"): day = 7
		default: return
		}

		var titles []string
		container := s.Closest("div")
		if container.Length() == 0 { container = s.Closest("table") }
		
		curr := container.Next()
		for curr.Length() > 0 {
			if curr.Find("td.date2").Length() > 0 { break }
			
			curr.Find("td.date_title, td.date_title_, td.date_title__").Each(func(j int, ts *goquery.Selection) {
				tn := ts.Clone()
				tn.Find("br").ReplaceWithHtml(" ")
				t := strings.TrimSpace(tn.Text())
				t = regexp.MustCompile(`\s+`).ReplaceAllString(t, " ")
				if t != "" { titles = append(titles, t) }
			})
			curr = curr.Next()
		}
		if len(titles) > 0 {
			weekdaysData[day] = titles
		}
	})

	// 3. 组装结果
	var result []WeekDayItem
	for i := 1; i <= 7; i++ {
		item := WeekDayItem{
			DayOfWeek: i,
			Label:     weekLabel[i],
			Items:     []core.TorrentItem{},
		}
		if titles, ok := weekdaysData[i]; ok {
			for _, t := range titles {
				metaData := coverMap[t]
				cover := metaData.src
				if cover == "" { cover = "/placeholder.jpg" }
				item.Items = append(item.Items, core.TorrentItem{
					Title:         t,
					CoverURL:      cover,
					TotalEpisodes: metaData.totalEps,
					IsFinished:    metaData.isFinished,
				})
			}
		}
		result = append(result, item)
	}
	return result, nil
}

func (y *YucWikiSource) FetchSPItems(ctx context.Context, year, season int) ([]core.TorrentItem, error) {
	url := y.baseURL + y.seasonPath(year, season)
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

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	var spItems []core.TorrentItem
	// YucWiki 的 SP/剧场版通常在 "网络放送 & 其他" 栏目下，或者没有明确日期标记的区域
	// 这里通过查找 "网络放送" 标题后的内容来提取
	doc.Find("td.date2").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		if strings.Contains(text, "网络") || strings.Contains(text, "其他") || strings.Contains(text, "剧场") {
			container := s.Closest("div")
			if container.Length() == 0 { container = s.Closest("table") }

			curr := container.Next()
			for curr.Length() > 0 {
				if curr.Find("td.date2").Length() > 0 { break }
				curr.Find("td.date_title, td.date_title_, td.date_title__").Each(func(j int, ts *goquery.Selection) {
					tNode := ts.Clone()
					tNode.Find("br").ReplaceWithHtml(" ")
					title := strings.TrimSpace(tNode.Text())
					title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
					if title != "" {
						spItems = append(spItems, core.TorrentItem{
							Title: title,
						})
					}
				})
				curr = curr.Next()
			}
		}
	})

	return spItems, nil
}

// ParseYucWikiForTest 导出 parseYucWiki 供测试使用
func (y *YucWikiSource) ParseYucWikiForTest(html string) ([]WeekDayItem, error) {
	return y.parseYucWiki(html)
}
