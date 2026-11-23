package organizer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

const volumeMarker = ".anigo-volume-id"
const maxVolumeIDBytes = 256

// LibraryVolume 保存移动提交时实际使用的库根和标识，供重启后核验下载源清理。
type LibraryVolume struct{ Root, ID string }

func volumeID(kind string) string {
	switch normalizeAnimeType(kind) {
	case "movie":
		return os.Getenv("MOVIE_VOLUME_ID")
	case "ova":
		return os.Getenv("OVA_VOLUME_ID")
	default:
		return os.Getenv("TV_VOLUME_ID")
	}
}

// heldVolume 让正在整理的任务持有原卷的根目录，并可检测路径在整理期间被替换。
type heldVolume struct {
	volume LibraryVolume
	root   *os.Root
}

func (v LibraryVolume) hold(dst string) (*heldVolume, error) {
	if v.ID == "" {
		return nil, nil
	}
	if v.Root == "" {
		return nil, fmt.Errorf("缺少受保护的媒体库根目录")
	}
	if len(v.ID) > maxVolumeIDBytes || strings.ContainsAny(v.ID, "\r\n") {
		return nil, fmt.Errorf("媒体卷标识应为 1 至 %d 字节的单行文本", maxVolumeIDBytes)
	}
	base, err := filepath.Abs(v.Root)
	if err != nil {
		return nil, err
	}
	v.Root = base
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, fmt.Errorf("媒体卷不可用 %s: %w", base, err)
	}
	h := &heldVolume{volume: v, root: root}
	if err := h.check(dst); err != nil {
		root.Close()
		return nil, err
	}
	return h, nil
}

func (h *heldVolume) close() {
	if h != nil {
		h.root.Close()
	}
}

func (h *heldVolume) check(dst string) error {
	if h == nil {
		return nil
	}
	if err := h.checkRoot(dst); err != nil {
		return fmt.Errorf("媒体卷核验失败 %s: %w", h.volume.Root, err)
	}
	return nil
}

func (h *heldVolume) checkRoot(dst string) error {
	actual, err := os.Stat(h.volume.Root)
	if err != nil {
		return err
	}
	pinned, err := h.root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(actual, pinned) {
		return fmt.Errorf("媒体库根目录已更换")
	}
	marker, err := h.root.Lstat(volumeMarker)
	if err != nil {
		return err
	}
	if !marker.Mode().IsRegular() || marker.Size() > maxVolumeIDBytes+2 {
		return fmt.Errorf("%s 应为普通的单行标识文件", volumeMarker)
	}
	f, err := h.root.Open(volumeMarker)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(marker, opened) {
		return fmt.Errorf("卷标识文件已更换")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxVolumeIDBytes+3))
	if err != nil {
		return err
	}
	id := string(data)
	if strings.HasSuffix(id, "\n") {
		id = strings.TrimSuffix(strings.TrimSuffix(id, "\n"), "\r")
	}
	if id != h.volume.ID {
		return fmt.Errorf("%s 与配置的卷标识不符", volumeMarker)
	}
	abs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(h.volume.Root, abs)
	if err != nil || rel == "." || !filepath.IsLocal(rel) || strings.EqualFold(rel, volumeMarker) {
		return fmt.Errorf("目标不在标记文件所属的媒体目录内: %s", dst)
	}
	// 库根本身可使用挂载入口或符号链接；下层链接不能将写入引向未核验的目录。
	for dir := filepath.Dir(rel); dir != "."; dir = filepath.Dir(dir) {
		info, err := h.root.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("媒体目录含有链接或非目录: %s", dir)
		}
	}
	return nil
}

// openSidecarDir 从本次整理使用的卷根打开副文件父目录。延迟事件可在整理句柄关闭后重新核验。
func (v LibraryVolume) openSidecarDir(target string) (*os.Root, error) {
	held, err := v.hold(target)
	if err != nil {
		return nil, err
	}
	defer held.close()
	absolute, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.Rel(held.volume.Root, filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	return held.root.OpenRoot(parent)
}

// PendingMoveVolume 按历史订阅的实际类别核对库根；同名的其他分类目录不能替它证明归属。
func (o *TVOrganizer) PendingMoveVolume(ctx context.Context, ep database.Episode) (LibraryVolume, error) {
	if volumeID("tv") == "" && volumeID("movie") == "" && volumeID("ova") == "" {
		return LibraryVolume{}, nil
	}
	var sub database.Subscription
	if err := database.DB.WithContext(ctx).Unscoped().Select("anime_type").First(&sub, ep.SubscriptionID).Error; err != nil {
		return LibraryVolume{}, fmt.Errorf("无法确认历史移动记录所属订阅: %w", err)
	}
	if ep.MoveVolumeRoot == "" && normalizeAnimeType(sub.AnimeType) == "other" {
		return LibraryVolume{}, fmt.Errorf("无法确认历史移动记录的媒体分类: %q", sub.AnimeType)
	}
	root, err := filepath.Abs(o.getBasePath(sub.AnimeType))
	if err != nil {
		return LibraryVolume{}, err
	}
	if err := checkPathEscape(root, ep.FinalPath); err != nil {
		return LibraryVolume{}, fmt.Errorf("历史移动目标不在该订阅当前的媒体库内: %w", err)
	}
	return LibraryVolume{Root: root, ID: volumeID(sub.AnimeType)}, nil
}
