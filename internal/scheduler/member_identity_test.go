package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
)

type memberListDownloader struct {
	pipelineDownloader
	files downloader.FileSelection
}

func (d *memberListDownloader) CompletedFiles(context.Context, core.DownloadTask) (downloader.FileSelection, error) {
	return d.files, nil
}

func TestUnlabelledPackKeepsVersionsAndSeparatesAmbiguousMembers(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", Season: 2, AnimeType: "tv", TotalEpisodes: 10}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, Season: 2, Title: "Show BD", OriginalName: "Show BD", Status: "downloaded", TorrentHash: "a1"}
	savePipelineRow(t, db, &ep)
	root, lib := t.TempDir(), t.TempDir()
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: "a1", SavePath: root, ContentPath: root, Progress: 1, Status: "completed"}}
	sources := []string{"Show S02E01 [AV1].mkv", "Show S02E01 [x265].mkv", "Show S02E02.mkv", "Show S00E15.mkv", "Show S01E29.mkv", "Extra/Mini Anime - 12.mkv", "Menu 1.mkv", "unidentified.mkv"}
	for _, name := range sources {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		dl.files.Files = append(dl.files.Files, downloader.CompletedFile{Path: p, Size: int64(len(name))})
	}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", true, nil)}
	s.pollOrganizer(context.Background())
	s.pollOrganizer(context.Background())
	var members []database.Episode
	if err := db.Where("subscription_id = ?", sub.ID).Find(&members).Error; err != nil {
		t.Fatal(err)
	}
	if len(members) != len(sources) {
		t.Fatalf("拆分 %d 条，期望 %d 条: %+v", len(members), len(sources), members)
	}
	seen := map[string]bool{}
	for _, m := range members {
		switch m.SourceFile {
		case sources[0], sources[1], sources[2], sources[3]:
			data, err := os.ReadFile(m.FinalPath)
			if m.Status != "organized" || seen[m.FinalPath] || err != nil || string(data) != m.SourceFile {
				t.Fatalf("版本未单独整理: %+v %v", m, err)
			}
			seen[m.FinalPath] = true
			if m.SourceFile == sources[3] && (m.Season != 0 || m.Number != 15) {
				t.Fatalf("特典身份错误: %+v", m)
			}
		default:
			if m.Status != "needs_review" || m.FinalPath != "" || m.DownloadRoot != root || m.SourceModTime == 0 {
				t.Fatalf("未保留歧义来源: %+v", m)
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("正确整理 %d 个，期望 4", len(seen))
	}
}
func TestUnknownSingleTVIsHeldAndMovieRemainsOrganizable(t *testing.T) {
	for _, kind := range []string{"tv", "movie"} {
		t.Run(kind, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "Show", AnimeType: kind}
			savePipelineRow(t, db, &sub)
			ep := database.Episode{SubscriptionID: sub.ID, Season: 1, TorrentHash: "f1", Title: "Show", OriginalName: "Show", Status: "downloaded"}
			savePipelineRow(t, db, &ep)
			root, lib := t.TempDir(), t.TempDir()
			path := filepath.Join(root, "Show.mkv")
			if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			dl := &memberListDownloader{files: downloader.FileSelection{Files: []downloader.CompletedFile{{Path: path, Size: 4}}}}
			dl.tasks = []core.DownloadTask{{Hash: "f1", SavePath: root, ContentPath: path, Progress: 1, Status: "completed"}}
			s := Scheduler{downloader: dl, organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "Show/Film{ext}", "", lib, lib, true, nil)}
			s.pollOrganizer(context.Background())
			if err := db.First(&ep, ep.ID).Error; err != nil {
				t.Fatal(err)
			}
			if kind == "movie" && ep.Status != "organized" {
				t.Fatalf("单部电影未整理: %+v", ep)
			}
			if kind == "tv" && (ep.Status != "needs_review" || ep.FinalPath != "" || ep.DownloadRoot != root || ep.SourceFile != "Show.mkv" || ep.SourceModTime == 0) {
				t.Fatalf("未识别单集未保留复核来源: %+v", ep)
			}
		})
	}
}

func TestRetryKeepsUnconfirmedMemberAndResumesVerifiedSibling(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", Season: 2, AnimeType: "tv", TotalEpisodes: 10}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, Season: 2, TorrentHash: "mixed", OriginalName: "Show BD", Status: "downloaded"}
	savePipelineRow(t, db, &ep)
	root, lib := t.TempDir(), t.TempDir()
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: "mixed", SavePath: root, ContentPath: root, Progress: 1, Status: "completed"}}
	extra := filepath.Join("Extra", "Mini Anime - 03.mkv")
	for _, name := range []string{extra, "S02E01.mkv", "S02E02.mkv"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		body := "four"
		if name == "S02E02.mkv" {
			body = "x"
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		dl.files.Files = append(dl.files.Files, downloader.CompletedFile{Path: p, Size: 4})
	}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", true, nil)}
	s.pollOrganizer(context.Background())
	var unknown, wrong database.Episode
	for n, r := range map[string]*database.Episode{extra: &unknown, "S02E02.mkv": &wrong} {
		if err := db.Where("source_file = ?", n).First(r).Error; err != nil {
			t.Fatal(err)
		}
		if r.Status != "needs_review" {
			t.Fatalf("未隔离: %+v", r)
		}
	}
	if unknown.Number != 0 || wrong.Number != 2 || wrong.SourceModTime != 0 {
		t.Fatalf("复核混淆未确认身份与大小不符: %#v/%#v", unknown, wrong)
	}
	if err := db.Model(&database.Episode{}).Where("id IN ?", []uint{unknown.ID, wrong.ID}).Updates(map[string]any{"status": "downloaded", "review_reason": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, wrong.SourceFile), []byte("good"), 0600); err != nil {
		t.Fatal(err)
	}
	s.pollOrganizer(context.Background())
	if err := db.First(&unknown, unknown.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&wrong, wrong.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unknown.Status != "needs_review" || wrong.Status != "organized" {
		t.Fatalf("重试处理错误: %+v/%+v", unknown, wrong)
	}
}

func TestStaleOrganizationSnapshotCannotCommitAfterReviewRetry(t *testing.T) {
	for _, hardlink := range []bool{true, false} {
		db := pipelineDB(t)
		sub := database.Subscription{TitleCN: "Show", AnimeType: "tv"}
		savePipelineRow(t, db, &sub)
		ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, TorrentHash: "c1", Status: "downloaded"}
		savePipelineRow(t, db, &ep)
		if err := db.Model(&database.Episode{}).Where("id = ?", ep.ID).Updates(map[string]any{"number": 2, "status": "downloaded"}).Error; err != nil {
			t.Fatal(err)
		}
		root, lib := t.TempDir(), t.TempDir()
		source := filepath.Join(root, "source.mkv")
		if err := os.WriteFile(source, []byte("file"), 0600); err != nil {
			t.Fatal(err)
		}
		s := Scheduler{organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", hardlink, nil)}
		if _, err := s.commitOrganization(context.Background(), ep, source, organizedAnime(sub), core.Episode{Season: 1, Number: 1}); err == nil {
			t.Fatal("已过期的整理快照仍能提交")
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(lib, "Show", "S01E01.mkv")); !os.IsNotExist(err) {
			t.Fatalf("已过期的整理快照仍发布了文件: %v", err)
		}
		var current database.Episode
		if err := db.First(&current, ep.ID).Error; err != nil || current.Status != "downloaded" || current.Number != 2 || current.MoveSourcePath != "" {
			t.Fatalf("新身份被陈旧提交改写: %+v %v", current, err)
		}
	}
}

func TestMemberIdentityCaseAndPollIdempotence(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", AnimeType: "tv", Season: 1}
	savePipelineRow(t, db, &sub)
	const infoHash = "aBcDeF0123456789aBcDeF0123456789aBcDeF01"
	original := database.Episode{SubscriptionID: sub.ID, TorrentHash: infoHash, Season: 1, Number: 1, SourceFile: "Show E01.mkv", Status: "organized"}
	key := database.EpisodeFileReservationKey(sub.ID, original.TorrentHash, original.SourceFile)
	original.ReservationKey = &key
	savePipelineRow(t, db, &original)
	parent := database.Episode{SubscriptionID: sub.ID, TorrentHash: strings.ToUpper(infoHash), Season: 1, OriginalName: "Show BD", Status: "downloaded"}
	savePipelineRow(t, db, &parent)
	root, lib := t.TempDir(), t.TempDir()
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: parent.TorrentHash, SavePath: root, ContentPath: root, Progress: 1, Status: "completed"}}
	for _, name := range []string{"Show E01.mkv", "Show E02.mkv"} {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		dl.files.Files = append(dl.files.Files, downloader.CompletedFile{Path: p, Size: int64(len(name))})
	}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", true, nil)}
	s.pollOrganizer(context.Background())
	s.pollOrganizer(context.Background())
	var got []database.Episode
	if err := db.Order("id").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != original.ID || got[0].Status != "organized" || got[1].Number != 2 || got[1].SourceFile != "Show E02.mkv" || got[1].Status != "organized" {
		t.Fatalf("大小写不同未按同一来源去重: %+v", got)
	}
}

func TestBatchMembersKeepDistinctOpaqueTaskIDs(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "多版本"}
	savePipelineRow(t, db, &sub)
	old := database.Episode{SubscriptionID: sub.ID, TorrentHash: "Task-A", SourceFile: "E01.mkv", Status: "organized"}
	savePipelineRow(t, db, &old)
	parent := database.Episode{SubscriptionID: sub.ID, TorrentHash: "task-a", Status: "downloaded"}
	savePipelineRow(t, db, &parent)
	member := parent
	member.SourceFile = old.SourceFile
	ready, err := storeBatchMembers(t.Context(), parent, []database.Episode{member})
	if err != nil || len(ready) != 1 || ready[0].ID != parent.ID {
		t.Fatalf("不同任务的同名文件应各自保留：%+v %v", ready, err)
	}
	var count int64
	if err := db.Model(&database.Episode{}).Where("source_file = ?", old.SourceFile).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("两个任务的源文件记录有 %d 条：%v", count, err)
	}
}

func TestMoviePackRequiresConfirmedOriginal(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Film", AnimeType: "movie"}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, TorrentHash: "movie-hash", OriginalName: "Film", Status: "downloaded"}
	savePipelineRow(t, db, &ep)
	root, lib := t.TempDir(), t.TempDir()
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: ep.TorrentHash, SavePath: root, ContentPath: root, Progress: 1, Status: "completed"}}
	for _, name := range []string{"Feature.mkv", "Trailer.mkv"} {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		dl.files.Files = append(dl.files.Files, downloader.CompletedFile{Path: p, Size: int64(len(name))})
	}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/TV{ext}", "Film/Movie{ext}", "", lib, lib, true, nil)}
	s.pollOrganizer(context.Background())
	var movie database.Episode
	if err := db.Where("source_file = ?", "Feature.mkv").First(&movie).Error; err != nil {
		t.Fatal(err)
	}
	if movie.Status != database.EpisodeNeedsReview || movie.ReviewKind != database.ReviewMovieSelection {
		t.Fatalf("电影归属未隔离: %+v", movie)
	}
	if err := db.Model(&movie).Updates(map[string]any{"status": "downloaded", "review_reason": ""}).Error; err != nil {
		t.Fatal(err)
	}
	s.pollOrganizer(context.Background())
	if err := db.First(&movie, movie.ID).Error; err != nil {
		t.Fatal(err)
	}
	if movie.Status != database.EpisodeNeedsReview || movie.FinalPath != "" {
		t.Fatalf("普通重试误认正片: %+v", movie)
	}
	signature := movie.ConfirmationSignature(database.ReviewMovieSelection)
	if signature == "" {
		t.Fatal("有效的正片无法确认")
	}
	if err := db.Model(&movie).Updates(map[string]any{"status": "downloaded", "review_kind": "", "review_reason": "", "confirmed_identity": signature}).Error; err != nil {
		t.Fatal(err)
	}
	s.pollOrganizer(context.Background())
	if err := db.First(&movie, movie.ID).Error; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(movie.FinalPath)
	if movie.Status != "organized" || err != nil || string(data) != "Feature.mkv" || movie.MovieFeature == nil || !*movie.MovieFeature {
		t.Fatalf("确认正片未入库: %+v %v", movie, err)
	}
	var trailer database.Episode
	if err := db.Where("source_file = ?", "Trailer.mkv").First(&trailer).Error; err != nil || trailer.Status != database.EpisodeNeedsReview || trailer.FinalPath != "" || trailer.MovieFeature != nil && *trailer.MovieFeature {
		t.Fatalf("其他电影文件被错误放行: %+v %v", trailer, err)
	}
}

func TestMissingConfirmedMemberDoesNotBlockItsSibling(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", Season: 1, AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, TorrentHash: "pack", OriginalName: "Show [01-02]", Season: 1, Status: "downloaded"}
	savePipelineRow(t, db, &ep)
	root, lib := t.TempDir(), t.TempDir()
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: "pack", SavePath: root, ContentPath: root, Progress: 1, Status: "completed"}}
	for _, name := range []string{"E01.mkv", "E02.mkv"} {
		dl.files.Files = append(dl.files.Files, downloader.CompletedFile{Path: filepath.Join(root, name), Size: 4})
	}
	if err := os.WriteFile(dl.files.Files[0].Path, []byte("good"), 0600); err != nil {
		t.Fatal(err)
	}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", true, nil)}
	s.pollOrganizer(context.Background())
	var members []database.Episode
	if err := db.Order("number").Find(&members).Error; err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0].Status != "organized" || members[1].Status != database.EpisodeNeedsReview || members[1].SourceFile != "E02.mkv" || members[1].SourceModTime != 0 || members[1].FinalPath != "" {
		t.Fatalf("缺失成员没有单独隔离: %+v", members)
	}
}

func TestBoundFileReplacementNeedsReviewBeforeRetry(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", Season: 1, AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	root, lib := t.TempDir(), t.TempDir()
	path := filepath.Join(root, "Show E01.mkv")
	if err := os.WriteFile(path, []byte("new!"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: sub.ID, TorrentHash: "abc", SourceFile: filepath.Base(path), DownloadRoot: root, FileSize: 4, SourceModTime: st.ModTime().UnixNano() - 10000, Number: 1, Season: 1, Status: "downloaded"}
	savePipelineRow(t, db, &ep)
	dl := &memberListDownloader{files: downloader.FileSelection{Files: []downloader.CompletedFile{{Path: path, Size: 4}}}}
	dl.tasks = []core.DownloadTask{{Hash: "abc", SavePath: root, ContentPath: path, Progress: 1, Status: "completed"}}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/E{ep:02}{ext}", "", "", lib, "", true, nil)}
	s.pollOrganizer(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Status != database.EpisodeNeedsReview || ep.SourceModTime != st.ModTime().UnixNano() || ep.FinalPath != "" || ep.ReviewKind != "" {
		t.Fatalf("同大小文件被替换后没有隔离: %+v", ep)
	}
	if err := db.Model(&ep).Updates(map[string]any{"status": "downloaded", "review_reason": ""}).Error; err != nil {
		t.Fatal(err)
	}
	s.pollOrganizer(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Status != "organized" {
		t.Fatalf("复验后的相同来源未入库: %+v", ep)
	}
}

func TestBatchUsesOnlyVerifiedScopedEpisodeMappings(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", Season: 2, TotalEpisodes: 10, AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, TorrentHash: "package", Season: 2, Status: "downloaded", OriginalName: "Show"}
	savePipelineRow(t, db, &ep)
	root, lib := t.TempDir(), t.TempDir()
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: "package", SavePath: root, ContentPath: root, Progress: 1, Status: "completed"}}
	for _, name := range []string{"Show - 29", "Show - 30", "Show - 31", "Extra/Show - 32"} {
		path := filepath.Join(root, name+".mkv")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		dl.files.Files = append(dl.files.Files, downloader.CompletedFile{Path: path, Size: 4})
	}
	for name, n := range map[string]float32{"Show - 29": 1, "Show - 31": 99, "Show - 32": 4} {
		if err := parser.SaveCachedTitleResult(fmt.Sprintf("sub/%d/s2/%s", sub.ID, name), "Show", 2, n, "", "", 1, "airdate"); err != nil {
			t.Fatal(err)
		}
	}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", true, nil)}
	s.pollOrganizer(context.Background())
	var rows []database.Episode
	if err := db.Order("source_file").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("错误的成员数: %d", len(rows))
	}
	for _, m := range rows {
		if m.SourceFile == "Show - 29.mkv" {
			if m.Status != "organized" || m.Season != 2 || m.Number != 1 {
				t.Fatalf("有效绝对集对应关系未采用: %+v", m)
			}
		} else if m.Status != database.EpisodeNeedsReview {
			t.Fatalf("无有效映射或属附加内容却已入库: %+v", m)
		}
	}
}

func TestSelectedBonusIsNotTheFeatureFilm(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Film", AnimeType: "movie"}
	savePipelineRow(t, db, &sub)
	ep := database.Episode{SubscriptionID: sub.ID, TorrentHash: "movie-menu", Status: "downloaded", OriginalName: "Film"}
	savePipelineRow(t, db, &ep)
	root, lib := t.TempDir(), t.TempDir()
	p := filepath.Join(root, "Extra", "Menu 1.mkv")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("menu"), 0600); err != nil {
		t.Fatal(err)
	}
	dl := &memberListDownloader{files: downloader.FileSelection{Partial: true, Files: []downloader.CompletedFile{{Path: p, Size: 4}}}}
	dl.tasks = []core.DownloadTask{{Hash: ep.TorrentHash, SavePath: root, ContentPath: root, Progress: 1, Status: "completed"}}
	s := Scheduler{downloader: dl, organizer: organizer.New("Show/TV{ext}", "Film/Movie{ext}", "", lib, lib, true, nil)}
	s.pollOrganizer(context.Background())
	if err := db.First(&ep, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Status != database.EpisodeNeedsReview || ep.ReviewKind != database.ReviewMovieSelection || ep.FinalPath != "" {
		t.Fatalf("电影附加内容被视为正片: %+v", ep)
	}
}

func TestUnknownLegacyRenameKeepsLibrarySourceForReview(t *testing.T) {
	for _, status := range []string{"renaming", "organized"} {
		t.Run(status, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "Show", Season: 1, AnimeType: "tv"}
			savePipelineRow(t, db, &sub)
			lib := t.TempDir()
			old := filepath.Join(lib, "old-E00.mkv")
			if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			ep := database.Episode{SubscriptionID: sub.ID, Title: "Show SP", OriginalName: "Show SP", Season: 1, Number: 0, Status: status, FinalPath: old}
			savePipelineRow(t, db, &ep)
			s := Scheduler{organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", true, nil)}
			if status == "organized" {
				if err := s.repairLegacyEpisodes(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			s.pollOrganizer(t.Context())
			s.pollOrganizer(t.Context())
			if err := db.First(&ep, ep.ID).Error; err != nil {
				t.Fatal(err)
			}
			if ep.Status != database.EpisodeNeedsReview || ep.ReviewKind != database.ReviewEpisodeIdentity || ep.FinalPath != old || ep.SourceModTime == 0 || filepath.Join(ep.DownloadRoot, ep.SourceFile) != old {
				t.Fatalf("历史 E00 未被原样隔离: %+v", ep)
			}
			data, err := os.ReadFile(old)
			if err != nil || string(data) != "old" {
				t.Fatalf("库内原件变更: %v", err)
			}
			if _, err := os.Stat(filepath.Join(lib, "Show")); !os.IsNotExist(err) {
				t.Fatalf("生成了额外 E00: %v", err)
			}
		})
	}
}

func TestLegacyRangeKeepsUnidentifiedOriginalAndProcessesKnownMembers(t *testing.T) {
	for _, member := range []string{"S01E01.mkv", "Extra/NCOP.mkv"} {
		t.Run(member, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "Show", Season: 1, AnimeType: "tv"}
			savePipelineRow(t, db, &sub)
			savePipelineRow(t, db, &database.Setting{Key: "scheduler_episode_identity_v2", Value: "true"})
			root, lib := t.TempDir(), t.TempDir()
			old := filepath.Join(lib, "old-E00.mkv")
			if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{member, "S01E02.mkv"} {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ep := database.Episode{SubscriptionID: sub.ID, Title: "Show [01-02]", OriginalName: "Show [01-02]", Season: 1, Status: "organized", FinalPath: old, TorrentHash: "hash"}
			savePipelineRow(t, db, &ep)
			dl := &pipelineDownloader{mockSchedDownloader: mockSchedDownloader{tasks: []core.DownloadTask{{Hash: "hash", ContentPath: root, Progress: 1, Status: "completed"}}}}
			s := Scheduler{downloader: dl, organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", true, nil)}
			if err := s.repairLegacyEpisodes(t.Context()); err != nil {
				t.Fatal(err)
			}
			s.pollOrganizer(t.Context())
			var original, known database.Episode
			if err := db.Where("final_path = ?", old).First(&original).Error; err != nil {
				t.Fatal(err)
			}
			if original.Status != database.EpisodeNeedsReview || original.ReviewKind != database.ReviewEpisodeIdentity || original.Number != 0 || original.SourceModTime == 0 {
				t.Fatalf("旧合集文件应独立复核：%+v", original)
			}
			if err := db.Where("status = ? AND number = ?", "organized", 2).First(&known).Error; err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(old); err != nil || string(data) != "old" {
				t.Fatalf("原文件变更：%q / %v", data, err)
			}
		})
	}
}

func TestConfirmedLegacySourceRemainsWhenAnotherRowOwnsTheLibraryPath(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", Season: 1, AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	lib := t.TempDir()
	old := filepath.Join(lib, "old.mkv")
	if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(old)
	if err != nil {
		t.Fatal(err)
	}
	ep := database.Episode{SubscriptionID: sub.ID, TorrentHash: "oldhash", Season: 1, Number: 1, Status: "downloaded", FinalPath: old, DownloadRoot: lib, SourceFile: "old.mkv", FileSize: st.Size(), SourceModTime: st.ModTime().UnixNano()}
	ep.ConfirmedIdentity = ep.ConfirmationSignature(database.ReviewEpisodeIdentity)
	savePipelineRow(t, db, &ep)
	savePipelineRow(t, db, &database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 3, Status: "organized", FinalPath: old})
	s := Scheduler{organizer: organizer.New("Show/S{season:02}E{ep:02}{ext}", "", "", lib, "", false, nil)}
	s.pollOrganizer(t.Context())
	if data, err := os.ReadFile(old); err != nil || string(data) != "old" {
		t.Fatalf("其他记录的库文件被移动：%q %v", data, err)
	}
	if err := db.First(&ep, ep.ID).Error; err != nil || ep.Status != database.EpisodeNeedsReview || ep.FinalPath != old {
		t.Fatalf("共享路径未保留待复核：%+v %v", ep, err)
	}
}
