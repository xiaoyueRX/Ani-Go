package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// NyaaSource 实现 core.Source 接口，从 nyaa.si 获取资源

// nyaaRSS Nyaa RSS 2.0 结构体
type nyaaRSS struct {
	XMLName xml.Name   `xml:"rss"`
	Channel nyaaChannel `xml:"channel"`
}

type nyaaChannel struct {
	Items []nyaaItem `xml:"item"`
}

type nyaaItem struct {
	Title       string      `xml:"title"`
	Link        string      `xml:"link"`
	GUID        string      `xml:"guid"`
	PubDate     string      `xml:"pubDate"`
	Enclosure   nyaaEnclosure `xml:"enclosure"`
}

type nyaaEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
}

type NyaaSource struct {
	httpClient *http.Client
	domain     string
}

func NewNyaaSource(domain string) *NyaaSource {
	if domain == "" {
		domain = "nyaa.si"
	}
	return &NyaaSource{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		domain:     domain,
	}
}

func (n *NyaaSource) Name() string { return "Nyaa" }

func (n *NyaaSource) IsAvailable(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+n.domain, nil)
	req.Header.Set("User-Agent", "Ani-Go/1.0")
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func (n *NyaaSource) FetchRSS(ctx context.Context, rssURL string) ([]core.TorrentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Nyaa 请求创建失败: %w", err)
	}
	req.Header.Set("User-Agent", "Ani-Go/1.0")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Nyaa 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Nyaa 返回状态码 %d: %s", resp.StatusCode, string(body[:min(300, len(body))]))
	}

	return n.parseRSS(resp.Body)
}

func (n *NyaaSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	// Nyaa 搜索: anime category (1_2), English-translated filter
	query := url.QueryEscape(title)
	rssURL := fmt.Sprintf("https://%s/?page=rss&q=%s&c=1_2&f=0", n.domain, query)
	return n.FetchRSS(ctx, rssURL)
}

func (n *NyaaSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	// Nyaa 没有番剧 ID 概念，使用关键词搜索
	query := url.QueryEscape(filter.PreferSubgroup)
	if query == "" {
		query = bangumiID
	}
	rssURL := fmt.Sprintf("https://%s/?page=rss&q=%s&c=1_2&f=0", n.domain, query)
	return n.FetchRSS(ctx, rssURL)
}

func (n *NyaaSource) parseRSS(r io.Reader) ([]core.TorrentItem, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取 Nyaa RSS 失败: %w", err)
	}

	var rss nyaaRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("解析 Nyaa RSS 失败: %w", err)
	}

	items := make([]core.TorrentItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		pubAt, _ := parsePubDate(item.PubDate)
		// Nyaa enclosure 通常包含 torrent 文件 URL
		torrentURL := item.Enclosure.URL
		if torrentURL == "" {
			torrentURL = item.Link
		}

		items = append(items, core.TorrentItem{
			Title:       item.Title,
			URL:         torrentURL,
			Size:        item.Enclosure.Length,
			PublishedAt: pubAt,
			SourceName:  "Nyaa",
		})
	}

	return items, nil
}
