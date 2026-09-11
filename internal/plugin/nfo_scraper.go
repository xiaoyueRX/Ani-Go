package plugin

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// TVShowNFO Kodi/Jellyfin/Emby 兼容的 tvshow.nfo 结构
type TVShowNFO struct {
	XMLName       xml.Name `xml:"tvshow"`
	Title         string   `xml:"title"`
	OriginalTitle string   `xml:"originaltitle"`
	ShowTitle     string   `xml:"showtitle"`
	Year          int      `xml:"year,omitempty"`
	Season        int      `xml:"season,omitempty"`
	Plot          string   `xml:"plot,omitempty"`
	Thumb         string   `xml:"thumb,omitempty"`
	BangumiID     string   `xml:"uniqueid"`
}

// EpisodeNFO Kodi/Jellyfin/Emby 兼容的 <episode>.nfo 结构
type EpisodeNFO struct {
	XMLName xml.Name `xml:"episodedetails"`
	Title   string   `xml:"title"`
	Season  int      `xml:"season"`
	Episode float32  `xml:"episode"`
	Plot    string   `xml:"plot,omitempty"`
	Aired   string   `xml:"aired,omitempty"`
}

// NFOScraperPlugin Bangumi NFO 媒体刮削器插件
type NFOScraperPlugin struct {
	mu      sync.RWMutex
	enabled bool
	subID   core.SubscriptionID
}

// NewNFOScraperPlugin 创建 NFO 刮削器插件实例
func NewNFOScraperPlugin() *NFOScraperPlugin {
	return &NFOScraperPlugin{}
}

func (p *NFOScraperPlugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID:          "nfo_scraper",
		Name:        "Bangumi NFO 刮削器",
		Description: "媒体库元数据自动生成插件。当剧集整理入库后，自动为 Jellyfin / Emby / Kodi 生成标准 tvshow.nfo 与 episode.nfo，并关联 Bangumi 番组信息，实现开箱即显的海报、简介与分集信息。",
		Version:     "1.0.0",
		Author:      "xiaoyue",
		AuthorURL:   "https://github.com/xiaoyueRX",
		Icon:        "Film",
		IsBuiltIn:   true,
		Type:        "builtin",
		Events:      []string{core.EventFileOrganized},
	}
}

// Init 启用插件：订阅文件整理完成事件
func (p *NFOScraperPlugin) Init(bus core.EventBus, ctx core.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.enabled = true
	if bus != nil {
		p.subID = bus.Subscribe(core.EventFileOrganized, func(ev core.Event) {
			p.handleFileOrganized(ev)
		})
	}
	log.Println("🔌 [插件] Bangumi NFO 刮削器已启用，将在剧集整理入库时自动生成媒体元数据")
	return nil
}

// Stop 停用插件：注销事件监听，保证 0 协程、0 网络、0 文件写入
func (p *NFOScraperPlugin) Stop(bus core.EventBus) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.enabled = false
	if bus != nil && p.subID != 0 {
		bus.Unsubscribe(core.EventFileOrganized, p.subID)
		p.subID = 0
	}
	log.Println("🔌 [插件] Bangumi NFO 刮削器已停用")
	return nil
}

func (p *NFOScraperPlugin) handleFileOrganized(ev core.Event) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️ [NFO刮削] 异常崩溃捕获: %v", r)
		}
	}()

	p.mu.RLock()
	enabled := p.enabled
	p.mu.RUnlock()
	if !enabled {
		return
	}

	payload := ev.Payload
	if payload == nil {
		return
	}

	newPath, _ := payload["new_path"].(string)
	if newPath == "" {
		return
	}

	anime, _ := payload["anime"].(core.Anime)
	episode, _ := payload["episode"].(core.Episode)

	// 1. 生成剧集对应的 episode.nfo
	p.generateEpisodeNFO(newPath, anime, episode)

	// 2. 生成番剧根目录下的 tvshow.nfo
	p.generateTVShowNFO(newPath, anime)
}

func (p *NFOScraperPlugin) generateEpisodeNFO(mediaPath string, anime core.Anime, ep core.Episode) {
	ext := filepath.Ext(mediaPath)
	nfoPath := strings.TrimSuffix(mediaPath, ext) + ".nfo"

	// 如果已存在则不重复覆写
	if _, err := os.Stat(nfoPath); err == nil {
		return
	}

	title := ep.Title
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("第 %g 集", ep.Number)
	}

	airedStr := ""
	if !ep.AiredAt.IsZero() {
		airedStr = ep.AiredAt.Format("2006-01-02")
	}

	nfo := EpisodeNFO{
		Title:   title,
		Season:  ep.Season,
		Episode: ep.Number,
		Plot:    anime.Description,
		Aired:   airedStr,
	}

	data, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		return
	}

	content := append([]byte(xml.Header), data...)
	_ = os.WriteFile(nfoPath, content, 0644)
	log.Printf("📄 [NFO刮削] 生成分集元数据: %s", filepath.Base(nfoPath))
}

func (p *NFOScraperPlugin) generateTVShowNFO(mediaPath string, anime core.Anime) {
	// 判断番剧根目录：通常 mediaPath 为 /AnimeDir/Season 1/Episode.mp4 或 /AnimeDir/Episode.mp4
	epDir := filepath.Dir(mediaPath)
	dirName := strings.ToLower(filepath.Base(epDir))
	tvshowDir := epDir
	if strings.HasPrefix(dirName, "season") || strings.HasPrefix(dirName, "specials") {
		tvshowDir = filepath.Dir(epDir)
	}

	nfoPath := filepath.Join(tvshowDir, "tvshow.nfo")
	if _, err := os.Stat(nfoPath); err == nil {
		return
	}

	titleCN := anime.TitleCN
	if titleCN == "" {
		titleCN = anime.TitleJP
	}
	if titleCN == "" {
		titleCN = anime.TitleEN
	}

	nfo := TVShowNFO{
		Title:         titleCN,
		OriginalTitle: anime.TitleJP,
		ShowTitle:     titleCN,
		Year:          anime.Year,
		Season:        anime.Season,
		Plot:          anime.Description,
		Thumb:         anime.CoverURL,
		BangumiID:     anime.ID,
	}

	data, err := xml.MarshalIndent(nfo, "", "  ")
	if err != nil {
		return
	}

	content := append([]byte(xml.Header), data...)
	_ = os.WriteFile(nfoPath, content, 0644)
	log.Printf("📄 [NFO刮削] 生成番剧元数据: %s", nfoPath)
}
