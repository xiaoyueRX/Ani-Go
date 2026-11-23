package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
)

// NyaaSource 实现 core.Source 接口，从 nyaa.si 获取资源

// nyaaRSS Nyaa RSS 2.0 结构体
type nyaaRSS struct {
	XMLName xml.Name    `xml:"rss"`
	Channel nyaaChannel `xml:"channel"`
}

type nyaaChannel struct {
	Items []nyaaItem `xml:"item"`
}

type nyaaItem struct {
	Title     string        `xml:"title"`
	Link      string        `xml:"link"`
	GUID      string        `xml:"guid"`
	PubDate   string        `xml:"pubDate"`
	Enclosure nyaaEnclosure `xml:"enclosure"`
	InfoHash  nyaaInfoHash  `xml:"infoHash"`
}

type nyaaInfoHash struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

func (h nyaaInfoHash) publicBTIH(domain string) string {
	// Nyaa 以当前站点的 /xmlns/nyaa 声明扩展，镜像也可保留官方命名空间。
	ns, err := url.Parse(h.XMLName.Space)
	if err != nil || (ns.Scheme != "http" && ns.Scheme != "https") || ns.RequestURI() != "/xmlns/nyaa" || ns.Fragment != "" || ns.User != nil ||
		(!strings.EqualFold(ns.Host, "nyaa.si") && !strings.EqualFold(ns.Host, domain)) {
		return ""
	}
	hash := strings.TrimSpace(h.Value)
	if len(hash) != 40 {
		return ""
	}
	return magnet.NormalizeBTIH(hash)
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
		httpClient: httpx.New(30 * time.Second),
		domain:     domain,
	}
}

func (n *NyaaSource) Name() string { return "Nyaa" }

func (n *NyaaSource) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+n.domain, nil)
	if err != nil {
		return false
	}
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
		return nil, fmt.Errorf("Nyaa 返回状态码 %d", resp.StatusCode)
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
	return searchHistoryTitles(ctx, filter.Titles, n.SearchAnime)
}

func (n *NyaaSource) parseRSS(r io.Reader) ([]core.TorrentItem, error) {
	var rss nyaaRSS
	if err := xml.NewDecoder(r).Decode(&rss); err != nil {
		return nil, fmt.Errorf("解析 Nyaa RSS 失败: %w", err)
	}

	items := make([]core.TorrentItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		pubAt, _ := parsePubDate(item.PubDate)
		result := core.TorrentItem{
			Title:       item.Title,
			Size:        item.Enclosure.Length,
			PublishedAt: pubAt,
			SourceName:  "Nyaa",
		}
		setSupplementalLinks(&result, item.Enclosure.URL, item.Link, item.InfoHash.publicBTIH(n.domain))
		items = append(items, result)
	}

	return items, nil
}
