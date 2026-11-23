package organizer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
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

func TestConfiguredTVSeriesYearInPreviewAndLibrary(t *testing.T) {
	const oldTemplate = "{title_cn}{year}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}"
	for _, tc := range []struct {
		name, title, english, template, existing, want string
		year, season                                   int
		standardNaming                                 bool
	}{
		{name: "新剧集年份带括号", title: "Cowboy Bebop", english: "Cowboy Bebop", year: 1998, season: 1,
			want: "Cowboy Bebop (1998)/Season 1/Cowboy Bebop S01E01.mkv"},
		{name: "仅中文标题的新剧集", title: "星际牛仔", year: 1998, season: 1,
			want: "星际牛仔 (1998)/Season 1/星际牛仔 S01E01.mkv"},
		{name: "年份未知不留括号", title: "Cowboy Bebop", english: "Cowboy Bebop", season: 1,
			want: "Cowboy Bebop/Season 1/Cowboy Bebop S01E01.mkv"},
		{name: "已有剧集的下一季续写原目录", title: "Cowboy Bebop", english: "Cowboy Bebop", year: 1998, season: 2,
			existing: "Cowboy Bebop1998/Season 1/Cowboy Bebop S01E01.mkv",
			want:     "Cowboy Bebop1998/Season 2/Cowboy Bebop S02E01.mkv"},
		{name: "剧名中的斜线保留旧季的多层目录", title: "Fate/stay night", english: "Fate", year: 2006, season: 2,
			existing: "Fate/stay night2006/Season 1/Fate S01E01.mkv",
			want:     "Fate/stay night2006/Season 2/Fate S02E01.mkv"},
		{name: "标准插件清理斜线两侧空格", title: "Fate / stay night", english: "Fate", year: 2006, season: 2, standardNaming: true,
			existing: "Fate/stay night2006/Season 1/Fate S01E01.mkv",
			want:     "Fate/stay night2006/Season 2/Fate S02E01.mkv"},
		{name: "末尾问号与标准插件", title: "Pourquoi pas ?", english: "Why Not", year: 2006, season: 2, standardNaming: true,
			existing: "Pourquoi pas 2006/Season 1/Why Not S01E01.mkv",
			want:     "Pourquoi pas 2006/Season 2/Why Not S02E01.mkv"},
		{name: "显式设置旧模板继续直接拼年份", title: "Cowboy Bebop", english: "Cowboy Bebop", year: 1998, season: 1,
			template: oldTemplate, want: "Cowboy Bebop1998/Season 1/Cowboy Bebop S01E01.mkv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib := t.TempDir()
			t.Setenv("TV_TEMPLATE", tc.template)
			t.Setenv("TV_BASE_PATH", lib)
			t.Setenv("USE_HARDLINK", "true")
			cfg := config.Load().Organizer
			org := New(cfg.TVTemplate, cfg.MovieTemplate, cfg.OtherTemplate, cfg.TVBasePath, cfg.MovieBasePath, cfg.UseHardLink, nil)
			if tc.standardNaming {
				plugin.InitGeekNaming(org.GetHookManager())
			}
			if tc.existing != "" {
				first := filepath.Join(lib, filepath.FromSlash(tc.existing))
				if err := os.MkdirAll(filepath.Dir(first), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(first, []byte("上一季"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			src := filepath.Join(t.TempDir(), "download.mkv")
			if err := os.WriteFile(src, []byte("本季样例"), 0600); err != nil {
				t.Fatal(err)
			}
			anime := core.Anime{Type: "TV", TitleCN: tc.title, TitleEN: tc.english, Year: tc.year}
			episode := core.Episode{Season: tc.season, Number: 1}
			want := filepath.Join(lib, filepath.FromSlash(tc.want))
			preview, err := org.Preview(t.Context(), src, anime, episode)
			if err != nil {
				t.Fatal(err)
			}
			if preview.FinalPath != want {
				t.Errorf("预览路径为 %q，应为 %q", preview.FinalPath, want)
			}
			actual, err := org.Organize(t.Context(), src, anime, episode)
			if err != nil {
				t.Fatal(err)
			}
			if actual != want {
				t.Errorf("实际整理路径为 %q，应为 %q", actual, want)
			}
			if got, err := os.ReadFile(actual); err != nil || string(got) != "本季样例" {
				t.Fatalf("整理后的文件内容为 %q，错误 %v", got, err)
			}
		})
	}
}

func TestExistingTVSeriesRootAfterHooks(t *testing.T) {
	t.Setenv("TV_TEMPLATE", "")
	for _, tc := range []struct {
		name, hookRoot, oldRoot, want string
		both, oldFile                 bool
	}{
		{name: "规范命名钩子折叠空白后复用旧根", hookRoot: "Star Trail (1998)", oldRoot: "Star Trail1998", want: "Star Trail1998"},
		{name: "规范根和旧根并存", hookRoot: "Star Trail (1998)", oldRoot: "Star Trail1998", both: true, want: "Star Trail (1998)"},
		{name: "旧名字是普通文件", hookRoot: "Star Trail (1998)", oldRoot: "Star Trail1998", oldFile: true, want: "Star Trail (1998)"},
		{name: "尊重钩子另选的剧根", hookRoot: "Different (1998)", oldRoot: "Different1998", want: "Different (1998)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib := t.TempDir()
			old := filepath.Join(lib, tc.oldRoot)
			if tc.oldFile {
				if err := os.WriteFile(old, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(old, 0700); err != nil {
				t.Fatal(err)
			}
			if tc.both {
				if err := os.Mkdir(filepath.Join(lib, tc.hookRoot), 0700); err != nil {
					t.Fatal(err)
				}
			}
			org := New(config.DefaultTVTemplate, "", "", lib, lib, true, nil)
			org.GetHookManager().RegisterNamingHook(core.PriorityHigh, "测试命名", func(_ context.Context, input any) (any, error) {
				in := input.(core.NamingHookInput)
				_, rest, _ := strings.Cut(filepath.ToSlash(in.RenderedPath), "/")
				return core.NamingHookOutput{RenderedPath: filepath.Join(tc.hookRoot, rest)}, nil
			})
			got, err := org.Preview(t.Context(), "download.mkv", core.Anime{Type: "TV", TitleCN: "Star  Trail", Year: 1998}, core.Episode{Season: 2, Number: 1})
			want := filepath.Join(lib, tc.want, "Season 2", "Star  Trail S02E01.mkv")
			if err != nil || got.FinalPath != want {
				t.Fatalf("预览路径 %q，应为 %q，错误 %v", got.FinalPath, want, err)
			}
		})
	}
}

func TestExplicitParenthesizedTVTemplate(t *testing.T) {
	for _, source := range []string{"env", "database"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("TV_TEMPLATE", "")
			if source == "env" {
				t.Setenv("TV_TEMPLATE", config.DefaultTVTemplate)
			} else {
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
				if err := db.Create(&database.Setting{Key: "TV_TEMPLATE", Value: config.DefaultTVTemplate}).Error; err != nil {
					t.Fatal(err)
				}
			}
			lib := t.TempDir()
			if err := os.Mkdir(filepath.Join(lib, "Cowboy Bebop1998"), 0700); err != nil {
				t.Fatal(err)
			}
			org := New(config.Load().Organizer.TVTemplate, "", "", lib, lib, true, nil)
			got, err := org.Preview(t.Context(), "download.mkv", core.Anime{Type: "TV", TitleCN: "Cowboy Bebop", TitleEN: "Cowboy Bebop", Year: 1998}, core.Episode{Season: 2, Number: 1})
			want := filepath.Join(lib, "Cowboy Bebop (1998)", "Season 2", "Cowboy Bebop S02E01.mkv")
			if err != nil || got.FinalPath != want {
				t.Fatalf("显式模板生成 %q，应为 %q，错误 %v", got.FinalPath, want, err)
			}
		})
	}
}

func TestNestedSeriesLegacyDirectoryChecks(t *testing.T) {
	lib := t.TempDir()
	old := filepath.Join(lib, "Fate", "stay night2006")
	if err := os.MkdirAll(old, 0700); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join("Fate", "stay night (2006)", "Season 2", "file.mkv")
	roots := tvSeriesRoots{current: "Fate. / stay  night (2006)", legacy: "Fate. / stay  night2006"}
	want := filepath.Join("Fate", "stay night2006", "Season 2", "file.mkv")
	if got := roots.existingPath(lib, final); got != want {
		t.Errorf("钩子逐层规范化后的下一季为 %q，应为 %q", got, want)
	}
	alias := t.TempDir()
	if err := os.Symlink(filepath.Join(lib, "Fate"), filepath.Join(alias, "Fate")); err != nil {
		t.Skipf("无法建立目录符号链接：%v", err)
	}
	if got := roots.existingPath(alias, final); got != final {
		t.Errorf("符号链接下的旧目录被当作实际剧根：%q", got)
	}
}
