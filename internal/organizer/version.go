package organizer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// ErrVersionConflict 表示模板路径和该来源的版本路径都已被其他内容占用，需要人工处理。
var ErrVersionConflict = errors.New("目标及版本路径均已被占用")

// PreviewVersion 预览自动整理路径。调用方应传入能稳定标识所选源文件的版本键。
func (o *TVOrganizer) PreviewVersion(ctx context.Context, filePath string, anime core.Anime, episode core.Episode, versionKey string) (PreviewResult, error) {
	plan, err := o.Preview(ctx, filePath, anime, episode)
	if err != nil {
		return plan, err
	}
	plan.FinalPath, plan.Exists, err = versionTarget(ctx, filePath, plan.FinalPath, versionKey)
	return plan, err
}

// OrganizeVersionAndCommit 发布自动整理的版本。不同内容遇到同名目标时保留原文件。
func (o *TVOrganizer) OrganizeVersionAndCommit(ctx context.Context, filePath string, anime core.Anime, episode core.Episode, versionKey string, commit func(string, *MoveSource) error) (string, error) {
	if versionKey == "" {
		return "", errors.New("自动整理缺少版本标识")
	}
	return o.organizeAndCommit(ctx, filePath, anime, episode, versionKey, commit)
}

func versionTarget(ctx context.Context, src, base, key string) (string, bool, error) {
	if key == "" {
		return "", false, errors.New("自动整理缺少版本标识")
	}
	ext := filepath.Ext(base)
	sum := sha256.Sum256([]byte(key))
	suffix := fmt.Sprintf(" [v-%x]%s", sum[:8], ext)
	stem := strings.TrimSuffix(filepath.Base(base), ext)
	// 常见 NAS 文件系统限制每段文件名为 255 字节。保留完整字符、版本键和扩展名。
	for len(stem) > 0 && len(stem)+len(suffix) > 255 {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	alternate := filepath.Join(filepath.Dir(base), stem+suffix)

	// 发布后但尚未提交数据库时可能重启。即使模板路径已空出，也应先复用该来源的既有副本。
	occupied, equal, err := compareDestination(ctx, src, alternate)
	if err != nil || equal {
		return alternate, equal, err
	}
	baseExists, same, err := compareDestination(ctx, src, base)
	if err != nil || same || !baseExists {
		return base, same, err
	}
	if occupied {
		return "", false, fmt.Errorf("%w: %s", ErrVersionConflict, alternate)
	}
	return alternate, false, nil
}

func compareDestination(ctx context.Context, src, dst string) (exists, equal bool, err error) {
	target, err := os.Lstat(dst)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !target.Mode().IsRegular() {
		return true, false, nil
	}
	equal, err = SameMediaContents(ctx, src, dst)
	return true, equal, err
}

func publishVersion(ctx context.Context, src, base, key string) (string, error) {
	// 若预览后有并发发布抢先占用路径，则重新判断两个候选路径的归属。
	for attempt := 0; attempt < 3; attempt++ {
		dst, exists, err := versionTarget(ctx, src, base, key)
		if err != nil {
			return "", err
		}
		if exists {
			return dst, nil
		}
		if err := installMedia(ctx, src, dst, publishNewFile); !errors.Is(err, os.ErrExist) {
			return dst, err
		}
	}
	return "", fmt.Errorf("发布期间目标路径有并发变更: %s", base)
}
