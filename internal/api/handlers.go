package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/config"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/downloader"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
	"github.com/xiaoyueRX/Ani-Go/internal/migrate"
	"github.com/xiaoyueRX/Ani-Go/internal/notifier"
	v2 "github.com/xiaoyueRX/Ani-Go/internal/notifier/v2"
	"github.com/xiaoyueRX/Ani-Go/internal/organizer"
	"github.com/xiaoyueRX/Ani-Go/internal/parser"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"github.com/xiaoyueRX/Ani-Go/internal/search"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
	"github.com/xiaoyueRX/Ani-Go/internal/update"
	"gorm.io/gorm"
)

// ============================================================
// 订阅 CRUD
// ============================================================

// subscriptionResponse API 返回的订阅数据结构
type subscriptionResponse struct {
	MikanID           string  `json:"mikan_id"`
	ID                uint    `json:"id"`
	TitleCN           string  `json:"title_cn"`
	TitleEN           string  `json:"title_en"`
	TitleJP           string  `json:"title_jp"`
	Year              int     `json:"year"`
	Season            int     `json:"season"`
	BangumiID         string  `json:"bangumi_id"`
	SubgroupName      string  `json:"subgroup_name"`
	MetadataID        string  `json:"metadata_id"`
	MetadataProvider  string  `json:"metadata_provider"`
	CoverURL          string  `json:"cover_url"`
	AvatarURL         string  `json:"avatar_url"` // 兼容前端 Layout.vue
	Description       string  `json:"description"`
	AnimeType         string  `json:"anime_type"`
	TotalEpisodes     int     `json:"total_episodes"`
	CurrentEpisodes   int     `json:"current_episodes"`
	Enabled           bool    `json:"enabled"`
	Completed         bool    `json:"completed"`
	FilterJSON        string  `json:"filter_json"`
	RSSURL            string  `json:"rss_url"`
	CustomPath        string  `json:"custom_path"`
	ExcludedKeywords  string  `json:"excluded_keywords"`
	AllowedSubgroups  string  `json:"allowed_subgroups"`
	SkipBulkUpdate    bool    `json:"skip_bulk_update"`
	StallTimeoutHours int     `json:"stall_timeout_hours"`
	StalledEpisodes   int     `json:"stalled_episodes"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	DeletedAt         *string `json:"deleted_at,omitempty"`
}

type createSubscriptionRequest struct {
	MikanID          string   `json:"mikan_id"`
	MetadataID       string   `json:"metadata_id"`
	MetadataProvider string   `json:"metadata_provider"`
	Season           *int     `json:"season"`
	TitleCN          string   `json:"title_cn"`
	AnimeType        string   `json:"anime_type"`
	BangumiID        string   `json:"bangumi_id"`
	SubgroupName     string   `json:"subgroup_name"`
	SubgroupNames    []string `json:"subgroup_names"`
	AllowedSubgroups []string `json:"allowed_subgroups"`
	ExcludedKeywords string   `json:"excluded_keywords"`
	RSSURL           string   `json:"rss_url"`
	FilterJSON       string   `json:"filter_json"`
	CustomPath       string   `json:"custom_path"`
	CoverURL         string   `json:"cover_url"`
}

// validateOptionalHTTPURL 校验可选的 http/https 链接，空字符串原样放行。
func validateOptionalHTTPURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("rss_url 格式无效，仅支持 http/https 协议的完整链接")
	}
	return trimmed, nil
}

type batchSubscriptionItem struct {
	Season           *int     `json:"season"`
	MikanID          string   `json:"mikan_id"`
	MetadataID       string   `json:"metadata_id"`
	MetadataProvider string   `json:"metadata_provider"`
	TitleCN          string   `json:"title_cn"`
	BangumiID        string   `json:"bangumi_id"`
	CoverURL         string   `json:"cover_url"`
	Subgroups        []string `json:"subgroups"`
	TotalEpisodes    int      `json:"total_episodes"`
	IsFinished       bool     `json:"is_finished"`
}

type batchSubscriptionRequest struct {
	Items []batchSubscriptionItem `json:"items"`
}

type batchSubscriptionResponse struct {
	Success []batchSubResult `json:"success"`
	Failed  []batchSubResult `json:"failed"`
}

type batchSubResult struct {
	Title string `json:"title"`
	ID    uint   `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

type updateSubscriptionRequest struct {
	TitleCN           *string         `json:"title_cn"`
	TitleEN           *string         `json:"title_en"`
	TitleJP           *string         `json:"title_jp"`
	Year              *int            `json:"year"`
	Season            *int            `json:"season"`
	BangumiID         *string         `json:"bangumi_id"`
	MikanID           *string         `json:"mikan_id"`
	SubgroupName      *string         `json:"subgroup_name"`
	MetadataID        *string         `json:"metadata_id"`
	MetadataProvider  *string         `json:"metadata_provider"`
	CoverURL          *string         `json:"cover_url"`
	Description       *string         `json:"description"`
	AnimeType         *string         `json:"anime_type"`
	TotalEpisodes     *int            `json:"total_episodes"`
	Enabled           *bool           `json:"enabled"`
	Completed         *bool           `json:"completed"`
	FilterJSON        *string         `json:"filter_json"`
	RSSURL            *string         `json:"rss_url"`
	CustomPath        json.RawMessage `json:"custom_path"`
	ExcludedKeywords  *string         `json:"excluded_keywords"`
	AllowedSubgroups  *string         `json:"allowed_subgroups"`
	SkipBulkUpdate    *bool           `json:"skip_bulk_update"`
	StallTimeoutHours *int            `json:"stall_timeout_hours"`
}

type batchDeleteRequest struct {
	IDs         []uint `json:"ids"`
	DeleteFiles bool   `json:"delete_files"`
}

type batchRestoreRequest struct {
	IDs []uint `json:"ids"`
}

type batchDeleteResponse struct {
	Deleted int `json:"deleted"`
}

type batchRestoreResponse struct {
	Restored int `json:"restored"`
}

// episodeResponse API 返回的剧集数据结构
type episodeResponse struct {
	ID                uint    `json:"id"`
	SubscriptionID    uint    `json:"subscription_id"`
	Season            int     `json:"season"`
	Number            float32 `json:"number"`
	Title             string  `json:"title"`
	Status            string  `json:"status"`
	ManualStatus      string  `json:"manual_status,omitempty"`
	ReviewReason      string  `json:"review_reason"`
	ReviewKind        string  `json:"review_kind"`
	CanRetryOrganize  bool    `json:"can_retry_organize"`
	CanConfirmMovie   bool    `json:"can_confirm_movie"`
	CanAssignEpisode  bool    `json:"can_assign_episode"`
	SourceFile        string  `json:"source_file,omitempty"`
	TorrentHash       string  `json:"torrent_hash"`
	TorrentURL        string  `json:"torrent_url"`
	OriginalName      string  `json:"original_name"`
	FinalPath         string  `json:"final_path"`
	FileSize          int64   `json:"file_size"`
	IsStalled         bool    `json:"is_stalled"`
	GroupName         string  `json:"group_name"`
	DownloadStartedAt string  `json:"download_started_at,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

func toSubscriptionResponse(sub database.Subscription) subscriptionResponse {
	return subscriptionResponse{
		ID:                sub.ID,
		TitleCN:           sub.TitleCN,
		TitleEN:           sub.TitleEN,
		TitleJP:           sub.TitleJP,
		Year:              sub.Year,
		Season:            sub.Season,
		BangumiID:         sub.BangumiID,
		MikanID:           sub.MikanID,
		SubgroupName:      sub.SubgroupName,
		MetadataID:        sub.MetadataID,
		MetadataProvider:  sub.MetadataProvider,
		CoverURL:          sub.CoverURL,
		AvatarURL:         sub.CoverURL, // 暂时兼容前端 Layout.vue 头像显示
		Description:       sub.Description,
		AnimeType:         sub.AnimeType,
		TotalEpisodes:     sub.TotalEpisodes,
		CurrentEpisodes:   sub.CurrentEpisodes, // 注意：这将在 handleListSubscriptions 中被实时统计值更新
		Enabled:           sub.Enabled != nil && *sub.Enabled,
		Completed:         sub.Completed,
		FilterJSON:        sub.FilterJSON,
		RSSURL:            sub.RSSURL,
		CustomPath:        sub.CustomPath,
		ExcludedKeywords:  sub.ExcludedKeywords,
		AllowedSubgroups:  sub.AllowedSubgroups,
		SkipBulkUpdate:    sub.SkipBulkUpdate,
		StallTimeoutHours: sub.StallTimeoutHours,
		CreatedAt:         sub.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         sub.UpdatedAt.Format(time.RFC3339),
		DeletedAt: func() *string {
			if sub.DeletedAt.Valid {
				t := sub.DeletedAt.Time.Format(time.RFC3339)
				return &t
			}
			return nil
		}(),
	}
}

func toEpisodeResponse(ep database.Episode) episodeResponse {
	r := episodeResponse{
		ID:             ep.ID,
		SubscriptionID: ep.SubscriptionID,
		Season:         ep.Season,
		Number:         ep.Number,
		Title:          ep.Title,
		Status:         ep.Status,
		ManualStatus:   ep.ManualStatus,
		ReviewReason:   ep.ReviewReason,
		ReviewKind:     ep.ReviewKind,
		TorrentHash:    ep.TorrentHash,
		TorrentURL:     ep.TorrentURL,
		OriginalName:   ep.OriginalName,
		FinalPath:      ep.FinalPath,
		FileSize:       ep.FileSize,
		GroupName:      ep.GroupName,
		CreatedAt:      ep.CreatedAt.Format(time.RFC3339),
	}
	if ep.Status == database.EpisodeNeedsReview {
		r.CanRetryOrganize = ep.CanRetryOrganization()
		if ep.HasBoundReviewFile() {
			r.SourceFile = ep.SourceFile
			if ep.ConfirmationSignature(ep.ReviewKind) != "" {
				r.CanAssignEpisode = ep.ReviewKind == database.ReviewEpisodeIdentity
				r.CanConfirmMovie = ep.ReviewKind == database.ReviewMovieSelection
			}
		}
	}
	if ep.DownloadStartedAt != nil {
		r.DownloadStartedAt = ep.DownloadStartedAt.Format(time.RFC3339)
	}
	return r
}

// handleListSubscriptions 获取订阅列表
// GET /api/subscriptions?enabled=true&completed=false&deleted=false
func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	isDeleted := r.URL.Query().Get("deleted") == "true"
	var query *gorm.DB
	if isDeleted {
		query = database.DB.Unscoped().Model(&database.Subscription{}).Where("deleted_at IS NOT NULL")
	} else {
		query = database.DB.Model(&database.Subscription{})
	}

	if v := r.URL.Query().Get("enabled"); v != "" {
		b, _ := strconv.ParseBool(v)
		query = query.Where("enabled = ?", b)
	}
	if v := r.URL.Query().Get("completed"); v != "" {
		b, _ := strconv.ParseBool(v)
		query = query.Where("completed = ?", b)
	}

	var subs []database.Subscription
	orderClause := "created_at DESC"
	if isDeleted {
		orderClause = "deleted_at DESC"
	}
	if err := query.Order(orderClause).Find(&subs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "查询订阅失败"})
		return
	}

	// 批量统计超时剧集
	subIDs := make([]uint, len(subs))
	for i, sub := range subs {
		subIDs[i] = sub.ID
	}
	stalledMap := batchStalledCounts(subIDs, getStallTimeout())

	counts, err := database.FinishedEpisodeCounts(database.DB.WithContext(r.Context()), subIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取剧集进度失败"})
		return
	}
	result := make([]subscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		sub.CurrentEpisodes = counts[sub.ID]

		r := toSubscriptionResponse(sub)
		r.StalledEpisodes = stalledMap[sub.ID]
		// 可以在 response 中额外带一个进度信息（已完成/总计/下载中）
		// 虽然进度条是前端算，但这里确保基础数据正确
		result = append(result, r)
	}
	writeJSON(w, http.StatusOK, result)
}

// handleGetSubgroups 获取系统中可用的字幕组列表
// GET /api/subgroups
func (s *Server) handleGetSubgroups(w http.ResponseWriter, r *http.Request) {
	var names []string
	database.DB.Model(&database.Episode{}).
		Where("group_name != ''").
		Distinct("group_name").
		Pluck("group_name", &names)
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, names)
}

func newSubscriptionSeason(title string, requested *int) int {
	if requested == nil {
		return parser.CatalogSeason(title)
	}
	return *requested
}

// handleBatchCreateSubscriptions 批量创建订阅
// POST /api/subscriptions/batch
func (s *Server) handleBatchCreateSubscriptions(w http.ResponseWriter, r *http.Request) {
	var req batchSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	// 限制批量数量
	maxItems := 20
	if len(req.Items) > maxItems {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("批量订阅最多 %d 部", maxItems)})
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "未提供任何订阅项"})
		return
	}

	resp := batchSubscriptionResponse{
		Success: []batchSubResult{},
		Failed:  []batchSubResult{},
	}

	for _, item := range req.Items {
		item.TitleCN = strings.TrimSpace(item.TitleCN)
		if item.TitleCN == "" {
			resp.Failed = append(resp.Failed, batchSubResult{Title: item.TitleCN, Error: "番剧标题不能为空"})
			continue
		}

		season := newSubscriptionSeason(item.TitleCN, item.Season)
		if season < 0 {
			resp.Failed = append(resp.Failed, batchSubResult{Title: item.TitleCN, Error: "季号不能为负数"})
			continue
		}

		subgroup, allowed := subscriptionGroups("", item.Subgroups, nil)

		sub := database.Subscription{
			TitleCN:          item.TitleCN,
			Season:           season,
			BangumiID:        item.BangumiID,
			SubgroupName:     subgroup,
			AllowedSubgroups: allowed,
			MikanID:          item.MikanID,
			MetadataID:       item.MetadataID,
			MetadataProvider: item.MetadataProvider,
			CoverURL:         item.CoverURL,
			TotalEpisodes:    item.TotalEpisodes,
			Completed:        item.IsFinished,
			Enabled:          &[]bool{true}[0],
		}

		if err := database.CreateSubscription(database.DB.WithContext(r.Context()), &sub); err != nil {
			message := "创建订阅失败"
			if errors.Is(err, database.ErrSubscriptionExists) {
				message = "已存在订阅"
			} else {
				log.Printf("创建订阅失败: %v", err)
			}
			resp.Failed = append(resp.Failed, batchSubResult{Title: item.TitleCN, Error: message})
			continue
		}

		resp.Success = append(resp.Success, batchSubResult{Title: item.TitleCN, ID: sub.ID})
		if s.eventBus != nil {
			s.eventBus.Publish(core.Event{
				Type: core.EventSubscriptionAdded,
				Payload: map[string]any{
					"subscription_id": sub.ID,
					"title":           sub.TitleCN,
					"bangumi_id":      sub.BangumiID,
					"mikan_id":        sub.MikanID,
				},
				Time: time.Now(),
			})
		}

		// 异步补全 ID 及触发首次补全
		go func(subID uint) {
			time.Sleep(1 * time.Second)
			if s.triggerSupplement != nil {
				_ = s.triggerSupplement(context.Background(), subID)
			}
		}(sub.ID)
	}

	writeJSON(w, http.StatusCreated, resp)
}

// 多选字幕组匹配任一候选，单选兼容历史首选组字段。
func subscriptionGroups(preferred string, groups, allowed []string) (string, string) {
	if len(groups) == 1 {
		preferred = groups[0]
	} else if len(groups) > 1 {
		preferred = ""
	}
	if len(allowed) == 0 && len(groups) > 1 {
		allowed = groups
	}
	if len(allowed) == 0 {
		return preferred, "[]"
	}
	data, _ := json.Marshal(allowed)
	return preferred, string(data)
}

// handleCreateSubscription 创建新订阅
// POST /api/subscriptions
func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	req.TitleCN = strings.TrimSpace(req.TitleCN)
	if req.TitleCN == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "番剧标题 (title_cn) 不能为空"})
		return
	}

	season := newSubscriptionSeason(req.TitleCN, req.Season)
	if season < 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "季号不能为负数"})
		return
	}
	validRSSURL, err := validateOptionalHTTPURL(req.RSSURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	subgroupName, allowedSubgroupsJSON := subscriptionGroups(req.SubgroupName, req.SubgroupNames, req.AllowedSubgroups)

	sub := database.Subscription{
		TitleCN:          req.TitleCN,
		Season:           season,
		AnimeType:        strings.TrimSpace(req.AnimeType),
		MikanID:          req.MikanID,
		MetadataID:       req.MetadataID,
		MetadataProvider: req.MetadataProvider,
		BangumiID:        req.BangumiID,
		SubgroupName:     subgroupName,
		AllowedSubgroups: allowedSubgroupsJSON,
		ExcludedKeywords: req.ExcludedKeywords,
		RSSURL:           validRSSURL,
		FilterJSON:       req.FilterJSON,
		CustomPath:       req.CustomPath,
		CoverURL:         req.CoverURL,
		Enabled:          &[]bool{true}[0],
	}

	if err := database.CreateSubscription(database.DB.WithContext(r.Context()), &sub); err != nil {
		if errors.Is(err, database.ErrSubscriptionExists) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: database.ErrSubscriptionExists.Error()})
			return
		}
		log.Printf("❌ 创建订阅失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建订阅失败"})
		return
	}

	if s.eventBus != nil {
		s.eventBus.Publish(core.Event{
			Type: core.EventSubscriptionAdded,
			Payload: map[string]any{
				"subscription_id": sub.ID,
				"title":           sub.TitleCN,
				"bangumi_id":      sub.BangumiID,
				"mikan_id":        sub.MikanID,
			},
			Time: time.Now(),
		})
	}

	// 如果有 BangumiID 但前端未提供 RSS URL，后台异步解析 Mikan 字幕组 RSS
	if sub.MikanID != "" && sub.RSSURL == "" && s.mikanSrc != nil {
		go func(subID uint, bangumiID string) {
			rssURL, err := s.mikanSrc.ResolveFirstRSSURL(context.Background(), bangumiID)
			if err != nil {
				log.Printf("⚠️  自动解析 RSS URL 失败 [%s]: %v (可手动设置)", bangumiID, err)
				return
			}
			if err := database.DB.Model(&database.Subscription{}).Where("id = ?", subID).Update("rss_url", rssURL).Error; err != nil {
				log.Printf("⚠️  保存 RSS URL 失败: %v", err)
			} else {
				log.Printf("✅ 已自动解析 RSS URL [%s]: %s", bangumiID, rssURL)
			}
		}(sub.ID, sub.MikanID)
	}

	// 如果没有 BangumiID，尝试自动找回（用于处理通过 API 创建但缺失 ID 的情况）
	if sub.BangumiID == "" {
		go func(subID uint, title string) {
			time.Sleep(1 * time.Second)
			// 手动补全触发器内部已有自动找回 BangumiID 的逻辑，此处只需触发一次补全即可自动补完 ID
			if s.triggerSupplement != nil {
				_ = s.triggerSupplement(context.Background(), subID)
			}
		}(sub.ID, sub.TitleCN)
	} else if s.triggerSupplement != nil {
		go func(subID uint) {
			_ = s.triggerSupplement(context.Background(), subID)
		}(sub.ID)
	}

	log.Printf("✅ 已创建订阅: %s (ID=%d)", sub.TitleCN, sub.ID)
	writeJSON(w, http.StatusCreated, toSubscriptionResponse(sub))
}

// handleGetSubscription 获取单个订阅详情（含剧集列表）
// GET /api/subscriptions/{id}
func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的订阅 ID"})
		return
	}

	db := database.DB.WithContext(r.Context())
	var sub database.Subscription
	if err := db.First(&sub, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		} else {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取订阅失败"})
		}
		return
	}

	var episodes []database.Episode
	if err := db.Where("subscription_id = ?", sub.ID).Order("season ASC, number ASC, id ASC").Find(&episodes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取剧集失败"})
		return
	}

	timeout := getStallTimeout(sub)
	eps := make([]episodeResponse, 0, len(episodes))
	for _, ep := range episodes {
		r := toEpisodeResponse(ep)
		r.IsStalled = isEpisodeStalled(ep, timeout)
		eps = append(eps, r)
	}

	// 计算卡住总数
	stalledCount := 0
	for _, ep := range eps {
		if ep.IsStalled {
			stalledCount++
		}
	}
	subResp := toSubscriptionResponse(sub)
	counts, err := database.FinishedEpisodeCounts(db, []uint{sub.ID})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取剧集进度失败"})
		return
	}
	subResp.CurrentEpisodes = counts[sub.ID]
	subResp.StalledEpisodes = stalledCount

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscription": subResp,
		"episodes":     eps,
	})
}

// handleUpdateSubscription 更新订阅
// PUT /api/subscriptions/{id}
func (s *Server) handleUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的订阅 ID"})
		return
	}

	var req updateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	updates := map[string]interface{}{}
	if req.TitleCN != nil {
		title := strings.TrimSpace(*req.TitleCN)
		if title == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "番剧标题不能为空"})
			return
		}
		updates["title_cn"] = title
	}
	if req.TitleEN != nil {
		updates["title_en"] = *req.TitleEN
	}
	if req.TitleJP != nil {
		updates["title_jp"] = *req.TitleJP
	}
	if req.Year != nil {
		updates["year"] = *req.Year
	}
	if req.Season != nil {
		if *req.Season < 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "季号不能为负数"})
			return
		}
		updates["season"] = *req.Season
	}
	if req.BangumiID != nil {
		updates["bangumi_id"] = *req.BangumiID
	}
	if req.MikanID != nil {
		updates["mikan_id"] = strings.TrimSpace(*req.MikanID)
	}
	if req.SubgroupName != nil {
		updates["subgroup_name"] = *req.SubgroupName
	}
	if req.MetadataID != nil {
		updates["metadata_id"] = *req.MetadataID
	}
	if req.MetadataProvider != nil {
		updates["metadata_provider"] = *req.MetadataProvider
	}
	if req.CoverURL != nil {
		updates["cover_url"] = *req.CoverURL
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.AnimeType != nil {
		updates["anime_type"] = *req.AnimeType
	}
	if req.TotalEpisodes != nil {
		updates["total_episodes"] = *req.TotalEpisodes
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Completed != nil {
		updates["completed"] = *req.Completed
	}
	if req.FilterJSON != nil {
		updates["filter_json"] = *req.FilterJSON
	}
	if req.RSSURL != nil {
		validRSS, err := validateOptionalHTTPURL(*req.RSSURL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		updates["rss_url"] = validRSS
	}
	if req.CustomPath != nil {
		var path string
		if err := json.Unmarshal(req.CustomPath, &path); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "下载目录必须是字符串或 null"})
			return
		}
		updates["custom_path"] = path
	}
	if req.ExcludedKeywords != nil {
		updates["excluded_keywords"] = *req.ExcludedKeywords
	}
	if req.AllowedSubgroups != nil {
		updates["allowed_subgroups"] = *req.AllowedSubgroups
	}
	if req.SkipBulkUpdate != nil {
		updates["skip_bulk_update"] = *req.SkipBulkUpdate
	}
	if req.StallTimeoutHours != nil {
		updates["stall_timeout_hours"] = *req.StallTimeoutHours
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "未提供任何更新字段"})
		return
	}

	sub, err := database.UpdateSubscription(database.DB.WithContext(r.Context()), uint(id), updates)
	if errors.Is(err, database.ErrTMDBKindMismatch) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if errors.Is(err, database.ErrSubscriptionExists) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		return
	}
	if err != nil {
		log.Printf("❌ 更新订阅失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "更新订阅失败"})
		return
	}
	log.Printf("✅ 已更新订阅: ID=%d", sub.ID)
	writeJSON(w, http.StatusOK, toSubscriptionResponse(sub))
}

// handleDeleteSubscription 删除订阅及其关联剧集
// DELETE /api/subscriptions/{id}
func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的订阅 ID"})
		return
	}

	deleted, failure := s.deleteSubscriptions(r.Context(), []uint{uint(id)}, r.URL.Query().Get("delete_files") == "true")
	if failure != nil {
		failure.write(w)
		return
	}
	if deleted == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "订阅已删除"})
}

// handleTriggerSupplement 手动触发单个订阅的补全扫描
// POST /api/subscriptions/{id}/trigger-supplement
func (s *Server) handleTriggerSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的订阅 ID"})
		return
	}

	var sub database.Subscription
	if err := database.DB.First(&sub, id).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		return
	}

	if sub.Enabled != nil && !*sub.Enabled {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "订阅未启用，无法触发补全"})
		return
	}

	if s.triggerSupplement == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "补全调度器未配置"})
		return
	}

	go func() {
		// 不能用 r.Context() — HTTP 响应返回后会被 Cancel
		ctx, cancel := context.WithTimeout(s.jobContext(), 5*time.Minute)
		defer cancel()
		if err := s.triggerSupplement(ctx, uint(id)); err != nil {
			log.Printf("❌ 手动补全失败 [%s]: %v", sub.TitleCN, err)
		}
	}()

	log.Printf("🔍 手动触发补全: %s (ID=%d)", sub.TitleCN, sub.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "补全任务已触发，将在后台执行",
	})
}

// handleSupplementAll 一键触发所有订阅的补全扫描（并发控制）
// POST /api/subscriptions/supplement-all
func (s *Server) handleSupplementAll(w http.ResponseWriter, r *http.Request) {
	if s.triggerSupplement == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "补全调度器未配置"})
		return
	}

	var subs []database.Subscription
	if err := database.DB.Where("enabled = ?", true).Find(&subs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "查询订阅列表失败"})
		return
	}

	// 并发控制：信号量限制为 3（获取到信号量才启动任务，避免海量订阅时瞬间创建过量 goroutine）
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	// 先过滤出真正需要补全的订阅
	var targets []database.Subscription
	for _, sub := range subs {
		if sub.Completed && sub.SkipBulkUpdate {
			log.Printf("⏭️  跳过批量补全 [%s]: 已完结且设置跳过", sub.TitleCN)
			continue
		}
		targets = append(targets, sub)
	}

	go func() {
		root := s.jobContext()
		defer wg.Wait()
		for _, sub := range targets {
			select {
			case <-root.Done():
				return
			case sem <- struct{}{}:
			}
			wg.Go(func() {
				defer func() { <-sem }()
				ctx, cancel := context.WithTimeout(root, 10*time.Minute)
				defer cancel()
				if err := s.triggerSupplement(ctx, sub.ID); err != nil && root.Err() == nil {
					log.Printf("批量补全失败 [%s]: %v", sub.TitleCN, err)
				}
			})
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": fmt.Sprintf("全量补全已异步触发，共 %d 个订阅（并发限制 3）", len(targets)),
	})
}

// ============================================================
// 下载队列
// ============================================================

// handleListDownloads 获取当前下载队列
// GET /api/downloads
func (s *Server) handleListDownloads(w http.ResponseWriter, r *http.Request) {
	if s.downloader == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "下载器未配置"})
		return
	}

	tasks, err := s.downloader.List(r.Context())
	if err != nil {
		log.Printf("⚠️  获取下载列表失败 (下载器可能未启动或连接断开): %v", err)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "无法取得下载任务，请检查下载器连接"})
		return
	}

	type downloadResponse struct {
		Hash          string  `json:"hash"`
		Name          string  `json:"name"`
		SavePath      string  `json:"save_path"`
		Status        string  `json:"status"`
		DisplayStatus string  `json:"display_status"`
		CanPause      bool    `json:"can_pause"`
		CanResume     bool    `json:"can_resume"`
		Complete      bool    `json:"complete"`
		Progress      float32 `json:"progress"`
		SpeedDown     int64   `json:"speed_down"`
		SpeedUp       int64   `json:"speed_up"`
		Size          int64   `json:"size"`
		Done          int64   `json:"done"`
		Uploaded      int64   `json:"uploaded"`
		Ratio         float64 `json:"ratio"`
		SeedingTime   int64   `json:"seeding_time"`
	}

	result := make([]downloadResponse, 0, len(tasks))
	for _, t := range tasks {
		complete := t.IsComplete()
		result = append(result, downloadResponse{
			Hash:          t.Hash,
			Name:          t.Name,
			SavePath:      t.SavePath,
			Status:        t.Status,
			DisplayStatus: downloadDisplayStatus(t.Status, complete),
			CanPause:      downloader.CanPause(t.Status),
			CanResume:     downloader.CanResume(t.Status),
			Complete:      complete,
			Progress:      t.Progress,
			SpeedDown:     t.SpeedDown,
			SpeedUp:       t.SpeedUp,
			Size:          t.Size,
			Done:          t.Done,
			Uploaded:      t.Uploaded,
			Ratio:         t.Ratio,
			SeedingTime:   t.SeedingTime,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// 展示状态统一转换，API 同时保留下载器的原始状态供调用方查询。
func downloadDisplayStatus(status string, complete bool) string {
	if downloader.CanResume(status) {
		return "paused"
	}
	switch strings.ToLower(status) {
	case "active":
		if complete {
			return "completed"
		}
		return "downloading"
	case "downloading", "stalleddl", "forceddl", "metadl", "forcedmetadl":
		return "downloading"
	case "uploading", "seeding", "stalledup", "forcedup":
		return "seeding"
	case "waiting", "queued", "queueddl", "queuedup":
		return "queued"
	case "checking", "checkingdl", "checkingup", "checkingresumedata":
		return "checking"
	case "error", "missingfiles":
		return "error"
	case "complete", "completed":
		return "completed"
	}
	return status
}

// handleCreateDownload 直接添加下载任务（无需订阅）
// POST /api/downloads
func (s *Server) handleCreateDownload(w http.ResponseWriter, r *http.Request) {
	if s.downloader == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "下载器未配置"})
		return
	}

	var req struct {
		Title      string `json:"title"`
		URL        string `json:"url"`
		Magnet     string `json:"magnet"`
		InfoHash   string `json:"info_hash"`
		SavePath   string `json:"save_path"`
		GroupName  string `json:"group_name"`
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求参数格式错误"})
		return
	}

	if req.URL == "" && req.Magnet == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "必须提供 url 或 magnet 下载链接"})
		return
	}
	if req.Title == "" {
		if req.Magnet != "" {
			req.Title = "Magnet Download"
		} else {
			req.Title = "Direct Download"
		}
	}

	item := core.TorrentItem{
		Title:      req.Title,
		URL:        req.URL,
		MagnetURL:  req.Magnet,
		InfoHash:   req.InfoHash,
		GroupName:  req.GroupName,
		Resolution: req.Resolution,
	}

	if err := s.downloader.Add(r.Context(), item, req.SavePath); err != nil {
		log.Printf("❌ 直接添加下载任务失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("添加下载失败: %v", err)})
		return
	}

	log.Printf("📥 用户通过 API 直接添加下载任务: %s", req.Title)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "添加下载任务成功",
		"title":   req.Title,
	})
}

// handleDeleteDownload 删除下载任务
// DELETE /api/downloads/{hash}
func (s *Server) handleDeleteDownload(w http.ResponseWriter, r *http.Request) {
	if s.downloader == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "下载器未配置"})
		return
	}

	hash := r.PathValue("hash")
	if hash == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "缺少 hash 参数"})
		return
	}

	deleteFiles := r.URL.Query().Get("delete_files") == "true"

	if err := s.downloader.Delete(r.Context(), hash, deleteFiles); err != nil {
		log.Printf("❌ 删除下载任务失败 [%s]: %v", hash, err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("删除下载任务失败: %v", err)})
		return
	}

	log.Printf("🗑️  已删除下载任务 [%s] (delete_files=%v)", hash, deleteFiles)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "删除任务成功",
		"hash":    hash,
	})
}

// handlePauseDownload 暂停下载任务
// POST /api/downloads/{hash}/pause
func (s *Server) handlePauseDownload(w http.ResponseWriter, r *http.Request) {
	if s.downloader == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "下载器未配置"})
		return
	}

	hash := r.PathValue("hash")
	if hash == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "缺少 hash 参数"})
		return
	}

	if err := s.downloader.Pause(r.Context(), hash); err != nil {
		log.Printf("❌ 暂停下载任务失败 [%s]: %v", hash, err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("暂停任务失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "暂停任务成功",
		"hash":    hash,
	})
}

// handleResumeDownload 恢复下载任务
// POST /api/downloads/{hash}/resume
func (s *Server) handleResumeDownload(w http.ResponseWriter, r *http.Request) {
	if s.downloader == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "下载器未配置"})
		return
	}

	hash := r.PathValue("hash")
	if hash == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "缺少 hash 参数"})
		return
	}

	if err := s.downloader.Resume(r.Context(), hash); err != nil {
		log.Printf("❌ 恢复下载任务失败 [%s]: %v", hash, err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("恢复任务失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "恢复任务成功",
		"hash":    hash,
	})
}

type testDownloaderRequest struct {
	Type             string `json:"type"` // "qbittorrent" / "transmission" / "aria2"
	Host             string `json:"host"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	Secret           string `json:"secret"`
	Category         string `json:"category"`
	ReuseCredentials bool   `json:"use_saved_credentials"`
}

// 仅同一地址与账号可以沿用服务端凭据。新输入的密码或令牌始终优先。
func (req *testDownloaderRequest) useSavedCredentials(cfg config.DownloadersConfig) {
	if req.Host == "" {
		return
	}
	switch req.Type {
	case "qbittorrent":
		if req.Password == "" && req.Host == cfg.QBittorrent.Host && req.Username == cfg.QBittorrent.Username {
			req.Password = cfg.QBittorrent.Password
		}
	case "transmission":
		if req.Password == "" && req.Host == cfg.Transmission.Host && req.Username == cfg.Transmission.Username {
			req.Password = cfg.Transmission.Password
		}
	case "aria2":
		if req.Secret == "" && req.Host == cfg.Aria2.Host {
			req.Secret = cfg.Aria2.Secret
		}
	}
}

// handleTestDownloader 测试下载器连接与健康状态
// POST /api/downloader/test
func (s *Server) handleTestDownloader(w http.ResponseWriter, r *http.Request) {
	var req testDownloaderRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	var targetDL core.Downloader

	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if req.ReuseCredentials && req.Host != "" {
		values, err := settingValues(database.DB.WithContext(ctx))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取下载器配置失败"})
			return
		}
		req.useSavedCredentials(configuredDownloaders(values))
	}
	if req.Type == "" && req.Host == "" {
		// 未指定具体配置，直接探测当前系统生效的下载器
		targetDL = s.downloader
	} else {
		// 根据前端传入的临时配置构造测试实例
		switch req.Type {
		case "transmission":
			host := req.Host
			if host == "" {
				host = "http://localhost:9091"
			}
			targetDL = downloader.NewTransmission(host, req.Username, req.Password)
		case "aria2":
			host := req.Host
			if host == "" {
				host = "http://localhost:6800/jsonrpc"
			}
			targetDL = downloader.NewAria2(host, req.Secret)
		default: // qbittorrent
			host := req.Host
			if host == "" {
				host = "http://localhost:8081"
			}
			cat := req.Category
			if cat == "" {
				cat = "ani-go"
			}
			targetDL = downloader.NewQBittorrent(host, req.Username, req.Password, cat)
		}
	}

	if targetDL == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "未配置下载器或下载器不可用",
		})
		return
	}

	start := time.Now()
	available := targetDL.IsAvailable(ctx)
	elapsed := time.Since(start).Milliseconds()

	if !available {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":    false,
			"name":       targetDL.Name(),
			"latency_ms": elapsed,
			"error":      fmt.Sprintf("%s 服务无法连通，请检查地址、端口或鉴权凭据", targetDL.Name()),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"name":       targetDL.Name(),
		"latency_ms": elapsed,
		"message":    fmt.Sprintf("%s 连接正常 (响应耗时 %dms)", targetDL.Name(), elapsed),
	})
}

// ============================================================
// 死种/超时检测
// ============================================================

// getStallTimeout 获取指定订阅的超时阈值，默认从设置表读取
func getStallTimeout(sub ...database.Subscription) time.Duration {
	if len(sub) > 0 && sub[0].StallTimeoutHours > 0 {
		return time.Duration(sub[0].StallTimeoutHours) * time.Hour
	}
	var setting database.Setting
	if err := database.DB.Where("key = ?", "stall_timeout_hours").First(&setting).Error; err == nil {
		if hours, err := strconv.Atoi(setting.Value); err == nil && hours > 0 {
			return time.Duration(hours) * time.Hour
		}
	}
	return 24 * time.Hour
}

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	v := s.version
	if v == "" {
		v = "v0.6.0"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version": v,
		"changelog": []string{
			"🏛️ 微内核与插件解耦架构：主干专注于五步极速流水线，周边生态全插拔，停用时 0 协程、0 网络、0 错误日志",
			"🧠 智能消歧与 Token 熔断保护：Bangumi 放送时间对齐（0 Token 化解 95% 歧义）、终身唯一 SQLite 解析缓存与每日限额熔断",
			"🤖 原生外部 AI 控制 (MCP Server)：标准 SSE + Bearer 强鉴权，暴露 6 项业务运维工具与 4 项系统治理工具",
			"🛡️ 做种生命周期与磁盘防爆急停：完成做种仅删任务记录永不删物理硬链接，剩余空间 < 5GB 自动熔断阻断新增下载",
			"👁️ 媒体整理 Dry-Run 预检：提供路径预览与冲突检测 API，无写入安全预测整理结果",
			"⚡ 下载器高可用与深度加固：Aria2 / Transmission URL 自动清洗、qB 超时防护、下载器连通性一键测试",
		},
	})
}

// handleCheckUpdate 检查云端最新版本
func (s *Server) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	mgr := update.DefaultManager
	if mgr == nil {
		mgr = update.InitManager(s.version)
	}
	info, err := mgr.CheckUpdate(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"current_version": s.version,
			"has_update":      false,
			"error":           err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleGetUpdateProgress 获取更新进度
func (s *Server) handleGetUpdateProgress(w http.ResponseWriter, r *http.Request) {
	mgr := update.DefaultManager
	if mgr == nil {
		mgr = update.InitManager(s.version)
	}
	writeJSON(w, http.StatusOK, mgr.GetProgress())
}

// handleExecuteUpdate 执行一键平滑更新
func (s *Server) handleExecuteUpdate(w http.ResponseWriter, r *http.Request) {
	mgr := update.DefaultManager
	if mgr == nil {
		mgr = update.InitManager(s.version)
	}
	if err := mgr.ExecuteUpdate(r.Context()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "已成功下发平滑更新指令"})
}

// countStalledEpisodes 统计订阅下所有卡住的剧集数
func countStalledEpisodes(subID uint, timeout time.Duration) int {
	cutoff := time.Now().Add(-timeout)
	var count int64
	database.DB.Model(&database.Episode{}).
		Where("subscription_id = ?", subID).
		Where(database.StalledEpisodePredicate).
		Where("(status = 'pending' AND created_at < ?) OR (status = 'downloading' AND download_started_at < ?)", cutoff, cutoff).
		Count(&count)
	return int(count)
}

// batchStalledCounts 批量获取多个订阅的超时剧集数
func batchStalledCounts(subIDs []uint, timeout time.Duration) map[uint]int {
	if len(subIDs) == 0 {
		return map[uint]int{}
	}
	cutoff := time.Now().Add(-timeout)
	type stalledResult struct {
		SubscriptionID uint
		Count          int64
	}
	var results []stalledResult
	database.DB.Model(&database.Episode{}).
		Select("subscription_id, count(*) as count").
		Where("subscription_id IN ?", subIDs).
		Where(database.StalledEpisodePredicate).
		Where("(status = 'pending' AND created_at < ?) OR (status = 'downloading' AND download_started_at < ?)", cutoff, cutoff).
		Group("subscription_id").
		Find(&results)

	m := make(map[uint]int, len(results))
	for _, r := range results {
		m[r.SubscriptionID] = int(r.Count)
	}
	return m
}

// isEpisodeStalled 判断单个剧集是否超时
func isEpisodeStalled(ep database.Episode, timeout time.Duration) bool {
	cutoff := time.Now().Add(-timeout)
	switch ep.Status {
	case "pending":
		return ep.CreatedAt.Before(cutoff)
	case "downloading":
		return ep.DownloadStartedAt != nil && ep.DownloadStartedAt.Before(cutoff)
	default:
		return false
	}
}

// handleUpdateEpisodeStatus 更新或清除手工标记，保留下载器的状态和重试时间。
func (s *Server) handleUpdateEpisodeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的剧集 ID"})
		return
	}
	var req struct {
		Status *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "需要提供状态值"})
		return
	}
	switch *req.Status {
	case "", "pending", "downloading", "completed", "failed":
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的状态值"})
		return
	}
	updated := database.DB.WithContext(r.Context()).Model(&database.Episode{}).Where("id = ?", id).UpdateColumn("manual_status", *req.Status)
	if updated.Error != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "更新失败"})
		return
	}
	if updated.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "剧集不存在"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": *req.Status, "manual_status": *req.Status})
}

// ============================================================
// 设置
// ============================================================

type settingsRequest struct {
	Settings map[string]string `json:"settings"`
}

// handleGetSettings 获取所有设置
// GET /api/settings
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	var settings []database.Setting
	if err := database.DB.WithContext(r.Context()).Find(&settings).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "获取设置失败"})
		return
	}

	result := make(map[string]string, len(settings))
	for _, setting := range settings {
		if internalPluginSetting(setting.Key) || strings.EqualFold(setting.Key, "MCP_TOKEN_CONFIGURED") {
			continue
		}
		val := setting.Value
		if strings.EqualFold(setting.Key, "MCP_TOKEN") {
			result["MCP_TOKEN_CONFIGURED"] = strconv.FormatBool(strings.TrimSpace(val) != "")
			continue
		}
		if sensitiveSetting(setting.Key) {
			val = ""
		}
		result[setting.Key] = val
	}
	writeJSON(w, http.StatusOK, result)
}

// handleUpdateSettings 批量更新设置
// PUT /api/settings
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if len(req.Settings) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "未提供任何设置项"})
		return
	}

	if err := validatePollingIntervals(req.Settings); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	var cfg config.DownloadersConfig
	var changed bool
	newAvatar, avatarChanged := req.Settings[avatarSettingKey]
	var previousAvatar string
	err := database.DB.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if avatarChanged {
			if err := tx.Model(&database.Setting{}).Select("value").Where("key = ?", avatarSettingKey).Scan(&previousAvatar).Error; err != nil {
				return err
			}
		}
		var err error
		cfg, changed, err = applySettings(tx, req.Settings)
		return err
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "保存设置失败"})
		return
	}

	if avatarChanged && previousAvatar != newAvatar {
		removeUploadedAvatar(previousAvatar)
	}

	log.Printf("✅ 已更新 %d 项设置", len(req.Settings))
	for _, key := range []string{"RSS_INTERVAL", "ORGANIZER_INTERVAL", "SUPPLEMENT_INTERVAL", "BGMTV_SYNC_INTERVAL", "SEED_CLEANUP_INTERVAL"} {
		if _, ok := req.Settings[key]; ok && s.reloadIntervals != nil {
			s.reloadIntervals()
			break
		}
	}

	// 热重载通知系统（全渠道即时生效）
	if s.notifyMgr != nil {
		s.notifyMgr.ReloadNotifiers(notifier.BuildAllNotifiersFromSettingsAndEnv())
	}

	// 热重载下载器实例（支持切换 qBittorrent/Transmission/Aria2 无需重启）
	if dyn, ok := s.downloader.(*downloader.DynamicDownloader); ok && changed {
		dyn.Swap(downloader.NewConfigured(cfg))
	}

	// 热重载自定义正则规则
	source.LoadCustomPatternsFromSettings(func(key string) (string, bool) {
		var setting database.Setting
		if err := database.DB.Where("key = ?", key).First(&setting).Error; err != nil {
			return "", false
		}
		return setting.Value, true
	})

	_, cronChanged := req.Settings["BACKUP_CRON"]
	_, keepChanged := req.Settings["BACKUP_KEEP_COUNT"]
	if s.backupManager != nil && (cronChanged || keepChanged) {
		if err := s.backupManager.ReloadSchedule(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "设置已保存，定时备份重载失败"})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "设置已更新"})
}

// handleGetLogs 读取系统日志尾部，默认返回最近 100 行。
// GET /api/logs?lines=50
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	linesStr := r.URL.Query().Get("lines")
	lines := 100
	if linesStr != "" {
		if n, err := strconv.Atoi(linesStr); err == nil && n > 0 && n <= 500 {
			lines = n
		}
	}

	f, err := os.Open(s.logPath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"lines": []string{},
			"total": 0,
		})
		return
	}
	defer f.Close()

	recent, total := readLogTail(f, lines)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lines": recent,
		"total": total,
	})
}

func readLogTail(f *os.File, limit int) ([]string, int) {
	info, err := f.Stat()
	if err != nil {
		return []string{}, 0
	}
	var buf [4096]byte
	var chunks []string
	breaks := 0
	for pos := info.Size(); pos > 0 && breaks < limit; {
		n := min(pos, int64(len(buf)))
		pos -= n
		if _, err := f.ReadAt(buf[:n], pos); err != nil {
			return []string{}, 0
		}
		// 每个块保留独立内容，只统计本块的换行，最后一次合并。
		chunk := string(buf[:n])
		if len(chunks) == 0 {
			chunk = strings.TrimRight(chunk, "\n")
			if chunk == "" {
				continue
			}
		}
		chunks = append(chunks, chunk)
		breaks += strings.Count(chunk, "\n")
	}
	if len(chunks) == 0 {
		return []string{}, 0
	}
	slices.Reverse(chunks)
	all := strings.Split(strings.Join(chunks, ""), "\n")
	return all[max(0, len(all)-limit):], len(all)
}

// handleGetCustomRegex 获取当前自定义正则规则
// GET /api/settings/custom-regex
func (s *Server) handleGetCustomRegex(w http.ResponseWriter, r *http.Request) {
	rawPatterns := make([]string, 10)
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("custom_regex_%d", i)
		var setting database.Setting
		if err := database.DB.Where("key = ?", key).First(&setting).Error; err == nil {
			rawPatterns[i] = strings.TrimSpace(setting.Value)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"patterns":      rawPatterns,
		"compiled":      source.GetCustomRegexPatterns(),
		"builtin_count": 8,
	})
}

// handleReloadCustomRegex 从数据库重新加载自定义正则
// POST /api/settings/custom-regex/reload
func (s *Server) handleReloadCustomRegex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}
	source.LoadCustomPatternsFromSettings(func(key string) (string, bool) {
		var setting database.Setting
		if err := database.DB.Where("key = ?", key).First(&setting).Error; err != nil {
			return "", false
		}
		return setting.Value, true
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "自定义正则已重新加载",
		"compiled": source.GetCustomRegexPatterns(),
	})
}

// handleTestCustomRegex 测试自定义正则解析效果
// POST /api/settings/custom-regex/test
func (s *Server) handleTestCustomRegex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请输入测试标题"})
		return
	}

	info := source.ParseMikanTitle(req.Title)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"title":          info.Title,
		"raw_title":      info.RawTitle,
		"subgroup":       info.Subgroup,
		"season":         info.Season,
		"catalog_season": parser.CatalogSeason(req.Title),
		"episode":        info.Episode,
		"resolution":     info.Resolution,
		"is_batch":       info.IsBatch,
		"is_special":     info.IsSpecial,
	})
}

// ============================================================
// 插件管理
// ============================================================

// ============================================================
// 搜索番剧
// ============================================================

// handleSearchAnime 搜索番剧资源
// GET /api/search?q=xxx
func (s *Server) handleSearchAnime(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "搜索关键词不能为空"})
		return
	}

	if s.multiSrc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "搜索服务未配置"})
		return
	}

	items, err := s.multiSrc.SearchAnime(r.Context(), q)
	if err != nil {
		log.Printf("⚠️  搜索失败 [%s]: %v", q, err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "搜索失败: " + err.Error()})
		return
	}

	if items == nil {
		items = []core.TorrentItem{}
	}

	log.Printf("🔍 搜索完成 [%s]: 找到 %d 个结果", q, len(items))
	writeJSON(w, http.StatusOK, items)
}

// handleMikanGroups 获取 Mikan 番剧的字幕组列表
// GET /api/mikan/groups?bangumi_id=xxx
func (s *Server) handleMikanGroups(w http.ResponseWriter, r *http.Request) {
	bangumiID := r.URL.Query().Get("bangumi_id")
	if bangumiID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "bangumi_id 不能为空"})
		return
	}

	if s.mikanSrc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "Mikan 服务未初始化"})
		return
	}

	groups, err := s.mikanSrc.FetchSubgroups(r.Context(), bangumiID)
	if err != nil {
		log.Printf("⚠️  获取 Mikan 字幕组失败 [%s]: %v", bangumiID, err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "获取字幕组失败: " + err.Error()})
		return
	}

	if groups == nil {
		groups = []source.SubgroupInfo{}
	}

	writeJSON(w, http.StatusOK, groups)
}

// handleSchedule 获取指定季度新番时间表
// GET /api/schedule?year=2025&season=2

// handleScheduleBangumi 获取 Bangumi 渠道的时间表
// GET /api/schedule/bangumi

// handleGetBangumiSubject 获取指定番剧的详细元数据（含简介、全集标题等）
// GET /api/bangumi/subject/:id
func (s *Server) getBGMProvider() *metadata.BGMTVProvider {
	if s.md != nil {
		if p, ok := s.md.(*metadata.BGMTVProvider); ok {
			return p
		}
	}
	var token string
	var setting database.Setting
	if err := database.DB.Where("key = ?", "BGMTV_USER_TOKEN").First(&setting).Error; err == nil {
		token = setting.Value
	}
	var mirrors []string
	if err := database.DB.Where("key = ?", "BGMTV_MIRROR_DOMAINS").First(&setting).Error; err == nil && setting.Value != "" {
		mirrors = strings.Split(setting.Value, ",")
	}
	if len(mirrors) == 0 {
		mirrors = []string{"api.bgm.tv", "api.bangumi.tv", "api.chii.in"}
	}
	return metadata.NewBGMTVProvider(token, mirrors)
}

func (s *Server) handleGetBangumiSubject(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/bangumi/subject/"):]
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "ID 不能为空"})
		return
	}

	p := s.getBGMProvider()
	if p == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "Bangumi 服务未就绪"})
		return
	}

	// 1. 获取基础元数据 (Anime 结构)
	anime, err := p.GetAnime(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	// 2. 获取剧集列表 (Episode 结构)
	episodes, err := p.GetEpisodes(r.Context(), id, 1)
	if err != nil {
		log.Printf("⚠️  获取 BGM 剧集列表失败 [%s]: %v", id, err)
		episodes = []core.Episode{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"anime":    anime,
		"episodes": episodes,
	})
}

func (s *Server) handleScheduleBangumi(w http.ResponseWriter, r *http.Request) {
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	season, _ := strconv.Atoi(r.URL.Query().Get("season"))

	now := time.Now()
	currYear := now.Year()
	currSeason := int((now.Month()-1)/3) + 1 // 1: 1-3月, 2: 4-6月, 3: 7-9月, 4: 10-12月

	weekLabel := map[int]string{
		1: "星期一", 2: "星期二", 3: "星期三", 4: "星期四",
		5: "星期五", 6: "星期六", 7: "星期日",
	}

	var schedule []source.WeekDayItem

	// 如果选定的是其他特定历史/未来季度，则通过季度新番源获取按星期归类的番剧，实现同步历史季度
	if year > 0 && season > 0 && (year != currYear || season != currSeason) {
		var err error
		if s.mikanSrc != nil {
			schedule, err = s.mikanSrc.FetchWeekSchedule(r.Context(), year, season)
		}
		if (err != nil || len(schedule) == 0) && s.yucSrc != nil && s.pluginManager != nil && s.pluginManager.IsPluginActive("yuc_schedule") {
			yucCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			schedule, _ = s.yucSrc.FetchWeekSchedule(yucCtx, year, season)
			cancel()
		}
	} else {
		// 当前季度/实时：优先使用 Bangumi 每日放送官方日历接口
		p := s.getBGMProvider()
		if p != nil {
			items, bErr := p.GetCalendar(r.Context())
			if bErr == nil && len(items) > 0 {
				groups := make(map[int][]core.TorrentItem)
				for _, item := range items {
					day, _ := strconv.Atoi(item.EpisodeURL)
					if day == 0 {
						day = 7
					} // Bangumi 0 可能表示周日
					item.EpisodeURL = "" // 清除 hack 字段
					groups[day] = append(groups[day], item)
				}

				for i := 1; i <= 7; i++ {
					dayItems := groups[i]
					if dayItems == nil {
						dayItems = []core.TorrentItem{}
					}
					schedule = append(schedule, source.WeekDayItem{
						DayOfWeek: i,
						Label:     weekLabel[i],
						Items:     dayItems,
					})
				}
			}
		}

		// 若 Bangumi 获取失败，回退至 Mikan 当前季度时间表
		if len(schedule) == 0 && s.mikanSrc != nil {
			schedule, _ = s.mikanSrc.FetchWeekSchedule(r.Context(), currYear, currSeason)
		}
	}

	// 所选季度（含当前季度）在全部数据源均无内容时，回退至最近一个有数据的季度
	fallbackYear, fallbackSeason := 0, 0
	if len(schedule) == 0 {
		targetYear, targetSeason := year, season
		if targetYear <= 0 || targetSeason <= 0 {
			targetYear, targetSeason = currYear, currSeason
		}
		if candidate, fy, fs := s.fallbackPreviousSeason(r.Context(), targetYear, targetSeason, 3); len(candidate) > 0 {
			schedule = candidate
			fallbackYear, fallbackSeason = fy, fs
			log.Printf("ℹ️  [时间表/Bangumi] %d 年 %d 季度暂无内容，已自动回退至 %d 年 %d 季度", targetYear, targetSeason, fy, fs)
		}
	}

	// 同时获取订阅列表，标注已订阅的番剧
	var subs []database.Subscription
	database.DB.Find(&subs)
	schedule, subscribed := scheduleSubscriptions(schedule, subs)
	subStats := make(map[uint]map[string]int)
	for _, sub := range subs {
		subStats[sub.ID] = map[string]int{
			"downloaded": int(sub.CurrentEpisodes),
			"total":      sub.TotalEpisodes,
		}
	}

	resp := map[string]interface{}{
		"days":       schedule,
		"subscribed": subscribed,
		"stats":      subStats,
	}
	if fallbackYear > 0 {
		resp["fallback_year"] = fallbackYear
		resp["fallback_season"] = fallbackSeason
		resp["requested_year"] = year
		resp["requested_season"] = season
	}
	writeJSON(w, http.StatusOK, resp)
}

// fallbackPreviousSeason 当所选季度在数据源暂无内容时，向前回溯最近的可用季度
// 蜜柑计划对新季度时间表存在发布延迟，回退可保证界面始终有内容可看；返回回退到的年月与季度
func (s *Server) fallbackPreviousSeason(ctx context.Context, year, season, maxBack int) ([]source.WeekDayItem, int, int) {
	if year <= 0 || season <= 0 || maxBack <= 0 {
		return nil, 0, 0
	}

	yucEnabled := s.pluginManager != nil && s.pluginManager.IsPluginActive("yuc_schedule")
	fy, fs := year, season

	// 整体预算 20s，避免多次回退叠加拖垮前端请求（前端超时 30s）
	budgetCtx, budgetCancel := context.WithTimeout(ctx, 20*time.Second)
	defer budgetCancel()

	for i := 0; i < maxBack; i++ {
		if budgetCtx.Err() != nil {
			break
		}
		fs--
		if fs < 1 {
			fs, fy = 4, fy-1
		}
		if fy < 2000 {
			break
		}

		prevCtx, cancel := context.WithTimeout(budgetCtx, 10*time.Second)
		var candidate []source.WeekDayItem
		if s.mikanSrc != nil {
			candidate, _ = s.mikanSrc.FetchWeekSchedule(prevCtx, fy, fs)
		}
		if len(candidate) == 0 && yucEnabled && s.yucSrc != nil {
			candidate, _ = s.yucSrc.FetchWeekSchedule(prevCtx, fy, fs)
		}
		cancel()

		if len(candidate) > 0 {
			return candidate, fy, fs
		}
	}
	return nil, 0, 0
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	season, _ := strconv.Atoi(r.URL.Query().Get("season"))
	sourceParam := r.URL.Query().Get("source")

	var schedule []source.WeekDayItem
	var err error

	yucEnabled := s.pluginManager != nil && s.pluginManager.IsPluginActive("yuc_schedule")

	// 1. 若显式请求 yuc 且 yuc_schedule 插件已启用
	if sourceParam == "yuc" && yucEnabled && s.yucSrc != nil {
		yucCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		schedule, err = s.yucSrc.FetchWeekSchedule(yucCtx, year, season)
		cancel()
	}

	// 2. 默认使用 Mikan 蜜柑计划数据源
	if len(schedule) == 0 && s.mikanSrc != nil {
		schedule, err = s.mikanSrc.FetchWeekSchedule(r.Context(), year, season)
		if err != nil {
			log.Printf("⚠️  Mikan 获取时间表失败: %v", err)
			// 若 Mikan 失败且启用了 yuc 插件，作为第二备用回退
			if yucEnabled && s.yucSrc != nil {
				yucCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				schedule, err = s.yucSrc.FetchWeekSchedule(yucCtx, year, season)
				cancel()
			}
		}
	}

	if len(schedule) == 0 {
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "获取时间表失败: " + err.Error()})
			return
		}
		schedule = []source.WeekDayItem{}
	}

	// 3. 所选季度暂无数据时，自动向前回溯最近一个已发布数据的季度（最多回溯 3 个季度）
	//    蜜柑计划对新季度时间表的发布存在延迟，此处保证界面始终有可用内容，并返回回退标记供前端提示
	fallbackYear, fallbackSeason := 0, 0
	if len(schedule) == 0 && year > 0 && season > 0 {
		if candidate, fy, fs := s.fallbackPreviousSeason(r.Context(), year, season, 3); len(candidate) > 0 {
			schedule = candidate
			fallbackYear, fallbackSeason = fy, fs
			log.Printf("ℹ️  [时间表] %d 年 %d 季度数据源暂无内容，已自动回退至 %d 年 %d 季度", year, season, fy, fs)
		}
	}

	// 同时获取订阅列表，标注已订阅的番剧（支持 ID 匹配和标准化标题模糊匹配）
	var subs []database.Subscription
	database.DB.Find(&subs)

	schedule, subscribed := scheduleSubscriptions(schedule, subs)
	subStats := make(map[uint]map[string]int) // subID → {downloaded, total}

	// 批量统计已下载集数（避免 N+1 查询）
	subIDs := make([]uint, len(subs))
	for i, sub := range subs {
		subIDs[i] = sub.ID
	}
	downloadedMap, err := database.FinishedEpisodeCounts(database.DB.WithContext(r.Context()), subIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "读取剧集进度失败"})
		return
	}

	for _, sub := range subs {
		subStats[sub.ID] = map[string]int{
			"downloaded": downloadedMap[sub.ID],
			"total":      sub.TotalEpisodes,
		}
	}

	resp := map[string]interface{}{
		"days":              schedule,
		"subscribed":        subscribed,
		"subscriptionCount": len(subs),
		"stats":             subStats,
	}
	// 若发生过季度回退，附带标记，前端据此提示用户实际展示的季度
	if fallbackYear > 0 {
		resp["fallback_year"] = fallbackYear
		resp["fallback_season"] = fallbackSeason
		resp["requested_year"] = year
		resp["requested_season"] = season
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleTestMirrors 测试所有 Mikan 镜像延迟
// POST /api/mikan/test-mirrors
// handleBGMTestMirrors 测试所有 BGM 镜像延迟
// POST /api/bgm/test-mirrors
func (s *Server) handleBGMTestMirrors(w http.ResponseWriter, r *http.Request) {
	p := s.getBGMProvider()
	if p == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "Bangumi 服务未就绪"})
		return
	}
	results := p.TestLatency(r.Context())
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleTestMirrors(w http.ResponseWriter, r *http.Request) {
	if s.mikanSrc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "Mikan 服务未初始化"})
		return
	}
	results := s.mikanSrc.TestLatency(r.Context())
	writeJSON(w, http.StatusOK, results)
}

// handleSelectMirror 选择镜像域名（保存到数据库）
// POST /api/mikan/select-mirror  body: {"domain": "mikanime.tv"}
func (s *Server) handleSelectMirror(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Domain == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "domain 不能为空"})
		return
	}
	database.DB.Save(&database.Setting{Key: "MIKAN_DOMAIN", Value: req.Domain})
	writeJSON(w, http.StatusOK, map[string]string{"domain": req.Domain})
}

func (s *Server) handleBGMSelectMirror(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Domain == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "domain 不能为空"})
		return
	}
	database.DB.Save(&database.Setting{Key: "BGMTV_DOMAIN", Value: req.Domain})
	writeJSON(w, http.StatusOK, map[string]string{"domain": req.Domain})
}

// handleProxyImage 代理图片请求（绕过 Bilibili CDN 热链保护，加白名单限制）
// GET /api/proxy/image?url=https://i0.hdslb.com/...
func (s *Server) handleProxyImage(w http.ResponseWriter, r *http.Request) {
	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.Parse(imageURL)
	if err != nil || !allowedImageURL(parsedURL) {
		http.Error(w, "图片地址不在允许范围内", http.StatusForbidden)
		return
	}

	client := httpx.New(15 * time.Second)
	client.CheckRedirect = checkImageRedirect
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, imageURL, nil)
	if err != nil {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	if strings.Contains(imageURL, "hdslb.com") || strings.Contains(imageURL, "bilibili.com") {
		req.Header.Set("Referer", "https://www.bilibili.com")
	} else if strings.Contains(imageURL, "lain.bgm.tv") || strings.Contains(imageURL, "bgm.tv") {
		req.Header.Set("Referer", "https://bgm.tv/")
	} else if strings.Contains(imageURL, "mikan") {
		req.Header.Set("Referer", "https://mikanime.tv/")
	} else if strings.Contains(imageURL, "yuc.wiki") || strings.Contains(imageURL, "yucc.wiki") {
		req.Header.Set("Referer", "https://yuc.wiki/")
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	serveRasterImage(w, resp)
}

// getSettingValue 从数据库获取设置值
func getSettingValue(key string) string {
	var s database.Setting
	if err := database.DB.Where("key = ?", key).Limit(1).Find(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

// ============================================================
// 任务解析
// ============================================================

type parseRequest struct {
	Input string `json:"input"`
}

// handleParseTask 自然语言解析订阅任务
// POST /api/parse
func (s *Server) handleParseTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}

	if s.taskParser == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "任务解析器未初始化"})
		return
	}

	var req parseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if req.Input == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请输入指令，如：追番 某科学的超电磁炮 第一季"})
		return
	}

	result, err := s.taskParser.Parse(r.Context(), req.Input)
	if err != nil {
		log.Printf("⚠️  任务解析失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "解析失败: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ============================================================
// 数据迁移
// ============================================================

type migrateRequest struct {
	SourcePath string `json:"source_path"`
}

// handleMigrateData 导入 AutoBangumi SQLite 或 ani-rss 的订阅清单与备份
// 支持 JSON ({"source_path": "data/..."}) 与 multipart/form-data (上传 .db 文件)
// POST /api/migrate
func (s *Server) handleMigrateData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}

	if s.pluginManager != nil && !s.pluginManager.IsPluginActive("data-migrate") {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "数据导入与迁移插件 (data-migrate) 当前未运行，请在插件管理中检查状态"})
		return
	}

	var sourcePath string
	var needCleanup bool

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// 处理文件上传
		r.Body = http.MaxBytesReader(w, r.Body, (50<<20)+(1<<20))
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "上传文件解析失败: " + err.Error()})
			return
		}
		file, handler, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请选择要上传的数据库文件 (file)"})
			return
		}
		defer file.Close()
		if handler.Size > 50<<20 {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "导入文件不能超过 50 MiB"})
			return
		}

		ext := strings.ToLower(filepath.Ext(handler.Filename))
		if ext != ".db" && ext != ".sqlite" && ext != ".sqlite3" && ext != ".json" && ext != ".zip" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请上传 AutoBangumi SQLite、ani.v2.json 或 ani-rss ZIP 备份"})
			return
		}

		if err := os.MkdirAll("data", 0755); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建 data 目录失败"})
			return
		}

		dest, err := os.CreateTemp("data", "upload_migrate_*"+ext)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "保存上传文件失败: " + err.Error()})
			return
		}
		tempPath := dest.Name()
		if _, err := io.Copy(dest, file); err != nil {
			dest.Close()
			os.Remove(tempPath)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "写入文件失败: " + err.Error()})
			return
		}
		if err := dest.Close(); err != nil {
			os.Remove(tempPath)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "导入文件写入失败"})
			return
		}

		sourcePath = tempPath
		needCleanup = true
	} else {
		// JSON 路径参数
		var req migrateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
			return
		}
		if req.SourcePath == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请提供 source_path（源数据库文件路径）或上传文件"})
			return
		}

		// 安全校验：限制迁移文件只能在 data/ 目录下，防止读取任意 SQLite 文件
		cleanPath := filepath.ToSlash(filepath.Clean(req.SourcePath))
		dataDir := "data"
		if !filepath.IsAbs(req.SourcePath) {
			if !strings.HasPrefix(cleanPath, dataDir+"/") && cleanPath != dataDir {
				writeJSON(w, http.StatusForbidden, errorResponse{Error: "迁移文件路径必须在 data/ 目录下"})
				return
			}
		} else {
			absDataDir, _ := filepath.Abs(dataDir)
			slashAbsDataDir := filepath.ToSlash(absDataDir)
			if !strings.HasPrefix(cleanPath, slashAbsDataDir+"/") && cleanPath != slashAbsDataDir {
				writeJSON(w, http.StatusForbidden, errorResponse{Error: "迁移文件路径必须在 data/ 目录下"})
				return
			}
		}
		sourcePath = req.SourcePath
	}

	if needCleanup {
		defer os.Remove(sourcePath)
	}

	stats, err := migrate.MigrateFromPath(sourcePath)
	if err != nil {
		log.Printf("❌ 数据迁移失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "迁移失败: " + err.Error()})
		return
	}

	log.Printf("✅ 数据迁移成功: 迁移了 %d 条订阅, %d 集, %d 下载记录",
		stats.Subscriptions, stats.Episodes, stats.Downloads)

	if s.eventBus != nil {
		s.eventBus.Publish(core.Event{
			Type: "data.migrate",
			Time: time.Now(),
			Payload: map[string]interface{}{
				"subscriptions": stats.Subscriptions,
				"episodes":      stats.Episodes,
				"downloads":     stats.Downloads,
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "迁移成功",
		"stats":   stats,
	})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "密码不能为空"})
		return
	}
	if len(req.NewPassword) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "新密码不能少于6位"})
		return
	}
	if req.OldPassword == req.NewPassword {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "新密码不能与旧密码相同"})
		return
	}
	claims, ok := r.Context().Value("claims").(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
		return
	}
	var user database.User
	if err := database.DB.WithContext(r.Context()).Where("username = ? AND token_version = ?", claims.Username, claims.TokenVersion).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "登录已过期，请重新登录"})
		} else {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "账户数据暂时不可用"})
		}
		return
	}
	if !auth.CheckPassword(req.OldPassword, user.PasswordHash) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "旧密码错误"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密码加密失败"})
		return
	}
	result := database.DB.WithContext(r.Context()).Model(&database.User{}).
		Where("id = ? AND token_version = ?", user.ID, user.TokenVersion).
		Updates(map[string]any{"password_hash": hash, "token_version": gorm.Expr("token_version + 1")})
	if result.Error != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "无法保存密码，请稍后重试"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "账户已经更新，请重新登录"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "密码修改成功"})
}

func (s *Server) handleBatchDeleteSubscriptions(w http.ResponseWriter, r *http.Request) {
	var req batchDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的请求体"})
		return
	}
	if len(req.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "ID 列表不能为空"})
		return
	}
	if len(req.IDs) > 100 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "单次最多删除 100 个订阅"})
		return
	}

	deleted, failure := s.deleteSubscriptions(r.Context(), req.IDs, req.DeleteFiles)
	if failure != nil {
		failure.write(w)
		return
	}
	writeJSON(w, http.StatusOK, batchDeleteResponse{Deleted: deleted})
}

func (s *Server) handleBatchRestoreSubscriptions(w http.ResponseWriter, r *http.Request) {
	var req batchRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的请求体"})
		return
	}
	if len(req.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "ID 列表不能为空"})
		return
	}
	if len(req.IDs) > 100 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "单次最多恢复 100 个订阅"})
		return
	}

	// 事务：恢复订阅 + 关联剧集（都用 Unscoped 绕过软删除过滤）
	tx := database.DB.Begin()
	if err := tx.Unscoped().Model(&database.Subscription{}).Where("id IN ?", req.IDs).Update("deleted_at", nil).Error; err != nil {
		tx.Rollback()
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "恢复失败"})
		return
	}
	if err := tx.Unscoped().Model(&database.Episode{}).Where("subscription_id IN ?", req.IDs).Update("deleted_at", nil).Error; err != nil {
		tx.Rollback()
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "恢复关联剧集失败"})
		return
	}
	tx.Commit()

	log.Printf("↩️  批量恢复订阅: %v", req.IDs)
	writeJSON(w, http.StatusOK, batchRestoreResponse{Restored: len(req.IDs)})
}

// ============================================================
// AI 智能搜索
// ============================================================

type smartSearchRequest struct {
	Query   string   `json:"query"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	Sources []string `json:"sources"`
}

type smartSearchResponse struct {
	Items           []core.TorrentItem `json:"items"`
	Total           int                `json:"total"`
	HasMore         bool               `json:"has_more"`
	ExpandedQueries []string           `json:"expanded_queries"`
	UsedAI          bool               `json:"used_ai"`
	AIError         string             `json:"ai_error,omitempty"`
}

// handleSmartSearch expands the query with optional AI suggestions, but every
// returned item is fetched by MultiSource from real configured sources.
// POST /api/search/smart
func (s *Server) handleSmartSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}
	if s.multiSrc == nil || s.smartAggregator == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "搜索服务未配置"})
		return
	}

	var req smartSearchRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "query 不能为空"})
		return
	}
	if utf8.RuneCountInString(req.Query) > 200 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "query 长度不能超过 200 字符"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > search.MaxResults {
		req.Limit = search.MaxResults
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	var usedAI bool
	aiError := ""
	queries, err := s.smartExpander.Expand(r.Context(), req.Query)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, search.ErrAITimeout):
			aiError = "timeout"
		case errors.Is(err, ai.ErrQuotaExceeded):
			aiError = "quota_exceeded"
		case errors.Is(err, search.ErrAIParse):
			aiError = "parse_failed"
		default:
			aiError = "model_unavailable"
		}
	} else if len(queries) > 1 && s.smartExpander != nil {
		usedAI = !queriesCircuitOpen(s.smartExpander)
	}

	items, err := s.smartAggregator.AggregateSearch(r.Context(), queries, req.Sources)
	if err != nil && len(items) == 0 {
		log.Printf("⚠️ 智能搜索失败 [%s]: %v", req.Query, err)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "资源站搜索失败"})
		return
	}
	if items == nil {
		items = make([]core.TorrentItem, 0)
	}
	total := len(items)
	end := min(total, req.Offset+req.Limit)
	page := items[min(req.Offset, total):end]

	writeJSON(w, http.StatusOK, smartSearchResponse{
		Items: page, Total: total, HasMore: end < total,
		ExpandedQueries: queries, UsedAI: usedAI, AIError: aiError,
	})
}

// queriesCircuitOpen 检查熔断器状态
func queriesCircuitOpen(expander *search.Expander) bool {
	return expander == nil || expander.CircuitOpen()
}

// ============================================================
// 通知测试
// ============================================================

type testNotifyRequest struct {
	Channel string `json:"channel"` // 空=所有已配置渠道
	Title   string `json:"title"`
	Message string `json:"message"`
}

// handleTestNotify 发送测试通知
// POST /api/notify/test
func (s *Server) handleTestNotify(w http.ResponseWriter, r *http.Request) {
	var req testNotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if req.Title == "" {
		req.Title = "Ani-Go 测试通知"
	}
	if req.Message == "" {
		req.Message = "这是一条来自 Ani-Go 的测试消息，如果您收到说明通知配置正常。"
	}

	ctx := r.Context()
	// 校验消息通知推送插件状态
	if s.pluginManager != nil && !s.pluginManager.IsPluginActive("extended-notifiers") {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "消息通知推送插件当前未运行，请在插件管理中检查状态",
		})
		return
	}

	var err error
	testExtended := func(ctx context.Context) error {
		return s.testExtendedNotifier(ctx, req.Channel, req.Title, req.Message)
	}
	switch {
	case s.notifyMgr != nil:
		err = s.notifyMgr.SendTestWithFallback(ctx, req.Channel, req.Title, req.Message, testExtended)
	case req.Channel != "":
		err = testExtended(ctx)
	default:
		err = errors.New("通知中心未初始化，无法测试已配置渠道")
	}

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "测试通知已发送",
	})
}

// testExtendedNotifier 测试扩展通知插件通道 (Bark, ServerChan, Discord, Slack, Gotify, Ntfy, Pushover, Email)
func (s *Server) testExtendedNotifier(ctx context.Context, channel, title, message string) error {
	getSetting := func(key string) string {
		var st database.Setting
		if err := database.DB.Where("key = ?", key).First(&st).Error; err == nil {
			return strings.TrimSpace(st.Value)
		}
		return ""
	}

	var target core.Notifier
	switch strings.ToLower(channel) {
	case "bark":
		key := getSetting("BARK_DEVICE_KEY")
		if key == "" {
			return fmt.Errorf("Bark 设备密钥 (BARK_DEVICE_KEY) 未配置")
		}
		target = notifier.NewPushNotifier(notifier.PushBark, getSetting("BARK_SERVER_URL"), key, "")
	case "serverchan":
		key := getSetting("SERVERCHAN_KEY")
		if key == "" {
			return fmt.Errorf("Server酱 SCKEY (SERVERCHAN_KEY) 未配置")
		}
		target = notifier.NewPushNotifier(notifier.PushServerChan, "", key, "")
	case "discord":
		webhook := getSetting("DISCORD_WEBHOOK")
		if webhook == "" {
			return fmt.Errorf("Discord Webhook 未配置")
		}
		target = notifier.NewDiscordNotifier(webhook)
	case "slack":
		webhook := getSetting("SLACK_WEBHOOK")
		if webhook == "" {
			return fmt.Errorf("Slack Webhook 未配置")
		}
		target = notifier.NewSlackNotifier(webhook)
	case "gotify":
		url := getSetting("GOTIFY_URL")
		token := getSetting("GOTIFY_TOKEN")
		if url == "" || token == "" {
			return fmt.Errorf("Gotify URL 或 Token 未配置")
		}
		target = notifier.NewPushNotifier(notifier.PushGotify, url, token, "")
	case "ntfy":
		topic := getSetting("NTFY_TOPIC")
		if topic == "" {
			return fmt.Errorf("Ntfy Topic 未配置")
		}
		target = notifier.NewPushNotifier(notifier.PushNtfy, getSetting("NTFY_URL"), topic, "")
	case "pushover":
		token := getSetting("PUSHOVER_TOKEN")
		user := getSetting("PUSHOVER_USER")
		if token == "" || user == "" {
			return fmt.Errorf("Pushover Token 或 User 未配置")
		}
		target = notifier.NewPushNotifier(notifier.PushPushover, "", token, user)
	case "email", "smtp":
		host := getSetting("EMAIL_SMTP_HOST")
		port := getSetting("EMAIL_SMTP_PORT")
		user := getSetting("EMAIL_SMTP_USER")
		pass := getSetting("EMAIL_SMTP_PASS")
		from := getSetting("EMAIL_SMTP_FROM")
		if from == "" {
			from = user
		}
		toStr := getSetting("EMAIL_SMTP_TO")
		if host == "" || user == "" || toStr == "" {
			return fmt.Errorf("SMTP 服务器、用户名或收件人邮箱未配置")
		}
		var toList []string
		for _, item := range strings.Split(toStr, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				toList = append(toList, trimmed)
			}
		}
		target = notifier.NewEmailNotifier(host, port, user, pass, from, toList)
	case "matrix":
		hs := getSetting("MATRIX_HOMESERVER")
		token := getSetting("MATRIX_TOKEN")
		room := getSetting("MATRIX_ROOM_ID")
		if hs == "" || token == "" || room == "" {
			return fmt.Errorf("Matrix 服务器、Token 或 Room ID 未配置")
		}
		target = notifier.NewMatrixNotifier(hs, token, room)
	case "line":
		token := getSetting("LINE_CHANNEL_TOKEN")
		user := getSetting("LINE_USER_ID")
		if token == "" || user == "" {
			return fmt.Errorf("LINE Channel Token 或 User ID 未配置")
		}
		target = notifier.NewLINENotifier(token, user)
	case "whatsapp":
		phone := getSetting("WHATSAPP_PHONE_ID")
		token := getSetting("WHATSAPP_TOKEN")
		to := getSetting("WHATSAPP_TO")
		if phone == "" || token == "" || to == "" {
			return fmt.Errorf("WhatsApp Phone ID, Token 或 接收者号码未配置")
		}
		target = notifier.NewWhatsAppNotifier(phone, token, to)
	case "signal":
		url := getSetting("SIGNAL_API_URL")
		sender := getSetting("SIGNAL_SENDER")
		recipients := getSetting("SIGNAL_RECIPIENTS")
		if url == "" || sender == "" || recipients == "" {
			return fmt.Errorf("Signal API URL, Sender 或 接收者未配置")
		}
		var rList []string
		for _, r := range strings.Split(recipients, ",") {
			if trimmed := strings.TrimSpace(r); trimmed != "" {
				rList = append(rList, trimmed)
			}
		}
		target = notifier.NewSignalNotifierWithParams(url, sender, rList)
	case "wechat", "weixin", "mp_wechat":
		appID := getSetting("WECHAT_APP_ID")
		appSecret := getSetting("WECHAT_APP_SECRET")
		userID := getSetting("WECHAT_USER_ID")
		templateID := getSetting("WECHAT_TEMPLATE_ID")
		if appID == "" || appSecret == "" || userID == "" {
			return fmt.Errorf("微信公众号 AppID、AppSecret 或 OpenID/UserID 未配置")
		}
		target = notifier.NewWeChatNotifierWithParams(appID, appSecret, userID, templateID)
	default:
		return fmt.Errorf("%w: %s", v2.ErrNotifierNotFound, channel)
	}

	return target.Send(ctx, title, message)
}

// ============================================================
// AI 模型列表获取
// ============================================================

type aiModelsRequest struct {
	Protocol string `json:"protocol"`
	Endpoint string `json:"endpoint"`
	ApiKey   string `json:"apiKey"`
}

// handleGetAIModels 获取 AI 模型列表
// POST /api/ai/models
func (s *Server) handleGetAIModels(w http.ResponseWriter, r *http.Request) {
	var req aiModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if req.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请先填写端点地址"})
		return
	}

	models, err := s.fetchModelsFromProvider(req.Protocol, req.Endpoint, req.ApiKey)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"models":  models,
	})
}

// fetchModelsFromProvider 根据协议从提供商获取模型列表
func (s *Server) fetchModelsFromProvider(protocol, endpoint, apiKey string) ([]string, error) {
	ctx := context.Background()

	switch protocol {
	case "openai", "":
		return s.fetchOpenAIModels(ctx, endpoint, apiKey)
	case "google":
		return s.fetchGoogleModels(ctx, endpoint, apiKey)
	case "anthropic":
		return s.fetchAnthropicModels(ctx, endpoint, apiKey)
	case "ollama":
		return s.fetchOllamaModels(ctx, endpoint)
	default:
		// 默认尝试 OpenAI 兼容格式
		return s.fetchOpenAIModels(ctx, endpoint, apiKey)
	}
}

// fetchOpenAIModels 获取 OpenAI 兼容格式的模型列表
func (s *Server) fetchOpenAIModels(ctx context.Context, endpoint, apiKey string) ([]string, error) {
	// 规范化端点：确保以 /v1/models 结尾
	modelsEndpoint := strings.TrimSuffix(endpoint, "/")
	if !strings.Contains(modelsEndpoint, "/v1") {
		modelsEndpoint = modelsEndpoint + "/v1"
	}
	modelsEndpoint = modelsEndpoint + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := httpx.New(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

// fetchGoogleModels 获取 Google Gemini 模型列表
func (s *Server) fetchGoogleModels(ctx context.Context, endpoint, apiKey string) ([]string, error) {
	// Google 使用不同的端点格式
	modelsEndpoint := strings.TrimSuffix(endpoint, "/")
	if !strings.Contains(modelsEndpoint, "generativelanguage.googleapis.com") {
		modelsEndpoint = "https://generativelanguage.googleapis.com"
	}
	modelsEndpoint = modelsEndpoint + "/v1beta/models?key=" + url.QueryEscape(apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := httpx.New(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []struct {
			Name string `json:"name"` // 格式: models/gemini-1.5-pro
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	models := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		// 提取模型名: models/gemini-1.5-pro -> gemini-1.5-pro
		if strings.HasPrefix(m.Name, "models/") {
			models = append(models, m.Name[7:])
		} else {
			models = append(models, m.Name)
		}
	}
	return models, nil
}

// fetchAnthropicModels 获取 Anthropic 模型列表
// Anthropic 没有公开的模型列表 API，返回常用模型
func (s *Server) fetchAnthropicModels(ctx context.Context, endpoint, apiKey string) ([]string, error) {
	// Anthropic 官方模型列表（硬编码备选）
	return []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}, nil
}

// fetchOllamaModels 获取 Ollama 本地模型列表
func (s *Server) fetchOllamaModels(ctx context.Context, endpoint string) ([]string, error) {
	modelsEndpoint := strings.TrimSuffix(endpoint, "/") + "/api/tags"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	client := httpx.New(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Models []struct {
			Name string `json:"name"` // 格式: llama3.1:8b
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	models := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		if m.Name != "" {
			models = append(models, m.Name)
		}
	}
	return models, nil
}

// handleEventStream 实时事件流 (SSE)
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	// 从 Query 参数提取 Token (针对 EventSource 无法设置 Header 的限制)
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		// 如果 Query 没带，尝试从 Header 拿
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = authHeader[7:]
		}
	}

	if tokenString != "" {
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// 简单的 Token 版本校验
		var user database.User
		if err := database.DB.WithContext(r.Context()).Where("username = ? AND token_version = ?", claims.Username, claims.TokenVersion).First(&user).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "账户数据暂时不可用", http.StatusServiceUnavailable)
				return
			}

			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else {
		// 强制要求鉴权
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 订阅所有事件
	eventChan := make(chan core.Event, 32)
	handler := func(ev core.Event) {
		select {
		case eventChan <- ev:
		default:
			// 缓冲区满，丢弃旧事件
		}
	}

	// 订阅所有类型的事件
	// 这里我们需要 EventBus 支持订阅所有，或者手动枚举
	// 暂且订阅几个关键事件
	types := []string{
		core.EventSubscriptionAdded,
		core.EventDownloadStarted,
		core.EventDownloadCompleted,
		core.EventFileOrganized,
		core.EventSupplementTriggered,
		core.EventSupplementCompleted,
		"supplement.progress", // 新增进度事件
	}

	ids := make([]core.SubscriptionID, 0, len(types))
	for _, t := range types {
		id := s.eventBus.Subscribe(t, handler)
		ids = append(ids, id)
	}
	defer func() {
		for i, t := range types {
			s.eventBus.Unsubscribe(t, ids[i])
		}
	}()

	if err := httpx.WriteSSEFrame(w, "data: {\"type\": \"connected\"}\n\n"); err != nil {
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := httpx.WriteSSEFrame(w, ": heartbeat\n\n"); err != nil {
				return
			}
		case ev := <-eventChan:
			data, _ := json.Marshal(ev)
			if err := httpx.WriteSSEFrame(w, fmt.Sprintf("data: %s\n\n", data)); err != nil {
				return
			}
		}
	}
}

// handleListPlugins 获取标准化插件列表（含健康状态、错误摘要与订阅事件）
func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if s.pluginManager == nil {
		writeJSON(w, http.StatusOK, []plugin.PluginInfo{})
		return
	}
	writeJSON(w, http.StatusOK, s.pluginManager.GetPluginList())
}

// handleGetCustomPlugins 获取所有自定义 Webhook 插件列表
func (s *Server) handleGetCustomPlugins(w http.ResponseWriter, r *http.Request) {
	if s.pluginManager == nil {
		writeJSON(w, http.StatusOK, []plugin.PluginInfo{})
		return
	}
	all := s.pluginManager.GetPluginList()
	custom := make([]plugin.PluginInfo, 0, len(all))
	for _, p := range all {
		if !p.IsBuiltIn {
			custom = append(custom, p)
		}
	}
	writeJSON(w, http.StatusOK, custom)
}

// handleTogglePlugin 界面化切换插件状态（支持 POST /api/plugins/toggle 与 POST /api/plugins/{id}/toggle）
func (s *Server) handleTogglePlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "插件管理器未初始化"})
		return
	}

	var req struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
	}

	if !decodePluginRequest(w, r, &req, false) {
		return
	}

	// 兼容支持路径变量: POST /api/plugins/{id}/toggle
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID != "" {
		req.ID = pathID
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "插件 ID 不能为空"})
		return
	}
	if req.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "缺少 enabled 参数"})
		return
	}

	s.pluginReloadMu.Lock()
	defer s.pluginReloadMu.Unlock()

	if err := s.pluginManager.TogglePlugin(req.ID, *req.Enabled); err != nil {
		if errors.Is(err, plugin.ErrPluginNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "插件不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "更新插件状态失败"})
		return
	}

	st, lastErr := s.pluginManager.GetPluginStatus(req.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "插件状态已更新",
		"id":      req.ID,
		"enabled": *req.Enabled,
		"status":  st,
		"error":   lastErr,
	})
}

// handleSavePlugin 添加或编辑自定义插件
func (s *Server) handleSavePlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "插件管理器未初始化"})
		return
	}
	var req struct {
		plugin.PluginInfo
		Secret *string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}
	p := req.PluginInfo
	s.pluginReloadMu.Lock()
	defer s.pluginReloadMu.Unlock()

	info, err := s.pluginManager.SaveCustomPluginInfo(p, req.Secret)
	if err != nil {
		switch {
		case errors.Is(err, plugin.ErrReservedPluginID):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case errors.Is(err, plugin.ErrInvalidPluginManifest):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "保存插件配置失败"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "插件配置已保存",
		"id":      info.ID,
		"enabled": info.Enabled,
		"status":  info.Status,
	})
}

// handleDeletePlugin 删除自定义插件
func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	if s.pluginManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "插件管理器未初始化"})
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		id = strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/plugins/"))
	}
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "缺少插件 ID"})
		return
	}

	s.pluginReloadMu.Lock()
	defer s.pluginReloadMu.Unlock()

	if err := s.pluginManager.DeletePlugin(id); err != nil {
		switch {
		case errors.Is(err, plugin.ErrReservedPluginID):
			writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		case errors.Is(err, plugin.ErrPluginNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "插件不存在"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "删除插件配置失败"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "插件已删除",
		"id":      id,
	})
}

// handleReloadPlugins 重新加载插件配置（并发防抖与恐慌隔离）
// POST /api/plugins/reload
func (s *Server) handleReloadPlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}
	if s.pluginManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "插件管理器未初始化"})
		return
	}

	s.pluginReloadMu.Lock()
	defer s.pluginReloadMu.Unlock()

	defer func() {
		if recover() != nil {
			log.Print("❌ [API] 重载插件时发生异常")
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "重载插件异常"})
		}
	}()

	if err := s.pluginManager.Load(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "重载插件配置失败，请检查数据库和插件配置后重试"})
		return
	}
	if s.notifyMgr != nil && s.notifyMgr.IsEnabled() {
		s.notifyMgr.Reload()
	}
	plugins := s.pluginManager.GetPluginList()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "插件已重新加载",
		"count":   len(plugins),
		"plugins": plugins,
	})
}

func (s *Server) handleBangumiAuthLink(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("BANGUMI_CLIENT_ID")
	if clientID == "" {
		var setting database.Setting
		if err := database.DB.Where("key = ?", "BANGUMI_CLIENT_ID").First(&setting).Error; err == nil && setting.Value != "" {
			clientID = setting.Value
		}
	}
	if clientID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "请先在下方配置 Client ID（需在 bgm.tv/dev/app 创建应用），或直接在下方填入 Bangumi 访问令牌 (Token)",
		})
		return
	}

	claims, ok := r.Context().Value("claims").(*auth.Claims)
	if !ok || claims == nil {
		http.Error(w, "管理员会话无效", http.StatusUnauthorized)
		return
	}
	redirectURI := s.getPublicBaseURL(r) + bangumiCallbackPath
	state, err := s.bgmFlows.start(w, claims, redirectURI)
	if err != nil {
		http.Error(w, "无法创建授权会话", http.StatusInternalServerError)
		return
	}
	params := url.Values{"client_id": {clientID}, "response_type": {"code"}, "redirect_uri": {redirectURI}, "state": {state}}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"url": "https://bgm.tv/oauth/authorize?" + params.Encode()})
}

func (s *Server) handleBangumiAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	redirectURI := s.getPublicBaseURL(r) + bangumiCallbackPath
	if !s.bgmFlows.consume(r, redirectURI) {
		http.Error(w, "授权会话无效或已过期，请从设置页重新授权", http.StatusBadRequest)
		return
	}
	clientID := os.Getenv("BANGUMI_CLIENT_ID")
	if clientID == "" {
		var setting database.Setting
		if err := database.DB.Where("key = ?", "BANGUMI_CLIENT_ID").First(&setting).Error; err == nil && setting.Value != "" {
			clientID = setting.Value
		}
	}

	clientSecret := os.Getenv("BANGUMI_CLIENT_SECRET")
	if clientSecret == "" {
		var setting database.Setting
		if err := database.DB.Where("key = ?", "BANGUMI_CLIENT_SECRET").First(&setting).Error; err == nil {
			clientSecret = setting.Value
		}
	}

	if clientID == "" || clientSecret == "" {
		http.Error(w, "BANGUMI_CLIENT_ID 或 BANGUMI_CLIENT_SECRET 未配置", http.StatusInternalServerError)
		return
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	exchange, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://bgm.tv/oauth/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		http.Error(w, "创建授权请求失败", http.StatusInternalServerError)
		return
	}
	exchange.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpx.Default.Do(exchange)
	if err != nil {
		http.Error(w, "连接 Bangumi 授权服务失败", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Bangumi 拒绝了授权请求", http.StatusBadRequest)
		return
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
		http.Error(w, "Bangumi 返回的授权令牌无效", http.StatusBadGateway)
		return
	}
	meCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	meReq, err := http.NewRequestWithContext(meCtx, http.MethodGet, "https://api.bgm.tv/v0/me", nil)
	if err != nil {
		http.Error(w, "创建账户查询失败", http.StatusInternalServerError)
		return
	}
	meReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	meReq.Header.Set("User-Agent", "Ani-Go/1.0")
	meResp, err := httpx.Default.Do(meReq)
	if err != nil {
		http.Error(w, "无法确认 Bangumi 账户", http.StatusBadGateway)
		return
	}
	defer meResp.Body.Close()
	var me struct {
		Username string `json:"username"`
		ID       int    `json:"id"`
	}
	if meResp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(meResp.Body, 1<<20)).Decode(&me) != nil || (me.Username == "" && me.ID <= 0) {
		http.Error(w, "无法确认 Bangumi 账户", http.StatusBadGateway)
		return
	}
	if me.Username == "" {
		me.Username = strconv.Itoa(me.ID)
	}
	err = database.DB.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		for _, setting := range []database.Setting{
			{Key: "BGMTV_USER_TOKEN", Value: tokenResp.AccessToken},
			{Key: "BANGUMI_USER_TOKEN", Value: tokenResp.AccessToken},
			{Key: "BGMTV_USERNAME", Value: me.Username},
		} {
			if err := tx.Save(&setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, "无法保存 Bangumi 账户绑定", http.StatusInternalServerError)
		return
	}
	if p, ok := s.md.(*metadata.BGMTVProvider); ok {
		p.SetUserToken(tokenResp.AccessToken)
	}
	http.Redirect(w, r, "/settings?tab=bangumi&status=success", http.StatusFound)
}
func (s *Server) getPublicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// ============================================================
// 批量下载控制
// ============================================================

// handlePauseAllDownloads 批量暂停所有活跃任务
// POST /api/downloads/pause-all
func (s *Server) handlePauseAllDownloads(w http.ResponseWriter, r *http.Request) {
	s.changeDownloadStates(w, r, true)
}

// handleResumeAllDownloads 恢复所有已暂停的任务。
func (s *Server) handleResumeAllDownloads(w http.ResponseWriter, r *http.Request) {
	s.changeDownloadStates(w, r, false)
}

func (s *Server) changeDownloadStates(w http.ResponseWriter, r *http.Request, pause bool) {
	dl := s.downloader
	if dynamic, ok := dl.(*downloader.DynamicDownloader); ok {
		// 同一批操作与取列表时使用同一个下载器实例。
		dl = dynamic.GetCurrent()
	}
	if dl == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "下载器未配置"})
		return
	}
	tasks, err := dl.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: fmt.Sprintf("获取任务列表失败: %v", err)})
		return
	}

	action, eligible, label := dl.Resume, downloader.CanResume, "恢复"
	if pause {
		action, eligible, label = dl.Pause, downloader.CanPause, "暂停"
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var succeeded, failed int
	slots := make(chan struct{}, 8)
	for _, task := range tasks {
		if !eligible(task.Status) {
			continue
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(hash string) {
			defer wg.Done()
			defer func() { <-slots }()
			err := action(r.Context(), hash)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
			} else {
				succeeded++
			}
		}(task.Hash)
	}
	wg.Wait()
	if failed > 0 {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false, "count": succeeded, "failed": failed,
			"error": fmt.Sprintf("已%s %d 个任务，%d 个失败", label, succeeded, failed),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "count": succeeded, "message": fmt.Sprintf("已成功%s %d 个下载任务", label, succeeded),
	})
}

// ============================================================
// 通知履历与投递监控 API
// ============================================================

// handleListNotificationLogs 查询通知投递流水
// GET /api/notifications/logs
func (s *Server) handleListNotificationLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := v2.NotificationLogFilter{
		Channel:   q.Get("channel"),
		Status:    q.Get("status"),
		EventType: q.Get("eventType"),
		Page:      page,
		PageSize:  pageSize,
	}

	total, logs, err := v2.GetNotificationLogs(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("查询通知日志失败: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"logs":     logs,
	})
}

// handleClearNotificationLogs 清空通知日志
// DELETE /api/notifications/logs
func (s *Server) handleClearNotificationLogs(w http.ResponseWriter, r *http.Request) {
	days := 0
	if values, present := r.URL.Query()["days"]; present {
		value, err := strconv.ParseUint(values[0], 10, 31)
		if err != nil || len(values) != 1 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "days 必须是单个非负整数"})
			return
		}
		days = int(value)
	}
	if err := v2.ClearNotificationLogs(days); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("清空通知日志失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "通知记录清理完成",
	})
}

// handleGetNotificationStats 查询通知汇总与渠道分布
// GET /api/notifications/stats
func (s *Server) handleGetNotificationStats(w http.ResponseWriter, r *http.Request) {
	stats, err := v2.GetNotificationStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("获取通知统计失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ============================================================
// RSS 源深度解析探针
// ============================================================

type rssPreviewItem struct {
	Title       string  `json:"title"`
	Link        string  `json:"link"`
	PubDate     string  `json:"pub_date"`
	Magnet      string  `json:"magnet"`
	Size        int64   `json:"size"`
	ParsedTitle string  `json:"parsed_title"`
	Episode     float32 `json:"episode"`
	Season      int     `json:"season"`
	Subgroup    string  `json:"subgroup"`
	Resolution  string  `json:"resolution"`
	IsBatch     bool    `json:"is_batch"`
	IsSpecial   bool    `json:"is_special"`
}

// handlePreviewRSS 实时拉取并解析任意 RSS 订阅源内容与正则特征
// POST /api/rss/preview
func (s *Server) handlePreviewRSS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "必须提供有效的 RSS URL"})
		return
	}

	targetURL := strings.TrimSpace(req.URL)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	urlsToTry := []string{targetURL}
	if u, err := url.Parse(targetURL); err == nil && (strings.Contains(u.Host, "mikanani.me") || strings.Contains(u.Host, "mikanime.tv")) {
		mirrors := []string{"mikanime.tv", "mikanani.me"}
		for _, m := range mirrors {
			altURL := *u
			altURL.Host = m
			s := altURL.String()
			if s != targetURL {
				urlsToTry = append(urlsToTry, s)
			}
		}
	}

	client := httpx.New(8 * time.Second)
	var resp *http.Response
	var lastErr error

	for _, tryURL := range urlsToTry {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tryURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (Ani-Go/0.5.4 RSS-Inspector)")

		r, err := client.Do(httpReq)
		if err == nil && r.StatusCode == http.StatusOK {
			resp = r
			break
		}
		if r != nil {
			r.Body.Close()
			lastErr = fmt.Errorf("RSS 服务器返回状态码: %d", r.StatusCode)
		} else {
			lastErr = err
		}
	}

	if resp == nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: fmt.Sprintf("拉取 RSS 内容失败: %v", lastErr)})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: fmt.Sprintf("读取 RSS 数据失败: %v", err)})
		return
	}

	type genericRSSItem struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		PubDate     string `xml:"pubDate"`
		Description string `xml:"description"`
		Enclosure   struct {
			URL    string `xml:"url,attr"`
			Length int64  `xml:"length,attr"`
		} `xml:"enclosure"`
		Torrent struct {
			MagnetURI string `xml:"magnetURI"`
		} `xml:"torrent"`
	}

	type genericRSS struct {
		Channel struct {
			Title string           `xml:"title"`
			Items []genericRSSItem `xml:"item"`
		} `xml:"channel"`
	}

	var parsed genericRSS
	if err := xml.Unmarshal(body, &parsed); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("解析 RSS XML 失败: %v", err)})
		return
	}

	var results []rssPreviewItem
	for _, it := range parsed.Channel.Items {
		info := source.ParseMikanTitle(it.Title)

		magnet := it.Torrent.MagnetURI
		if magnet == "" && strings.HasPrefix(it.Enclosure.URL, "magnet:") {
			magnet = it.Enclosure.URL
		}
		if magnet == "" && strings.Contains(it.Description, "magnet:?xt=") {
			start := strings.Index(it.Description, "magnet:?xt=")
			end := strings.IndexAny(it.Description[start:], " \"'<>\n\r\t")
			if end > 0 {
				magnet = it.Description[start : start+end]
			} else {
				magnet = it.Description[start:]
			}
		}

		results = append(results, rssPreviewItem{
			Title:       it.Title,
			Link:        it.Link,
			PubDate:     it.PubDate,
			Magnet:      magnet,
			Size:        it.Enclosure.Length,
			ParsedTitle: info.Title,
			Episode:     info.Episode,
			Season:      info.Season,
			Subgroup:    info.Subgroup,
			Resolution:  info.Resolution,
			IsBatch:     info.IsBatch,
			IsSpecial:   info.IsSpecial,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"feed": map[string]interface{}{
			"title": parsed.Channel.Title,
			"url":   targetURL,
		},
		"total": len(results),
		"items": results,
	})
}

// handleGenerateMCPToken 生成并持久化新的 MCP Token
// POST /api/mcp/token/generate
func (s *Server) handleGenerateMCPToken(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "生成 Token 失败"})
		return
	}
	token := hex.EncodeToString(b)

	setting := database.Setting{Key: "MCP_TOKEN", Value: token}
	if err := database.DB.Where("key = ?", "MCP_TOKEN").Assign(setting).FirstOrCreate(&setting).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "持久化 Token 失败"})
		return
	}

	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
	})
}

type organizePreviewRequest struct {
	FilePath string                  `json:"file_path"`
	Anime    core.Anime              `json:"anime"`
	Episode  core.Episode            `json:"episode"`
	Items    []organizer.PreviewItem `json:"items"`
}

// handlePreviewOrganize 文件整理 Dry-Run 路径预览（不产生任何文件操作）
// POST /api/organize/preview
func (s *Server) handlePreviewOrganize(w http.ResponseWriter, r *http.Request) {
	if s.organizer == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "文件整理器未初始化"})
		return
	}

	var req organizePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的请求数据: " + err.Error()})
		return
	}

	ctx := r.Context()

	// 批量预览模式
	if len(req.Items) > 0 {
		results, err := s.organizer.PreviewBatch(ctx, req.Items)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "批量整理预览失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": results,
		})
		return
	}

	// 单项预览模式
	if req.FilePath == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "file_path 或 items 不能为空"})
		return
	}

	result, err := s.organizer.Preview(ctx, req.FilePath, req.Anime, req.Episode)
	if err != nil && result.Action != "cancelled" {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "整理预览失败: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": result,
	})
}

// ============================================================
// TMDB Collection 系列自动归并
// ============================================================

type mergeCollectionsRequest struct {
	SubscriptionIDs []uint `json:"subscription_ids"`
	Apply           bool   `json:"apply"`
	Overwrite       bool   `json:"overwrite"`
}

// handleMergeCollections 批量同系列 TMDB Collection 自动归并（支持 Dry-Run 预测与落地应用）
// POST /api/subscriptions/merge-collections
func (s *Server) handleMergeCollections(w http.ResponseWriter, r *http.Request) {
	if database.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "数据库未就绪"})
		return
	}

	var req mergeCollectionsRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的归并参数"})
			return
		}
	}

	ctx := r.Context()

	var subs []database.Subscription
	query := database.DB.WithContext(ctx)
	if len(req.SubscriptionIDs) > 0 {
		query = query.Where("id IN ?", req.SubscriptionIDs)
	}
	if err := query.Find(&subs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "查询订阅列表失败: " + err.Error()})
		return
	}

	var tmdbProvider *metadata.TMDBProvider
	if p, ok := s.md.(*metadata.TMDBProvider); ok {
		tmdbProvider = p
	} else {
		apiKey := ""
		lang := "zh-CN"
		var mirrorDomains []string
		var apiKeySetting, langSetting, mirrorSetting database.Setting
		if err := database.DB.Where("key = ?", "TMDB_API_KEY").First(&apiKeySetting).Error; err == nil && apiKeySetting.Value != "" {
			apiKey = strings.TrimSpace(apiKeySetting.Value)
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("TMDB_API_KEY"))
		}
		if err := database.DB.Where("key = ?", "TMDB_LANGUAGE").First(&langSetting).Error; err == nil && langSetting.Value != "" {
			lang = strings.TrimSpace(langSetting.Value)
		}
		if err := database.DB.Where("key = ?", "TMDB_MIRROR_DOMAINS").First(&mirrorSetting).Error; err == nil && mirrorSetting.Value != "" {
			for _, d := range strings.Split(mirrorSetting.Value, ",") {
				if trimmed := strings.TrimSpace(d); trimmed != "" {
					mirrorDomains = append(mirrorDomains, trimmed)
				}
			}
		}
		if apiKey != "" {
			tmdbProvider = metadata.NewTMDBProvider(apiKey, lang, mirrorDomains)
		}
	}

	merger := metadata.NewCollectionMerger(database.DB, tmdbProvider)

	summary, err := merger.GenerateMergePlans(ctx, subs, req.Overwrite)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "生成系列归并计划失败: " + err.Error()})
		return
	}

	if req.Apply {
		applied, err := merger.ApplyMergePlans(ctx, summary.Plans)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "应用系列归并失败: " + err.Error()})
			return
		}
		summary.TotalApplied = applied
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": summary,
	})
}

// handleResolveSubscriptionCollection 单部订阅解析系列归并建议
// POST /api/subscriptions/{id}/resolve-collection
func (s *Server) handleResolveSubscriptionCollection(w http.ResponseWriter, r *http.Request) {
	if database.DB == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "数据库未就绪"})
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的订阅 ID"})
		return
	}

	ctx := r.Context()
	var sub database.Subscription
	if err := database.DB.WithContext(ctx).First(&sub, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "查询订阅失败: " + err.Error()})
		return
	}

	var tmdbProvider *metadata.TMDBProvider
	if p, ok := s.md.(*metadata.TMDBProvider); ok {
		tmdbProvider = p
	}

	merger := metadata.NewCollectionMerger(database.DB, tmdbProvider)
	info, err := merger.ResolveSingleSubscription(ctx, &sub)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "解析系列归并失败: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": info,
	})
}

// ============================================================
// 通知模板系统 (Notification Templates)
// ============================================================

func (s *Server) getTemplateManager() *v2.TemplateManager {
	if s.notifyMgr != nil {
		return s.notifyMgr.GetTemplateManager()
	}
	return v2.NewTemplateManagerWithDB(database.DB)
}

// handleListNotificationTemplates 获取全部通知模板配置（内置与自定义合并）
// GET /api/notifications/templates
func (s *Server) handleListNotificationTemplates(w http.ResponseWriter, r *http.Request) {
	tm := s.getTemplateManager()
	catalog := tm.ListTemplates()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": catalog,
	})
}

type updateTemplateRequest struct {
	EventType string `json:"event_type"`
	Channel   string `json:"channel"`
	Template  string `json:"template"`
}

// handleUpdateNotificationTemplate 更新指定事件/渠道的通知模板
// PUT /api/notifications/templates
func (s *Server) handleUpdateNotificationTemplate(w http.ResponseWriter, r *http.Request) {
	var req updateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的请求体: " + err.Error()})
		return
	}

	req.EventType = strings.TrimSpace(req.EventType)
	req.Channel = strings.TrimSpace(req.Channel)
	if req.EventType == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "event_type 不能为空"})
		return
	}

	tm := s.getTemplateManager()
	if err := tm.SetTemplate(req.EventType, req.Channel, req.Template); err != nil {
		if errors.Is(err, v2.ErrInvalidTemplate) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		} else {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "保存通知模板失败"})
		}
		return
	}

	if s.notifyMgr != nil {
		_ = s.notifyMgr.ReloadTemplates()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "模板已成功更新并生效",
	})
}

type previewTemplateRequest struct {
	EventType  string         `json:"event_type"`
	Channel    string         `json:"channel"`
	Template   string         `json:"template"`
	SampleData map[string]any `json:"sample_data"`
}

// handlePreviewNotificationTemplate 预览模板渲染效果
// POST /api/notifications/templates/preview
func (s *Server) handlePreviewNotificationTemplate(w http.ResponseWriter, r *http.Request) {
	var req previewTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的请求体: " + err.Error()})
		return
	}

	req.EventType = strings.TrimSpace(req.EventType)
	if req.EventType == "" {
		req.EventType = core.EventDownloadCompleted
	}

	tm := s.getTemplateManager()
	rendered, err := tm.PreviewTemplate(req.EventType, req.Channel, req.Template, req.SampleData)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "模板渲染失败: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0,
		"data": map[string]string{
			"rendered": rendered,
		},
	})
}

type resetTemplateRequest struct {
	EventType string `json:"event_type"`
	Channel   string `json:"channel"`
}

// handleResetNotificationTemplate 移除指定事件与渠道的覆盖模板。
func (s *Server) handleResetNotificationTemplate(w http.ResponseWriter, r *http.Request) {
	var req resetTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "需要包含 event_type 的通知模板请求"})
		return
	}
	req.EventType = strings.TrimSpace(req.EventType)
	req.Channel = strings.TrimSpace(req.Channel)
	if req.EventType == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "event_type 不能为空"})
		return
	}

	tm := s.getTemplateManager()
	if err := tm.DeleteTemplate(req.EventType, req.Channel); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "重置模板失败: " + err.Error()})
		return
	}

	if s.notifyMgr != nil {
		_ = s.notifyMgr.ReloadTemplates()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "通知模板已成功重置为内置默认值",
	})
}
