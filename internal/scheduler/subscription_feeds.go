package scheduler

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

func subscriptionTitles(sub database.Subscription) []string {
	return []string{sub.TitleCN, sub.TitleJP, sub.TitleEN}
}

// TMDB 的电视剧与电影各自分配数字 ID；明确的 movie/ 标识须保留到详情及分集查询。
func (s *Scheduler) subscriptionMetadataID(sub database.Subscription) string {
	md := s.metadataProvider
	if md == nil || sub.MetadataProvider != "" && !strings.EqualFold(sub.MetadataProvider, md.Name()) {
		return ""
	}
	id := strings.TrimSpace(sub.MetadataID)
	if sub.MetadataProvider == "" && id != "" {
		return ""
	}
	if !strings.EqualFold(md.Name(), "TMDB") {
		return id
	}
	_, lookup := sub.TMDBIdentity()
	return lookup
}

func sameAnimeTitle(sub database.Subscription, anime core.Anime) bool {
	for _, name := range subscriptionTitles(sub) {
		norm := core.NormalizeTitle(strings.TrimSpace(name))
		if norm == "" {
			continue
		}
		for _, candidate := range []string{anime.TitleCN, anime.TitleJP, anime.TitleEN} {
			if norm == core.NormalizeTitle(strings.TrimSpace(candidate)) {
				return true
			}
		}
	}
	return false
}

// 用蜜柑 RSS 自带的作品标识校验独立源，其余可包含多部作品的地址仍核对标题。
func mikanFeedMatchesID(feedURL, id string) bool {
	if id == "" {
		return false
	}
	u, err := url.Parse(feedURL)
	if err != nil || !strings.EqualFold(u.Path, "/RSS/Bangumi") {
		return false
	}
	for key, values := range u.Query() {
		if strings.EqualFold(key, "bangumiId") && len(values) == 1 && values[0] == id {
			return true
		}
	}
	return false
}

// 第二个返回值表示资源是否同时匹配不同季的订阅。
// 此时经典模式跳过自动创建，以免为已追踪的多季作品重复建订阅。
func matchFeedSubscription(item core.TorrentItem, subs []database.Subscription) (*database.Subscription, bool) {
	title := source.ParseMikanTitle(item.Title).Title
	var best *database.Subscription
	var ambiguous bool
	detail := parser.ExtractEpisodeDetailed(item.Title)
	for i := range subs {
		sub := &subs[i]
		if detail.ExplicitSeason && detail.Season > 0 && max(sub.Season, 1) != detail.Season {
			continue
		}
		if strings.EqualFold(item.SourceName, "Mikan") && item.BangumiID != "" && sub.MikanID != "" {
			if item.BangumiID == sub.MikanID {
				return sub, false
			}
			continue
		}
		if !matchesReleaseTitle(title, *sub) {
			continue
		}
		if best == nil {
			best = sub
		} else if max(best.Season, 1) != max(sub.Season, 1) {
			ambiguous = true
		}
	}
	if ambiguous {
		return nil, true
	}
	return best, false
}

var trailingReleaseTags = regexp.MustCompile(`[\[【(（].*$`)
var releaseSeason = regexp.MustCompile(`(?i)(?:\s+(?:S(?:eason)?\s*\d{1,2}|\d{1,2}(?:st|nd|rd|th)\s+season)|第\s*[一二三四五六七八九十0-9]+\s*[季期])\s*$`)

var bilingualSeparator = regexp.MustCompile(`\s+[/／]\s+|\s*[|｜]\s*`)

func releaseTitleKey(title string) string {
	title = strings.Trim(title, " \t[]【】-_")
	return core.NormalizeTitle(strings.TrimSpace(releaseSeason.ReplaceAllString(title, "")))
}

// 整个片名和带分隔符的双语别名均可匹配，无空格的斜线必须由两侧别名共同确认。
func matchesReleaseTitle(title string, sub database.Subscription) bool {
	names := subscriptionTitles(sub)
	for i := range names {
		names[i] = releaseTitleKey(names[i])
	}
	match := func(part string) bool {
		for _, candidate := range []string{part, trailingReleaseTags.ReplaceAllString(strings.TrimLeft(strings.TrimSpace(part), "[【"), "")} {
			key := releaseTitleKey(candidate)
			for _, name := range names {
				if name != "" && name == key {
					return true
				}
			}
		}
		return false
	}
	for _, part := range bilingualSeparator.Split(title, -1) {
		if match(part) {
			return true
		}
		left, right, found := strings.Cut(strings.ReplaceAll(part, "／", "/"), "/")
		if found && match(left) && match(right) {
			return true
		}
	}
	return false
}

func matchesHistoryRelease(item core.TorrentItem, sub database.Subscription, filter core.Filter) bool {
	if !matchSubscriptionFilter(item, filter) {
		return false
	}
	parsed := parser.ExtractEpisodeDetailed(item.Title)
	if parsed.ExplicitSeason && parsed.Season > 0 && sub.Season > 0 && sub.Season != parsed.Season {
		return false
	}
	if strings.EqualFold(item.SourceName, "Mikan") && sub.MikanID != "" {
		return item.BangumiID == "" || item.BangumiID == sub.MikanID
	}
	return matchesReleaseTitle(source.ParseMikanTitle(item.Title).Title, sub)
}

// 只有提供者范围内的 ID 可用于其详情接口，旧记录按标题重新核实。
func (s *Scheduler) resolveSubscriptionIDs(ctx context.Context, sub *database.Subscription) {
	updates := map[string]any{}
	if md := s.metadataProvider; md != nil && sub.MetadataID == "" &&
		(!strings.EqualFold(md.Name(), "TMDB") || sub.TMDBID == "" && !movieSubscription(*sub)) {
		lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		results, err := md.SearchAnime(lookupCtx, sub.TitleCN)
		cancel()
		if err == nil {
			var match *core.Anime
			for i := range results {
				anime := &results[i]
				if anime.ID == "" || !sameAnimeTitle(*sub, *anime) || sub.Year > 0 && anime.Year > 0 && sub.Year != anime.Year {
					continue
				}
				if match != nil && match.ID != anime.ID {
					match = nil
					break
				}
				match = anime
			}
			if match != nil {
				sub.MetadataID, sub.MetadataProvider = match.ID, md.Name()
				updates["metadata_id"], updates["metadata_provider"] = sub.MetadataID, sub.MetadataProvider
				if match.TMDBID != "" {
					sub.TMDBID = match.TMDBID
					updates["tmdb_id"] = match.TMDBID
				}
			}
		}
	}
	if sub.MikanID == "" {
		if m := source.MikanFrom(s.source); m != nil {
			if id := lookupMikanID(ctx, m, *sub); id != "" {
				sub.MikanID = id
				updates["mikan_id"] = id
				if sub.BangumiID == "" {
					sub.BangumiID = id
					updates["bangumi_id"] = id
				}
			}
		}
	}
	if len(updates) > 0 && sub.ID != 0 {
		database.DB.Model(sub).Updates(updates)
	}
}

type mikanCatalog interface {
	SearchAnime(context.Context, string) ([]core.TorrentItem, error)
	GetAnimeInfo(context.Context, string) (*source.MikanAnimeInfo, error)
}

func lookupMikanID(ctx context.Context, catalog mikanCatalog, sub database.Subscription) string {
	subject := sub.BGMSubjectID()
	searched := make(map[string]bool)
	inspected := make(map[string]bool)
	for _, alias := range subscriptionTitles(sub) {
		name := core.NormalizeTitle(strings.TrimSpace(alias))
		if name == "" || searched[name] || ctx.Err() != nil {
			continue
		}
		searched[name] = true
		results, err := catalog.SearchAnime(ctx, alias)
		if err != nil {
			continue
		}
		var match string
		for _, item := range results {
			id := item.BangumiID
			num, err := strconv.ParseUint(id, 10, 64)
			if err != nil || num == 0 || core.NormalizeTitle(strings.TrimSpace(item.Title)) != name {
				continue
			}
			if subject != "" {
				if inspected[id] {
					continue
				}
				inspected[id] = true
				info, err := catalog.GetAnimeInfo(ctx, id)
				if err == nil && info != nil && info.SubjectID == subject {
					return id
				}
				continue
			}
			if match != "" && match != id {
				return ""
			}
			match = id
		}
		if match != "" {
			return match
		}
	}
	return ""
}

func disambiguationSetting(key string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	if database.DB != nil {
		var setting database.Setting
		if database.DB.Where("key = ?", key).Find(&setting).Error == nil {
			return strings.TrimSpace(setting.Value)
		}
	}
	return ""
}

// 缓存键同时限定订阅、季和原始标题；旧映射也须符合当前订阅可验证的范围。
func cachedSubscriptionEpisode(key string, season, total int) (*parser.CachedTitleResult, bool) {
	m, ok := parser.GetCachedTitleResult(key)
	if !ok || m.Season != season || !parser.ValidEpisodeIdentity(m.Season, m.Episode) ||
		(total > 0 && m.Episode > float32(total)) || (m.ResolvedBy != "airdate" && m.ResolvedBy != "ai") {
		return nil, false
	}
	return m, true
}

func (s *Scheduler) resolveSubscriptionEpisode(ctx context.Context, item core.TorrentItem, sub database.Subscription) (int, float32) {
	info := parser.ExtractEpisodeDetailed(item.Title)
	season := info.Season
	if !info.ExplicitSeason && season == 1 && sub.Season > 1 {
		season = sub.Season
	}
	if info.IsBatch || info.IsSpecial || info.MatchedBy == "custom" || info.ExplicitSeason {
		return season, info.Episode
	}
	// 仅疑难集号需要外部消歧；历史重发的普通集数依然以标题为准。
	ambiguous := info.Episode == 0 || season > 1 && sub.TotalEpisodes > 0 && info.Episode > float32(sub.TotalEpisodes)
	if !ambiguous {
		return season, info.Episode
	}
	key := fmt.Sprintf("sub/%d/s%d/%s", sub.ID, season, item.Title)
	if result, ok := cachedSubscriptionEpisode(key, season, sub.TotalEpisodes); ok {
		return result.Season, result.Episode
	}
	if id := s.subscriptionMetadataID(sub); id != "" && !item.PublishedAt.IsZero() {
		matchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		match, err := parser.MatchEpisodeFromProvider(matchCtx, s.metadataProvider, id, season, item.PublishedAt, item.Title)
		cancel()
		if err == nil && match.Matched && match.Season == season {
			_ = parser.SaveCachedTitleResult(key, sub.TitleCN, match.Season, match.Episode, item.GroupName, item.Resolution, match.Confidence, "airdate")
			return match.Season, match.Episode
		}
	}
	enabled, _ := strconv.ParseBool(disambiguationSetting("AI_AUTO_DISAMBIGUATE"))
	if enabled && s.aiClient != nil && ctx.Err() == nil {
		limit, err := strconv.Atoi(disambiguationSetting("AI_DAILY_QUOTA_LIMIT"))
		if err != nil || limit <= 0 {
			limit = 30
		}
		ai.GetDefaultQuotaManager().SetDailyLimit(limit)
		aiCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		result, err := s.aiClient.DisambiguateTitle(aiCtx, item.Title, sub.TitleCN, item.PublishedAt)
		cancel()
		if err == nil && result != nil && result.Confidence >= 0.85 && result.Confidence <= 1 && result.Season >= 0 && result.Season <= 99 && result.Episode > 0 && result.Episode <= 2000 {
			_ = parser.SaveCachedTitleResult(key, sub.TitleCN, result.Season, result.Episode, result.Subgroup, result.Resolution, result.Confidence, "ai")
			return result.Season, result.Episode
		}
	}
	return season, info.Episode
}
