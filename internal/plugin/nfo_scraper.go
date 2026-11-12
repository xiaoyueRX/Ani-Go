package plugin

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

const (
	// posterDownloadTimeout 封面海报下载超时时间，兼顾 EventBus 5 秒处理器预算
	posterDownloadTimeout = 4 * time.Second
	// maxPosterSize 封面海报单文件最大体积限制 (10 MiB)
	maxPosterSize = 10 * 1024 * 1024
)

// seasonDirPattern 匹配标准季目录或特典目录（避免误伤名称中包含 Season 的番剧根目录）
var seasonDirPattern = regexp.MustCompile(`(?i)^(?:season[\s._-]*\d*|s\d{1,3}|specials?|sps?|ovas?|oad|extras?|第\s*[0-9一二三四五六七八九十百]+\s*季)$`)

// UniqueID Kodi/Jellyfin/Emby 标准多源唯一标识节点
type UniqueID struct {
	Type    string `xml:"type,attr"`
	Default bool   `xml:"default,attr,omitempty"`
	Value   string `xml:",chardata"`
}

// TVShowNFO Kodi/Jellyfin/Emby 兼容的 tvshow.nfo 结构
type TVShowNFO struct {
	XMLName       xml.Name   `xml:"tvshow"`
	Title         string     `xml:"title"`
	OriginalTitle string     `xml:"originaltitle,omitempty"`
	ShowTitle     string     `xml:"showtitle,omitempty"`
	Year          int        `xml:"year,omitempty"`
	Season        int        `xml:"season,omitempty"`
	Plot          string     `xml:"plot,omitempty"`
	Thumb         string     `xml:"thumb,omitempty"`
	TMDBID        string     `xml:"tmdbid,omitempty"`
	IMDBID        string     `xml:"imdbid,omitempty"`
	BangumiID     string     `xml:"bangumiid,omitempty"`
	UniqueIDs     []UniqueID `xml:"uniqueid,omitempty"`
}

// EpisodeNFO Kodi/Jellyfin/Emby 兼容的 <episode>.nfo 结构
type EpisodeNFO struct {
	XMLName   xml.Name `xml:"episodedetails"`
	Title     string   `xml:"title"`
	ShowTitle string   `xml:"showtitle,omitempty"`
	Season    int      `xml:"season"`
	Episode   float32  `xml:"episode"`
	Plot      string   `xml:"plot,omitempty"`
	Aired     string   `xml:"aired,omitempty"`
}

// MovieNFO Kodi/Jellyfin/Emby 兼容的 movie.nfo 结构
type MovieNFO struct {
	XMLName       xml.Name   `xml:"movie"`
	Title         string     `xml:"title"`
	OriginalTitle string     `xml:"originaltitle,omitempty"`
	Year          int        `xml:"year,omitempty"`
	Plot          string     `xml:"plot,omitempty"`
	Thumb         string     `xml:"thumb,omitempty"`
	TMDBID        string     `xml:"tmdbid,omitempty"`
	IMDBID        string     `xml:"imdbid,omitempty"`
	BangumiID     string     `xml:"bangumiid,omitempty"`
	UniqueIDs     []UniqueID `xml:"uniqueid,omitempty"`
}

// NFOScraperPlugin Bangumi NFO 媒体刮削器插件
type NFOScraperPlugin struct {
	mu                  sync.RWMutex
	enabled             bool
	subID               core.SubscriptionID
	subBus              core.EventBus
	runCtx              context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	fileMu              sync.Mutex
	posterMu            sync.Mutex
	posterInFlight      map[string]struct{}
	allowPrivateForTest bool // 仅限单元测试使用，允许访问 httptest 本地回环服务器
}

// NewNFOScraperPlugin 创建 NFO 刮削器插件实例
func NewNFOScraperPlugin() *NFOScraperPlugin {
	return &NFOScraperPlugin{
		posterInFlight: make(map[string]struct{}),
	}
}

func (p *NFOScraperPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "nfo_scraper",
		Name:        "Bangumi NFO 刮削器",
		Description: "媒体库元数据自动生成插件。当剧集整理入库后，自动为 Jellyfin / Emby / Kodi 生成标准 tvshow.nfo、movie.nfo 与 episode.nfo，下载海报并关联 Bangumi / TMDB / IMDB 信息。",
		Version:     "1.1.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "Film",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{core.EventFileOrganized},
	}
}

// Init 启用插件：幂等订阅文件整理完成事件
func (p *NFOScraperPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 防止重复调用 Init 造成重复订阅或旧上下文泄漏
	if p.subID != 0 {
		unsubBus := p.subBus
		if unsubBus == nil {
			unsubBus = bus
		}
		if unsubBus != nil {
			unsubBus.Unsubscribe(core.EventFileOrganized, p.subID)
		}
		p.subID = 0
		p.subBus = nil
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	p.runCtx, p.cancel = context.WithCancel(context.Background())
	p.enabled = true
	p.subBus = bus
	if bus != nil {
		p.subID = bus.Subscribe(core.EventFileOrganized, func(ev core.Event) {
			p.handleFileOrganized(ev)
		})
	}
	log.Println("🔌 [插件] Bangumi NFO 刮削器已启用，将在剧集整理入库时自动生成媒体元数据")
	return nil
}

// Stop 停用插件：注销事件监听并等待在途任务退出，保证 0 协程、0 网络、0 文件写入
func (p *NFOScraperPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	wasEnabled := p.enabled
	p.enabled = false
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.runCtx = nil

	unsubBus := bus
	if unsubBus == nil {
		unsubBus = p.subBus
	}
	if unsubBus != nil && p.subID != 0 {
		unsubBus.Unsubscribe(core.EventFileOrganized, p.subID)
	}
	p.subID = 0
	p.subBus = nil
	p.mu.Unlock()

	// 在锁外等待所有在途处理完成（因 runCtx 已取消，网络请求会立即中断）
	p.wg.Wait()

	if wasEnabled {
		log.Println("🔌 [插件] Bangumi NFO 刮削器已停用")
	}
	return nil
}

func (p *NFOScraperPlugin) isActive(ctx context.Context) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	p.mu.RLock()
	active := p.enabled
	p.mu.RUnlock()
	return active
}

func (p *NFOScraperPlugin) handleFileOrganized(ev core.Event) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️ [NFO刮削] 异常崩溃捕获: %v", r)
		}
	}()

	p.mu.RLock()
	if !p.enabled || p.runCtx == nil || p.runCtx.Err() != nil {
		p.mu.RUnlock()
		return
	}
	runCtx := p.runCtx
	p.wg.Add(1)
	p.mu.RUnlock()
	defer p.wg.Done()

	payload := ev.Payload
	if payload == nil {
		return
	}

	newPath, _ := payload["new_path"].(string)
	newPath = strings.TrimSpace(newPath)
	if newPath == "" {
		return
	}

	anime := extractAnimeFromPayload(payload)
	episode := extractEpisodeFromPayload(payload)

	if isMovieAnime(anime, payload) {
		movieDir := filepath.Dir(newPath)
		p.generateMovieNFO(runCtx, movieDir, anime)
		if anime.CoverURL != "" {
			p.downloadPoster(runCtx, movieDir, anime.CoverURL)
		}
		return
	}

	// 1. 生成剧集对应的 <episode>.nfo
	p.generateEpisodeNFO(runCtx, newPath, anime, episode)

	// 2. 解析番剧根目录并生成 tvshow.nfo
	showDir := resolveShowRootDir(newPath)
	p.generateTVShowNFO(runCtx, showDir, anime)

	// 3. 若存在封面 URL 且尚无 poster.jpg，安全下载海报
	if anime.CoverURL != "" {
		p.downloadPoster(runCtx, showDir, anime.CoverURL)
	}
}

func extractAnimeFromPayload(payload map[string]interface{}) core.Anime {
	if a, ok := payload["anime"].(core.Anime); ok {
		return a
	}
	if a, ok := payload["anime"].(*core.Anime); ok && a != nil {
		return *a
	}
	return core.Anime{}
}

func extractEpisodeFromPayload(payload map[string]interface{}) core.Episode {
	if ep, ok := payload["episode"].(core.Episode); ok {
		return ep
	}
	if ep, ok := payload["episode"].(*core.Episode); ok && ep != nil {
		return *ep
	}
	return core.Episode{}
}

func isMovieAnime(anime core.Anime, payload map[string]interface{}) bool {
	t := anime.Type
	if t == "" && payload != nil {
		t, _ = payload["anime_type"].(string)
	}
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "movie", "film", "剧场版", "劇場版":
		return true
	default:
		return false
	}
}

// resolveShowRootDir 根据已整理媒体路径解析番剧根目录（支持 Season X、S01、Specials、OVA、第X季 或无季目录结构）
func resolveShowRootDir(mediaPath string) string {
	epDir := filepath.Dir(mediaPath)
	dirName := strings.TrimSpace(filepath.Base(epDir))
	if seasonDirPattern.MatchString(dirName) {
		parent := filepath.Dir(epDir)
		if parent != "" && parent != epDir {
			return parent
		}
	}
	return epDir
}

func resolveDisplayTitle(anime core.Anime) string {
	if t := strings.TrimSpace(anime.TitleCN); t != "" {
		return t
	}
	if t := strings.TrimSpace(anime.TitleJP); t != "" {
		return t
	}
	return strings.TrimSpace(anime.TitleEN)
}

func normalizeTMDBID(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "tv/")
	s = strings.TrimPrefix(s, "movie/")
	return strings.TrimSpace(s)
}

func buildUniqueIDs(bangumiID, tmdbID, imdbID string) []UniqueID {
	var ids []UniqueID
	bgm := strings.TrimSpace(bangumiID)
	tmdb := normalizeTMDBID(tmdbID)
	imdb := strings.TrimSpace(imdbID)

	if bgm != "" {
		ids = append(ids, UniqueID{
			Type:    "bangumi",
			Default: true,
			Value:   bgm,
		})
	}
	if tmdb != "" {
		ids = append(ids, UniqueID{
			Type:    "tmdb",
			Default: bgm == "",
			Value:   tmdb,
		})
	}
	if imdb != "" {
		ids = append(ids, UniqueID{
			Type:    "imdb",
			Default: bgm == "" && tmdb == "",
			Value:   imdb,
		})
	}
	return ids
}

func (p *NFOScraperPlugin) generateEpisodeNFO(ctx context.Context, mediaPath string, anime core.Anime, ep core.Episode) {
	if !p.isActive(ctx) {
		return
	}
	ext := filepath.Ext(mediaPath)
	nfoPath := strings.TrimSuffix(mediaPath, ext) + ".nfo"

	title := strings.TrimSpace(ep.Title)
	if title == "" {
		title = fmt.Sprintf("第 %g 集", ep.Number)
	}

	airedStr := ""
	if !ep.AiredAt.IsZero() {
		airedStr = ep.AiredAt.Format("2006-01-02")
	}

	nfo := EpisodeNFO{
		Title:     title,
		ShowTitle: resolveDisplayTitle(anime),
		Season:    ep.Season,
		Episode:   ep.Number,
		Plot:      anime.Description,
		Aired:     airedStr,
	}

	data, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		log.Printf("⚠️ [NFO刮削] 序列化分集元数据失败 [%s]: %v", nfoPath, err)
		return
	}
	content := append([]byte(xml.Header), data...)

	p.fileMu.Lock()
	defer p.fileMu.Unlock()
	if !p.isActive(ctx) {
		return
	}
	if _, err := os.Stat(nfoPath); err == nil {
		return
	}
	if err := atomicWriteFile(nfoPath, content, 0644); err != nil {
		log.Printf("⚠️ [NFO刮削] 写入分集元数据失败 [%s]: %v", nfoPath, err)
		return
	}
	log.Printf("📄 [NFO刮削] 生成分集元数据: %s", filepath.Base(nfoPath))
}

func (p *NFOScraperPlugin) generateTVShowNFO(ctx context.Context, tvshowDir string, anime core.Anime) {
	if !p.isActive(ctx) {
		return
	}
	nfoPath := filepath.Join(tvshowDir, "tvshow.nfo")
	titleCN := resolveDisplayTitle(anime)
	tmdbID := normalizeTMDBID(anime.TMDBID)
	imdbID := strings.TrimSpace(anime.IMDBID)

	nfo := TVShowNFO{
		Title:         titleCN,
		OriginalTitle: anime.TitleJP,
		ShowTitle:     titleCN,
		Year:          anime.Year,
		Season:        anime.Season,
		Plot:          anime.Description,
		Thumb:         anime.CoverURL,
		TMDBID:        tmdbID,
		IMDBID:        imdbID,
		BangumiID:     strings.TrimSpace(anime.ID),
		UniqueIDs:     buildUniqueIDs(anime.ID, tmdbID, imdbID),
	}

	data, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		log.Printf("⚠️ [NFO刮削] 序列化番剧元数据失败 [%s]: %v", nfoPath, err)
		return
	}
	content := append([]byte(xml.Header), data...)

	p.fileMu.Lock()
	defer p.fileMu.Unlock()
	if !p.isActive(ctx) {
		return
	}
	if _, err := os.Stat(nfoPath); err == nil {
		return
	}
	if err := atomicWriteFile(nfoPath, content, 0644); err != nil {
		log.Printf("⚠️ [NFO刮削] 写入番剧元数据失败 [%s]: %v", nfoPath, err)
		return
	}
	log.Printf("📄 [NFO刮削] 生成番剧元数据: %s", nfoPath)
}

func (p *NFOScraperPlugin) generateMovieNFO(ctx context.Context, movieDir string, anime core.Anime) {
	if !p.isActive(ctx) {
		return
	}
	nfoPath := filepath.Join(movieDir, "movie.nfo")
	titleCN := resolveDisplayTitle(anime)
	tmdbID := normalizeTMDBID(anime.TMDBID)
	imdbID := strings.TrimSpace(anime.IMDBID)

	nfo := MovieNFO{
		Title:         titleCN,
		OriginalTitle: anime.TitleJP,
		Year:          anime.Year,
		Plot:          anime.Description,
		Thumb:         anime.CoverURL,
		TMDBID:        tmdbID,
		IMDBID:        imdbID,
		BangumiID:     strings.TrimSpace(anime.ID),
		UniqueIDs:     buildUniqueIDs(anime.ID, tmdbID, imdbID),
	}

	data, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		log.Printf("⚠️ [NFO刮削] 序列化剧场版元数据失败 [%s]: %v", nfoPath, err)
		return
	}
	content := append([]byte(xml.Header), data...)

	p.fileMu.Lock()
	defer p.fileMu.Unlock()
	if !p.isActive(ctx) {
		return
	}
	if _, err := os.Stat(nfoPath); err == nil {
		return
	}
	if err := atomicWriteFile(nfoPath, content, 0644); err != nil {
		log.Printf("⚠️ [NFO刮削] 写入剧场版元数据失败 [%s]: %v", nfoPath, err)
		return
	}
	log.Printf("📄 [NFO刮削] 生成剧场版元数据: %s", nfoPath)
}

func (p *NFOScraperPlugin) downloadPoster(ctx context.Context, targetDir, coverURL string) {
	if !p.isActive(ctx) {
		return
	}
	posterPath := filepath.Join(targetDir, "poster.jpg")

	// 快速跳过已存在海报，同时对同路径并发下载去重
	p.posterMu.Lock()
	if _, err := os.Stat(posterPath); err == nil {
		p.posterMu.Unlock()
		return
	}
	if _, busy := p.posterInFlight[posterPath]; busy {
		p.posterMu.Unlock()
		return
	}
	p.posterInFlight[posterPath] = struct{}{}
	p.posterMu.Unlock()

	defer func() {
		p.posterMu.Lock()
		delete(p.posterInFlight, posterPath)
		p.posterMu.Unlock()
	}()

	if _, err := validateExternalHTTPURL(coverURL, p.allowPrivateForTest); err != nil {
		log.Printf("⚠️ [NFO刮削] 海报地址未通过安全校验 [%s]: %v", coverURL, err)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, posterDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, coverURL, nil)
	if err != nil {
		log.Printf("⚠️ [NFO刮削] 构建海报请求失败 [%s]: %v", coverURL, err)
		return
	}
	req.Header.Set("User-Agent", "Ani-Go-NFOScraper/1.1")

	client := newSSRFSafeHTTPClient(posterDownloadTimeout, p.allowPrivateForTest)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("⚠️ [NFO刮削] 下载海报失败 [%s]: %v", coverURL, err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		log.Printf("⚠️ [NFO刮削] 下载海报返回异常状态码 %d [%s]", resp.StatusCode, coverURL)
		return
	}
	if resp.ContentLength > maxPosterSize {
		log.Printf("⚠️ [NFO刮削] 海报体积超限 (%d bytes > %d bytes): %s", resp.ContentLength, maxPosterSize, coverURL)
		return
	}

	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	if ct != "" && !strings.HasPrefix(ct, "image/") && ct != "application/octet-stream" && ct != "binary/octet-stream" {
		log.Printf("⚠️ [NFO刮削] 海报响应 Content-Type 非图片类型 (%s): %s", ct, coverURL)
		return
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPosterSize+1))
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("⚠️ [NFO刮削] 读取海报响应失败 [%s]: %v", coverURL, err)
		}
		return
	}
	if len(data) == 0 {
		log.Printf("⚠️ [NFO刮削] 海报内容为空: %s", coverURL)
		return
	}
	if len(data) > maxPosterSize {
		log.Printf("⚠️ [NFO刮削] 海报实际大小超出 %d bytes 限制: %s", maxPosterSize, coverURL)
		return
	}

	p.fileMu.Lock()
	defer p.fileMu.Unlock()
	if !p.isActive(ctx) {
		return
	}
	if _, err := os.Stat(posterPath); err == nil {
		return
	}
	if err := atomicWriteFile(posterPath, data, 0644); err != nil {
		log.Printf("⚠️ [NFO刮削] 保存海报失败 [%s]: %v", posterPath, err)
		return
	}
	log.Printf("🖼️ [NFO刮削] 已保存海报: %s", posterPath)
}

// atomicWriteFile 通过同目录临时文件写入后原子重命名，防止并发或中断产生半残文件
func atomicWriteFile(targetPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, ".anigo-nfo-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
