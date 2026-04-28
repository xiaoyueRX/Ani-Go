package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
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
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type createSubscriptionRequest struct {
	TitleCN      string `json:"title_cn"`
	BangumiID    string `json:"bangumi_id"`
	SubgroupName string `json:"subgroup_name"`
	FilterJSON   string `json:"filter_json"`
	CustomPath   string `json:"custom_path"`
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

	result := make([]subscriptionResponse, 0, len(subs))
	for _, sub := range subs {
		result = append(result, toSubscriptionResponse(sub))
	}
	writeJSON(w, http.StatusOK, result)
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
		FilterJSON:   req.FilterJSON,
		CustomPath:   req.CustomPath,
		Enabled:      true,
	}

	if err := database.DB.Create(&sub).Error; err != nil {
		log.Printf("❌ 创建订阅失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "创建订阅失败"})
		return
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

	eps := make([]episodeResponse, 0, len(episodes))
	for _, ep := range episodes {
		eps = append(eps, toEpisodeResponse(ep))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscription": toSubscriptionResponse(sub),
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

	// 删除关联剧集
	database.DB.Where("subscription_id = ?", id).Delete(&database.Episode{})
	// 删除订阅
	database.DB.Delete(&sub)

	log.Printf("🗑️  已删除订阅: %s (ID=%d)", sub.TitleCN, sub.ID)
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
		if err := s.triggerSupplement(r.Context(), uint(id)); err != nil {
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
		result[setting.Key] = setting.Value
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
