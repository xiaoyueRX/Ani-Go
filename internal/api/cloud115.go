package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/cloud115"
	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

type cloud115Tasks interface {
	Add(context.Context, string, string) error
	List(context.Context, string, int) (cloud115.TaskPage, error)
}

func (s *Server) withCloud115(next func(http.ResponseWriter, *http.Request, cloud115Tasks, plugin.PluginContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.pluginManager == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "插件管理器不可用"})
			return
		}
		_, pctx, _ := s.pluginManager.PanelSnapshot(plugin.Cloud115ID)
		if pctx == nil {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "请先启用 115 云下载插件"})
			return
		}
		service, _ := pctx.GetService(plugin.Cloud115Service)
		client, ok := service.(cloud115Tasks)
		if !ok || pctx.Context().Err() != nil {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "115 云下载插件未就绪"})
			return
		}
		// HTTP 客户端断开或管理员停用插件时，取消已经发出的查询或提交。
		ctx, cancel := panelRequestContext(r, pctx)
		defer cancel()
		next(w, r.WithContext(ctx), client, pctx)
	}
}

func (s *Server) handleCloud115Config(w http.ResponseWriter, r *http.Request, _ cloud115Tasks, scope plugin.PluginContext) {
	s.servePanelCredential(w, r, scope, false)
}

func (s *Server) handleCloud115Tasks(w http.ResponseWriter, r *http.Request, client cloud115Tasks, scope plugin.PluginContext) {
	credential, err := s.pluginManager.AccessPanelCredential(r.Context(), scope, nil)
	if err != nil {
		writePanelStorageError(w, err)
		return
	}
	token := credential.Token
	if token == "" {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "请先配置 115 开放平台访问令牌"})
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Magnet string `json:"magnet"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req) != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "无效的磁力提交请求"})
			return
		}
		magnet := strings.TrimSpace(req.Magnet)
		if cloud115.ValidateMagnet(magnet) != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请输入有效的 BitTorrent 磁力链接"})
			return
		}
		err = client.Add(r.Context(), token, magnet)
		if err == nil {
			writeJSON(w, http.StatusCreated, map[string]bool{"submitted": true})
			return
		}
	} else {
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			page, err = strconv.Atoi(raw)
			if err != nil || page < 1 {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "页码必须是正整数"})
				return
			}
		}
		var tasks cloud115.TaskPage
		tasks, err = client.List(r.Context(), token, page)
		if err == nil {
			writeJSON(w, http.StatusOK, tasks)
			return
		}
	}
	switch {
	case errors.Is(err, cloud115.ErrUnauthorized):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "115 授权无效或已过期，请检查权限并更新访问令牌"})
	case errors.Is(err, cloud115.ErrRateLimited):
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "115 请求频率受限，请稍后重试"})
	case errors.Is(err, context.Canceled):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "115 请求已取消，重新提交前请先刷新云端任务"})
	default:
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "115 暂未确认请求成功，请检查云端任务、离线配额及服务状态"})
	}
}
