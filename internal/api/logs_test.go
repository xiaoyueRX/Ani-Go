package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSettingsLogTailPreservesNewestLines(t *testing.T) {
	var history []string
	for i := range 750 {
		history = append(history, fmt.Sprintf("%04d 已整理 %s", i, strings.Repeat("番剧", 9)))
	}
	long := "本行超过一个读取块 " + strings.Repeat("长", 5000)
	for _, tc := range []struct {
		name, content string
		limit         int
		want          []string
	}{
		{"网页请求最近三百行", strings.Join(history, "\n") + "\n", 300, history[len(history)-300:]},
		{"没有结尾换行", strings.Join(history, "\n"), 100, history[len(history)-100:]},
		{"超长行与空行", "旧\n" + long + "\n\n最新", 3, []string{long, "", "最新"}},
		{"尾部多个换行", "此前\n新行\n\n", 1, []string{"新行"}},
		{"空文件", "", 10, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ani-go.log")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			(&Server{logPath: path}).handleGetLogs(w, httptest.NewRequest("GET", fmt.Sprintf("/api/logs?lines=%d", tc.limit), nil))
			var result struct {
				Lines []string
				Total int
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Lines == nil || !slices.Equal(result.Lines, tc.want) || result.Total < len(tc.want) {
				t.Fatalf("日志窗口内容不符，期望 %d 行，返回 %d 行，总计 %d", len(tc.want), len(result.Lines), result.Total)
			}
		})
	}
}
