package organizer

import (
	"context"
	"testing"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

func TestWaterfallHookEffect(t *testing.T) {
	o := New("", "", "", "", "", false, nil)
	mgr := o.GetHookManager()

	// 注册一个“极客命名”插件钩子
	mgr.RegisterNamingHook(core.PriorityHigh, "GeekPlugin", func(ctx context.Context, input interface{}) (interface{}, error) {
		in := input.(core.NamingHookInput)
		return core.NamingHookOutput{
			RenderedPath: "[Ani-Go] " + in.RenderedPath,
		}, nil
	})

	// 执行模拟整理
	finalPath, cancelled, _ := mgr.ExecuteNamingHooks(context.Background(), core.NamingHookInput{
		RenderedPath: "Mushoku Tensei S02E01.mp4",
	})

	if cancelled {
		t.Fatal("应该不被取消")
	}
	if finalPath != "[Ani-Go] Mushoku Tensei S02E01.mp4" {
		t.Errorf("钩子未生效，得到: %s", finalPath)
	}
}
