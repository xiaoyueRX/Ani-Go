package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/cloud115"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestCloud115AdapterTranslatesProviderErrors(t *testing.T) {
	for _, tc := range []struct{ provider, panel error }{
		{cloud115.ErrInvalidMagnet, ErrPanelInput},
		{cloud115.ErrUnauthorized, ErrPanelUnauthorized},
		{cloud115.ErrRateLimited, ErrPanelRateLimited},
		{context.Canceled, context.Canceled},
	} {
		if err := cloud115PanelError(tc.provider); !errors.Is(err, tc.panel) {
			t.Fatalf("错误映射为 %v，预期 %v", err, tc.panel)
		}
	}
}

type nativeCredentialFixture struct{ provenanceBuiltin }

func (p nativeCredentialFixture) Init(_ core.EventBus, ctx core.Context) error {
	ctx.SetService(PanelCredentialService, PanelCredentialStorage{EnvironmentKey: "ANIGO_TEST_NATIVE_CREDENTIAL"})
	return nil
}

func TestAdditionalNativePluginCanSelectOwnCredential(t *testing.T) {
	setupTestDB(t)
	const id = "native-file-index"
	t.Setenv("ANIGO_TEST_NATIVE_CREDENTIAL", "native-fixture-value")
	m := NewManager(newTestBus())
	t.Cleanup(m.StopAll)
	info := PluginInfo{ID: id, Name: "文件索引", Panel: &Panel{Credential: &PanelCredential{Label: "索引令牌"}}}
	m.builtInPlugins = append(m.builtInPlugins, nativeCredentialFixture{provenanceBuiltin{info}})
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	manifest, scope, found := m.PanelSnapshot(id)
	if !found || !manifest.IsBuiltIn || scope == nil {
		t.Fatal("第二个本地插件没有激活")
	}
	value, err := m.AccessPanelCredential(t.Context(), scope, nil)
	if err != nil || value.Source != "environment" || value.Token != "native-fixture-value" {
		t.Fatal("已注册的本地实现没有获得自己的凭据", err)
	}
	empty := ""
	if _, err := m.AccessPanelCredential(t.Context(), scope, &empty); !errors.Is(err, ErrPanelCredentialReadOnly) {
		t.Fatal("只读来源允许删除", err)
	}
}
