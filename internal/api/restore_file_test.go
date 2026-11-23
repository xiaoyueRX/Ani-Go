package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

type repairTestDownloader struct {
	stubDownloader
	task       core.DownloadTask
	selected   downloader.FileSelection
	beforeList func()
}

func (d *repairTestDownloader) GetStatus(ctx context.Context, hash string) (core.DownloadTask, error) {
	return d.task, nil
}
func (d *repairTestDownloader) CompletedFiles(ctx context.Context, task core.DownloadTask) (downloader.FileSelection, error) {
	if d.beforeList != nil {
		d.beforeList()
	}
	return d.selected, nil
}

type repairFixture struct {
	s       *Server
	dl      *repairTestDownloader
	ep      database.Episode
	sub     database.Subscription
	src     string
	library string
}

func newRepairFixture(t *testing.T) *repairFixture {
	t.Helper()
	setupTestDB(t)
	t.Setenv("TV_VOLUME_ID", "library")
	t.Setenv("MOVIE_VOLUME_ID", "library")
	base := t.TempDir()
	lib, downloads := filepath.Join(base, "library"), filepath.Join(base, "downloads")
	for _, dir := range []string{lib, downloads} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(lib, ".anigo-volume-id"), []byte("library\n"), 0600); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(downloads, "edition.mkv")
	if err := os.WriteFile(src, []byte("correct source"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	sub := seedSubscription(t, database.Subscription{TitleCN: "Series", AnimeType: "TV", CurrentEpisodes: 1, TotalEpisodes: 1})
	ep := database.Episode{SubscriptionID: sub.ID, Status: "organized", Season: 1, Number: 1, ManualStatus: "failed", FileSize: st.Size(), SourceModTime: st.ModTime().UnixNano(), DownloadRoot: downloads, SourceFile: filepath.Base(src), TorrentHash: strings.Repeat("a", 40), FinalPath: filepath.Join(lib, "recorded [v-one].mkv")}
	key := database.EpisodeFileReservationKey(sub.ID, ep.TorrentHash, ep.SourceFile)
	ep.ReservationKey = &key
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	dl := &repairTestDownloader{task: core.DownloadTask{Hash: ep.TorrentHash, Status: "stalledUP", Progress: 1, SavePath: downloads}, selected: downloader.FileSelection{Files: []downloader.CompletedFile{{Path: src, Size: st.Size()}}}}
	s := &Server{organizer: organizer.New("", "", "", lib, lib, false, nil), downloader: downloader.NewDynamicDownloader(dl)}
	return &repairFixture{s, dl, ep, sub, src, lib}
}

func (f *repairFixture) post(body string) *httptest.ResponseRecorder {
	return webRequest(f.s, "POST", fmt.Sprintf("/api/subscriptions/%d/episodes/%d/restore-file", f.sub.ID, f.ep.ID), body)
}

func TestRestoreFileRepairsOnlyTheRequestedEdition(t *testing.T) {
	f := newRepairFixture(t)
	other := database.Episode{SubscriptionID: f.sub.ID, Status: "organized", Season: 1, Number: 1, ManualStatus: "completed", FinalPath: filepath.Join(f.library, "other.mkv")}
	f.dl.selected.Files = append(f.dl.selected.Files, downloader.CompletedFile{Path: filepath.Join(f.ep.DownloadRoot, "another.mkv"), Size: 42})
	if err := database.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other.FinalPath, []byte("other version"), 0600); err != nil {
		t.Fatal(err)
	}
	var before, after []database.Episode
	database.DB.Order("id").Find(&before)
	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { responses <- f.post(`{"confirm_unmarked_volume":false}`) }()
	}
	for range 2 {
		w := <-responses
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"restored":true`) {
			t.Fatalf("HTTP %d %s", w.Code, w.Body.String())
		}
	}
	source, err := os.Stat(f.src)
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.Stat(f.ep.FinalPath)
	if err != nil || !os.SameFile(source, target) {
		t.Fatalf("未链接原件: %v", err)
	}
	if b, err := os.ReadFile(other.FinalPath); err != nil || string(b) != "other version" {
		t.Fatal("同集的另一版本被修改", err)
	}
	database.DB.Order("id").Find(&after)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("记录、人工标记或归属发生变化")
	}
}

func TestRestoreFileRequiresEvidenceAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *repairFixture)
	}{
		{"wrong volume", func(t *testing.T, f *repairFixture) {
			if err := os.WriteFile(filepath.Join(f.library, ".anigo-volume-id"), []byte("wrong"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unconfirmed volume", func(t *testing.T, f *repairFixture) { t.Setenv("TV_VOLUME_ID", "") }},
		{"offline client", func(t *testing.T, f *repairFixture) { f.s.downloader = downloader.NewDynamicDownloader(nil) }},
		{"incomplete task", func(t *testing.T, f *repairFixture) { f.dl.task.Progress = 0.6 }},
		{"wrong task", func(t *testing.T, f *repairFixture) { f.dl.task.Hash = strings.Repeat("b", 40) }},
		{"unselected member", func(t *testing.T, f *repairFixture) { f.dl.selected.Files = nil }},
		{"ambiguous member", func(t *testing.T, f *repairFixture) {
			f.dl.selected.Files = append(f.dl.selected.Files, f.dl.selected.Files[0])
		}},
		{"unverified size", func(t *testing.T, f *repairFixture) { f.dl.selected.Files[0].Size++ }},
		{"file fingerprint changed", func(t *testing.T, f *repairFixture) {
			if err := database.DB.Model(&f.ep).UpdateColumn("source_mod_time", int64(1)).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"moved download root", func(t *testing.T, f *repairFixture) { f.dl.task.SavePath = t.TempDir() }},
		{"unknown source fingerprint", func(t *testing.T, f *repairFixture) { database.DB.Model(&f.ep).UpdateColumn("source_mod_time", 0) }},
		{"missing member key", func(t *testing.T, f *repairFixture) { database.DB.Model(&f.ep).UpdateColumn("reservation_key", nil) }},
		{"manual-only status", func(t *testing.T, f *repairFixture) {
			database.DB.Model(&f.ep).Updates(map[string]any{"status": "downloaded", "manual_status": "completed"})
		}},
		{"pending move", func(t *testing.T, f *repairFixture) { database.DB.Model(&f.ep).UpdateColumn("move_source_path", f.src) }},
		{"recycled owner", func(t *testing.T, f *repairFixture) {
			e := database.Episode{FinalPath: f.ep.FinalPath, SubscriptionID: f.sub.ID}
			database.DB.Create(&e)
			database.DB.Delete(&e)
		}},
		{"alternate spelling of occupied path", func(t *testing.T, f *repairFixture) {
			alias := f.library + string(filepath.Separator) + "." + string(filepath.Separator) + filepath.Base(f.ep.FinalPath)
			if err := database.DB.Create(&database.Episode{FinalPath: alias, SubscriptionID: f.sub.ID}).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"season or episode changed without a new timestamp", func(t *testing.T, f *repairFixture) {
			f.dl.beforeList = func() {
				if err := database.DB.Model(&f.ep).UpdateColumn("number", 7).Error; err != nil {
					t.Fatal(err)
				}
			}
		}},
		{"changed during downloader request", func(t *testing.T, f *repairFixture) {
			f.dl.beforeList = func() { database.DB.Model(&f.ep).Update("final_path", filepath.Join(f.library, "moved.mkv")) }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRepairFixture(t)
			tc.change(t, f)
			w := f.post(`{}`)
			if w.Code != 409 || strings.Contains(w.Body.String(), f.library) || strings.Contains(w.Body.String(), f.src) {
				t.Fatalf("HTTP %d %s", w.Code, w.Body.String())
			}
			if _, err := os.Lstat(f.ep.FinalPath); !os.IsNotExist(err) {
				t.Fatalf("目标状态 %v", err)
			}
		})
	}
}

func TestRestoreFileDeleteDuringDownloaderLookupDoesNotRevive(t *testing.T) {
	f := newRepairFixture(t)
	f.dl.beforeList = func() {
		w := webRequest(f.s, "POST", fmt.Sprintf("/api/subscriptions/%d/episodes/batch-delete", f.sub.ID), fmt.Sprintf(`{"ids":[%d],"delete_files":false}`, f.ep.ID))
		if w.Code != 200 {
			t.Fatalf("删除时 HTTP %d", w.Code)
		}
	}
	w := f.post(`{}`)
	if w.Code != 404 && w.Code != 409 {
		t.Fatalf("删除后 HTTP %d", w.Code)
	}
	if _, err := os.Lstat(f.ep.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("已删记录重新建立了文件 %v", err)
	}
}

func TestRestoreFileUnmarkedConfirmationAndRequestScope(t *testing.T) {
	f := newRepairFixture(t)
	t.Setenv("TV_VOLUME_ID", "")
	for _, body := range []string{`{"path":"from-client"}`, `null`, `{} {}`, `{"confirm_unmarked_volume":null}`, `{"confirm_unmarked_volume":"true"}`} {
		if w := f.post(body); w.Code != 400 {
			t.Fatalf("%s HTTP %d", body, w.Code)
		}
	}
	other := seedSubscription(t, database.Subscription{TitleCN: "另一订阅"})
	wrong := webRequest(f.s, "POST", fmt.Sprintf("/api/subscriptions/%d/episodes/%d/restore-file", other.ID, f.ep.ID), `{"confirm_unmarked_volume":true}`)
	if wrong.Code != 404 {
		t.Fatalf("跨订阅 HTTP %d", wrong.Code)
	}
	if w := f.post(`{"confirm_unmarked_volume":true}`); w.Code != 200 {
		t.Fatalf("明确确认后 HTTP %d %s", w.Code, w.Body.String())
	}
}

func TestRestoreFileRejectsRootChangeDuringConfirmation(t *testing.T) {
	f := newRepairFixture(t)
	t.Setenv("TV_VOLUME_ID", "")
	f.dl.beforeList = func() {
		// 新根也包含同一个目标路径，单纯在提交时检查包含关系无法识别这次修改。
		err := database.DB.Save(&database.Setting{Key: "TV_BASE_PATH", Value: filepath.Dir(f.library)}).Error
		if err != nil {
			t.Fatal(err)
		}
	}
	if w := f.post(`{"confirm_unmarked_volume":true}`); w.Code != 409 {
		t.Fatalf("库根改变后 HTTP %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Lstat(f.ep.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("写入新配置的根 %v", err)
	}
}

func TestRestoreFileRejectsDownloadRootOutsideClientDirectory(t *testing.T) {
	f := newRepairFixture(t)
	container := t.TempDir()
	alias := filepath.Join(container, "pack")
	if err := os.Symlink(f.ep.DownloadRoot, alias); err != nil {
		t.Skipf("无法创建目录符号链接: %v", err)
	}
	if err := database.DB.Model(&f.ep).UpdateColumn("download_root", alias).Error; err != nil {
		t.Fatal(err)
	}
	f.dl.task.SavePath = container
	f.dl.selected.Files[0].Path = filepath.Join(alias, f.ep.SourceFile)
	if w := f.post(`{}`); w.Code != 409 {
		t.Fatalf("源在实际下载目录外 HTTP %d", w.Code)
	}
	if _, err := os.Lstat(f.ep.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("目标状态 %v", err)
	}
}

func TestRestoreFileRequiresCurrentReviewIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, animeType, signedAs, pending string
		reassigned                         bool
		want                               int
	}{
		{"confirmed episode", "TV", database.ReviewEpisodeIdentity, "", false, 200},
		{"confirmed feature", "Movie", database.ReviewMovieSelection, "", false, 200},
		{"episode reassigned", "TV", database.ReviewEpisodeIdentity, "", true, 409},
		{"movie signed as episode", "Movie", database.ReviewEpisodeIdentity, "", false, 409},
		{"episode signed as movie", "TV", database.ReviewMovieSelection, "", false, 409},
		{"still awaiting episode review", "TV", "", database.ReviewEpisodeIdentity, false, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRepairFixture(t)
			if err := database.DB.Model(&f.sub).Update("anime_type", tc.animeType).Error; err != nil {
				t.Fatal(err)
			}
			if tc.animeType == "Movie" {
				f.ep.Number = 0
			}
			f.ep.ConfirmedIdentity = f.ep.ConfirmationSignature(tc.signedAs)
			if tc.reassigned {
				f.ep.Number++
			}
			f.ep.ReviewKind = tc.pending
			if err := database.DB.Save(&f.ep).Error; err != nil {
				t.Fatal(err)
			}
			w := f.post(`{}`)
			if w.Code != tc.want {
				t.Fatalf("HTTP %d %s", w.Code, w.Body.String())
			}
			if _, err := os.Lstat(f.ep.FinalPath); (tc.want == 409 && !os.IsNotExist(err)) || (tc.want == 200 && err != nil) {
				t.Fatalf("目标状态 %v", err)
			}
		})
	}
}

func TestRestoreFileValidatesPortableNestedMember(t *testing.T) {
	f := newRepairFixture(t)
	dir := filepath.Join(f.ep.DownloadRoot, "pack")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	member := filepath.Join(dir, "edition.mkv")
	if err := os.Rename(f.src, member); err != nil {
		t.Fatal(err)
	}
	f.dl.selected.Files[0].Path = member
	for _, name := range []string{"pack/edition.mkv", "pack//edition.mkv", "pack/./edition.mkv", "pack/../pack/edition.mkv"} {
		ep := f.ep
		ep.SourceFile = name
		key := database.EpisodeFileReservationKey(ep.SubscriptionID, ep.TorrentHash, name)
		ep.ReservationKey = &key
		source, err := repairSource(t.Context(), f.dl, ep, f.sub.AnimeType)
		if name == "pack/edition.mkv" {
			if err != nil || source != member {
				t.Errorf("规范嵌套成员 %q: %v", name, err)
			}
		} else if err == nil {
			t.Errorf("含模糊层级的成员被接受: %q", name)
		}
	}
}

func TestRestoreFileChecksOtherDirectoriesWithTheSameFilename(t *testing.T) {
	for _, tc := range []struct {
		name string
		want int
	}{
		{"distinct directory", 200}, {"missing directory", 200}, {"inaccessible directory", 409}, {"invalid historical directory", 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRepairFixture(t)
			base := t.TempDir()
			parent := filepath.Join(base, "another series")
			if tc.name == "invalid historical directory" {
				parent += "\x00"
			} else if tc.name != "missing directory" {
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "inaccessible directory" {
				if err := os.Chmod(base, 0); err != nil {
					t.Skipf("此文件系统无法设置目录访问权限: %v", err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(base, 0700); err != nil {
						t.Error(err)
					}
				})
				if _, err := os.Stat(parent); !os.IsPermission(err) {
					t.Skipf("当前进程仍可访问该目录: %v", err)
				}
			}
			sibling := database.Episode{SubscriptionID: f.sub.ID, FinalPath: filepath.Join(parent, filepath.Base(f.ep.FinalPath))}
			if err := database.DB.Create(&sibling).Error; err != nil {
				t.Fatal(err)
			}
			w := f.post(`{}`)
			if w.Code != tc.want || strings.Contains(w.Body.String(), base) {
				t.Fatalf("HTTP %d %s", w.Code, w.Body.String())
			}
			if tc.want == 409 {
				if !strings.Contains(w.Body.String(), "无法核验") {
					t.Fatalf("未说明核验失败: %s", w.Body.String())
				}
				if _, err := os.Lstat(f.ep.FinalPath); !os.IsNotExist(err) {
					t.Fatalf("目标状态: %v", err)
				}
				if _, err := os.Stat(f.src); err != nil {
					t.Fatalf("下载原件: %v", err)
				}
			}
		})
	}
}
