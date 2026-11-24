package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
)

// 合集使用季内范围、成员使用全剧编号。总集数未知时也不能把 29–38 入库作第二季第 29–38 集。
func TestOrganizerReviewsMembersOutsideDeclaredBatchRange(t *testing.T) {
	db := pipelineDB(t)
	sub := database.Subscription{TitleCN: "Series", Season: 2, AnimeType: "tv"}
	savePipelineRow(t, db, &sub)
	root, library := t.TempDir(), t.TempDir()
	for _, version := range []string{"720p", "1080p"} {
		dest := filepath.Join(library, "Series S02E01 "+version+".mkv")
		if err := os.WriteFile(dest, []byte(version), 0600); err != nil {
			t.Fatal(err)
		}
		savePipelineRow(t, db, &database.Episode{SubscriptionID: sub.ID, Season: 2, Number: 1,
			TorrentHash: version, Status: "organized", FinalPath: dest})
	}
	parent := database.Episode{SubscriptionID: sub.ID, Season: 2, OriginalName: "[Group] Series 2nd Season [01-10] [1080p]",
		TorrentHash: "bluray", Status: "downloaded"}
	savePipelineRow(t, db, &parent)
	dl := &memberListDownloader{}
	dl.tasks = []core.DownloadTask{{Hash: parent.TorrentHash, ContentPath: root, SavePath: root, Progress: 1, Status: "completed"}}
	for absolute := 29; absolute <= 38; absolute++ {
		label := fmt.Sprint(absolute)
		if absolute == 38 {
			label += " Fin"
		}
		name := fmt.Sprintf("[LinRip][Series Season 2][BDRip][1920x1080][%s][HEVC-10bit].mkv", label)
		src := filepath.Join(root, name)
		if err := os.WriteFile(src, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		dl.files.Files = append(dl.files.Files, downloader.CompletedFile{Path: src, Size: int64(len(name))})
	}
	s := Scheduler{downloader: dl, organizer: organizer.New("Series/S{season:02}E{ep:02}{ext}", "", "", library, "", true, nil)}
	s.pollOrganizer(t.Context())
	s.pollOrganizer(t.Context())
	var records []database.Episode
	if err := db.Order("id").Find(&records).Error; err != nil || len(records) != 12 {
		t.Fatalf("应保留两个已有版本及十个独立成员：%d %v", len(records), err)
	}
	progress, err := database.FinishedEpisodeCounts(db, []uint{sub.ID})
	if err != nil || progress[sub.ID] != 1 {
		t.Fatalf("两个已整理版本应只算一集：%v %v", progress, err)
	}
	for _, ep := range records {
		if ep.TorrentHash != parent.TorrentHash {
			if data, err := os.ReadFile(ep.FinalPath); ep.Status != "organized" || err != nil || string(data) != ep.TorrentHash {
				t.Fatalf("已有版本被改写：%d %v", ep.ID, err)
			}
			continue
		}
		if ep.Status != database.EpisodeNeedsReview || ep.ReviewKind != database.ReviewEpisodeIdentity || ep.Number != 0 || ep.FinalPath != "" ||
			!ep.CanRetryOrganization() || ep.SourceModTime == 0 {
			t.Fatalf("成员没有保留来源并等待赋号：%s S%dE%g %s %q", ep.SourceFile, ep.Season, ep.Number, ep.Status, ep.ReviewReason)
		}
		if data, err := os.ReadFile(filepath.Join(ep.DownloadRoot, ep.SourceFile)); err != nil || string(data) != ep.SourceFile {
			t.Fatalf("待复核原件发生变化：%s %v", ep.SourceFile, err)
		}
		if !strings.Contains(ep.ReviewReason, "1–10") && !strings.Contains(ep.ReviewReason, "文件名不能确定") {
			t.Fatalf("复核原因没有指明冲突：%s %s", ep.SourceFile, ep.ReviewReason)
		}
	}
}

func TestBoundMemberRespectsReleaseRangeAndConfirmedIdentity(t *testing.T) {
	sub := database.Subscription{Season: 2, AnimeType: "tv"}
	ep := database.Episode{Season: 2, Number: 29, TorrentHash: "bluray", DownloadRoot: t.TempDir(),
		SourceFile: "Series S02E29.mkv", OriginalName: "Series 第二季 [01-10]", FileSize: 4, SourceModTime: 8}
	if kind, _ := boundIdentityProblem(ep, sub, 4, 8); kind != database.ReviewEpisodeIdentity {
		t.Fatalf("已绑定的旧绝对集号没有进入身份复核：%q", kind)
	}
	ep.Number = 1
	ep.ConfirmedIdentity = ep.ConfirmationSignature(database.ReviewEpisodeIdentity)
	if _, why := boundIdentityProblem(ep, sub, 4, 8); why != "" {
		t.Fatalf("已人工确认的成员仍被阻止：%s", why)
	}
}

func TestBatchRangeKeepsSeasonalMappingAndSpecials(t *testing.T) {
	for _, tc := range []struct {
		start, end, season int
		number, from       float32
		wantReview         bool
	}{
		{1, 10, 2, 1, 29, false},
		{29, 38, 2, 1, 29, false},
		{1, 10, 0, 29, 29, false},
		{1, 10, 2, 7.5, 7.5, false},
		{11, 15, 1, 9, 9, true},
		{0, 0, 2, 29, 29, false},
	} {
		if why := batchRangeReviewReason(tc.start, tc.end, tc.season, tc.number, tc.from); (why != "") != tc.wantReview {
			t.Errorf("季 %d 集 %g，源编号 %g，发行范围 %d–%d：%q", tc.season, tc.number, tc.from, tc.start, tc.end, why)
		}
	}
}
