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

// AnimeToshoSource 实现 core.Source 接口，从 animetosho.org 获取资源

type animetoshoRSS struct {
	XMLName xml.Name          `xml:"rss"`
	Channel animetoshoChannel `xml:"channel"`
}

type animetoshoChannel struct {
	Items []animetoshoItem `xml:"item"`
}

type animetoshoItem struct {
	Title       string             `xml:"title"`
	Link        string             `xml:"link"`
	GUID        string             `xml:"guid"`
	PubDate     string             `xml:"pubDate"`
	Description string             `xml:"description"`
	Enclosure   animetoshoEnclosure `xml:"enclosure"`
}

type animetoshoEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
}

type AnimeToshoSource struct {
	httpClient *http.Client
	domain     string
}

func NewAnimeToshoSource(domain string) *AnimeToshoSource {
	if domain == "" {
		domain = "feed.animetosho.org"
	}
	return &AnimeToshoSource{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		domain:     domain,
	}
}

func (at *AnimeToshoSource) Name() string { return "AnimeTosho" }

func (at *AnimeToshoSource) IsAvailable(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+at.domain, nil)
	req.Header.Set("User-Agent", "Ani-Go/1.0")
	resp, err := at.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func (at *AnimeToshoSource) FetchRSS(ctx context.Context, rssURL string) ([]core.TorrentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("AnimeTosho 请求创建失败: %w", err)
	}
	req.Header.Set("User-Agent", "Ani-Go/1.0")

	resp, err := at.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AnimeTosho 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AnimeTosho 返回状态码 %d", resp.StatusCode)
	}

	return at.parseRSS(resp.Body)
}

func (at *AnimeToshoSource) SearchAnime(ctx context.Context, title string) ([]core.TorrentItem, error) {
	query := url.QueryEscape(title)
	rssURL := fmt.Sprintf("https://%s/rss?q=%s", at.domain, query)
	return at.FetchRSS(ctx, rssURL)
}

func (at *AnimeToshoSource) FetchHistory(ctx context.Context, bangumiID string, filter core.Filter) ([]core.TorrentItem, error) {
	query := filter.PreferSubgroup
	if query == "" {
		query = bangumiID
	}
	rssURL := fmt.Sprintf("https://%s/rss?q=%s", at.domain, url.QueryEscape(query))
	return at.FetchRSS(ctx, rssURL)
}

func (at *AnimeToshoSource) parseRSS(r io.Reader) ([]core.TorrentItem, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("读取 AnimeTosho RSS 失败: %w", err)
	}

	var rss animetoshoRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("解析 AnimeTosho RSS 失败: %w", err)
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
			SourceName:  "AnimeTosho",
		})
	}

	return items, nil
}
