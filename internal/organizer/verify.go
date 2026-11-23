package organizer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

// LibraryFileCheck 只返回核对所需的证据；宿主路径及系统错误不离开服务端。
type LibraryFileCheck struct {
	ID           uint   `json:"id"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	ExpectedSize *int64 `json:"expected_size,omitempty"`
	ActualSize   *int64 `json:"actual_size,omitempty"`
}

func reportFile(id uint, status, reason string) LibraryFileCheck {
	return LibraryFileCheck{ID: id, Status: status, Reason: reason}
}

// recordedMediaPath 只解释当前安装能识别的路径。旧版相对目标是相对于工作目录写入的。
func recordedMediaPath(configured, base, name string) (string, bool) {
	if name == "" || !core.IsVideoFile(name) || strings.ContainsRune(name, 0) {
		return "", false
	}
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".." {
			return "", false
		}
	}
	if !filepath.IsAbs(name) {
		if filepath.IsAbs(configured) || filepath.VolumeName(name) != "" || strings.HasPrefix(name, `\`) || strings.HasPrefix(name, "/") || (len(name) > 1 && name[1] == ':') {
			return "", false
		}
	}
	path, err := filepath.Abs(name)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(base, path)
	return rel, err == nil && filepath.IsLocal(rel) && rel != "."
}

func inspectRecordedFile(root *os.Root, rel string, ep database.Episode) LibraryFileCheck {
	dir := "."
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		dir = filepath.Join(dir, part)
		info, err := root.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			return reportFile(ep.ID, "missing", "")
		}
		if err != nil {
			return reportFile(ep.ID, "unverified", "access")
		}
		if i < len(parts)-1 {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return reportFile(ep.ID, "unverified", "file_type")
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return reportFile(ep.ID, "unverified", "file_type")
		}
		actual := info.Size()
		// 早期 FileSize 也可能来自 RSS 的种子大小。仅以逐文件的证据比对字节数。
		if ep.FileSize > 0 && ep.SourceFile != "" && (ep.SourceModTime != 0 || ep.MatchesFileReservation(filepath.ToSlash(ep.SourceFile))) {
			if actual != ep.FileSize {
				return LibraryFileCheck{ID: ep.ID, Status: "size_mismatch", ExpectedSize: &ep.FileSize, ActualSize: &actual}
			}
			return reportFile(ep.ID, "size_matches", "")
		}
		if actual == 0 {
			return reportFile(ep.ID, "empty", "")
		}
		return reportFile(ep.ID, "present", "")
	}
	return reportFile(ep.ID, "unverified", "path")
}

// VerifyLibraryFiles 从同一已固定的库根检查整批记录；卷标或根身份在核对中变化时废弃结果。
func (o *TVOrganizer) VerifyLibraryFiles(ctx context.Context, kind string, episodes []database.Episode) ([]LibraryFileCheck, bool, error) {
	configured := o.getBasePath(kind)
	base, err := filepath.Abs(configured)
	volume := LibraryVolume{Root: base, ID: volumeID(kind)}
	guarded := volume.ID != ""
	results := make([]LibraryFileCheck, len(episodes))
	paths := make(map[int]string)
	for i, ep := range episodes {
		switch {
		case ep.Status != "organized":
			results[i] = reportFile(ep.ID, "not_applicable", "")
		case err != nil || filepath.Dir(base) == base:
			results[i] = reportFile(ep.ID, "unverified", "library_unavailable")
		default:
			rel, ok := recordedMediaPath(configured, base, ep.FinalPath)
			if !ok {
				results[i] = reportFile(ep.ID, "unverified", "path")
			} else {
				paths[i] = rel
			}
		}
	}
	if len(paths) == 0 {
		return results, guarded, nil
	}
	root, openErr := volume.openRoot()
	if openErr == nil {
		defer root.close()
		openErr = root.checkIdentity()
	}
	if openErr == nil {
		for i, rel := range paths {
			if err := ctx.Err(); err != nil {
				return nil, guarded, err
			}
			results[i] = inspectRecordedFile(root.root, rel, episodes[i])
		}
		openErr = root.checkIdentity()
	}
	if openErr != nil {
		reason := "library_unavailable"
		if guarded {
			reason = "volume_unavailable"
		}
		for i := range paths {
			results[i] = reportFile(episodes[i].ID, "unverified", reason)
		}
	}
	return results, guarded, nil
}
