package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/cloud115"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

const Cloud115ID = "cloud-115"
const Cloud115Service core.ServiceType = Cloud115ID
const Cloud115TokenKey = "CLOUD115_ACCESS_TOKEN"

type Cloud115Plugin struct{}

func (*Cloud115Plugin) GetInfo() PluginInfo {
	return PluginInfo{
		ID: Cloud115ID, Name: "115 磁力云下载", Description: "提交磁力链接到自己的 115 云盘，查看云端下载进度。",
		Version: "1.0.0", Author: "xiaoyueRX", AuthorURL: "https://github.com/xiaoyueRX",
		Icon: "CloudDownload", IsBuiltIn: true, Type: "builtin", Events: []string{},
		Panel: &Panel{
			Version:    1,
			Credential: &PanelCredential{Label: "settings.cloud115.accessToken", Required: true},
			Actions: []PanelAction{
				{ID: "submit", Placement: SearchResultPlacement, Label: "settings.cloud115.submit", Result: "message",
					Input: &PanelInput{Key: "magnet", Label: "settings.cloud115.magnet", Placeholder: "magnet:?xt=...", Format: "magnet"}},
				{ID: "tasks", Label: "settings.cloud115.tasks", Result: "tasks"},
			},
		},
	}
}

func (*Cloud115Plugin) Init(_ core.EventBus, ctx core.Context) error {
	pctx, ok := ctx.(PluginContext)
	if !ok {
		return fmt.Errorf("115 云下载缺少插件上下文")
	}
	client := cloud115.NewClient()
	pctx.OnDispose(client.CloseIdleConnections)
	pctx.RegisterService(Cloud115Service, client)
	pctx.SetService(PanelService, cloud115Panel{client: client})
	pctx.SetService(PanelCredentialService, PanelCredentialStorage{EnvironmentKey: Cloud115TokenKey, legacySettingKey: Cloud115TokenKey})
	return nil
}

// 插件管理器负责在停用时取消上下文并注销服务。插件没有常驻轮询。
func (*Cloud115Plugin) Stop(core.EventBus) error { return nil }

type cloud115Panel struct{ client *cloud115.Client }

func (p cloud115Panel) Run(ctx context.Context, action PanelAction, token, input string, page int) (PanelResult, error) {
	switch action.ID {
	case "submit":
		if err := p.client.Add(ctx, token, input); err != nil {
			return PanelResult{}, cloud115PanelError(err)
		}
		return PanelResult{Message: "settings.cloud115.submitted"}, nil
	case "tasks":
		tasks, err := p.client.List(ctx, token, page)
		if err != nil {
			return PanelResult{}, cloud115PanelError(err)
		}
		items := make([]PanelTask, len(tasks.Tasks))
		for i, task := range tasks.Tasks {
			items[i] = PanelTask(task)
			if strings.TrimSpace(items[i].Name) == "" {
				items[i].Name = task.InfoHash
			}
		}
		return PanelResult{Page: tasks.Page, PageCount: tasks.PageCount, Tasks: items}, nil
	default:
		return PanelResult{}, ErrPanelInput
	}
}

func cloud115PanelError(err error) error {
	switch {
	case errors.Is(err, cloud115.ErrInvalidMagnet):
		return ErrPanelInput
	case errors.Is(err, cloud115.ErrUnauthorized):
		return ErrPanelUnauthorized
	case errors.Is(err, cloud115.ErrRateLimited):
		return ErrPanelRateLimited
	default:
		return err
	}
}
