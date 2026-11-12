package parser

import (
	"context"
	"fmt"
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
	if p == nil {
		return core.ParseResult{RawInput: input}, fmt.Errorf("CompositeParser 未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return core.ParseResult{RawInput: input}, err
	}

	rp := p.regexParser
	if rp == nil {
		rp = NewRegexParser()
	}

	result, err := rp.Parse(ctx, input)
	if err != nil {
		return result, err
	}

	if result.Title == "" || result.Confidence < 0.4 {
		if p.aiFallback && p.aiParser != nil && ctx.Err() == nil {
			log.Printf("🤖 正则置信度 %.2f < 0.4，启用 AI 回退解析: %q", result.Confidence, truncateRunes(input, 120))
			aiResult, aiErr := p.aiParser.Parse(ctx, input)
			if aiErr == nil && aiResult.Confidence > result.Confidence {
				// 仅当 AI 返回了有效标题（list 操作允许空标题）时才采用回退结果
				if aiResult.Title != "" || aiResult.Action == "list" {
					return aiResult, nil
				}
			}
			if aiErr != nil {
				log.Printf("⚠️ AI 解析失败: %v", aiErr)
			}
		}
	}

	return result, nil
}
