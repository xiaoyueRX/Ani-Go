package app

import (
	"context"
	"errors"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type testChat struct{ ai.Classifier }

func (testChat) IsAvailable(ctx context.Context) bool { return ctx.Err() == nil }
func (testChat) Chat(ctx context.Context, system, prompt string) (string, error) {
	if system != "" {
		return "", errors.New("unexpected system prompt")
	}
	return prompt, ctx.Err()
}

func TestPluginAIUsesSharedChatAndCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	services := core.NewContext(ctx)
	services.SetService(core.ServiceAI, PluginAI(testChat{}, "ollama"))
	classifier := services.GetAIClassifier()
	if classifier.Name() != "ollama" || !classifier.IsAvailable(ctx) {
		t.Fatal("插件未获得已配置的 AI 服务")
	}
	if result, err := classifier.Classify(ctx, "番剧分类"); result != "番剧分类" || err != nil {
		t.Fatalf("处理插件提示词: %q, %v", result, err)
	}
	cancel()
	if _, err := classifier.Classify(ctx, ""); !errors.Is(err, context.Canceled) || classifier.IsAvailable(ctx) {
		t.Fatal("插件取消后仍接受请求")
	}
}
