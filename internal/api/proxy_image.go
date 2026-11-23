package api

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func allowedImageURL(u *url.URL) bool {
	if u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return false
	}
	if port := u.Port(); port != "" && !(u.Scheme == "http" && port == "80") && !(u.Scheme == "https" && port == "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"i0.hdslb.com", "lain.bgm.tv", "img.mikanani.me", "mikanani.me", "mikanani.kas.pub", "image.tmdb.org", "bilibili.com", "bgm.tv", "mikanime.tv", "yuc.wiki", "yucc.wiki", "yucwiki.net"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func checkImageRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 || !allowedImageURL(req.URL) {
		return errors.New("图片重定向目标无效")
	}
	return nil
}

func serveRasterImage(w http.ResponseWriter, resp *http.Response) {
	const maxSize = 20 << 20
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "图片服务器未返回可用图片", http.StatusBadGateway)
		return
	}
	if resp.ContentLength > maxSize {
		http.Error(w, "图片超过大小限制", http.StatusBadGateway)
		return
	}
	body := bufio.NewReader(resp.Body)
	header, err := body.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "无法读取图片", http.StatusBadGateway)
		return
	}
	mime := http.DetectContentType(header)
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/x-icon":
	default:
		http.Error(w, "不支持的图片格式", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=604800")
	io.Copy(w, io.LimitReader(body, maxSize))
}
