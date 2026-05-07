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
	weekLabel := map[int]string{
		1: "星期一", 2: "星期二", 3: "星期三", 4: "星期四",
		5: "星期五", 6: "星期六", 7: "星期日",
	}

	weekdays := []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}

	// 提取详情卡片: 标题→封面 映射
	titleCover := make(map[string]string)
	for _, card := range regexp.MustCompile(`<div class="div_date"[^>]*>(.*?)</div>\s*</div>\s*</div>`).FindAllString(html, -1) {
		titleM := regexp.MustCompile(`date_title_[^"]*"[^>]*>(.*?)</td>`).FindStringSubmatch(card)
		if titleM == nil {
			continue
		}
		title := strings.TrimSpace(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(titleM[1], ""))
		if title == "" {
			continue
		}

		imgM := regexp.MustCompile(`data-src="(https?://i0\.hdslb\.com[^"]+)"`).FindStringSubmatch(card)
		if imgM != nil {
			titleCover[title] = imgM[1]
		}
	}

	// 解析时间表文本
	cleanText := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")
	cleanText = regexp.MustCompile(`\s+`).ReplaceAllString(cleanText, " ")

	var result []WeekDayItem

	for i, wd := range weekdays {
		// 找到该星期的区间
		wdStart := strings.Index(cleanText, wd+" (")
		if wdStart < 0 {
			continue
		}

		// 找到下一个星期或结尾
		wdEnd := len(cleanText)
		for j := i + 1; j < len(weekdays); j++ {
			nextIdx := strings.Index(cleanText[wdStart+1:], weekdays[j]+" (")
			if nextIdx >= 0 {
				wdEnd = wdStart + 1 + nextIdx
				break
			}
		}

		section := cleanText[wdStart:wdEnd]

		// 提取每个番剧条目: 时间~日期~标题 区域
		entryRe := regexp.MustCompile(`(\d+:\d+)~([\d/]+)~(.+?)(?:\s+(?:环大陆|港台|大陆|网络)\s*)`)
		matches := entryRe.FindAllStringSubmatch(section, -1)

		var items []core.TorrentItem
		for _, m := range matches {
			title := strings.TrimSpace(m[3])
			if title == "" {
				continue
			}

			// 匹配封面图
			cover := ""
			for t, c := range titleCover {
				if strings.Contains(title, t) || strings.Contains(t, title) {
					cover = c
					break
				}
			}
			// 部分匹配
			if cover == "" {
				for t, c := range titleCover {
					if len(t) > 4 && len(title) > 4 {
						if t[:4] == title[:4] {
							cover = c
							break
						}
					}
				}
			}

			items = append(items, core.TorrentItem{
				Title:      title,
				SourceName: "YucWiki",
				InfoHash:   strings.TrimSpace(m[1]),
				BangumiID:  strings.TrimSpace(m[2]),
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
