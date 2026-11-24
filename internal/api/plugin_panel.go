package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/plugin"
)

// 在一次快照中确定面板声明和实例，避免重载时错用另一代插件的服务。
func (s *Server) activePanel(w http.ResponseWriter, r *http.Request) (plugin.PluginInfo, plugin.PluginContext, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if s.pluginManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "插件管理器不可用"})
		return plugin.PluginInfo{}, nil, false
	}
	info, scope, found := s.pluginManager.PanelSnapshot(r.PathValue("id"))
	if !found || info.Panel == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "未找到插件面板"})
		return plugin.PluginInfo{}, nil, false
	}
	if scope == nil || scope.Context().Err() != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "插件面板当前不可用，请检查插件运行状态"})
		return plugin.PluginInfo{}, nil, false
	}
	return info, scope, true
}

func panelRequestContext(r *http.Request, scope plugin.PluginContext) (context.Context, func()) {
	ctx, cancel := context.WithCancel(r.Context())
	lifetime := scope.Context()
	stop := context.AfterFunc(lifetime, cancel)
	if lifetime.Err() != nil {
		cancel()
	}
	return ctx, func() {
		stop()
		cancel()
	}
}

// 插件操作只接受一个 JSON 对象，可选请求体缺省时沿用零值。
func decodePluginRequest[T any](w http.ResponseWriter, r *http.Request, dst *T, optional bool) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	err := dec.Decode(&dst)
	if errors.Is(err, io.EOF) && optional {
		return true
	}
	if err != nil || dst == nil || dec.Decode(new(json.RawMessage)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "插件请求参数无效"})
		return false
	}
	return true
}

func (s *Server) handlePanelCredential(w http.ResponseWriter, r *http.Request) {
	info, scope, ok := s.activePanel(w, r)
	if !ok {
		return
	}
	if info.Panel.Credential == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "此插件没有声明凭据"})
		return
	}
	ctx, cancel := panelRequestContext(r, scope)
	defer cancel()
	s.servePanelCredential(w, r.WithContext(ctx), info.URL, scope, true)
}

func (s *Server) servePanelCredential(w http.ResponseWriter, r *http.Request, currentURL string, scope plugin.PluginContext, strict bool) {
	var update *string
	switch r.Method {
	case http.MethodPut:
		var req struct {
			Token       string          `json:"access_token"`
			ExpectedURL json.RawMessage `json:"expected_url"`
		}
		if strict {
			if !decodePluginRequest(w, r, &req, false) {
				return
			}
			expectedURL, urlOK := panelRequestField(req.ExpectedURL, "")
			if !urlOK || req.ExpectedURL == nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请提供已确认的插件目标地址"})
				return
			}
			if expectedURL != currentURL {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "插件目标已变更，请刷新后重新保存凭据"})
				return
			}
		} else if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req) != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "插件凭据请求无效"})
			return
		}
		req.Token = strings.TrimSpace(req.Token)
		if req.Token == "" || len(req.Token) > 8192 || strings.IndexFunc(req.Token, func(c rune) bool { return c < 33 || c > 126 }) >= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请输入有效的插件访问令牌"})
			return
		}
		update = &req.Token
	case http.MethodDelete:
		update = new(string)
	}

	config, err := s.pluginManager.AccessPanelCredential(r.Context(), scope, update)
	if err != nil {
		writePanelStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func writePanelStorageError(w http.ResponseWriter, err error) {
	if errors.Is(err, plugin.ErrPanelCredentialReadOnly) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "凭据由环境变量配置，请在部署环境中修改"})
		return
	}
	if errors.Is(err, plugin.ErrPanelInactive) || errors.Is(err, context.Canceled) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "插件面板已停用"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "访问插件凭据失败"})
}

func (s *Server) handlePanelAction(w http.ResponseWriter, r *http.Request) {
	info, scope, ok := s.activePanel(w, r)
	if !ok {
		return
	}
	for _, action := range info.Panel.Actions {
		if action.ID == r.PathValue("action") {
			s.runPanelAction(w, r, info, scope, action)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, errorResponse{Error: "插件未声明此操作"})
}

// 可选字段省略时使用默认值，显式 null 和类型不符都属于无效输入。
func panelRequestField[T any](raw json.RawMessage, fallback T) (T, bool) {
	if raw == nil {
		return fallback, true
	}
	var value *T
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return fallback, false
	}
	return *value, true
}

func (s *Server) runPanelAction(w http.ResponseWriter, r *http.Request, info plugin.PluginInfo, scope plugin.PluginContext, action plugin.PanelAction) {
	var request struct {
		Input       json.RawMessage `json:"input"`
		Page        json.RawMessage `json:"page"`
		Placement   json.RawMessage `json:"placement"`
		ExpectedURL json.RawMessage `json:"expected_url"`
	}
	if !decodePluginRequest(w, r, &request, true) {
		return
	}
	if request.Placement != nil || request.ExpectedURL != nil {
		placement, placementOK := panelRequestField(request.Placement, "")
		expectedURL, urlOK := panelRequestField(request.ExpectedURL, "")
		if !placementOK || placement != plugin.SearchResultPlacement || !urlOK || request.ExpectedURL == nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请同时提供搜索入口和已确认的插件目标"})
			return
		}
		if placement != action.Placement || expectedURL != info.URL {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "插件目标或操作已变更，请重新选择"})
			return
		}
	}
	if request.Page != nil && action.Result != "tasks" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "此操作不接受页码"})
		return
	}
	page, pageOK := panelRequestField(request.Page, 1)
	input, inputOK := panelRequestField(request.Input, "")
	input = strings.TrimSpace(input)
	if !pageOK || !inputOK || action.ValidateInput(input, page) != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "操作输入或页码无效"})
		return
	}
	runner, ready := plugin.LocalPanelRunner(scope)
	if !ready {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "插件操作尚未就绪"})
		return
	}
	ctx, cancel := panelRequestContext(r, scope)
	defer cancel()
	var token string
	if info.Panel.Credential != nil {
		credential, err := s.pluginManager.AccessPanelCredential(ctx, scope, nil)
		if err != nil {
			writePanelStorageError(w, err)
			return
		}
		token = credential.Token
		if token == "" && info.Panel.Credential.Required {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "请先配置插件凭据"})
			return
		}
	}
	if ctx.Err() != nil || scope.Context().Err() != nil {
		writePanelStorageError(w, context.Canceled)
		return
	}
	result, err := runner.Run(ctx, action, token, input, page)
	if err == nil {
		err = result.Validate(action.Result, token)
	}
	if ctx.Err() != nil || scope.Context().Err() != nil {
		writePanelStorageError(w, context.Canceled)
		return
	}
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.Is(err, plugin.ErrPanelInput):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "插件操作参数无效"})
	case errors.Is(err, plugin.ErrPanelUnauthorized):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "插件授权无效或已过期"})
	case errors.Is(err, plugin.ErrPanelRateLimited):
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "插件请求频率受限"})
	default:
		// 上游错误可能包含请求目标或凭据，只返回可以公开的固定描述。
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "插件服务未确认操作成功"})
	}
}
