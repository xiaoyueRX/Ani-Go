package plugin

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestNFOScraperPlugin_LifecycleAndGeneration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "anigo-nfo-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	bus := newTestBus()
	p := NewNFOScraperPlugin()

	// 1. 启动插件
	if err := p.Init(bus, nil); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}

	seasonDir := filepath.Join(tempDir, "Frieren", "Season 1")
	if err := os.MkdirAll(seasonDir, 0755); err != nil {
		t.Fatalf("创建剧集目录失败: %v", err)
	}
	videoPath := filepath.Join(seasonDir, "Frieren S01E01.mp4")
	_ = os.WriteFile(videoPath, []byte("fake video content"), 0644)

	// 2. 模拟触发 EventFileOrganized
	anime := core.Anime{
		ID:          "398601",
		TitleCN:     "葬送的芙莉莲",
		TitleJP:     "葬送のフリーレン",
		Year:        2023,
		Season:      1,
		Description: "千年以上生きるエルフの魔法使い・フリーレン...",
		CoverURL:    "", // 暂不触发海报网络下载
		TMDBID:      "tv/209867",
		IMDBID:      "tt22240212",
	}
	ep := core.Episode{
		AnimeID: "398601",
		Season:  1,
		Number:  1,
		Title:   "冒险的终点",
		AiredAt: time.Date(2023, 9, 29, 0, 0, 0, 0, time.UTC),
	}

	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": videoPath,
			"anime":    anime,
			"episode":  ep,
		},
		Time: time.Now(),
	})

	// 3. 校验 tvshow.nfo 生成与 XML 规范内容
	tvshowNFOPath := filepath.Join(tempDir, "Frieren", "tvshow.nfo")
	tvshowData, err := os.ReadFile(tvshowNFOPath)
	if err != nil {
		t.Fatalf("tvshow.nfo 未生成: %v", err)
	}

	contentStr := string(tvshowData)
	if !strings.HasPrefix(contentStr, "<?xml") {
		t.Errorf("tvshow.nfo 应包含 XML 声明头: %s", contentStr)
	}

	var tvshow TVShowNFO
	if err := xml.Unmarshal(tvshowData, &tvshow); err != nil {
		t.Fatalf("解析 tvshow.nfo 失败: %v", err)
	}
	if tvshow.Title != "葬送的芙莉莲" || tvshow.OriginalTitle != "葬送のフリーレン" || tvshow.BangumiID != "398601" {
		t.Errorf("tvshow.nfo 核心字段不匹配: %+v", tvshow)
	}
	if tvshow.TMDBID != "209867" || tvshow.IMDBID != "tt22240212" {
		t.Errorf("tvshow.nfo 外部 ID 未规范清洗: %+v", tvshow)
	}

	// 校验 UniqueIDs 包含 bangumi (default=true), tmdb, imdb
	bgmFound, tmdbFound, imdbFound := false, false, false
	for _, uid := range tvshow.UniqueIDs {
		if uid.Type == "bangumi" && uid.Value == "398601" && uid.Default {
			bgmFound = true
		}
		if uid.Type == "tmdb" && uid.Value == "209867" {
			tmdbFound = true
		}
		if uid.Type == "imdb" && uid.Value == "tt22240212" {
			imdbFound = true
		}
	}
	if !bgmFound || !tmdbFound || !imdbFound {
		t.Errorf("UniqueIDs 节点未按规范生成: %+v", tvshow.UniqueIDs)
	}

	// 4. 校验 episode.nfo 生成与内容
	epNFOPath := filepath.Join(seasonDir, "Frieren S01E01.nfo")
	epData, err := os.ReadFile(epNFOPath)
	if err != nil {
		t.Fatalf("episode.nfo 未生成: %v", err)
	}

	var epNFO EpisodeNFO
	if err := xml.Unmarshal(epData, &epNFO); err != nil {
		t.Fatalf("解析 episode.nfo 失败: %v", err)
	}
	if epNFO.Title != "冒险的终点" || epNFO.Episode != 1 || epNFO.Season != 1 || epNFO.Aired != "2023-09-29" {
		t.Errorf("episode.nfo 字段不匹配: %+v", epNFO)
	}

	// 5. 测试停用插件后不生成文件
	if err := p.Stop(bus); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}

	videoPath2 := filepath.Join(seasonDir, "Frieren S01E02.mp4")
	_ = os.WriteFile(videoPath2, []byte("fake video content 2"), 0644)
	ep2NFOPath := filepath.Join(seasonDir, "Frieren S01E02.nfo")

	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": videoPath2,
			"anime":    anime,
			"episode":  core.Episode{Season: 1, Number: 2, Title: "另外一集"},
		},
		Time: time.Now(),
	})

	if _, err := os.Stat(ep2NFOPath); err == nil {
		t.Errorf("停用插件后不应生成 episode.nfo")
	}
}

func TestNFOScraperPlugin_MovieNFOAndRootDirResolution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "anigo-nfo-movie-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	bus := newTestBus()
	p := NewNFOScraperPlugin()
	if err := p.Init(bus, nil); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer p.Stop(bus)

	// 场景 1: 剧场版归档 (Movie)
	movieDir := filepath.Join(tempDir, "YourName (2016)")
	_ = os.MkdirAll(movieDir, 0755)
	movieVideoPath := filepath.Join(movieDir, "YourName.2016.1080p.mkv")
	_ = os.WriteFile(movieVideoPath, []byte("movie content"), 0644)

	movieAnime := core.Anime{
		ID:          "160447",
		Type:        "movie",
		TitleCN:     "你的名字。",
		TitleJP:     "君の名は。",
		Year:        2016,
		Description: "新海诚执导动画电影...",
		TMDBID:      "movie/372058",
		IMDBID:      "tt5311514",
	}

	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": movieVideoPath,
			"anime":    movieAnime,
			"episode":  core.Episode{Number: 1},
		},
		Time: time.Now(),
	})

	movieNFOPath := filepath.Join(movieDir, "movie.nfo")
	movieData, err := os.ReadFile(movieNFOPath)
	if err != nil {
		t.Fatalf("movie.nfo 未生成: %v", err)
	}

	var movie MovieNFO
	if err := xml.Unmarshal(movieData, &movie); err != nil {
		t.Fatalf("解析 movie.nfo 失败: %v", err)
	}
	if movie.Title != "你的名字。" || movie.Year != 2016 || movie.TMDBID != "372058" || movie.IMDBID != "tt5311514" {
		t.Errorf("movie.nfo 字段不匹配: %+v", movie)
	}
	if movie.BangumiID != "160447" {
		t.Errorf("movie.nfo 缺少 BangumiID 兼容节点: %+v", movie)
	}

	// 场景 2: 无 Season 子目录的剧集（直接放在番剧根目录下）
	showDirDirect := filepath.Join(tempDir, "DirectShow")
	_ = os.MkdirAll(showDirDirect, 0755)
	directVideoPath := filepath.Join(showDirDirect, "DirectShow E01.mkv")
	_ = os.WriteFile(directVideoPath, []byte("ep content"), 0644)

	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": directVideoPath,
			"anime": core.Anime{
				ID:      "99901",
				Type:    "tv",
				TitleCN: "直属番剧",
			},
			"episode": core.Episode{Number: 1},
		},
		Time: time.Now(),
	})

	directTVShowNFO := filepath.Join(showDirDirect, "tvshow.nfo")
	if _, err := os.Stat(directTVShowNFO); err != nil {
		t.Errorf("无季目录时 tvshow.nfo 未能在剧集同级目录生成: %v", err)
	}

	// 场景 3: Specials 特典目录与第1季目录解析
	specialsDir := filepath.Join(tempDir, "ShowWithSpecials", "Specials")
	_ = os.MkdirAll(specialsDir, 0755)
	spVideoPath := filepath.Join(specialsDir, "Show SP01.mkv")
	_ = os.WriteFile(spVideoPath, []byte("sp content"), 0644)

	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": spVideoPath,
			"anime": core.Anime{
				ID:      "99902",
				Type:    "tv",
				TitleCN: "特典番剧",
			},
			"episode": core.Episode{Number: 1},
		},
		Time: time.Now(),
	})

	spRootNFO := filepath.Join(tempDir, "ShowWithSpecials", "tvshow.nfo")
	if _, err := os.Stat(spRootNFO); err != nil {
		t.Errorf("Specials 目录未能正确回溯至番剧根目录: %v", err)
	}
}

func TestNFOScraperPlugin_PosterDownloadAndSSRF(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "anigo-nfo-poster-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fakeImageData := []byte("FAKE_JPEG_IMAGE_BINARY_DATA_FOR_TESTING")

	// 搭建 Mock 静态图片服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cover.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fakeImageData)
		case "/large.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Content-Length", "20000000") // 20MB
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, 1024))
		case "/fake.html":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>Not an image</html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	bus := newTestBus()
	p := NewNFOScraperPlugin()
	// 允许连接测试服务器回环 IP
	p.allowPrivateForTest = true
	if err := p.Init(bus, nil); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer p.Stop(bus)

	showDir := filepath.Join(tempDir, "PosterShow")
	_ = os.MkdirAll(showDir, 0755)
	videoPath := filepath.Join(showDir, "PosterShow E01.mp4")
	_ = os.WriteFile(videoPath, []byte("video"), 0644)

	// 1. 成功下载海报
	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": videoPath,
			"anime": core.Anime{
				ID:       "111",
				TitleCN:  "海报测试番剧",
				CoverURL: server.URL + "/cover.jpg",
			},
			"episode": core.Episode{Number: 1},
		},
		Time: time.Now(),
	})

	posterPath := filepath.Join(showDir, "poster.jpg")
	posterData, err := os.ReadFile(posterPath)
	if err != nil {
		t.Fatalf("poster.jpg 未生成: %v", err)
	}
	if string(posterData) != string(fakeImageData) {
		t.Errorf("下载的海报内容不符合预期: %s", string(posterData))
	}

	// 2. 超大文件 (>10MB) 拒绝
	largeShowDir := filepath.Join(tempDir, "LargeShow")
	_ = os.MkdirAll(largeShowDir, 0755)
	largeVideoPath := filepath.Join(largeShowDir, "LargeShow E01.mp4")
	_ = os.WriteFile(largeVideoPath, []byte("video"), 0644)

	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": largeVideoPath,
			"anime": core.Anime{
				ID:       "222",
				TitleCN:  "超大海报番剧",
				CoverURL: server.URL + "/large.jpg",
			},
			"episode": core.Episode{Number: 1},
		},
		Time: time.Now(),
	})

	if _, err := os.Stat(filepath.Join(largeShowDir, "poster.jpg")); err == nil {
		t.Errorf("超大海报本应被拒绝下载")
	}

	// 3. 非图片类型 Content-Type 拒绝
	htmlShowDir := filepath.Join(tempDir, "HTMLShow")
	_ = os.MkdirAll(htmlShowDir, 0755)
	htmlVideoPath := filepath.Join(htmlShowDir, "HTMLShow E01.mp4")
	_ = os.WriteFile(htmlVideoPath, []byte("video"), 0644)

	bus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": htmlVideoPath,
			"anime": core.Anime{
				ID:       "333",
				TitleCN:  "HTML响应番剧",
				CoverURL: server.URL + "/fake.html",
			},
			"episode": core.Episode{Number: 1},
		},
		Time: time.Now(),
	})

	if _, err := os.Stat(filepath.Join(htmlShowDir, "poster.jpg")); err == nil {
		t.Errorf("非图片 Content-Type 本应被拒绝下载")
	}

	_ = p.Stop(bus)

	// 4. SSRF 防御校验：当 allowPrivateForTest=false 时，回环与私网地址必须被严格阻断
	ssrfBus := newTestBus()
	ssrfPlugin := NewNFOScraperPlugin()
	ssrfPlugin.allowPrivateForTest = false
	if err := ssrfPlugin.Init(ssrfBus, nil); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer ssrfPlugin.Stop(ssrfBus)

	ssrfShowDir := filepath.Join(tempDir, "SSRFShow")
	_ = os.MkdirAll(ssrfShowDir, 0755)
	ssrfVideoPath := filepath.Join(ssrfShowDir, "SSRFShow E01.mp4")
	_ = os.WriteFile(ssrfVideoPath, []byte("video"), 0644)

	ssrfBus.Publish(core.Event{
		Type: core.EventFileOrganized,
		Payload: map[string]interface{}{
			"new_path": ssrfVideoPath,
			"anime": core.Anime{
				ID:       "444",
				TitleCN:  "SSRF番剧",
				CoverURL: server.URL + "/cover.jpg", // 回环地址
			},
			"episode": core.Episode{Number: 1},
		},
		Time: time.Now(),
	})

	if _, err := os.Stat(filepath.Join(ssrfShowDir, "poster.jpg")); err == nil {
		t.Errorf("SSRF 本地回环地址下载海报未被拦截")
	}
}

func TestNFOScraperPlugin_IdempotencyAndDoubleInit(t *testing.T) {
	bus := newTestBus()
	p := NewNFOScraperPlugin()

	// 连续重复调用 Init 必须幂等无 panic
	if err := p.Init(bus, nil); err != nil {
		t.Fatalf("第 1 次 Init 失败: %v", err)
	}
	if err := p.Init(bus, nil); err != nil {
		t.Fatalf("第 2 次 Init 失败: %v", err)
	}

	// 连续重复调用 Stop 必须幂等无 panic
	if err := p.Stop(bus); err != nil {
		t.Fatalf("第 1 次 Stop 失败: %v", err)
	}
	if err := p.Stop(bus); err != nil {
		t.Fatalf("第 2 次 Stop 失败: %v", err)
	}
}
