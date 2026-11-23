package backup

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

type snapshotCounts struct {
	settings, subscriptions, episodes int
}

// 同一条 JSON 值只编码一次。v1 摘要取紧凑字节，归档本身保留缩进。
type snapshotWriter struct {
	file   *bufio.Writer
	digest hash.Hash
	pretty bytes.Buffer
}

func (w *snapshotWriter) syntax(compact, formatted string) {
	io.WriteString(w.digest, compact)
	w.file.WriteString(formatted) // bufio 保存第一个写入错误，在写值或最后 Flush 时返回。
}

func (w *snapshotWriter) value(v any, prefix string) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.digest.Write(raw)
	w.pretty.Reset()
	if err := json.Indent(&w.pretty, raw, prefix, "  "); err != nil {
		return err
	}
	_, err = w.file.Write(w.pretty.Bytes())
	return err
}

func (m *BackupManager) writeSnapshot(tx *gorm.DB, file io.Writer, includeEpisodes bool, now time.Time) (snapshotCounts, error) {
	var counts snapshotCounts
	var rows []database.Setting
	if err := tx.Find(&rows).Error; err != nil {
		return counts, err
	}
	settings := make(map[string]string, len(rows))
	for _, s := range rows {
		settings[s.Key] = s.Value
	}
	if raw, ok := settings[pluginStatusKey]; ok {
		if _, err := decodePluginStatus(raw); err != nil {
			return counts, err
		}
	}
	if _, ok := settings[customPluginsKey]; !ok {
		// 新快照显式区分空插件清单与未导出清单的旧备份。
		settings[customPluginsKey] = "[]"
	}
	if err := m.filterBackupPanelCredentials(settings); err != nil {
		return counts, err
	}
	counts.settings = len(settings)

	w := snapshotWriter{file: bufio.NewWriter(file), digest: sha256.New()}
	w.syntax("{", fmt.Sprintf("{\n  \"format_version\": %d", archiveFormat))
	for i, field := range []struct {
		name  string
		value any
	}{
		{"timestamp", now.Format(time.RFC3339)}, {"version", "v0.5.0"}, {"settings", settings},
	} {
		key := fmt.Sprintf("%q", field.name)
		if i > 0 {
			w.digest.Write([]byte(","))
		}
		w.syntax(key+":", ",\n  "+key+": ")
		if err := w.value(field.value, "  "); err != nil {
			return counts, err
		}
	}
	var err error
	counts.subscriptions, err = writeSnapshotRows[database.Subscription](tx, &w, "subscriptions", false)
	if err == nil && includeEpisodes {
		counts.episodes, err = writeSnapshotRows[database.Episode](tx, &w, "episodes", true)
	}
	if err != nil {
		return counts, err
	}
	// v1 校验的是不带 format_version 和 sha256 的对象及末尾换行。
	w.digest.Write([]byte("}\n"))
	fmt.Fprintf(w.file, ",\n  \"sha256\": \"%x\"\n}\n", w.digest.Sum(nil))
	return counts, w.file.Flush()
}

func writeSnapshotRows[T any](tx *gorm.DB, w *snapshotWriter, name string, omitEmpty bool) (int, error) {
	key := fmt.Sprintf("%q", name)
	var batch []T
	count := 0
	err := tx.FindInBatches(&batch, 128, func(_ *gorm.DB, _ int) error {
		for i := range batch {
			if count == 0 {
				w.syntax(","+key+":[", ",\n  "+key+": [")
			} else {
				w.syntax(",", ",")
			}
			w.file.WriteString("\n    ")
			if err := w.value(&batch[i], "    "); err != nil {
				return err
			}
			count++
		}
		return tx.Statement.Context.Err()
	}).Error
	if err != nil {
		return count, err
	}
	if count > 0 {
		w.syntax("]", "\n  ]")
	} else if !omitEmpty {
		w.syntax(","+key+":[]", ",\n  "+key+": []")
	}
	return count, nil
}
