package parser

import (
	"context"
	"log"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type CompositeParser struct {
	regexParser *RegexParser
	aiParser    *AIParser
	aiFallback  bool
}

func NewCompositeParser(aiClient aiChatClient) *CompositeParser {
	cp := &CompositeParser{
		regexParser: NewRegexParser(),
		aiFallback:  aiClient != nil,
	}
	if aiClient != nil {
		cp.aiParser = NewAIParser(aiClient)
	}
	return cp
}

func (p *CompositeParser) Name() string { return "composite" }

func (p *CompositeParser) Parse(ctx context.Context, input string) (core.ParseResult, error) {
	result, err := p.regexParser.Parse(ctx, input)
	if err != nil {
		return result, err
	}

	if result.Title == "" || result.Confidence < 0.4 {
		if p.aiFallback && p.aiParser != nil {
			log.Printf("🤖 正则置信度 %.2f < 0.4，启用 AI 回退解析: %q", result.Confidence, input)
			aiResult, aiErr := p.aiParser.Parse(ctx, input)
			if aiErr == nil && aiResult.Confidence > result.Confidence {
				return aiResult, nil
			}
			if aiErr != nil {
				log.Printf("⚠️ AI 解析失败: %v", aiErr)
			}
		}
	}

	return result, nil
}
