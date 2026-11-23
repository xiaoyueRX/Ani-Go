package source

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestSupplementalSearchPublishesMagnetsWithoutChangingDownloadURL(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	const torrent = "https://publisher.example/download/123.torrent"
	const base = "magnet:?xt=urn:btih:" + hash
	const full = base + "&dn=Fixture&tr=https%3A%2F%2Ftracker.example%2Fannounce"
	const info = `<n:infoHash>0123456789ABCDEF0123456789ABCDEF01234567</n:infoHash>`
	const base32Magnet = "magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	v2 := strings.Repeat("b1", 32)
	v2Magnet := "magnet:?xt=urn:btmh:1220" + v2
	var feed string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "Fixture" && r.URL.Query().Get("term") != "Fixture" {
			t.Errorf("搜索请求缺少关键词：%s", r.URL)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, feed)
	}))
	defer server.Close()
	domain := strings.TrimPrefix(server.URL, "https://")
	n, a, at := NewNyaaSource(domain), NewACGRIPSource(domain), NewAnimeToshoSource(domain)
	n.httpClient, a.httpClient, at.httpClient = server.Client(), server.Client(), server.Client()

	for _, src := range []core.Source{n, a, at} {
		t.Run(src.Name(), func(t *testing.T) {
			type example struct {
				name, enclosure, link, extra string
				url, magnet, alternate, hash string
			}
			items := []example{
				{"种子包及显式磁力", torrent, full, "", torrent, "", full, hash},
				{"磁力 enclosure", full, "https://publisher.example/view/123", "", full, full, "", hash},
				{"磁力 link", "", base32Magnet, "", base32Magnet, base32Magnet, "", strings.Repeat("0", 40)},
				{"纯 v2 磁力", "", v2Magnet, "", v2Magnet, v2Magnet, "", v2},
				{"普通下载地址", torrent, "https://publisher.example/view/123", "", torrent, "", "", ""},
				{"畸形磁力", torrent, "magnet:?xt=urn:btih:short", "", torrent, "", "", ""},
				{"无关字段含散列", "", "https://publisher.example/" + hash + ".torrent", `<description><![CDATA[` + full + `]]></description><guid>` + hash + `</guid><infoHash>` + hash + `</infoHash>`, "https://publisher.example/" + hash + ".torrent", "", "", ""},
				{"其他命名空间", "", torrent, `<other:infoHash>` + hash + `</other:infoHash>`, torrent, "", "", ""},
				{"无效来源散列", "", torrent, `<n:infoHash>short</n:infoHash>`, torrent, "", "", ""},
				{"显式磁力保留查询参数", torrent, full, info, torrent, "", full, hash},
			}
			for _, ns := range []string{"n", "mirror"} {
				x := example{"来源声明 " + ns, "", torrent, "<" + ns + ":infoHash>" + strings.ToUpper(hash) + "</" + ns + ":infoHash>", torrent, "", "", ""}
				if src.Name() == "Nyaa" {
					x.alternate, x.hash = base, hash
				}
				items = append(items, x)
			}
			var xml strings.Builder
			fmt.Fprintf(&xml, `<rss version="2.0" xmlns:n="https://nyaa.si/xmlns/nyaa" xmlns:mirror="%s/xmlns/nyaa" xmlns:other="https://example.com/xmlns/other"><channel>`, server.URL)
			for _, x := range items {
				fmt.Fprintf(&xml, `<item><title>%s</title><link>%s</link>%s`, x.name, html.EscapeString(x.link), x.extra)
				if x.enclosure != "" {
					fmt.Fprintf(&xml, `<enclosure url="%s" type="application/x-bittorrent" length="12345"/>`, html.EscapeString(x.enclosure))
				}
				xml.WriteString(`</item>`)
			}
			xml.WriteString(`</channel></rss>`)
			feed = xml.String()
			results, err := src.SearchAnime(context.Background(), "Fixture")
			if err != nil || len(results) != len(items) {
				t.Fatalf("搜索得到 %d 个结果，错误 %v", len(results), err)
			}
			for i, x := range items {
				r := results[i]
				data, err := json.Marshal(r)
				var wire map[string]any
				if err != nil || json.Unmarshal(data, &wire) != nil {
					t.Fatalf("序列化搜索结果失败：%v", err)
				}
				alt, hasAlt := wire["alternate_magnet"]
				if r.Title != x.name || r.URL != x.url || r.MagnetURL != x.magnet || r.InfoHash != x.hash || hasAlt != (x.alternate != "") || (hasAlt && alt != x.alternate) {
					t.Errorf("%s：url=%q magnet=%q alternate=%v hash=%q", x.name, r.URL, r.MagnetURL, alt, r.InfoHash)
				}
			}
		})
	}
}
