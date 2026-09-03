package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/metadata"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/httpx"
	"github.com/xiaoyueRX/Ani-Go/internal/migrate"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
	"github.com/xiaoyueRX/Ani-Go/internal/search"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

// ============================================================
// 订阅 CRUD
// ============================================================

// subscriptionResponse API 返回的订阅数据结构
type subscriptionResponse struct {
	ID               uint   `json:"id"`
	TitleCN          string `json:"title_cn"`
	TitleEN          string `json:"title_en"`
	TitleJP          string `json:"title_jp"`
	Year             int    `json:"year"`
	Season           int    `json:"season"`
	BangumiID        string `json:"bangumi_id"`
	SubgroupName     string `json:"subgroup_name"`
	MetadataID       string `json:"metadata_id"`
	MetadataProvider string `json:"metadata_provider"`
	CoverURL         string `json:"cover_url"`
	AvatarURL        string `json:"avatar_url"` // 兼容前端 Layout.vue
	Description      string `json:"description"`
	AnimeType        string `json:"anime_type"`
	TotalEpisodes    int    `json:"total_episodes"`
	CurrentEpisodes  int    `json:"current_episodes"`
	Enabled          bool   `json:"enabled"`
	Completed        bool   `json:"completed"`
	FilterJSON         string `json:"filter_json"`
	CustomPath         string `json:"custom_path"`
	ExcludedKeywords   string `json:"excluded_keywords"`
	AllowedSubgroups   string `json:"allowed_subgroups"`
	SkipBulkUpdate     bool   `json:"skip_bulk_update"`
	StallTimeoutHours  int    `json:"stall_timeout_hours"`
	StalledEpisodes    int    `json:"stalled_episodes"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type createSubscriptionRequest struct {
	TitleCN      string `json:"title_cn"`
	BangumiID    string `json:"bangumi_id"`
	SubgroupName string `json:"subgroup_name"`
	RSSURL       string `json:"rss_url"`
	FilterJSON   string `json:"filter_json"`
	CustomPath   string `json:"custom_path"`
	CoverURL     string `json:"cover_url"`
}

type batchSubscriptionItem struct {
	TitleCN       string   `json:"title_cn"`
	BangumiID     string   `json:"bangumi_id"`
	CoverURL      string   `json:"cover_url"`
	Subgroups     []string `json:"subgroups"`
	TotalEpisodes int      `json:"total_episodes"`
	IsFinished    bool     `json:"is_finished"`
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
	TitleCN          *string `json:"title_cn"`
	TitleEN          *string `json:"title_en"`
	TitleJP          *string `json:"title_jp"`
	Year             *int    `json:"year"`
	Season           *int    `json:"season"`
	BangumiID        *string `json:"bangumi_id"`
	SubgroupName     *string `json:"subgroup_name"`
	MetadataID       *string `json:"metadata_id"`
	MetadataProvider *string `json:"metadata_provider"`
	CoverURL         *string `json:"cover_url"`
	Description      *string `json:"description"`
	AnimeType        *string `json:"anime_type"`
	TotalEpisodes    *int    `json:"total_episodes"`
	Enabled          *bool   `json:"enabled"`
	Completed        *bool   `json:"completed"`
	FilterJSON         *string `json:"filter_json"`
	CustomPath         *string `json:"custom_path"`
	ExcludedKeywords   *string `json:"excluded_keywords"`
	AllowedSubgroups   *string `json:"allowed_subgroups"`
	SkipBulkUpdate     *bool   `json:"skip_bulk_update"`
	StallTimeoutHours  *int    `json:"stall_timeout_hours"`
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
		ID:               sub.ID,
		TitleCN:          sub.TitleCN,
		TitleEN:          sub.TitleEN,
		TitleJP:          sub.TitleJP,
		Year:             sub.Year,
		Season:           sub.Season,
		BangumiID:        sub.BangumiID,
		SubgroupName:     sub.SubgroupName,
		MetadataID:       sub.MetadataID,
		MetadataProvider: sub.MetadataProvider,
		CoverURL:         sub.CoverURL,
		AvatarURL:        sub.CoverURL, // 暂时兼容前端 Layout.vue 头像显示
		Description:      sub.Description,
		AnimeType:        sub.AnimeType,
		TotalEpisodes:    sub.TotalEpisodes,
		CurrentEpisodes:  sub.CurrentEpisodes, // 注意：这将在 handleListSubscriptions 中被实时统计值更新
		Enabled:          sub.Enabled != nil && *sub.Enabled,
		Completed:        sub.Completed,
		FilterJSON:        sub.FilterJSON,
		CustomPath:        sub.CustomPath,
		ExcludedKeywords:  sub.ExcludedKeywords,
		AllowedSubgroups:  sub.AllowedSubgroups,
		SkipBulkUpdate:    sub.SkipBulkUpdate,
		StallTimeoutHours: sub.StallTimeoutHours,
		CreatedAt:         sub.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         sub.UpdatedAt.Format(time.RFC3339),
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
		TorrentHash:    ep.TorrentHash,
		TorrentURL:     ep.TorrentURL,
		OriginalName:   ep.OriginalName,
		FinalPath:      ep.FinalPath,
		FileSize:       ep.FileSize,
		GroupName:      ep.GroupName,
		CreatedAt:      ep.CreatedAt.Format(time.RFC3339),
	}
	if ep.DownloadStartedAt != nil {
		r.DownloadStartedAt = ep.DownloadStartedAt.Format(time.RFC3339)
	}
	return r
}

// handleListSubscriptions 获取订阅列表
// GET /api/subscriptions?enabled=true&completed=false
func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	query := database.DB.Model(&database.Subscription{})

	if v := r.URL.Query().Get("enabled"); v != "" {
		b, _ := strconv.ParseBool(v)
		query = query.Where("enabled = ?", b)
	}
	if v := r.URL.Query().Get("completed"); v != "" {
		b, _ := strconv.ParseBool(v)
		query = query.Where("completed = ?", b)
	}

	var subs []database.Subscription
	if err := query.Order("created_at DESC").Find(&subs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "查询订阅失败"})
		return
	}

	// 批量统计超时剧集
	subIDs := make([]uint, len(subs))
	for i, sub := range subs {
		subIDs[i] = sub.ID
	}
	stalledMap := batchStalledCounts(subIDs, getStallTimeout())

	// 批量获取每个订阅的集数统计（包含已整理和下载中）
	type statusCount struct {
		SubscriptionID uint
		Status         string
		Count          int
	}
	var statusCounts []statusCount
	database.DB.Model(&database.Episode{}).
		Select("subscription_id, status, count(*) as count").
		Where("subscription_id IN ? AND deleted_at IS NULL", subIDs).
		Group("subscription_id, status").
		Scan(&statusCounts)

	organizedMap := make(map[uint]int)
	downloadingMap := make(map[uint]int)
	for _, c := range statusCounts {
		if c.Status == "organized" {
			organizedMap[c.SubscriptionID] += c.Count
		} else if c.Status == "downloading" || c.Status == "pending" || c.Status == "downloaded" {
			downloadingMap[c.SubscriptionID] += c.Count
		}
	}

	result := make([]subscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		// 实时修正 current_episodes
		realOrganized := organizedMap[sub.ID]
		if sub.CurrentEpisodes != realOrganized {
			sub.CurrentEpisodes = realOrganized
			go database.DB.Model(&sub).Update("current_episodes", realOrganized)
		}

		// 如果元数据缺失，后台异步补全一次
		if (sub.Year == 0 || sub.TotalEpisodes == 0) && s.triggerSupplement != nil {
			go s.triggerSupplement(context.Background(), sub.ID)
		}

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
		if item.TitleCN == "" {
			resp.Failed = append(resp.Failed, batchSubResult{Title: item.TitleCN, Error: "番剧标题不能为空"})
			continue
		}

		// 检查是否已订阅（通过 BangumiID 或标题）
		var existing database.Subscription
		query := database.DB.Where("bangumi_id = ?", item.BangumiID)
		if item.BangumiID == "" {
			query = database.DB.Where("title_cn = ?", item.TitleCN)
		}
		if query.First(&existing).RowsAffected > 0 {
			resp.Failed = append(resp.Failed, batchSubResult{Title: item.TitleCN, Error: "已存在订阅"})
			continue
		}

		subgroup := strings.Join(item.Subgroups, ",")

		sub := database.Subscription{
			TitleCN:       item.TitleCN,
			BangumiID:     item.BangumiID,
			SubgroupName:  subgroup,
			CoverURL:      item.CoverURL,
			TotalEpisodes: item.TotalEpisodes,
			Completed:     item.IsFinished,
			Enabled:       &[]bool{true}[0],
		}

		if err := database.DB.Create(&sub).Error; err != nil {
			resp.Failed = append(resp.Failed, batchSubResult{Title: item.TitleCN, Error: err.Error()})
			continue
		}

		resp.Success = append(resp.Success, batchSubResult{Title: item.TitleCN, ID: sub.ID})

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

// handleCreateSubscription 创建新订阅
// POST /api/subscriptions
func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if req.TitleCN == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "番剧标题 (title_cn) 不能为空"})
		return
	}

	// 查重：防止重复订阅
	var existing database.Subscription
	query := database.DB.Where("title_cn = ?", req.TitleCN)
	if req.BangumiID != "" {
		query = database.DB.Where("title_cn = ? OR bangumi_id = ?", req.TitleCN, req.BangumiID)
	}
	if err := query.First(&existing).Error; err == nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "该番剧已在订阅列表中"})
		return
	}

	sub := database.Subscription{
		TitleCN:      req.TitleCN,
		BangumiID:    req.BangumiID,
		SubgroupName: req.SubgroupName,
		RSSURL:       req.RSSURL,
		FilterJSON:   req.FilterJSON,
		CustomPath:   req.CustomPath,
		CoverURL:     req.CoverURL,
		Enabled:      &[]bool{true}[0],
	}

	if err := database.DB.Create(&sub).Error; err != nil {
		log.Printf("❌ 创建订阅失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建订阅失败"})
		return
	}

	// 如果有 BangumiID 但前端未提供 RSS URL，后台异步解析 Mikan 字幕组 RSS
	if sub.BangumiID != "" && sub.RSSURL == "" && s.mikanSrc != nil {
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
		}(sub.ID, sub.BangumiID)
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

	var sub database.Subscription
	if err := database.DB.First(&sub, id).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		return
	}

	var episodes []database.Episode
	database.DB.Where("subscription_id = ?", sub.ID).
		Order("season ASC, number ASC").
		Find(&episodes)

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

	var sub database.Subscription
	if err := database.DB.First(&sub, id).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		return
	}

	var req updateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	updates := map[string]interface{}{}
	if req.TitleCN != nil {
		updates["title_cn"] = *req.TitleCN
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
		updates["season"] = *req.Season
	}
	if req.BangumiID != nil {
		updates["bangumi_id"] = *req.BangumiID
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
	if req.CustomPath != nil {
		updates["custom_path"] = *req.CustomPath
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

	if err := database.DB.Model(&sub).Updates(updates).Error; err != nil {
		log.Printf("❌ 更新订阅失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "更新订阅失败"})
		return
	}

	if err := database.DB.First(&sub, id).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "更新后查询订阅失败"})
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

	var sub database.Subscription
	if err := database.DB.First(&sub, id).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "订阅不存在"})
		return
	}

	deleteFiles := r.URL.Query().Get("delete_files") == "true"

	// 先删 qB 种子和文件（有操作失败只记录日志，不中断流程）
	if deleteFiles && s.downloader != nil {
		var episodes []database.Episode
		database.DB.Where("subscription_id = ?", id).Find(&episodes)
		for _, ep := range episodes {
			if ep.TorrentHash == "" {
				continue
			}
			if err := s.downloader.Delete(r.Context(), ep.TorrentHash, true); err != nil {
				log.Printf("⚠️  删除种子失败 (hash=%s): %v", ep.TorrentHash, err)
			}
		}
	}

	// 删除关联剧集
	database.DB.Where("subscription_id = ?", id).Delete(&database.Episode{})
	// 删除订阅
	database.DB.Delete(&sub)

	log.Printf("🗑️  已删除订阅: %s (ID=%d, deleteFiles=%v)", sub.TitleCN, sub.ID, deleteFiles)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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

	for _, sub := range targets {
		wg.Add(1)
		go func(subID uint, title string) {
			defer wg.Done()
			sem <- struct{}{} // 获取信号量
			defer func() { <-sem }() // 释放信号量

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			if err := s.triggerSupplement(ctx, subID); err != nil {
				log.Printf("❌ 批量补全失败 [%s]: %v", title, err)
			} else {
				log.Printf("✅ 批量补全完成 [%s]", title)
			}
		}(sub.ID, sub.TitleCN)
	}

	go func() {
		wg.Wait()
		log.Println("✅ 全量补全任务全部完成")
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
		tasks = []core.DownloadTask{}
	}

	if tasks == nil {
		tasks = []core.DownloadTask{}
	}

	type downloadResponse struct {
		Hash      string  `json:"hash"`
		Name      string  `json:"name"`
		SavePath  string  `json:"save_path"`
		Status    string  `json:"status"`
		Progress  float32 `json:"progress"`
		SpeedDown int64   `json:"speed_down"`
		Size      int64   `json:"size"`
		Done      int64   `json:"done"`
	}

	result := make([]downloadResponse, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, downloadResponse{
			Hash:      t.Hash,
			Name:      t.Name,
			SavePath:  t.SavePath,
			Status:    t.Status,
			Progress:  t.Progress,
			SpeedDown: t.SpeedDown,
			Size:      t.Size,
			Done:      t.Done,
		})
	}

	writeJSON(w, http.StatusOK, result)
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
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version": "v0.5.1",
		"changelog": []string{
			"🛡️ 架构稳固：剔除数据库事务内网络依赖，杜绝 SQLite 并发锁死隐患",
			"🔐 凭证保护：修复下载器密钥脱敏保存与空密码覆盖缺陷",
			"🚀 插件生态：重构插件管理系统，支持多事件 Webhook 联动与配置导入导出",
			"⏱️ 细粒度控制：支持番剧级 Stall 超时配置，死种检测更灵活精确",
			"📂 路径安全：全面加固数据迁移与文件管理接口的路径穿越防护",
		},
	})
}

// countStalledEpisodes 统计订阅下所有卡住的剧集数
func countStalledEpisodes(subID uint, timeout time.Duration) int {
	cutoff := time.Now().Add(-timeout)
	var count int64
	database.DB.Model(&database.Episode{}).
		Where("subscription_id = ?", subID).
		Where("(status = 'pending' AND created_at < ?) OR (status = 'downloading' AND download_started_at < ?)", cutoff, cutoff).
		Count(&count)
	return int(count)
}

// batchStalledCounts 批量获取多个订阅的超时剧集数
func batchStalledCounts(subIDs []uint, timeout time.Duration) map[uint]int {
	cutoff := time.Now().Add(-timeout)
	type stalledResult struct {
		SubscriptionID uint
		Count          int64
	}
	var results []stalledResult
	database.DB.Model(&database.Episode{}).
		Select("subscription_id, count(*) as count").
		Where("subscription_id IN ?", subIDs).
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

// handleUpdateEpisodeStatus 手动更新剧集状态
// PUT /api/episodes/{id}/status  body: {"status": "completed"}
func (s *Server) handleUpdateEpisodeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的剧集 ID"})
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	valid := map[string]bool{"pending": true, "downloading": true, "completed": true, "failed": true}
	if !valid[req.Status] {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的状态值"})
		return
	}

	if err := database.DB.Model(&database.Episode{}).Where("id = ?", id).Update("status", req.Status).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "更新失败"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": req.Status})
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
	database.DB.Find(&settings)

	result := make(map[string]string, len(settings))
	for _, setting := range settings {
		val := setting.Value
		keyUpper := strings.ToUpper(setting.Key)
		// 脱敏敏感字段：包含 PASS, SECRET, KEY 的键返回空字符串
		if strings.Contains(keyUpper, "PASS") || strings.Contains(keyUpper, "SECRET") || strings.Contains(keyUpper, "KEY") {
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

	for key, value := range req.Settings {
		// 阻止通过 API 修改内部 JWT 密钥等系统配置
		keyUpper := strings.ToUpper(key)
		if keyUpper == "JWT_SECRET" || keyUpper == "TOKEN_SECRET" {
			log.Printf("⚠️  拒绝通过 API 修改内部鉴权密钥: %s", key)
			continue
		}

		// 如果值为空，且属于敏感凭证且数据库中已有值，则保留原值（避免前端脱敏展示后提交空值覆盖原有密钥）
		if value == "" && (strings.Contains(keyUpper, "PASS") || strings.Contains(keyUpper, "SECRET") || strings.Contains(keyUpper, "KEY") || strings.Contains(keyUpper, "TOKEN")) {
			var existing database.Setting
			if err := database.DB.Where("key = ?", key).First(&existing).Error; err == nil && existing.Value != "" {
				continue // 保持原有密码/密钥不变
			}
		}

		setting := database.Setting{Key: key, Value: value}
		database.DB.Where("key = ?", key).Assign(setting).FirstOrCreate(&setting)

		if key == "USER_AVATAR_URL" {
			database.DB.Model(&database.User{}).Where("1 = 1").Update("avatar_url", value)
		}
	}

	log.Printf("✅ 已更新 %d 项设置", len(req.Settings))
	writeJSON(w, http.StatusOK, map[string]string{"message": "设置已更新"})
}

// handleUploadAvatar 上传管理员头像图片
// POST /api/user/avatar
func (s *Server) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value("claims").(*auth.Claims)
	if !ok || claims == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "未登录或登录已过期"})
		return
	}

	// 限制文件大小最大 5MB
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "文件过大，头像不能超过 5MB"})
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "未找到上传的文件，字段名应为 avatar"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" && ext != ".gif" && ext != ".svg" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "格式不支持，仅允许 PNG, JPG, WebP, GIF 或 SVG 图片"})
		return
	}

	uploadDir := "./data/uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建上传目录失败"})
		return
	}

	// 删除此用户旧的头像文件
	oldFiles, _ := filepath.Glob(filepath.Join(uploadDir, fmt.Sprintf("avatar_%s.*", claims.Username)))
	for _, old := range oldFiles {
		os.Remove(old)
	}

	destFilename := fmt.Sprintf("avatar_%s%s", claims.Username, ext)
	destPath := filepath.Join(uploadDir, destFilename)

	destFile, err := os.Create(destPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建头像文件失败"})
		return
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "保存头像文件内容失败"})
		return
	}

	avatarURL := fmt.Sprintf("/api/user/avatar?t=%d", time.Now().Unix())

	// 同步到数据库
	database.DB.Model(&database.User{}).Where("username = ?", claims.Username).Update("avatar_url", avatarURL)
	database.DB.Save(&database.Setting{Key: "USER_AVATAR_URL", Value: avatarURL})

	log.Printf("✅ 用户 %s 上传了新头像: %s", claims.Username, destFilename)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"avatar_url": avatarURL,
		"message":    "头像上传成功",
	})
}

// handleGetAvatar 读取并返回管理员头像
// GET /api/user/avatar
func (s *Server) handleGetAvatar(w http.ResponseWriter, r *http.Request) {
	uploadDir := "./data/uploads"
	matches, err := filepath.Glob(filepath.Join(uploadDir, "avatar_*"))
	if err != nil || len(matches) == 0 {
		http.NotFound(w, r)
		return
	}

	// 读取最新的头像文件
	latest := matches[len(matches)-1]
	ext := strings.ToLower(filepath.Ext(latest))
	switch ext {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, latest)
}

// handleGetLogs 获取系统日志（最近 100 行，过滤认证和心跳噪音）
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

	// 逆向 Seek 读取尾部 N 行（类 tail -n）
	const chunkSize = 4096
	stat, _ := f.Stat()
	fileSize := stat.Size()

	var tail []byte
	pos := fileSize
	buf := make([]byte, chunkSize)
	linesFound := 0

	for pos > 0 && linesFound <= lines {
		readSize := int64(chunkSize)
		if pos < chunkSize {
			readSize = pos
			buf = make([]byte, readSize)
		}
		pos -= readSize
		if _, err := f.Seek(pos, 0); err != nil {
			break
		}
		if _, err := f.Read(buf); err != nil {
			break
		}
		tail = append(buf, tail...)
		linesFound = 0
		for _, b := range tail {
			if b == '\n' {
				linesFound++
			}
		}
		if pos == 0 {
			break
		}
	}

	content := strings.TrimRight(string(tail), "\n")
	allLines := []string{}
	if content != "" {
		allLines = strings.Split(content, "\n")
	}

	// 提取最后 N 行
	total := len(allLines)
	start := 0
	if total > lines {
		start = total - lines
	}
	recent := allLines[start:]

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lines": recent,
		"total": total,
	})
}

// handleGetCustomRegex 获取当前自定义正则规则
// GET /api/settings/custom-regex
func (s *Server) handleGetCustomRegex(w http.ResponseWriter, r *http.Request) {
	var rawPatterns []string
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("custom_regex_%d", i)
		var setting database.Setting
		if err := database.DB.Where("key = ?", key).First(&setting).Error; err != nil {
			break
		}
		if v := strings.TrimSpace(setting.Value); v != "" {
			rawPatterns = append(rawPatterns, v)
		} else {
			break
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

// ============================================================
// 插件管理
// ============================================================

// handleGetPlugins 获取当前已加载的插件列表
// GET /api/plugins
func (s *Server) handleGetPlugins(w http.ResponseWriter, r *http.Request) {
	if s.pluginManager == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	plugins := s.pluginManager.GetPluginList()
	writeJSON(w, http.StatusOK, plugins)
}

// handleReloadPlugins 重新加载插件配置
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
	s.pluginManager.Load()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "插件已重新加载",
		"count":   len(s.pluginManager.GetPluginList()),
	})
}

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
	p := s.getBGMProvider()
	if p == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "Bangumi 服务未就绪"})
		return
	}
	
	items, err := p.GetCalendar(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	// 转换为 WeekDayItem 结构
	weekLabel := map[int]string{
		1: "星期一", 2: "星期二", 3: "星期三", 4: "星期四",
		5: "星期五", 6: "星期六", 7: "星期日",
	}
	
	groups := make(map[int][]core.TorrentItem)
	for _, item := range items {
		day, _ := strconv.Atoi(item.EpisodeURL)
		if day == 0 { day = 7 } // Bangumi 0 可能表示周日
		item.EpisodeURL = "" // 清除 hack 字段
		groups[day] = append(groups[day], item)
	}

	schedule := make([]source.WeekDayItem, 0, 7)
	for i := 1; i <= 7; i++ {
		dayItems := groups[i]
		if dayItems == nil { dayItems = []core.TorrentItem{} }
		schedule = append(schedule, source.WeekDayItem{
			DayOfWeek: i,
			Label:     weekLabel[i],
			Items:     dayItems,
		})
	}

	// 同时获取订阅列表，标注已订阅的番剧
	var subs []database.Subscription
	database.DB.Find(&subs)
	subscribed := make(map[string]uint)
	subStats := make(map[uint]map[string]int)
	for _, sub := range subs {
		if sub.BangumiID != "" { subscribed[sub.BangumiID] = sub.ID }
		if sub.TitleCN != "" { subscribed[sub.TitleCN] = sub.ID }
		subStats[sub.ID] = map[string]int{
			"downloaded": int(sub.CurrentEpisodes),
			"total":      sub.TotalEpisodes,
		}
	}

	for _, day := range schedule {
		for i := range day.Items {
			item := &day.Items[i]
			if id, ok := subscribed[item.BangumiID]; ok {
				item.InfoHash = fmt.Sprintf("%d", id)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"days":       schedule,
		"subscribed": subscribed,
		"stats":      subStats,
	})
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	season, _ := strconv.Atoi(r.URL.Query().Get("season"))

	var schedule []source.WeekDayItem
	var err error

	if s.yucSrc != nil {
		yucCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		schedule, err = s.yucSrc.FetchWeekSchedule(yucCtx, year, season)
		if err != nil {
			log.Printf("⚠️  Yucwiki 获取时间表失败 (%v)，回退至 Mikan 数据源", err)
		} else {
			// yucwiki 获取成功后，额外获取 SP 条目
			spItems, spErr := s.yucSrc.FetchSPItems(yucCtx, year, season)
			if spErr == nil && len(spItems) > 0 {
				schedule = append(schedule, source.WeekDayItem{
					DayOfWeek: 0,
					Label:     "网络放送 & 其他",
					Items:     spItems,
				})
			}
		}
		cancel()
	}

	if (err != nil || len(schedule) == 0) && s.mikanSrc != nil {
		schedule, err = s.mikanSrc.FetchWeekSchedule(r.Context(), year, season)
		if err != nil {
			log.Printf("⚠️  Mikan 获取时间表失败: %v", err)
		}
	}

	if len(schedule) == 0 {
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "获取时间表失败: " + err.Error()})
			return
		}
		schedule = []source.WeekDayItem{}
	}

	// 同时获取订阅列表，标注已订阅的番剧（支持 ID 匹配和标准化标题模糊匹配）
	var subs []database.Subscription
	database.DB.Find(&subs)

	subscribed := make(map[string]uint)
	normSubs := make(map[string]uint)
	subStats := make(map[uint]map[string]int) // subID → {downloaded, total}

	// 批量统计已下载集数（避免 N+1 查询）
	type episodeCountResult struct {
		SubscriptionID uint
		Count          int64
	}
	var epResults []episodeCountResult
	subIDs := make([]uint, len(subs))
	for i, sub := range subs {
		subIDs[i] = sub.ID
	}
	if len(subIDs) > 0 {
		database.DB.Model(&database.Episode{}).
			Select("subscription_id, count(*) as count").
			Where("subscription_id IN ? AND status IN ?", subIDs, []string{"downloaded", "downloading"}).
			Group("subscription_id").
			Find(&epResults)
	}
	downloadedMap := make(map[uint]int, len(epResults))
	for _, r := range epResults {
		downloadedMap[r.SubscriptionID] = int(r.Count)
	}

	for _, sub := range subs {
		if sub.BangumiID != "" {
			subscribed[sub.BangumiID] = sub.ID
		}
		if sub.TitleCN != "" {
			subscribed[sub.TitleCN] = sub.ID
			normSubs[core.NormalizeTitle(sub.TitleCN)] = sub.ID
		}
		subStats[sub.ID] = map[string]int{
			"downloaded": downloadedMap[sub.ID],
			"total":      sub.TotalEpisodes,
		}
	}

	for _, day := range schedule {
		for i := range day.Items {
			item := &day.Items[i]
			if id, ok := subscribed[item.BangumiID]; ok {
				item.InfoHash = fmt.Sprintf("%d", id)
			} else if id, ok := subscribed[item.Title]; ok {
				item.InfoHash = fmt.Sprintf("%d", id)
			} else if id, ok := normSubs[core.NormalizeTitle(item.Title)]; ok {
				item.InfoHash = fmt.Sprintf("%d", id)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"days":              schedule,
		"subscribed":        subscribed,
		"subscriptionCount": len(subs),
			"stats":             subStats,
	})
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

	// 域名白名单校验（使用 URL 解析 + 后缀匹配，防止 SSRF 绕过）
	allowedDomains := []string{"i0.hdslb.com", "lain.bgm.tv", "img.mikanani.me", "mikanani.me", "image.tmdb.org", "bilibili.com", "bgm.tv", "mikanime.tv", "yuc.wiki", "yucc.wiki", "yucwiki.net"}
	parsedURL, err := url.Parse(imageURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	host := parsedURL.Hostname()
	allowed := false
	for _, domain := range allowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			allowed = true
			break
		}
	}
	if !allowed {
		log.Printf("⚠️  非法图片代理请求被拦截: %s", imageURL)
		http.Error(w, "domain not allowed", http.StatusForbidden)
		return
	}

	client := httpx.NewInsecure(15 * time.Second)
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

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.WriteHeader(resp.StatusCode)
	// 限制代理响应体最大 20MB，防止恶意 URL 消耗服务器内存
	lr := io.LimitReader(resp.Body, 20*1024*1024)
	if _, err := io.Copy(w, lr); err != nil && err != io.EOF {
		log.Printf("⚠️  图片代理传输中断: %v", err)
	}
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

// handleMigrateData 从 AutoBangumi / ani-rss SQLite 数据库迁移数据
// POST /api/migrate
func (s *Server) handleMigrateData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "仅支持 POST"})
		return
	}

	var req migrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}

	if req.SourcePath == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请提供 source_path（源数据库文件路径）"})
		return
	}

	// 安全校验：限制迁移文件只能在 data/ 目录下，防止读取任意 SQLite 文件
	cleanPath := filepath.ToSlash(filepath.Clean(req.SourcePath))
	dataDir := "data"
	if !filepath.IsAbs(req.SourcePath) {
		// 相对路径检查
		if !strings.HasPrefix(cleanPath, dataDir+"/") && cleanPath != dataDir {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "迁移文件路径必须在 data/ 目录下"})
			return
		}
	} else {
		// 绝对路径检查：解析后必须在 data/ 下
		absDataDir, _ := filepath.Abs(dataDir)
		slashAbsDataDir := filepath.ToSlash(absDataDir)
		if !strings.HasPrefix(cleanPath, slashAbsDataDir+"/") && cleanPath != slashAbsDataDir {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "迁移文件路径必须在 data/ 目录下"})
			return
		}
	}

	stats, err := migrate.MigrateFromPath(req.SourcePath)
	if err != nil {
		log.Printf("❌ 数据迁移失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "迁移失败: " + err.Error()})
		return
	}

	log.Printf("✅ 数据迁移成功: 迁移了 %d 条订阅", stats.Subscriptions)
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
	if err := database.DB.Where("username = ?", claims.Username).First(&user).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "用户不存在"})
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
	user.PasswordHash = hash
	user.TokenVersion++
	if err := database.DB.Save(&user).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存失败"})
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

	// 先删 qB 种子和文件（有操作失败只记录日志，不中断流程）
	if req.DeleteFiles && s.downloader != nil {
		var episodes []database.Episode
		database.DB.Where("subscription_id IN ?", req.IDs).Find(&episodes)
		for _, ep := range episodes {
			if ep.TorrentHash == "" {
				continue
			}
			if err := s.downloader.Delete(r.Context(), ep.TorrentHash, true); err != nil {
				log.Printf("⚠️  批量删除种子失败 (hash=%s): %v", ep.TorrentHash, err)
			}
		}
	}

	// 事务：软删除订阅 + 关联剧集
	tx := database.DB.Begin()
	if err := tx.Where("id IN ?", req.IDs).Delete(&database.Subscription{}).Error; err != nil {
		tx.Rollback()
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "删除失败"})
		return
	}
	if err := tx.Where("subscription_id IN ?", req.IDs).Delete(&database.Episode{}).Error; err != nil {
		tx.Rollback()
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "删除关联剧集失败"})
		return
	}
	tx.Commit()

	log.Printf("🗑️  批量删除订阅: %v（deleteFiles=%v）", req.IDs, req.DeleteFiles)
	writeJSON(w, http.StatusOK, batchDeleteResponse{Deleted: len(req.IDs)})
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
	if req.Channel != "" {
		// 单渠道测试
		err := s.notifyMgr.SendTest(ctx, req.Channel, req.Title, req.Message)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	} else {
		// 全渠道测试
		err := s.notifyMgr.SendTest(ctx, "", req.Title, req.Message)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "测试通知已发送",
	})
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
	modelsEndpoint = modelsEndpoint + "/v1beta/models?key=" + apiKey

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
		if err := database.DB.Where("username = ? AND token_version = ?", claims.Username, claims.TokenVersion).First(&user).Error; err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	} else {
		// 强制要求鉴权
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
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

	fmt.Fprintf(w, "data: {\"type\": \"connected\"}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case ev := <-eventChan:
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleListPlugins 获取标准化插件列表
func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.pluginManager.GetPluginList())
}

// handleTogglePlugin 界面化切换插件状态
func (s *Server) handleTogglePlugin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效请求"})
		return
	}
	if err := s.pluginManager.TogglePlugin(req.ID, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "插件状态已更新"})
}

// handleSavePlugin 添加或编辑自定义插件
func (s *Server) handleSavePlugin(w http.ResponseWriter, r *http.Request) {
	var p plugin.PluginInfo
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求格式错误"})
		return
	}
	if p.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "插件名称不能为空"})
		return
	}
	if err := s.pluginManager.AddOrUpdatePlugin(p); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "插件已保存并启用"})
}

// handleDeletePlugin 删除自定义插件
func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/api/plugins/")
	}
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "缺少插件 ID"})
		return
	}
	if err := s.pluginManager.DeletePlugin(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "插件已删除"})
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

	redirectURI := fmt.Sprintf("%s/api/bangumi/auth/callback", s.getPublicBaseURL(r))
	authURL := fmt.Sprintf("https://bgm.tv/oauth/authorize?client_id=%s&response_type=code&redirect_uri=%s", 
		clientID, url.QueryEscape(redirectURI))
	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

func (s *Server) handleBangumiAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
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

	redirectURI := fmt.Sprintf("%s/api/bangumi/auth/callback", s.getPublicBaseURL(r))

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm("https://bgm.tv/oauth/access_token", data)
	if err != nil {
		http.Error(w, "failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		http.Error(w, fmt.Sprintf("Bangumi 授权换取令牌失败 (HTTP %d): %s", resp.StatusCode, string(body)), http.StatusBadRequest)
		return
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil || tokenResp.AccessToken == "" {
		http.Error(w, "解析 Bangumi 访问令牌失败或令牌为空", http.StatusInternalServerError)
		return
	}

	database.DB.Save(&database.Setting{Key: "BGMTV_USER_TOKEN", Value: tokenResp.AccessToken})
	database.DB.Save(&database.Setting{Key: "BANGUMI_USER_TOKEN", Value: tokenResp.AccessToken})

	if p, ok := s.md.(*metadata.BGMTVProvider); ok {
		p.SetUserToken(tokenResp.AccessToken)
	}

	// 自动获取当前登录用户的 username 并持久化
	client := httpx.New(10 * time.Second)
	meReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.bgm.tv/v0/me", nil)
	if err == nil {
		meReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		meReq.Header.Set("User-Agent", "Ani-Go/1.0")
		if meResp, err := client.Do(meReq); err == nil {
			defer meResp.Body.Close()
			if meResp.StatusCode == http.StatusOK {
				var me struct {
					Username string `json:"username"`
					ID       int    `json:"id"`
				}
				if err := json.NewDecoder(meResp.Body).Decode(&me); err == nil {
					uname := me.Username
					if uname == "" && me.ID > 0 {
						uname = fmt.Sprintf("%d", me.ID)
					}
					if uname != "" {
						database.DB.Save(&database.Setting{Key: "BGMTV_USERNAME", Value: uname})
						log.Printf("✅ Bangumi OAuth 授权成功，已自动关联用户: %s", uname)
					}
				}
			}
		}
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
