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
		seasonStr = fmt.Sprintf("%d01", y-1)
	case m >= 4 && m <= 6:
		seasonStr = fmt.Sprintf("%d04", y)
	case m >= 7 && m <= 9:
		seasonStr = fmt.Sprintf("%d07", y)
	default:
		seasonStr = fmt.Sprintf("%d10", y)
	}
	return "/" + seasonStr + "/"
}

// FetchWeekSchedule 获取按星期分组的新番时间表
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

		// 提取番剧条目: 时间~日期~标题
		entryRe := regexp.MustCompile(`(\d+:\d+)~\s*([\d/]+)~\s*(.+?)(?:\s+(?:环大陆|港台|大陆|网络)\s*|\s+\d+:\d+~|\s*$)`)
		matches := entryRe.FindAllStringSubmatch(section, -1)

		var items []core.TorrentItem
		for _, m := range matches {
			title := strings.TrimSpace(m[3])
			if title == "" {
				continue
			}

			items = append(items, core.TorrentItem{
				Title:      title,
				SourceName: "YucWiki",
				InfoHash:   strings.TrimSpace(m[1]),
				BangumiID:  strings.TrimSpace(m[2]),
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
