package organizer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestDefaultMediaTitlesInPreviewAndLibrary(t *testing.T) {
	for _, key := range []string{"TV_TEMPLATE", "MOVIE_TEMPLATE", "OTHER_TEMPLATE"} {
		t.Setenv(key, "")
	}
	cfg := config.Load().Organizer
	const historicalTV = "{title_cn}{year}/Season {season}/{title_en} [tmdbid={tmdb_id}] S{season:02}E{ep:02}{ext}"
	const historicalMovie = "{title_cn} ({year})/{title_en}{ext}"
	const historicalSpecial = "{title_cn}{year}/Specials/{title_en} S00E{ep:02}{ext}"

	for _, tc := range []struct {
		name, kind, template, chinese, english, japanese, tmdb, want string
	}{
		{"运行配置的电视默认模板", "TV", cfg.TVTemplate, "星轨验收", "", "", "", "星轨验收/Season 1/星轨验收 S01E01.mkv"},
		{"运行配置的电影默认模板", "Movie", cfg.MovieTemplate, "星轨验收", "", "", "", "星轨验收/星轨验收.mkv"},
		{"有英文时继续采用英文文件名", "TV", cfg.TVTemplate, "星轨验收", "Star Trail", "", "", "星轨验收/Season 1/Star Trail S01E01.mkv"},
		{"旧电视模板省略空元数据标记", "TV", historicalTV, "星轨验收", "", "", "", "星轨验收/Season 1/星轨验收 S01E01.mkv"},
		{"旧电视模板保留已有元数据", "TV", historicalTV, "星轨验收", "Star Trail", "", "tv/42", "星轨验收/Season 1/Star Trail [tmdbid=42] S01E01.mkv"},
		{"旧电影模板", "Movie", historicalMovie, "星轨验收", "", "", "", "星轨验收/星轨验收.mkv"},
		{"特别篇模板和空白英文", "OVA", historicalSpecial, "星轨验收", "  ", "", "", "星轨验收/Specials/星轨验收 S00E01.mkv"},
		{"未知分类的特别篇模板", "other", historicalSpecial, "星轨验收", "", "", "", "星轨验收/Specials/星轨验收 S00E01.mkv"},
		{"只有日文时使用已有标题", "TV", cfg.TVTemplate, "", "", "星の軌跡", "", "星の軌跡/Season 1/星の軌跡 S01E01.mkv"},
		{"只有英文时使用已有标题", "Movie", cfg.MovieTemplate, "", "Star Trail", "", "", "Star Trail/Star Trail.mkv"},
		{"自定义模板保留英文变量原义", "TV", "{title_cn}/English-{title_en}-S{season:02}E{ep:02}{ext}", "星轨验收", "", "", "", "星轨验收/English--S01E01.mkv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			src := filepath.Join(t.TempDir(), "download.mkv")
			if err := os.WriteFile(src, []byte("样例"), 0600); err != nil {
				t.Fatal(err)
			}
			org := New(tc.template, tc.template, tc.template, base, base, true, nil, base)
			a := core.Anime{Type: tc.kind, TitleCN: tc.chinese, TitleEN: tc.english, TitleJP: tc.japanese, TMDBID: tc.tmdb}
			ep := core.Episode{Season: 1, Number: 1}
			want := filepath.Join(base, filepath.FromSlash(tc.want))
			preview, err := org.Preview(context.Background(), src, a, ep)
			if err != nil || preview.FinalPath != want {
				t.Fatalf("预览路径 %q，应为 %q，错误 %v", preview.FinalPath, want, err)
			}
			actual, err := org.Organize(context.Background(), src, a, ep)
			if err != nil || actual != want {
				t.Fatalf("实际路径 %q，应为 %q，错误 %v", actual, want, err)
			}
			if got, err := os.ReadFile(actual); err != nil || string(got) != "样例" {
				t.Fatalf("整理后未读到原文件: %v", err)
			}
		})
	}
}

func TestSpecialsInheritConfiguredTVLayout(t *testing.T) {
	t.Setenv("TV_TEMPLATE", "")
	t.Setenv("OTHER_TEMPLATE", "")
	cfg := config.Load().Organizer
	root := t.TempDir()
	org := New(cfg.TVTemplate, cfg.MovieTemplate, cfg.OtherTemplate, root, root, true, nil, root)
	an := core.Anime{Type: "OVA", TitleCN: "星轨验收"}
	ep := core.Episode{Season: 1, Number: 1}
	src := filepath.Join(t.TempDir(), "extra.mkv")
	if err := os.WriteFile(src, []byte("特别篇"), 0600); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "星轨验收", "Season 1", "星轨验收 S01E01.mkv")
	preview, err := org.Preview(context.Background(), src, an, ep)
	if err != nil || preview.FinalPath != want {
		t.Fatalf("预览 %q，应为 %q，错误 %v", preview.FinalPath, want, err)
	}
	actual, err := org.Organize(context.Background(), src, an, ep)
	if err != nil || actual != want {
		t.Fatalf("实际路径 %q，应为 %q，错误 %v", actual, want, err)
	}
}

func TestPersistedDefaultTitleAndCustomOverride(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; conn, _ := db.DB(); conn.Close() })
	if err := db.AutoMigrate(&database.Setting{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	org := New("custom", "", "", dir, dir, true, nil)
	for _, tc := range []struct{ template, want string }{
		{"{title_cn}{year}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}", "星轨验收 S01E01.mkv"},
		{"fixed/explicit-{title_en}{ext}", "explicit-.mkv"},
	} {
		if err := db.Save(&database.Setting{Key: "TV_TEMPLATE", Value: tc.template}).Error; err != nil {
			t.Fatal(err)
		}
		res, err := org.Preview(context.Background(), "download.mkv", core.Anime{Type: "TV", TitleCN: "星轨验收"}, core.Episode{Season: 1, Number: 1})
		if err != nil || filepath.Base(res.FinalPath) != tc.want {
			t.Errorf("持久化模板生成 %q，应为 %q，错误 %v", res.FinalPath, tc.want, err)
		}
	}
}
