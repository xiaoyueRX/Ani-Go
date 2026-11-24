package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
)

// 卷身份故障按本轮、同一个库根和同一原因合并。只延后日志输出，逐文件校验照常执行。
type volumeIssue struct {
	volume organizer.LibraryVolume
	reason string
}

type volumeDiagnostic struct {
	count int
	title string
}

type volumeDiagnostics map[volumeIssue]volumeDiagnostic

func (d volumeDiagnostics) collect(ep database.Episode, err error) bool {
	var issue *organizer.VolumeUnavailableError
	if !errors.As(err, &issue) {
		return false
	}
	key := volumeIssue{volume: issue.Volume, reason: err.Error()}
	entry := d[key]
	if entry.count == 0 {
		entry.title = ep.Title
	}
	entry.count++
	d[key] = entry
	return true
}

func (d volumeDiagnostics) report() {
	for issue, entry := range d {
		if entry.count == 1 {
			log.Printf("文件整理失败 [%s]: %s", entry.title, issue.reason)
		} else {
			log.Printf("媒体卷不可用，本轮有 %d 个文件整理失败（首项 %s）: %s", entry.count, entry.title, issue.reason)
		}
	}
}

func (s *Scheduler) reportOrganizationError(ctx context.Context, ep database.Episode, err error) {
	log.Printf("文件整理失败 [%s]: %v", ep.Title, err)
	if errors.Is(err, errOrganizationReview) || errors.Is(err, organizer.ErrVersionConflict) {
		if _, markErr := database.MarkEpisodeForReview(ctx, ep, err.Error()); markErr != nil {
			log.Printf("记录剧集 %d 整理复核原因失败: %v", ep.ID, markErr)
		}
	}
}

func movieExtraSource(sub database.Subscription, source string, ep database.Episode) bool {
	titles := []string{sub.TitleCN, sub.TitleJP, sub.TitleEN}
	release := ep.OriginalName
	if release == "" {
		release = ep.Title
	}
	return core.MovieExtra(source, titles...) || core.MovieExtra(release, titles...)
}

// 只要原先确认的源指纹或身份发生变化，就要求重新确认。
func boundIdentityProblem(ep database.Episode, sub database.Subscription, size, modTime int64) (string, string) {
	movie := movieSubscription(sub)
	kind := database.ReviewEpisodeIdentity
	if movie {
		kind = database.ReviewMovieSelection
	}
	if ep.ConfirmedIdentity != "" {
		if ep.HasConfirmedIdentity(kind, size, modTime) {
			return "", ""
		}
		return kind, "上次确认的源文件或身份已经变化，请重新核对"
	}
	if ep.ReviewKind != "" {
		return ep.ReviewKind, "该文件的归属仍需人工确认"
	}
	if !movie {
		if !parser.ValidEpisodeIdentity(ep.Season, ep.Number) {
			return kind, "已绑定文件的季集身份待确认"
		}
		if reason := batchEpisodeReviewReason(ep.SourceFile, sub, ep.Season, ep.Number); reason != "" {
			return kind, reason
		}
		name := filepath.Base(ep.SourceFile)
		parsed := parser.ExtractEpisodeDetailed(strings.TrimSuffix(name, filepath.Ext(name)))
		first, last := parser.DeclaredEpisodeRange(ep.OriginalName)
		if reason := batchRangeReviewReason(first, last, ep.Season, ep.Number, parsed.Episode); reason != "" {
			return kind, reason
		}
		if parsed.ExplicitSeason && parsed.Episode > 0 && (ep.Season != parsed.Season || ep.Number != parsed.Episode) {
			return kind, "绑定文件标记的季集发生冲突，请确认"
		}
	}
	if ep.SourceModTime != 0 && (ep.SourceModTime != modTime || ep.FileSize != size) {
		return "", "下载源的大小或修改时间已变化，请核对文件后重试"
	}
	return "", ""
}

func holdBoundMember(ctx context.Context, ep database.Episode, kind, reason string, file downloader.CompletedFile) error {
	updates := map[string]any{
		"status": database.EpisodeNeedsReview, "review_kind": kind, "review_reason": reason,
		"file_size": file.Size, "source_mod_time": file.ModTime, "confirmed_identity": "", "movie_feature": false,
	}
	return ep.OrganizationSnapshot(database.DB.WithContext(ctx)).Updates(updates).Error
}

// 已绑定文件的记录始终按原成员核对；未绑定的多视频任务依据每个文件的身份拆分。
func (s *Scheduler) organizationSource(ctx context.Context, ep database.Episode, sub database.Subscription, dl core.Downloader) (string, []database.Episode, error) {
	movie := movieSubscription(sub)
	verified := verifiedBatchSource(ep, dl)
	if ep.FinalPath != "" && !movie && !legacyBatchCandidate(ep) {
		if ep.SourceFile == "" && unknownLegacyEpisode(ep, sub) {
			return "", nil, s.holdLegacyEpisode(ctx, ep, sub)
		}
		if ep.HasBoundReviewFile() && filepath.Join(ep.DownloadRoot, ep.SourceFile) == ep.FinalPath {
			current := s.legacyReviewMember(ep, sub, "核对当前媒体库原件")
			if !verified || current.DownloadRoot != ep.DownloadRoot || current.SourceModTime != ep.SourceModTime {
				return "", nil, s.holdLegacyEpisode(ctx, ep, sub)
			}
			var others int64
			if err := database.DB.WithContext(ctx).Unscoped().Model(&database.Episode{}).Where("id <> ? AND final_path = ?", ep.ID, ep.FinalPath).Count(&others).Error; err != nil {
				return "", nil, err
			}
			if others > 0 {
				return "", nil, fmt.Errorf("%w: 其他记录仍使用此历史库路径，请先核对原件归属", errOrganizationReview)
			}
		}
	}
	// 旧文件行未记录任务有几个视频。自动认定正片前重查完整清单，明确的人工确认可沿用源指纹。
	if verified && (!movie || ep.ConfirmedIdentity != "" || ep.ReviewKind != "") {
		if kind, reason := boundIdentityProblem(ep, sub, ep.FileSize, ep.SourceModTime); reason != "" {
			return "", nil, holdBoundMember(ctx, ep, kind, reason, downloader.CompletedFile{Size: ep.FileSize, ModTime: ep.SourceModTime})
		}
		return filepath.Join(ep.DownloadRoot, ep.SourceFile), nil, nil
	}
	if dl == nil || unboundHash(ep.TorrentHash) {
		return "", nil, nil
	}
	task, err := dl.GetStatus(ctx, ep.TorrentHash)
	if err != nil {
		return "", nil, err
	}
	if !task.IsComplete() {
		return "", nil, nil
	}
	content := task.ContentPath
	if content == "" && task.Name != "" {
		content = filepath.Join(task.SavePath, task.Name)
	}
	batch := ep.Number == 0 && parser.ExtractEpisodeDetailed(ep.OriginalName).IsBatch
	if lister, ok := dl.(downloader.FileLister); ok {
		files, err := lister.CompletedFiles(ctx, task)
		if err != nil {
			return "", nil, err
		}
		singleVideo := movie && files.IsSingleVideo() // 绑定成员会缩减下面的清单，必须先查整份任务。
		if ep.SourceFile != "" {
			if !filepath.IsLocal(ep.SourceFile) {
				return "", nil, fmt.Errorf("%w: 已绑定成员不是本地相对路径", errOrganizationReview)
			}
			matching := files
			matching.Files = nil
			for _, f := range files.Files {
				if f.Path == filepath.Join(task.SavePath, ep.SourceFile) || f.Path == filepath.Join(content, ep.SourceFile) {
					matching.Files = append(matching.Files, f)
				}
			}
			if len(matching.Files) != 1 {
				return "", nil, fmt.Errorf("%w: 下载器未列出唯一的已绑定视频成员", errOrganizationReview)
			}
			files = matching
		}
		selected, err := availableVideos(task.SavePath, files, batch && ep.FinalPath != "")
		if err != nil {
			return "", nil, err
		}
		selected.singleVideo = singleVideo
		if ep.SourceFile != "" {
			file := selected.Files[0]
			// 旧记录的成员名可能相对内容目录。只复用下载器实际选中的路径。
			if ep.ConfirmedIdentity != "" && filepath.Join(ep.DownloadRoot, ep.SourceFile) != file.Path {
				kind := database.ReviewEpisodeIdentity
				if movie {
					kind = database.ReviewMovieSelection
				}
				return "", nil, holdBoundMember(ctx, ep, kind, "已确认的源文件路径发生变化，需重新核对下载目录", downloader.CompletedFile{Size: ep.FileSize})
			}
			kind, why := boundIdentityProblem(ep, sub, file.Size, file.ModTime)
			if issue := selected.issues[file.Path]; issue != "" {
				return "", nil, holdBoundMember(ctx, ep, kind, issue, file)
			}
			if why != "" {
				return "", nil, holdBoundMember(ctx, ep, kind, why, file)
			}
			if movie && ep.ConfirmedIdentity == "" && (!singleVideo || movieExtraSource(sub, ep.SourceFile, ep)) {
				return "", nil, holdBoundMember(ctx, ep, database.ReviewMovieSelection, "请核对下载任务并确认该文件为电影正片", file)
			}
			return file.Path, nil, nil
		}
		members, err := s.expandBatch(ctx, ep, sub, task.SavePath, &selected)
		return "", members, err
	}
	// 兼容不提供逐文件清单的下载器，仅在任务自身的内容目录内拆分。
	if content == "" {
		return "", nil, nil
	}
	if ep.SourceFile != "" {
		if !filepath.IsLocal(ep.SourceFile) {
			return "", nil, fmt.Errorf("%w: 已绑定成员不是本地相对路径", errOrganizationReview)
		}
		if movie && ep.ConfirmedIdentity == "" {
			// 无清单的下载器只有明确以该文件为整个任务内容时，才能证明它是唯一视频。
			if filepath.Clean(content) != filepath.Join(ep.DownloadRoot, ep.SourceFile) || movieExtraSource(sub, ep.SourceFile, ep) {
				return "", nil, holdBoundMember(ctx, ep, database.ReviewMovieSelection, "请确认任务选择的电影正片", downloader.CompletedFile{Size: ep.FileSize, ModTime: ep.SourceModTime})
			}
		} else {
			content = filepath.Join(content, ep.SourceFile)
		}
	}
	info, err := os.Lstat(content)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		if !batch || ep.SourceFile != "" || (task.SavePath != "" && filepath.Clean(content) == filepath.Clean(task.SavePath)) {
			return "", nil, fmt.Errorf("%w: 下载器未提供任务专属的视频路径", errOrganizationReview)
		}
		members, err := s.expandBatch(ctx, ep, sub, content, nil)
		return "", members, err
	}
	if !existingVideo(content) || !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%w: 任务源文件不是非空的普通视频文件", errOrganizationReview)
	}
	if ep.SourceFile != "" {
		file := downloader.CompletedFile{Path: content, Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		if kind, why := boundIdentityProblem(ep, sub, file.Size, file.ModTime); why != "" {
			return "", nil, holdBoundMember(ctx, ep, kind, why, file)
		}
		return content, nil, nil
	}
	one := verifiedSelection{singleVideo: true, FileSelection: downloader.FileSelection{Files: []downloader.CompletedFile{{Path: content, Size: info.Size(), ModTime: info.ModTime().UnixNano()}}}}
	members, err := s.expandBatch(ctx, ep, sub, filepath.Dir(content), &one)
	return "", members, err
}
