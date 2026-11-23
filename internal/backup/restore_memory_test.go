package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func TestRestoreReorderedArchiveWithUnknownEpisodeFields(t *testing.T) {
	const stamp = "2026-03-02T10:20:30Z"
	settings := json.RawMessage(`{"marker":"快照"}`)
	subs := json.RawMessage(`[{"ID":7,"TitleCN":"前任 <发布>","Season":1}]`)
	type testCase struct {
		name, episodes string
		wantError      bool
	}
	for _, tc := range []testCase{
		{"未知嵌套字段", `[{"ID":101,"SubscriptionID":7,"Number":1,"Title":"首映 🌙","Status":"organized","future":[1.00,{"a":"<&>","s":"\u2028"}]}]`, false},
		{"后段集号类型错误", `[{"ID":111,"SubscriptionID":7,"Number":1},{"ID":112,"SubscriptionID":7,"Number":"无效"}]`, true},
		{"后段引用无效", `[{"ID":111,"SubscriptionID":7,"Number":1},{"ID":112,"SubscriptionID":998,"Number":2}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newArchiveFixture(t, 2)
			if err := database.DB.Create(&database.Setting{Key: "marker", Value: "本机旧值"}).Error; err != nil {
				t.Fatal(err)
			}
			body := archiveBody[json.RawMessage, json.RawMessage, json.RawMessage]{stamp, "future", settings, subs, json.RawMessage(tc.episodes)}
			sum, err := archiveDigest(body)
			if err != nil {
				t.Fatal(err)
			}
			raw := fmt.Sprintf(`{"episodes":%s,"version":"future","settings":%s,"sha256":%q,"subscriptions":%s,"format_version":1,"timestamp":%q}`, tc.episodes, settings, sum, subs, stamp)
			name := "anigo_backup_20260302_102030_full.json"
			path := filepath.Join(m.backupDir, name)
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			err = m.RestoreBackup(t.Context(), name)
			if tc.wantError {
				if !errors.Is(err, ErrInvalidArchive) {
					t.Fatalf("无效数据返回 %v", err)
				}
				var count int64
				if err := database.DB.Model(&database.Episode{}).Where("id IN ?", []int{111, 112}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("未回滚: %d, %v", count, err)
				}
				var setting database.Setting
				if err := database.DB.First(&setting, "key = ?", "marker").Error; err != nil || setting.Value != "本机旧值" {
					t.Fatalf("设置未回滚: %+v, %v", setting, err)
				}
				if err := database.DB.Model(&database.Subscription{}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("订阅未回滚: %d, %v", count, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var row database.Episode
			if err := database.DB.First(&row, 101).Error; err != nil || row.Title != "首映 🌙" {
				t.Fatalf("恢复剧集 %+v, %v", row, err)
			}
			tampered := bytes.Replace([]byte(raw), []byte("1.00"), []byte("1.01"), 1)
			if err := os.WriteFile(path, tampered, 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.RestoreBackup(t.Context(), name); !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("未知字段篡改返回 %v", err)
			}
		})
	}
}
