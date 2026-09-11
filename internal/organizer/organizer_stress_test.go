package organizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// TestOrganizer_PathEscapeEdgeCases 针对路径越界逃逸、系统关键文件渗透进行极端边界测试
func TestOrganizer_PathEscapeEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "anigo-org-escape-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tvBase := filepath.Join(tempDir, "TV")
	movieBase := filepath.Join(tempDir, "Movie")
	ovaBase := filepath.Join(tempDir, "OVA")

	org := New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"{title_cn} ({year})/{title_en}{ext}",
		"{title_cn}/Specials/{title_en} S00E{ep:02}{ext}",
		tvBase,
		movieBase,
		false,
		nil,
		ovaBase,
	)

	// 创建一个源测试文件
	srcFile := filepath.Join(tempDir, "source.mkv")
	if err := os.WriteFile(srcFile, []byte("dummy video content"), 0644); err != nil {
		t.Fatalf("创建测试源文件失败: %v", err)
	}

	escapeVectors := []struct {
		name     string
		anime    core.Anime
		episode  core.Episode
		desc     string
	}{
		{
			name: "Unix Root Escape in Title",
			anime: core.Anime{
				TitleCN: "../../../../../../etc",
				TitleEN: "passwd",
				Type:    "TV",
			},
			episode: core.Episode{Season: 1, Number: 1},
			desc:    "尝试通过 TitleCN 中的相对路径穿越逃逸到 /etc",
		},
		{
			name: "Windows Root Escape in Title",
			anime: core.Anime{
				TitleCN: "..\\..\\..\\..\\Windows\\System32",
				TitleEN: "cmd.exe",
				Type:    "Movie",
			},
			episode: core.Episode{Season: 1, Number: 1},
			desc:    "尝试通过 Windows 反斜杠相对路径逃逸",
		},
		{
			name: "DotDot in Extension",
			anime: core.Anime{
				TitleCN: "NormalTitle",
				TitleEN: "NormalEN",
				Type:    "TV",
			},
			episode: core.Episode{Season: 1, Number: 1},
			desc:    "TitleCN 与 TitleEN 包含连续点与斜杠",
		},
		{
			name: "Null Byte Injection",
			anime: core.Anime{
				TitleCN: "NormalTitle\x00/../../etc",
				TitleEN: "Injected\x00File",
				Type:    "OVA",
			},
			episode: core.Episode{Season: 1, Number: 1},
			desc:    "空字节截断注入攻击",
		},
		{
			name: "CRLF Injection",
			anime: core.Anime{
				TitleCN: "Title\r\n/../../root",
				TitleEN: "File\r\nName",
				Type:    "TV",
			},
			episode: core.Episode{Season: 1, Number: 1},
			desc:    "CRLF 换行符注入测试",
		},
	}

	for _, tc := range escapeVectors {
		t.Run(tc.name, func(t *testing.T) {
			// 1. 测试 Preview (Dry-Run)
			res, prevErr := org.Preview(context.Background(), srcFile, tc.anime, tc.episode)
			if prevErr == nil {
				// 如果未报错，必须严格保证 FinalPath 依然在 basePath 内部，绝不能逃逸
				base := org.getBasePath(tc.anime.Type)
				absBase, _ := filepath.Abs(base)
				absFinal, _ := filepath.Abs(res.FinalPath)
				rel, relErr := filepath.Rel(absBase, absFinal)
				if relErr != nil || strings.HasPrefix(rel, "..") {
					t.Fatalf("[%s] Preview 路径发生逃逸! FinalPath: %s, Base: %s", tc.name, res.FinalPath, base)
				}
			}

			// 2. 测试 Organize (实际执行)
			outPath, orgErr := org.Organize(context.Background(), srcFile, tc.anime, tc.episode)
			if orgErr == nil {
				base := org.getBasePath(tc.anime.Type)
				absBase, _ := filepath.Abs(base)
				absOut, _ := filepath.Abs(outPath)
				rel, relErr := filepath.Rel(absBase, absOut)
				if relErr != nil || strings.HasPrefix(rel, "..") {
					t.Fatalf("[%s] Organize 路径发生逃逸! OutPath: %s, Base: %s", tc.name, outPath, base)
				}
			}
		})
	}
}

// TestCheckPathEscape_BoundaryCases 严密测试路径逃逸判定函数的正向与反向边界
func TestCheckPathEscape_BoundaryCases(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "media")
	_ = os.MkdirAll(basePath, 0755)

	validCases := []struct {
		name     string
		fullPath string
	}{
		{"Normal file in base", filepath.Join(basePath, "movie.mp4")},
		{"Deep subfolder in base", filepath.Join(basePath, "Anime", "Season 1", "ep1.mp4")},
		{"Subfolder starting with double dot inside base", filepath.Join(basePath, "..Special", "ep1.mp4")},
		{"File starting with double dot inside base", filepath.Join(basePath, "..hidden.mp4")},
	}

	for _, vc := range validCases {
		t.Run("Valid_"+vc.name, func(t *testing.T) {
			if err := checkPathEscape(basePath, vc.fullPath); err != nil {
				t.Fatalf("预期合法路径却报错: %v (path: %s)", err, vc.fullPath)
			}
		})
	}

	invalidCases := []struct {
		name     string
		fullPath string
	}{
		{"Exact base directory itself", basePath},
		{"Sibling directory with similar prefix", filepath.Join(tempDir, "media2", "file.mp4")},
		{"Parent directory escape", filepath.Join(basePath, "..", "escape.mp4")},
		{"Root escape", filepath.Join(basePath, "..", "..", "..", "etc", "passwd")},
	}

	for _, ic := range invalidCases {
		t.Run("Invalid_"+ic.name, func(t *testing.T) {
			if err := checkPathEscape(basePath, ic.fullPath); err == nil {
				t.Fatalf("预期非法路径应报错却通过! (path: %s)", ic.fullPath)
			}
		})
	}
}

// TestOrganizer_DirtyAndExtremeData 测试极端脏数据、巨大字符串、异常集数
func TestOrganizer_DirtyAndExtremeData(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "anigo-org-dirty-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	org := New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"{title_cn} ({year})/{title_en}{ext}",
		"{title_cn}/Specials/{title_en} S00E{ep:02}{ext}",
		filepath.Join(tempDir, "TV"),
		filepath.Join(tempDir, "Movie"),
		false,
		nil,
	)

	testCases := []struct {
		name    string
		anime   core.Anime
		episode core.Episode
	}{
		{
			name: "Ultra-long title (10KB)",
			anime: core.Anime{
				TitleCN: strings.Repeat("番剧超长标题测试_", 1000),
				TitleEN: strings.Repeat("LongTitle_", 1000),
				Type:    "TV",
				Year:    2024,
			},
			episode: core.Episode{Season: 1, Number: 1},
		},
		{
			name: "All illegal chars",
			anime: core.Anime{
				TitleCN: `<>:"/\|?*` + "\x00\r\n\t",
				TitleEN: `*?<>|:"` + "\x00",
				Type:    "TV",
			},
			episode: core.Episode{Season: 2, Number: 3.5},
		},
		{
			name: "Negative & extreme numbers",
			anime: core.Anime{
				TitleCN: "ExtremeNumbers",
				TitleEN: "ExtremeNumbers",
				Type:    "TV",
				Year:    -2024,
			},
			episode: core.Episode{Season: -5, Number: -12.5},
		},
		{
			name: "Case variant types",
			anime: core.Anime{
				TitleCN: "TypeVariation",
				TitleEN: "TypeVariation",
				Type:    "mOvIe",
			},
			episode: core.Episode{Season: 1, Number: 1},
		},
		{
			name: "OAD type",
			anime: core.Anime{
				TitleCN: "AttackOnTitan",
				TitleEN: "AOT",
				Type:    "OAD",
			},
			episode: core.Episode{Season: 0, Number: 1},
		},
		{
			name: "Empty strings",
			anime: core.Anime{
				TitleCN: "",
				TitleEN: "",
				Type:    "",
			},
			episode: core.Episode{Season: 0, Number: 0},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("[%s] 发生 panic: %v", tc.name, r)
				}
			}()

			res, _ := org.Preview(context.Background(), "test.mkv", tc.anime, tc.episode)
			if res.FinalPath == "" && res.Error == "" {
				t.Errorf("[%s] Preview 结果不应同时为空且无错误", tc.name)
			}
		})
	}
}

// TestOrganizer_HighConcurrencyStress 测试高并发整理与锁竞争
func TestOrganizer_HighConcurrencyStress(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "anigo-org-stress-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	org := New(
		"{title_cn}/Season {season}/{title_en} S{season:02}E{ep:02}{ext}",
		"{title_cn} ({year})/{title_en}{ext}",
		"{title_cn}/Specials/{title_en} S00E{ep:02}{ext}",
		filepath.Join(tempDir, "TV"),
		filepath.Join(tempDir, "Movie"),
		false,
		nil,
		filepath.Join(tempDir, "OVA"),
	)

	// 动态注册钩子
	hookMgr := org.GetHookManager()
	hookMgr.RegisterNamingHook(core.PriorityNormal, "stress-hook-1", func(ctx context.Context, input interface{}) (interface{}, error) {
		in := input.(core.NamingHookInput)
		return core.NamingHookOutput{
			RenderedPath: strings.ReplaceAll(in.RenderedPath, "Original", "Modified"),
		}, nil
	})

	const numWorkers = 50
	const numOpsPerWorker = 20

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < numOpsPerWorker; i++ {
				// 创建并发独占文件
				srcFile := filepath.Join(tempDir, fmt.Sprintf("src_w%d_i%d.mkv", workerID, i))
				_ = os.WriteFile(srcFile, []byte(fmt.Sprintf("worker %d data %d", workerID, i)), 0644)

				anime := core.Anime{
					TitleCN: fmt.Sprintf("番剧Original_%d", workerID%5),
					TitleEN: fmt.Sprintf("AnimeOriginal_%d", workerID%5),
					Type:    []string{"TV", "Movie", "OVA"}[workerID%3],
					Year:    2020 + (workerID % 5),
				}
				ep := core.Episode{
					Season: (workerID % 3) + 1,
					Number: float32(i + 1),
				}

				// 并发 Preview
				_, _ = org.Preview(context.Background(), srcFile, anime, ep)

				// 并发 Organize
				_, _ = org.Organize(context.Background(), srcFile, anime, ep)
			}
		}()
	}

	wg.Wait()
}
