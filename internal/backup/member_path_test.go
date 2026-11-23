package backup

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

// 使用源系统传给哈希的实际字节制作快照，同一测试可在任一目标系统运行。
func archivedMemberKey(parent uint, task, path string) string {
	return fmt.Sprintf("%d/file/%x", parent, sha256.Sum256([]byte(task+"\x00"+path)))
}

func saveMemberArchive(t *testing.T, sub database.Subscription, ep database.Episode) (*BackupManager, string) {
	t.Helper()
	m := newArchiveFixture(t, 1)
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	ep.SubscriptionID = sub.ID
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.CreateBackup(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	files, err := m.ListBackups()
	if err != nil || len(files) != 1 {
		t.Fatalf("备份清单 %+v: %v", files, err)
	}
	return m, files[0].Name
}

func TestRestorePortableMemberPaths(t *testing.T) {
	sub := database.Subscription{TitleCN: "跨机合集", Season: 1}
	const task = "Task-A"
	const windows = `Season 1\节目.mkv`
	const portable = "Season 1/节目.mkv"
	const root = `R:\下载\节目`
	const final = `R:\媒体\节目.mkv`
	windowsKey := archivedMemberKey(1, task, portable)
	ep := database.Episode{Season: 1, Number: 1, Status: "organized", TorrentHash: task, SourceFile: windows,
		ReservationKey: &windowsKey, DownloadRoot: root, FinalPath: final, FileSize: 5, SourceModTime: 123}
	m, archive := saveMemberArchive(t, sub, ep)
	newArchiveFixture(t, 1)
	occupied := database.Subscription{TitleCN: "其他订阅"}
	if err := database.DB.Create(&occupied).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	key := archivedMemberKey(sub.ID, task, portable)
	local := database.Episode{Model: gorm.Model{ID: 7}, SubscriptionID: sub.ID, Season: 1, Number: 6, TorrentHash: task,
		SourceFile: filepath.FromSlash(portable), ReservationKey: &key}
	if err := database.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	literal := database.Episode{}
	if runtime.GOOS != "windows" {
		literalKey := archivedMemberKey(sub.ID, task, windows)
		literal = database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 8, TorrentHash: task,
			SourceFile: windows, ReservationKey: &literalKey, FinalPath: "/library/literal.mkv"}
		if err := database.DB.Create(&literal).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := m.RestoreBackup(t.Context(), archive); err != nil {
			t.Fatal(err)
		}
		var rows []database.Episode
		if err := database.DB.Order("id").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) < 1 || rows[0].ID != local.ID || rows[0].Number != 1 || rows[0].ReservationKey == nil ||
			*rows[0].ReservationKey != key || rows[0].SourceFile != local.SourceFile || rows[0].DownloadRoot != root ||
			rows[0].FinalPath != final || rows[0].SourceModTime != 0 {
			t.Fatalf("未按目标系统已有成员恢复，或更改了历史路径和本机证据: %+v", rows)
		}
		if literal.ID != 0 {
			if len(rows) != 2 || rows[1].ID != literal.ID || rows[1].Number != literal.Number || rows[1].FinalPath != literal.FinalPath ||
				rows[1].ReservationKey == nil || *rows[1].ReservationKey != *literal.ReservationKey {
				t.Fatalf("同任务的字面反斜线文件被合并或改写: %+v", rows)
			}
		} else if len(rows) != 1 {
			t.Fatalf("重复导入成员: %+v", rows)
		}
	}
}

func TestRestoreLiteralAndInvalidMemberPaths(t *testing.T) {
	for _, tc := range []struct {
		name, original, signed string
		keep                   bool
	}{
		{"字面反斜线", `season\file.mkv`, `season\file.mkv`, true},
		{"上级目录", `..\file.mkv`, "../file.mkv", false},
		{"盘符相对", `C:dir\file.mkv`, "C:dir/file.mkv", false},
		{"网络共享", `\\server\share\file.mkv`, "//server/share/file.mkv", false},
		{"键不匹配", `season\file.mkv`, "other/file.mkv", false},
		{"缺少键", `season\file.mkv`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := database.Subscription{TitleCN: "路径", Season: 1}
			ep := database.Episode{SourceFile: tc.original, TorrentHash: "Task-A", Number: 1}
			if tc.signed != "" {
				key := archivedMemberKey(1, ep.TorrentHash, tc.signed)
				ep.ReservationKey = &key
			}
			m, archive := saveMemberArchive(t, sub, ep)
			newArchiveFixture(t, 1)
			err := m.RestoreBackup(t.Context(), archive)
			if tc.keep && runtime.GOOS == "windows" {
				var n int64
				if !errors.Is(err, ErrInvalidArchive) || database.DB.Model(&database.Subscription{}).Count(&n).Error != nil || n != 0 {
					t.Fatalf("无法表示的字面文件未整体回滚: %v, %d", err, n)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var row database.Episode
			if err := database.DB.First(&row).Error; err != nil || row.SourceFile != tc.original ||
				(row.ReservationKey != nil) != tc.keep || (tc.keep && *row.ReservationKey != *ep.ReservationKey) {
				t.Fatalf("来源路径或成员键有误: %+v, %v", row, err)
			}
		})
	}
}

func TestReplayPreviouslyImportedWindowsMember(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("仅 POSIX 系统曾丢失 Windows 的嵌套成员键")
	}
	for _, tc := range []struct {
		name               string
		conflict, verified bool
	}{
		{"旧来源丢失键", false, false}, {"旧来源已有键", false, true}, {"冲突", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := database.Subscription{TitleCN: "重放原归档", Season: 1}
			const rel = `Season 1\file.mkv`
			const portable = "Season 1/file.mkv"
			const task = "Task-A"
			key := archivedMemberKey(1, task, portable)
			ep := database.Episode{Model: gorm.Model{CreatedAt: time.Now().UTC().Truncate(time.Second)}, SubscriptionID: 1,
				SourceFile: rel, DownloadRoot: `D:\下载`, FileSize: 42, TorrentHash: task, ReservationKey: &key, FinalPath: `E:\媒体\file.mkv`}
			m, archive := saveMemberArchive(t, sub, ep)
			newArchiveFixture(t, 1)
			if err := database.DB.Create(&sub).Error; err != nil {
				t.Fatal(err)
			}
			old := ep
			if !tc.verified {
				old.ReservationKey = nil
			}
			old.ID = 4
			if err := database.DB.Create(&old).Error; err != nil {
				t.Fatal(err)
			}
			if tc.conflict {
				native := ep
				native.SourceFile, native.FinalPath = portable, "/library/native.mkv"
				if err := database.DB.Create(&native).Error; err != nil {
					t.Fatal(err)
				}
			}
			err := m.RestoreBackup(t.Context(), archive)
			if tc.conflict {
				if !errors.Is(err, ErrInvalidArchive) {
					t.Fatalf("两种历史路径各有候选时应整体回滚: %v", err)
				}
				var got database.Episode
				if err := database.DB.First(&got, old.ID).Error; err != nil || got.SourceFile != rel || got.ReservationKey != nil {
					t.Fatalf("歧义路径上的历史行被改变: %+v, %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var rows []database.Episode
			if err := database.DB.Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].ID != old.ID ||
				rows[0].ReservationKey == nil || *rows[0].ReservationKey != key || rows[0].SourceFile != portable || rows[0].FinalPath != ep.FinalPath {
				t.Fatalf("旧归档行未原地修复: %+v, %v", rows, err)
			}
		})
	}
}

func TestUnverifiedArchiveDoesNotReplaceBoundFile(t *testing.T) {
	const task = "Task-A"
	const rel = `Season 1\file.mkv`
	stamp := time.Now().UTC().Truncate(time.Second)
	sub := database.Subscription{TitleCN: "缺少来源证明", Season: 1}
	incoming := database.Episode{Model: gorm.Model{CreatedAt: stamp}, TorrentHash: task, SourceFile: rel, Number: 1, FinalPath: "archive.mkv"}
	m, archive := saveMemberArchive(t, sub, incoming)
	newArchiveFixture(t, 1)
	if err := database.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	local := incoming
	key := database.EpisodeFileReservationKey(sub.ID, task, rel)
	local.SubscriptionID, local.ReservationKey, local.FinalPath = sub.ID, &key, "local.mkv"
	if err := database.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := m.RestoreBackup(t.Context(), archive); err != nil {
			t.Fatal(err)
		}
		var rows []database.Episode
		if err := database.DB.Order("id").Find(&rows).Error; err != nil || len(rows) != 2 || rows[0].ID != local.ID ||
			rows[0].FinalPath != "local.mkv" || rows[0].ReservationKey == nil || *rows[0].ReservationKey != key ||
			rows[1].ReservationKey != nil || rows[1].SourceFile != rel || rows[1].FinalPath != incoming.FinalPath {
			t.Fatalf("缺少成员键的记录覆盖了本机已核实的来源或重复导入: %+v, %v", rows, err)
		}
	}
}
