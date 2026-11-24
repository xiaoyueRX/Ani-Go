package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

func TestOrganizationKeepsMeasuredFileSizeAfterRSSDownload(t *testing.T) {
	for _, historical := range []bool{false, true} {
		name := "新下载"
		if historical {
			name = "仅留下相对内容目录的成员名"
		}
		t.Run(name, func(t *testing.T) {
			db := pipelineDB(t)
			sub := database.Subscription{TitleCN: "迷宫饭", Season: 1, AnimeType: "tv"}
			savePipelineRow(t, db, &sub)
			root := t.TempDir()
			pack := filepath.Join(root, "Season 1")
			if err := os.Mkdir(pack, 0700); err != nil {
				t.Fatal(err)
			}
			member := "Show S01E01.mp4"
			path := filepath.Join(pack, member)
			content := []byte("matched download bytes")
			if err := os.WriteFile(path, content, 0600); err != nil {
				t.Fatal(err)
			}
			size := int64(len(content))
			ep := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "downloading",
				TorrentHash: "abcd", FileSize: 123456, OriginalName: "Show S01E01"}
			if historical {
				ep.SourceFile = member
			}
			savePipelineRow(t, db, &ep)
			dl := &memberListDownloader{}
			dl.tasks = []core.DownloadTask{{Hash: ep.TorrentHash, SavePath: root, ContentPath: pack,
				Status: "completed", Progress: 1, Size: size + 10}}
			dl.files = downloader.FileSelection{Files: []downloader.CompletedFile{{Path: path, Size: size}}}
			org := organizer.New("Show/S01E{ep:02}{ext}", "", "", t.TempDir(), "", true, nil)
			s := Scheduler{downloader: dl, organizer: org}
			ctx := context.Background()
			s.pollDownloads(ctx)
			s.pollOrganizer(ctx)
			var saved database.Episode
			if err := db.First(&saved, ep.ID).Error; err != nil {
				t.Fatal(err)
			}
			rel := filepath.Join("Season 1", member)
			st, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Status != "organized" || saved.FileSize != size || saved.SourceModTime != st.ModTime().UnixNano() ||
				saved.DownloadRoot != root || saved.SourceFile != rel || !saved.MatchesFileReservation(filepath.ToSlash(rel)) {
				t.Fatalf("入库后未保留逐文件证据：状态 %s，大小 %d，源根 %q，成员 %q，修改时间 %d，预留键 %v", saved.Status, saved.FileSize, saved.DownloadRoot, saved.SourceFile, saved.SourceModTime, saved.ReservationKey)
			}
			check, _, err := org.VerifyLibraryFiles(ctx, sub.AnimeType, []database.Episode{saved})
			if err != nil || len(check) != 1 || check[0].Status != "size_matches" {
				t.Fatalf("库文件核对失败：%+v，%v", check, err)
			}
		})
	}
}

func TestVerifiedMemberAlreadyOwnedKeepsBothHistories(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Show", Season: 1, AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	root := t.TempDir()
	member := "Show S01E01.mp4"
	src := filepath.Join(root, member)
	if err := os.WriteFile(src, []byte("same source"), 0600); err != nil {
		t.Fatal(err)
	}
	old := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 1, Status: "downloaded",
		TorrentHash: "abcd", SourceFile: member, FileSize: 123456}
	savePipelineRow(t, db, &old)
	owner := old
	owner.ID = 0
	owner.Status = "organized"
	owner.FinalPath = "prior.mp4"
	savePipelineRow(t, db, &owner)
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: old.TorrentHash, SavePath: root, ContentPath: src, Progress: 1, Status: "completed"}}
	dl.files.Files = []downloader.CompletedFile{{Path: src, Size: 11}}
	s := Scheduler{downloader: dl, organizer: organizer.New("", "", "", t.TempDir(), "", true, nil)}
	s.pollOrganizer(context.Background())
	for _, item := range []struct {
		id     uint
		status string
		path   string
	}{{old.ID, database.EpisodeNeedsReview, ""}, {owner.ID, "organized", owner.FinalPath}} {
		var ep database.Episode
		if err := db.First(&ep, item.id).Error; err != nil || ep.Status != item.status || ep.FinalPath != item.path {
			t.Fatalf("既有成员发生冲突后的记录不符：%+v，%v", ep, err)
		}
	}
}
