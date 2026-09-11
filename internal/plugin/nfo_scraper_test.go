package plugin

import (
	"encoding/xml"
	"os"
	"path/filepath"
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
		CoverURL:    "https://lain.bgm.tv/pic/cover/l/test.jpg",
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

	// 3. 校验 tvshow.nfo 生成与内容
	tvshowNFOPath := filepath.Join(tempDir, "Frieren", "tvshow.nfo")
	tvshowData, err := os.ReadFile(tvshowNFOPath)
	if err != nil {
		t.Fatalf("tvshow.nfo 未生成: %v", err)
	}

	var tvshow TVShowNFO
	if err := xml.Unmarshal(tvshowData, &tvshow); err != nil {
		t.Fatalf("解析 tvshow.nfo 失败: %v", err)
	}
	if tvshow.Title != "葬送的芙莉莲" || tvshow.OriginalTitle != "葬送のフリーレン" || tvshow.BangumiID != "398601" {
		t.Errorf("tvshow.nfo 字段不匹配: %+v", tvshow)
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
