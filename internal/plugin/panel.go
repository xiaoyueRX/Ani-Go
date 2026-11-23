package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/magnet"
)

// PanelService 仅注册在所属插件的上下文中。
const PanelService core.ServiceType = "plugin-panel"

type Panel struct {
	Version    int              `json:"version,omitempty"`
	Credential *PanelCredential `json:"credential,omitempty"`
	Actions    []PanelAction    `json:"actions"`
}
type PanelCredential struct {
	Label    string `json:"label"`
	Required bool   `json:"required,omitempty"`
}

const SearchResultPlacement = "search_result"

type PanelAction struct {
	Placement string      `json:"placement,omitempty"`
	ID        string      `json:"id"`
	Label     string      `json:"label"`
	Input     *PanelInput `json:"input,omitempty"`
	Result    string      `json:"result"`
}
type PanelInput struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Format      string `json:"format,omitempty"`
}
type PanelResult struct {
	Message   string      `json:"message,omitempty"`
	Page      int         `json:"page,omitempty"`
	PageCount int         `json:"page_count,omitempty"`
	Tasks     []PanelTask `json:"tasks,omitempty"`
}

// PanelTask 是所有任务面板对前端公开的字段。
type PanelTask struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

func (r PanelResult) MarshalJSON() ([]byte, error) {
	if r.Tasks != nil || r.Page > 0 {
		if r.Tasks == nil {
			r.Tasks = []PanelTask{}
		}
		return json.Marshal(struct {
			Page      int         `json:"page"`
			PageCount int         `json:"page_count"`
			Tasks     []PanelTask `json:"tasks"`
		}{r.Page, r.PageCount, r.Tasks})
	}
	return json.Marshal(struct {
		Message string `json:"message"`
	}{r.Message})
}

type PanelRunner interface {
	Run(context.Context, PanelAction, string, string, int) (PanelResult, error)
}

var (
	panelKey             = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	ErrPanelInput        = errors.New("面板操作参数无效")
	ErrPanelUnauthorized = errors.New("面板凭据无效或缺少授权")
	ErrPanelRateLimited  = errors.New("面板请求过于频繁")
	errPanelResponse     = errors.New("插件服务未确认操作结果")
)

func strictPanelJSON(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var rest any
	if err := dec.Decode(&rest); err != io.EOF {
		return errors.New("无效的面板 JSON")
	}
	return nil
}

func (p *Panel) UnmarshalJSON(data []byte) error {
	if len(data) > 64<<10 {
		return errors.New("面板定义过大")
	}
	type schema Panel
	var decoded struct {
		schema
		Version json.RawMessage `json:"version"`
	}
	if err := strictPanelJSON(data, &decoded); err != nil {
		return err
	}
	if decoded.Version != nil {
		if string(decoded.Version) != "1" {
			return errors.New("不支持的插件面板版本，仅支持 version 1")
		}
		decoded.schema.Version = 1
	}
	*p = Panel(decoded.schema)
	return nil
}

func (p *Panel) clone() *Panel {
	if p == nil {
		return nil
	}
	out := &Panel{Version: p.Version, Actions: append([]PanelAction(nil), p.Actions...)}
	if p.Credential != nil {
		v := *p.Credential
		out.Credential = &v
	}
	for i, a := range out.Actions {
		if a.Input != nil {
			v := *a.Input
			out.Actions[i].Input = &v
		}
	}
	return out
}

func panelText(s string, maxRunes int) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= maxRunes && strings.IndexFunc(s, unicode.IsControl) < 0
}

func validateWebhookPanel(p PluginInfo, allowPrivate bool) error {
	if p.Panel == nil {
		return nil
	}
	if p.Panel.Version != 0 && p.Panel.Version != 1 {
		return errors.New("不支持的插件面板版本，仅支持 version 1")
	}
	u, err := validateExternalHTTPURL(p.URL, allowPrivate)
	if err != nil || p.Type != "webhook" || u.Scheme != "https" || u.Fragment != "" {
		return errors.New("社区面板需要有效的 HTTPS Webhook 目标")
	}
	if len(p.Secret) > 8192 || strings.IndexFunc(p.Secret, unicode.IsControl) >= 0 {
		return errors.New("Webhook 密钥无效")
	}
	if v := p.Panel.Credential; v != nil && !panelText(v.Label, 128) {
		return errors.New("面板凭据标签无效")
	}
	if len(p.Panel.Actions) == 0 || len(p.Panel.Actions) > 16 {
		return errors.New("面板需要 1 到 16 个操作")
	}
	ids := make(map[string]bool, len(p.Panel.Actions))
	for _, action := range p.Panel.Actions {
		if !panelKey.MatchString(action.ID) || ids[action.ID] || !panelText(action.Label, 128) || (action.Result != "message" && action.Result != "tasks") {
			return errors.New("面板操作定义无效或 ID 重复")
		}
		ids[action.ID] = true
		if action.Placement != "" && (action.Placement != SearchResultPlacement || action.Input == nil || action.Input.Format != "magnet" || action.Result != "message") {
			return errors.New("搜索结果操作需要磁力输入和消息结果")
		}
		if in := action.Input; in != nil && (!panelKey.MatchString(in.Key) || !panelText(in.Label, 128) || (in.Placeholder != "" && !panelText(in.Placeholder, 256)) || (in.Format != "" && in.Format != "magnet")) {
			return errors.New("面板输入定义无效")
		}
	}
	return nil
}

func (a PanelAction) ValidateInput(input string, page int) error {
	if page < 1 || page > 1000000 || (a.Input == nil && input != "") {
		return ErrPanelInput
	}
	if a.Input != nil {
		if len(input) > 8192 || strings.TrimSpace(input) == "" || !utf8.ValidString(input) {
			return ErrPanelInput
		}
		if a.Input.Format == "magnet" {
			if _, err := magnet.Parse(input); err != nil {
				return ErrPanelInput
			}
		}
	}
	return nil
}

// PanelSnapshot 一次加读锁，同时获取清单及其对应的运行时上下文。
func (m *Manager) PanelSnapshot(id string) (PluginInfo, PluginContext, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var info PluginInfo
	for _, p := range m.builtInPlugins {
		if v := p.GetInfo(); v.ID == id {
			info = v
			info.IsBuiltIn = true
			info.Origin = OriginCommunity
			if officialPluginIDs[id] {
				info.Origin = OriginOfficial
			}
			info.Enabled = !isDefaultDisabled(id)
			break
		}
	}
	if info.ID == "" {
		for _, p := range m.customPlugins {
			if p.ID == id {
				info = p
				break
			}
		}
	}
	if info.ID == "" || info.Panel == nil {
		return PluginInfo{}, nil, false
	}
	info = clonePluginInfo(info)
	info.Secret = ""
	info.Status = m.pluginStates[id]
	info.Enabled = m.enabledLocked(info.ID, info.Enabled)
	if m.restoring || info.Status != StateActive {
		return info, nil, true
	}
	return info, m.pluginContexts[id], true
}

// LocalPanelRunner 只查找当前插件的面板服务。
func LocalPanelRunner(scope PluginContext) (PanelRunner, bool) {
	return localPanelService[PanelRunner](scope, PanelService)
}

func localPanelService[T any](scope PluginContext, name core.ServiceType) (T, bool) {
	var zero T
	c, ok := scope.(*pluginContextImpl)
	if !ok || c == nil {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disposed || c.ctx.Err() != nil {
		return zero, false
	}
	svc, _ := c.coreCtx.GetService(name)
	service, ok := svc.(T)
	return service, ok
}

type webhookPanelRunner struct {
	plugin    PluginInfo
	client    *http.Client
	lifecycle context.Context
}

func newWebhookPanelRunner(p PluginInfo, allowPrivate bool) *webhookPanelRunner {
	client := newSSRFSafeHTTPClient(10*time.Second, allowPrivate)
	if transport, ok := client.Transport.(*http.Transport); ok {
		client.Transport = transport.Clone()
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &webhookPanelRunner{plugin: clonePluginInfo(p), client: client}
}

func registerWebhookPanel(scope PluginContext, p PluginInfo, allowPrivate bool) {
	if p.Panel == nil {
		return
	}
	r := newWebhookPanelRunner(p, allowPrivate)
	r.lifecycle = scope.Context()
	scope.SetService(PanelService, r)
	scope.OnDispose(r.client.CloseIdleConnections)
}

func (r *webhookPanelRunner) Run(ctx context.Context, selected PanelAction, credential, input string, page int) (PanelResult, error) {
	var action PanelAction
	for _, a := range r.plugin.Panel.Actions {
		if a.ID == selected.ID {
			action = a
			break
		}
	}
	if action.ID == "" || action.ValidateInput(input, page) != nil {
		return PanelResult{}, ErrPanelInput
	}
	declaration := r.plugin.Panel.Credential
	if (credential != "" && declaration == nil) || (declaration != nil && declaration.Required && credential == "") || len(credential) > 8192 || strings.IndexFunc(credential, func(c rune) bool { return c < 33 || c > 126 }) >= 0 {
		return PanelResult{}, ErrPanelUnauthorized
	}
	if r.lifecycle != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(r.lifecycle, cancel)
		defer stop()
		if err := r.lifecycle.Err(); err != nil {
			return PanelResult{}, err
		}
	}
	body, err := json.Marshal(struct {
		Action string `json:"action"`
		Input  string `json:"input"`
		Page   int    `json:"page"`
	}{action.ID, input, page})
	if err != nil {
		return PanelResult{}, ErrPanelInput
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.plugin.URL, bytes.NewReader(body))
	if err != nil {
		return PanelResult{}, errPanelResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	if r.plugin.Secret != "" {
		req.Header.Set("X-AniGo-Secret", r.plugin.Secret)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return PanelResult{}, ctx.Err()
		}
		return PanelResult{}, errPanelResponse
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return PanelResult{}, ErrPanelUnauthorized
	case http.StatusTooManyRequests:
		return PanelResult{}, ErrPanelRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PanelResult{}, errPanelResponse
	}
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) > limit {
		return PanelResult{}, errPanelResponse
	}
	return decodePanelResult(data, action.Result, credential, r.plugin.Secret)
}

// Validate 检查原生和 Webhook 执行器的响应是否符合对外约定。
func (r PanelResult) Validate(kind string, secrets ...string) error {
	publicText := func(text string, limit int) bool {
		if !panelText(text, limit) {
			return false
		}
		for _, secret := range secrets {
			if secret != "" && strings.Contains(text, secret) {
				return false
			}
		}
		return true
	}
	if kind == "message" {
		if !publicText(r.Message, 2048) || r.Page != 0 || r.PageCount != 0 || r.Tasks != nil {
			return errPanelResponse
		}
		return nil
	}
	if kind != "tasks" || r.Message != "" || r.Page < 1 || r.Page > 1000000 || r.PageCount < 0 || r.PageCount > 1000000 || len(r.Tasks) > 200 {
		return errPanelResponse
	}
	for _, task := range r.Tasks {
		if !publicText(task.InfoHash, 256) || !publicText(task.Name, 512) || task.Progress < 0 || task.Progress > 100 {
			return errPanelResponse
		}
		switch task.Status {
		case "pending", "downloading", "completed", "failed", "unknown":
		default:
			return errPanelResponse
		}
	}
	return nil
}

func decodePanelResult(data []byte, kind, credential, secret string) (PanelResult, error) {
	if kind == "message" {
		var v struct {
			Message string `json:"message"`
		}
		if strictPanelJSON(data, &v) != nil {
			return PanelResult{}, errPanelResponse
		}
		result := PanelResult{Message: v.Message}
		if err := result.Validate(kind, credential, secret); err != nil {
			return PanelResult{}, err
		}
		return result, nil
	}
	var v struct {
		Page      int  `json:"page"`
		PageCount *int `json:"page_count"`
		Tasks     []struct {
			InfoHash string `json:"info_hash"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			Progress *int   `json:"progress"`
		} `json:"tasks"`
	}
	if strictPanelJSON(data, &v) != nil || v.PageCount == nil || v.Tasks == nil || len(v.Tasks) > 200 {
		return PanelResult{}, errPanelResponse
	}
	result := PanelResult{Page: v.Page, PageCount: *v.PageCount, Tasks: make([]PanelTask, 0, len(v.Tasks))}
	for _, t := range v.Tasks {
		if t.Progress == nil {
			return PanelResult{}, errPanelResponse
		}
		result.Tasks = append(result.Tasks, PanelTask{InfoHash: t.InfoHash, Name: t.Name, Status: t.Status, Progress: *t.Progress})
	}
	if err := result.Validate(kind, credential, secret); err != nil {
		return PanelResult{}, err
	}
	return result, nil
}
