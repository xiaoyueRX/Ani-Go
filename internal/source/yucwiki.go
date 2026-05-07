// Package source 提供番剧资源站接口实现
package source

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// YucWikiSource 从 yuc.wiki 获取新番时间表
// yuc.wiki 是手动维护的日本动画时间表站，提供标准海报图
type YucWikiSource struct {
	httpClient *http.Client
	baseURL    string
	cache      sync.Map
}

func NewYucWikiSource() *YucWikiSource {
	return &YucWikiSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    "https://yuc.wiki",
	}
}

func (y *YucWikiSource) Name() string { return "YucWiki" }

// seasonPath 根据当前日期返回 yuc.wiki 的季度页面路径
func seasonPath() string {
	now := time.Now()
	y, m := now.Year(), now.Month()
	var seasonStr string
	switch {
	case m >= 1 && m <= 3:
		seasonStr = fmt.Sprintf("%d01", y-1) // 冬季番属于上一年
	case m >= 4 && m <= 6:
		seasonStr = fmt.Sprintf("%d04", y) // 春季
	case m >= 7 && m <= 9:
		seasonStr = fmt.Sprintf("%d07", y) // 夏季
	default:
		seasonStr = fmt.Sprintf("%d10", y) // 秋季
	}
	return "/" + seasonStr + "/"
}

// FetchWeekSchedule 获取按星期分组的新番时间表（含海报图）
func (y *YucWikiSource) FetchWeekSchedule(ctx context.Context) ([]WeekDayItem, error) {
	url := y.baseURL + seasonPath()

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

	return parseYucWiki(string(body))
}

func parseYucWiki(html string) ([]WeekDayItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	// === 1. 提取详情卡片的 标题→封面 映射 ===
	titleCover := make(map[string]string)
	doc.Find("div.div_date").Each(func(_ int, card *goquery.Selection) {
		// 标题在 .date_title_ 中
		titleEl := card.Find(".date_title_")
		title := strings.TrimSpace(titleEl.Text())
		if title == "" {
			return
		}
		// 取主标题（br 前的内容，不含副标题）
		if idx := strings.Index(title, "\n"); idx > 0 {
			title = title[:idx]
		}
		title = strings.TrimSpace(title)
		if title == "" {
			return
		}

		// 封面图在 img[data-src] 中（优先取 hdslb 的图）
		cover := ""
		card.Find("img[data-src]").Each(func(_ int, img *goquery.Selection) {
			src, exists := img.Attr("data-src")
			if exists && strings.Contains(src, "hdslb.com") {
				cover = src
			}
		})

		if cover != "" && title != "" {
			titleCover[title] = cover
		}
	})

	// 也提取 og:image 作为备用
	ogImages := []string{}
	doc.Find("meta[property='og:image']").Each(func(_ int, sel *goquery.Selection) {
		content, exists := sel.Attr("content")
		if exists {
			ogImages = append(ogImages, content)
		}
	})

	// === 2. 解析文本时间表 ===
	weekLabel := map[string]int{
		"周一": 1, "周二": 2, "周三": 3, "周四": 4,
		"周五": 5, "周六": 6, "周日": 7,
	}
	dayOrder := []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}

	// 获取页面文本
	pageText := doc.Find("body").Text()
	pageText = strings.ReplaceAll(pageText, "\n", " ")
	pageText = strings.ReplaceAll(pageText, "\t", " ")

	// 按星期分割
	var result []WeekDayItem
	imgIdx := 0

	for _, dayName := range dayOrder {
		// 找到星期标题的位置
		dayPattern := dayName + " "
		idx := strings.Index(pageText, dayPattern)
		if idx < 0 {
			continue
		}
		start := idx + len(dayName) + 3 // 跳过 "周一 (月) "

		// 找到下一个星期或结尾
		end := len(pageText)
		for _, nextDay := range dayOrder {
			if nextDay == dayName {
				continue
			}
			if ni := strings.Index(pageText[start:], nextDay+" "); ni > 0 && start+ni < end {
				end = start + ni
			}
		}

		section := pageText[start:end]

		// 解析该天的每个番剧条目
		// 格式: 21:00~4/6~标题 区域
		re := regexp.MustCompile(`(\d+:\d+)~([\d/]+)~(.+?)(?:\s+(?:环大陆|港台|大陆|网络)\s*)`)
		matches := re.FindAllStringSubmatch(section, -1)

		var items []core.TorrentItem
		for _, m := range matches {
			title := strings.TrimSpace(m[3])
			if title == "" {
				continue
			}

			// 尝试匹配封面
			cover := ""
			if c, ok := titleCover[title]; ok {
				cover = c
			} else {
				// 尝试部分匹配
				for t, c := range titleCover {
					if strings.Contains(title, t) || strings.Contains(t, title) {
						cover = c
						break
					}
				}
			}

			// 备用：用 og:image 按位置匹配
			if cover == "" && imgIdx < len(ogImages) {
				cover = ogImages[imgIdx]
			}
			imgIdx++

			items = append(items, core.TorrentItem{
				Title:      title,
				SourceName: "YucWiki",
				InfoHash:   strings.TrimSpace(m[1]), // 放送时间
				BangumiID:  strings.TrimSpace(m[2]), // 开始日期
				CoverURL:   cover,
			})
		}

		if len(items) > 0 {
			result = append(result, WeekDayItem{
				DayOfWeek: weekLabel[dayName],
				Label:     dayName,
				Items:     items,
			})
		}
	}

	if len(result) == 0 {
		log.Printf("⚠️ yuc.wiki 解析结果为空，请检查页面结构")
	}

	return result, nil
}
