package app

import (
	"context"

	"github.com/xiaoyueRX/Ani-Go/internal/ai"
	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// PluginAI 将插件的单段提示词交给与应用共用的 AI 客户端。
func PluginAI(client ai.Classifier, protocol string) core.AIClassifier {
	return pluginAI{client: client, protocol: protocol}
}

type pluginAI struct {
	client   ai.Classifier
	protocol string
}

func (p pluginAI) Name() string { return p.protocol }

func (p pluginAI) IsAvailable(ctx context.Context) bool { return p.client.IsAvailable(ctx) }

func (p pluginAI) Classify(ctx context.Context, prompt string) (string, error) {
	return p.client.Chat(ctx, "", prompt)
}
