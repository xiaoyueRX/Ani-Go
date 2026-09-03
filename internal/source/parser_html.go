// Package source 实现各资源站点的 Source 接口
// Mikan HTML 解析与测速逻辑
package source

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// ============================================================
// Mikan 搜索结果 HTML 解析
// ============================================================

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
		
		// AllowedSubgroups 过滤：如果指定了允许的字幕组列表且当前组不在列表中则跳过
		if len(filter.AllowedSubgroups) > 0 {
			allowed := false
			for _, allowedGroup := range filter.AllowedSubgroups {
				if strings.Contains(groupName, allowedGroup) {
					allowed = true
					break
				}
			}
			if !allowed {
				return
			}
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
					if table.Length() == 0 {
						log.Printf("⚠️ 找不到 table (anchorID=%s)", anchorID)
						return
					}
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
			Resolution:  info.Resolution,
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