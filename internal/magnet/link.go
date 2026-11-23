// Package magnet 解析 BitTorrent v1 和 v2 磁力链接的内容标识。
package magnet

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"unicode"
)

var ErrInvalid = errors.New("无效的磁力链接")

type Hashes struct {
	V1 string
	V2 string
}

func Parse(link string) (Hashes, error) {
	var hashes Hashes
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "magnet" || u.Host != "" || u.Path != "" || u.Opaque != "" || u.User != nil || u.Fragment != "" || strings.IndexFunc(link, unicode.IsControl) >= 0 {
		return hashes, ErrInvalid
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return hashes, ErrInvalid
	}
	for _, topic := range query["xt"] {
		topic = strings.ToLower(topic)
		if hash, ok := strings.CutPrefix(topic, "urn:btih:"); ok {
			v1 := NormalizeBTIH(hash)
			if v1 == "" || (hashes.V1 != "" && hashes.V1 != v1) {
				return Hashes{}, ErrInvalid
			}
			hashes.V1 = v1
		}
		if v2, ok := strings.CutPrefix(topic, "urn:btmh:1220"); ok {
			if _, err := hex.DecodeString(v2); err != nil || len(v2) != 64 || (hashes.V2 != "" && hashes.V2 != v2) {
				return Hashes{}, ErrInvalid
			}
			hashes.V2 = v2
		}
	}
	if hashes.V1 == "" && hashes.V2 == "" {
		return Hashes{}, ErrInvalid
	}
	return hashes, nil
}

func NormalizeBTIH(hash string) string {
	switch len(hash) {
	case 40:
		if _, err := hex.DecodeString(hash); err == nil {
			return strings.ToLower(hash)
		}
	case 32:
		decoded, err := base32.StdEncoding.DecodeString(strings.ToUpper(hash))
		if err == nil && len(decoded) == 20 {
			return hex.EncodeToString(decoded)
		}
	}
	return ""
}
