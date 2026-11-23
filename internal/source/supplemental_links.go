package source

import (
	"cmp"
	"net/url"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
)

func setSupplementalLinks(item *core.TorrentItem, enclosure, link, publicBTIH string) {
	enclosure, link = strings.TrimSpace(enclosure), strings.TrimSpace(link)
	item.URL = enclosure
	if item.URL == "" {
		item.URL = link
	}
	if hashes, err := magnet.Parse(item.URL); err == nil {
		item.MagnetURL = item.URL
		item.InfoHash = cmp.Or(hashes.V1, hashes.V2)
		return
	}
	u, err := url.Parse(item.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return
	}
	// 下载器继续使用原种子地址，插件可以独立使用 RSS 给出的完整磁力。
	if hashes, err := magnet.Parse(link); err == nil {
		item.AlternateMagnet = link
		item.InfoHash = cmp.Or(hashes.V1, hashes.V2)
	} else if publicBTIH != "" {
		item.AlternateMagnet = "magnet:?xt=urn:btih:" + publicBTIH
		item.InfoHash = publicBTIH
	}
}
