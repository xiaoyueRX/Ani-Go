package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
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
	IsStalled        bool    `json:"is_stalled"`
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

	// 从数据库获取搜索源配置
	var sources []core.Source
	mikanDomain := "mikanime.tv" // 默认使用国内可访问的域名
	var proxyDomain string
	var mirrorDomains []string

	if v := getSettingValue("MIKAN_DOMAIN"); v != "" {
		mikanDomain = v
	}
	if v := getSettingValue("MIKAN_PROXY_DOMAIN"); v != "" {
		proxyDomain = v
	}
	if v := getSettingValue("MIKAN_MIRROR_DOMAINS"); v != "" {
		for _, d := range strings.Split(v, ",") {
			if d = strings.TrimSpace(d); d != "" {
				mirrorDomains = append(mirrorDomains, d)
			}
		}
	}
	if len(mirrorDomains) == 0 {
		mirrorDomains = []string{"mikanime.tv", "mikanani.me"}
	}

	mikanSrc := source.NewMikanSource(mikanDomain, proxyDomain, mirrorDomains)
	sources = append(sources, mikanSrc)

	// 额外资源站
	if v := getSettingValue("NYAA_DOMAIN"); v != "" {
		sources = append(sources, source.NewNyaaSource(v))
	}
	if v := getSettingValue("ACGRIP_DOMAIN"); v != "" {
		sources = append(sources, source.NewACGRIPSource(v))
	}
	if v := getSettingValue("ANIMETOSHO_DOMAIN"); v != "" {
		sources = append(sources, source.NewAnimeToshoSource(v))
	}

	multiSrc := source.NewMultiSource(sources...)

	items, err := multiSrc.SearchAnime(r.Context(), q)
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":       "迁移完成",
		"subscriptions": stats.Subscriptions,
		"episodes":      stats.Episodes,
		"downloads":     stats.Downloads,
		"errors":        stats.Errors,
	})
}
