package scheduler

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

// 聚合源也保留用于读取蜜柑详情的专用客户端。
func (s *Scheduler) mikanSource() *source.MikanSource {
	if m, ok := s.source.(*source.MikanSource); ok {
		return m
	}
	if multi, ok := s.source.(*source.MultiSource); ok {
		for _, src := range multi.Sources() {
			if m, ok := src.(*source.MikanSource); ok {
				return m
			}
		}
	}
	return nil
}

func aliases(sub database.Subscription) []string {
	return []string{sub.TitleCN, sub.TitleEN, sub.TitleJP}
}

func matchesAlias(title string, names []string) bool {
	title = core.NormalizeTitle(strings.Trim(strings.TrimSpace(title), "[]【】"))
	if title == "" {
		return false
	}
	for _, alias := range names {
		if title == core.NormalizeTitle(strings.TrimSpace(alias)) {
			return true
		}
	}
	return false
}

// 双语发布名通常以带空格的斜线或竖线分隔；保留 Fate/Zero 一类片名内的斜线。
var releaseAliasSeparator = regexp.MustCompile(`\s+/\s+|\s*\|\s*`)

func matchesRelease(info source.TitleInfo, sub database.Subscription) bool {
	if info.ExplicitSeason && sub.Season > 0 && info.Season != sub.Season {
		return false
	}
	names := aliases(sub)
	if matchesAlias(info.Title, names) {
		return true
	}
	for _, part := range releaseAliasSeparator.Split(info.Title, -1) {
		if matchesAlias(part, names) {
			return true
		}
	}
	// 无空格的中日双语片名需两侧都能核对，避免把 Fate/Zero 匹配为 Fate。
	parts := strings.FieldsFunc(info.Title, func(r rune) bool { return r == '/' || r == '／' })
	if len(parts) == 2 && matchesAlias(parts[0], names) && matchesAlias(parts[1], names) {
		return true
	}

	return false
}

type mikanCatalog interface {
	SearchAnime(context.Context, string) ([]core.TorrentItem, error)
	GetAnimeInfo(context.Context, string) (*source.MikanAnimeInfo, error)
}

// BGM.tv 收藏必须由蜜柑详情页的 subject 链接确认；绝不将两个站的数字编号互换。
func resolveMikanID(ctx context.Context, catalog mikanCatalog, sub database.Subscription) string {
	names := aliases(sub)
	searched := map[string]bool{}
	inspected := map[string]bool{}
	for _, name := range names {
		if name == "" || searched[name] || ctx.Err() != nil {
			continue
		}
		searched[name] = true
		items, err := catalog.SearchAnime(ctx, name)
		if err != nil {
			continue
		}
		matched := ""
		for _, item := range items {
			n, err := strconv.Atoi(item.BangumiID)
			if err != nil || n <= 0 || !matchesAlias(item.Title, names) {
				continue
			}
			if sub.MetadataProvider == "BGM.tv" && sub.MetadataID != "" {
				if inspected[item.BangumiID] {
					continue
				}
				inspected[item.BangumiID] = true
				info, err := catalog.GetAnimeInfo(ctx, item.BangumiID)
				if err == nil && info.SubjectID == sub.MetadataID {
					return item.BangumiID
				}
				continue
			}
			if matched != "" && matched != item.BangumiID {
				return ""
			}
			matched = item.BangumiID
		}
		if matched != "" {
			return matched
		}
	}
	return ""
}

// 老版 BangumiID 既可能是蜜柑条目号，也可能是元数据站的编号。
// 先核对已有编号；网络错误或标题不吻合期间仅允许按片名检索其他源。
func verifiedMikanID(ctx context.Context, catalog mikanCatalog, sub database.Subscription) string {
	if sub.BangumiID != "" {
		if sub.MetadataID != "" && sub.MetadataID != sub.BangumiID {
			return sub.BangumiID
		}
		info, err := catalog.GetAnimeInfo(ctx, sub.BangumiID)
		if err == nil {
			if sub.MetadataProvider == "BGM.tv" && sub.MetadataID != "" {
				if info.SubjectID == sub.MetadataID {
					return sub.BangumiID
				}
			} else if matchesAlias(info.Title, aliases(sub)) {
				return sub.BangumiID
			}
		}
	}
	return resolveMikanID(ctx, catalog, sub)
}

// 单轮轮询按订阅复用官方单集表，明确季集、自定义规则和合集无需请求元数据。
type episodeCalendars map[uint][]core.Episode

func (s *Scheduler) releaseEpisode(ctx context.Context, item core.TorrentItem, sub database.Subscription, info source.TitleInfo, calendars episodeCalendars) (int, float32) {
	ambiguous := info.Episode == 0 || (sub.Season > 1 && (sub.TotalEpisodes == 0 || info.Episode > float32(sub.TotalEpisodes)))
	if ambiguous && !info.CustomEpisode && !info.ExplicitSeason && !info.IsSpecial && !info.IsBatch && !item.PublishedAt.IsZero() && sub.MetadataID != "" && s.metadataProvider != nil && sub.MetadataProvider == s.metadataProvider.Name() {
		eps, loaded := calendars[sub.ID]
		if !loaded {
			c, cancel := context.WithTimeout(ctx, 8*time.Second)
			eps, _ = s.metadataProvider.GetEpisodes(c, sub.MetadataID, max(sub.Season, 1))
			cancel()
			calendars[sub.ID] = eps
		}
		if sub.TotalEpisodes == 0 && info.Episode > 0 {
			for _, ep := range eps {
				if ep.Number == info.Episode && ep.Season == sub.Season {
					return sub.Season, info.Episode
				}
			}
		}
		if len(eps) > 0 {
			info = source.ParseMikanTitleAdvanced(item.Title, item.PublishedAt, eps)
		}
	}
	if info.Season == 1 && sub.Season > 1 && !info.ExplicitSeason {
		info.Season = sub.Season
	}
	return info.Season, info.Episode
}
