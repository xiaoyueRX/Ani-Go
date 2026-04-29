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

// ACGRIPSource 实现 core.Source 接口，从 acg.rip 获取资源

type acgripRSS struct {
	XMLName xml.Name      `xml:"rss"`
	Channel acgripChannel `xml:"channel"`
}

type acgripChannel struct {
	Items []acgripItem `xml:"item"`
}

type acgripItem struct {
	Title       string         `xml:"title"`
	Link        string         `xml:"link"`
	GUID        string         `xml:"guid"`
	PubDate     string         `xml:"pubDate"`
	Enclosure   acgripEnclosure `xml:"enclosure"`
}

type acgripEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
}

type ACGRIPSource struct {
	httpClient *http.Client
	domain     string
}

func NewACGRIPSource(domain string) *ACGRIPSource {
	if domain == "" {
		domain = "acg.rip"
	}
	return &ACGRIPSource{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		domain:     domain,
	}
}

func (a *ACGRIPSource) Name() string { return "ACG.RIP" }

func (a *ACGRIPSource) IsAvailable(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+a.domain, nil)
	req.Header.Set("User-Agent", "Ani-Go/1.0")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func (a *ACGRIPSource) FetchRSS(ctx context.Context, rssURL string) ([]core.TorrentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ACG.RIP 请求创建失败: %w", err)
	}
	req.Header.Set("User-Agent", "Ani-Go/1.0")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ACG.RIP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ACG.RIP 返回状态码 %d", resp.StatusCode)
	}

	return a.parseRSS(resp.Body)
}

func (a *ACGRIPSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	query := url.QueryEscape(title)
	rssURL := fmt.Sprintf("https://%s/.xml?term=%s", a.domain, query)
	return a.FetchRSS(ctx, rssURL)
}

func (a *ACGRIPSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	query := filter.PreferSubgroup
	if query == "" {
		query = bangumiID
	}
	rssURL := fmt.Sprintf("https://%s/.xml?term=%s", a.domain, url.QueryEscape(query))
	return a.FetchRSS(ctx, rssURL)
}

func (a *ACGRIPSource) parseRSS(r io.Reader) ([]core.TorrentItem, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取 ACG.RIP RSS 失败: %w", err)
	}

	var rss acgripRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("解析 ACG.RIP RSS 失败: %w", err)
	}

	items := make([]core.TorrentItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		pubAt, _ := parsePubDate(item.PubDate)
		torrentURL := item.Enclosure.URL
		if torrentURL == "" {
			torrentURL = item.Link
		}

		items = append(items, core.TorrentItem{
			Title:       item.Title,
			URL:         torrentURL,
			Size:        item.Enclosure.Length,
			PublishedAt: pubAt,
			SourceName:  "ACG.RIP",
		})
	}

	return items, nil
}
