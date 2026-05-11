package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/migrate"
	"github.com/xiaoyueRX/Ani-Go/internal/source"
)

// ============================================================
// 订阅 CRUD
// ============================================================

// subscriptionResponse API 返回的订阅数据结构
type subscriptionResponse struct {
	ID              uint   `json:"id"`
	TitleCN         string `json:"title_cn"`
	TitleEN         string `json:"title_en"`
	TitleJP         string `json:"title_jp"`
	Year            int    `json:"year"`
	Season          int    `json:"season"`
	BangumiID       string `json:"bangumi_id"`
	SubgroupName    string `json:"subgroup_name"`
	MetadataID      string `json:"metadata_id"`
	MetadataProvider string `json:"metadata_provider"`
	CoverURL        string `json:"cover_url"`
	Description     string `json:"description"`
	AnimeType       string `json:"anime_type"`
	TotalEpisodes   int    `json:"total_episodes"`
	CurrentEpisodes int    `json:"current_episodes"`
	Enabled         bool   `json:"enabled"`
	Completed       bool   `json:"completed"`
	FilterJSON      string `json:"filter_json"`
	CustomPath      string `json:"custom_path"`
	StalledEpisodes int    `json:"stalled_episodes"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
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
	TitleCN   string   `json:"title_cn"`
	BangumiID string   `json:"bangumi_id"`
	CoverURL  string   `json:"cover_url"`
	Subgroups []string `json:"subgroups"`
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
	TitleCN         *string `json:"title_cn"`
	TitleEN         *string `json:"title_en"`
	TitleJP         *string `json:"title_jp"`
	Year            *int    `json:"year"`
	Season          *int    `json:"season"`
	BangumiID       *string `json:"bangumi_id"`
	SubgroupName    *string `json:"subgroup_name"`
	MetadataID      *string `json:"metadata_id"`
	MetadataProvider *string `json:"metadata_provider"`
	CoverURL        *string `json:"cover_url"`
	Description     *string `json:"description"`
	AnimeType       *string `json:"anime_type"`
	TotalEpisodes   *int    `json:"total_episodes"`
	Enabled         *bool   `json:"enabled"`
	Completed       *bool   `json:"completed"`
	FilterJSON      *string `json:"filter_json"`
	CustomPath      *string `json:"custom_path"`
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
	ID               uint    `json:"id"`
	SubscriptionID   uint    `json:"subscription_id"`
	Season           int     `json:"season"`
	Number           float32 `json:"number"`
	Title            string  `json:"title"`
	Status           string  `json:"status"`
	TorrentHash      string  `json:"torrent_hash"`
	TorrentURL       string  `json:"torrent_url"`
	OriginalName     string  `json:"original_name"`
	FinalPath        string  `json:"final_path"`
	FileSize         int64   `json:"file_size"`
	IsStalled        bool    `json:"is_stalled"`
	GroupName        string  `json:"group_name"`
	DownloadStartedAt string  `json:"download_started_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
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
		Description:      sub.Description,
		AnimeType:        sub.AnimeType,
		TotalEpisodes:    sub.TotalEpisodes,
		CurrentEpisodes:  sub.CurrentEpisodes,
		Enabled:          sub.Enabled,
		Completed:        sub.Completed,
		FilterJSON:       sub.FilterJSON,
		CustomPath:       sub.CustomPath,
		CreatedAt:        sub.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        sub.UpdatedAt.Format(time.RFC3339),
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
		if b, err := strconv.ParseBool(v); err == nil {
			query = query.Where("enabled = ?", b)
		}
	}
	if v := r.URL.Query().Get("completed"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			query = query.Where("completed = ?", b)
		}
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

	result := make([]subscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		r := toSubscriptionResponse(sub)
		r.StalledEpisodes = stalledMap[sub.ID]
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
			TitleCN:      item.TitleCN,
			BangumiID:    item.BangumiID,
			SubgroupName: subgroup,
			CoverURL:     item.CoverURL,
			Enabled:      true,
		}

		if err := database.DB.Create(&sub).Error; err != nil {
			resp.Failed = append(resp.Failed, batchSubResult{Title: item.TitleCN, Error: err.Error()})
			continue
		}

		resp.Success = append(resp.Success, batchSubResult{Title: item.TitleCN, ID: sub.ID})
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

	sub := database.Subscription{
		TitleCN:      req.TitleCN,
		BangumiID:    req.BangumiID,
		SubgroupName: req.SubgroupName,
		RSSURL:       req.RSSURL,
		FilterJSON:   req.FilterJSON,
		CustomPath:   req.CustomPath,
		CoverURL:     req.CoverURL,
		Enabled:      true,
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

	if s.triggerSupplement != nil {
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

	timeout := getStallTimeout()
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

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "未提供任何更新字段"})
		return
	}

	if err := database.DB.Model(&sub).Updates(updates).Error; err != nil {
		log.Printf("❌ 更新订阅失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "更新订阅失败"})
		return
	}

	database.DB.First(&sub, id)
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

	if !sub.Enabled {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "订阅未启用，无法触发补全"})
		return
	}

	if s.triggerSupplement == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "补全调度器未配置"})
		return
	}

	go func() {
		// 不能用 r.Context() — HTTP 响应返回后会被 Cancel
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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
		log.Printf("❌ 获取下载列表失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "获取下载列表失败: " + err.Error()})
		return
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

// getStallTimeout 获取超时阈值，默认 48 小时
func getStallTimeout() time.Duration {
	var setting database.Setting
	if err := database.DB.Where("key = ?", "stall_timeout_hours").First(&setting).Error; err == nil {
		if hours, err := strconv.Atoi(setting.Value); err == nil && hours > 0 {
			return time.Duration(hours) * time.Hour
		}
	}
	return 48 * time.Hour
}

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version": s.version,
		"changelog": []string{
			"新增引导弹窗单次会话逻辑，不再频繁打扰",
			"新增版本更新日志提示，及时了解新功能",
			"新增自动检查更新功能，支持检测 GitHub 最新版本",
			"优化设置页布局，增加自动更新开关",
			"修复部分 UI 显示问题",
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
		setting := database.Setting{Key: key, Value: value}
		database.DB.Where("key = ?", key).Assign(setting).FirstOrCreate(&setting)
	}

	log.Printf("✅ 已更新 %d 项设置", len(req.Settings))
	writeJSON(w, http.StatusOK, map[string]string{"message": "设置已更新"})
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
		f.Seek(pos, 0)
		f.Read(buf)
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
		"patterns":            rawPatterns,
		"compiled":            source.GetCustomRegexPatterns(),
		"builtin_count":       8,
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
		"message":   "自定义正则已重新加载",
		"compiled":  source.GetCustomRegexPatterns(),
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
	plugins := s.pluginManager.GetPlugins()
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
	s.pluginManager.Reload()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "插件已重新加载",
		"count":   len(s.pluginManager.GetPlugins()),
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
func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	year, _ := strconv.Atoi(r.URL.Query().Get("year"))
	season, _ := strconv.Atoi(r.URL.Query().Get("season"))

	var schedule []source.WeekDayItem
	var err error

	if s.yucSrc != nil {
		schedule, err = s.yucSrc.FetchWeekSchedule(r.Context(), year, season)
		if err != nil {
			log.Printf("⚠️  Yucwiki 获取时间表失败: %v，尝试使用 mikan", err)
		} else {
			// yucwiki 获取成功后，额外获取 SP 条目
			spGroups, spErr := s.yucSrc.FetchSPItems(r.Context())
			if spErr == nil {
				for _, g := range spGroups {
					schedule = append(schedule, source.WeekDayItem{
						DayOfWeek: 0,
						Label:     fmt.Sprintf("%s · %s", g.Month, g.Type),
						Items:     g.Items,
					})
				}
			} else if spErr != nil {
				log.Printf("⚠️  Yucwiki 获取 SP 失败: %v", spErr)
			}
		}
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
	for _, sub := range subs {
		if sub.BangumiID != "" {
			subscribed[sub.BangumiID] = sub.ID
		}
		if sub.TitleCN != "" {
			subscribed[sub.TitleCN] = sub.ID
			normSubs[normalizeTitle(sub.TitleCN)] = sub.ID
		}
		// 统计已下载集数和总集数
		var downloaded int64
		database.DB.Model(&database.Episode{}).
			Where("subscription_id = ? AND status IN ?", sub.ID, []string{"downloaded", "downloading"}).
			Count(&downloaded)
		subStats[sub.ID] = map[string]int{
			"downloaded": int(downloaded),
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
			} else if id, ok := normSubs[normalizeTitle(item.Title)]; ok {
				item.InfoHash = fmt.Sprintf("%d", id)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"days":              schedule,
		"subscribed":        subscribed,
		"subscriptionCount": len(subs),
		"sub_stats":         subStats,
	})
}

// handleTestMirrors 测试所有 Mikan 镜像延迟
// POST /api/mikan/test-mirrors
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

// handleProxyImage 代理图片请求（绕过 Bilibili CDN 热链保护，加白名单限制）
// GET /api/proxy/image?url=https://i0.hdslb.com/...
func (s *Server) handleProxyImage(w http.ResponseWriter, r *http.Request) {
	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	// 域名白名单校验
	allowedDomains := []string{"i0.hdslb.com", "lain.bgm.tv", "img.mikanani.me", "image.tmdb.org", "bilibili.com", "bgm.tv", "mikanime.tv"}
	allowed := false
	for _, domain := range allowedDomains {
		if strings.Contains(imageURL, domain) {
			allowed = true
			break
		}
	}
	if !allowed {
		log.Printf("⚠️  非法图片代理请求被拦截: %s", imageURL)
		http.Error(w, "domain not allowed", http.StatusForbidden)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
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
	io.Copy(w, resp.Body)
}

// normalizeTitle 用于增强番剧标题匹配的鲁棒性（去除空格、统一简繁/变体等）
func normalizeTitle(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "　", "")
	s = strings.ReplaceAll(s, "：", ":")
	s = strings.ReplaceAll(s, "坊", "房") // 统一常见歧义字
	s = strings.ReplaceAll(s, "・", "")
	return strings.ToLower(s)
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
