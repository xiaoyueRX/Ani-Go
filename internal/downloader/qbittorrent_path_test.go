package downloader

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestQBDirectoryOverrideOnlyForExplicitPath(t *testing.T) {
	t.Setenv("DISK_MIN_FREE_GB", "0")
	for _, path := range []string{"", "/client/downloads/series"} {
		t.Run(path, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/auth/login" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.URL.Path != "/api/v2/torrents/add" {
					t.Errorf("unexpected route %s", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					return
				}
				if r.PostForm.Has("savepath") != (path != "") || r.PostForm.Get("savepath") != path {
					t.Errorf("savepath %q sent=%t", r.PostForm.Get("savepath"), r.PostForm.Has("savepath"))
				}
				if (path == "" && r.PostForm.Has("autoTMM")) || (path != "" && r.PostForm.Get("autoTMM") != "false") {
					t.Error("unexpected autoTMM override")
				}
				_, _ = w.Write([]byte("Ok."))
			}))
			defer srv.Close()
			if err := NewQBittorrent(srv.URL, "user", "pass", "anime").Add(t.Context(), core.TorrentItem{URL: "https://example.org/t"}, path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
