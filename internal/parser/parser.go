// Package parser 实现自然语言任务解析与种子标题消歧
package parser

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// CompositeParser 组合式任务与标题解析器（微内核支持流水线拦截器挂载与可选 AI 回退）
type CompositeParser struct {
	regexParser  *RegexParser
	aiParser     *AIParser
	aiFallback   bool
	interceptors []core.ParseInterceptor
	mu           sync.RWMutex
}

var _ core.TaskParser = (*CompositeParser)(nil)

// NewCompositeParser 创建组合式解析器
func NewCompositeParser(aiClient aiChatClient) *CompositeParser {
	cp := &CompositeParser{
		regexParser:  NewRegexParser(),
		aiFallback:   aiClient != nil,
		interceptors: make([]core.ParseInterceptor, 0),
	}
	if aiClient != nil {
		cp.aiParser = NewAIParser(aiClient)
	}
	return cp
}

// AddInterceptor 为解析流水线注册自定义拦截器（支持语义增强、插件规则消歧）
func (p *CompositeParser) AddInterceptor(interceptor core.ParseInterceptor) {
	if p == nil || interceptor == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.interceptors = append(p.interceptors, interceptor)
}

func (p *CompositeParser) Name() string { return "composite" }

// Parse 解析输入文本：按「本地高速正则 ──► 流水线拦截器 ──► 可选 AI 语义消歧」三级流水线处理
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

	// 1. 本地正则高速解析（微内核基准：零网络、零额外开销）
	result, err := rp.Parse(ctx, input)
	if err != nil {
		return result, err
	}

	// 2. 流水线拦截器链（允许插件同步介入、修正或增强解析结果）
	p.mu.RLock()
	hasInterceptors := len(p.interceptors) > 0
	var interceptors []core.ParseInterceptor
	if hasInterceptors {
		interceptors = make([]core.ParseInterceptor, len(p.interceptors))
		copy(interceptors, p.interceptors)
	}
	p.mu.RUnlock()

	if hasInterceptors {
		for _, interceptor := range interceptors {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			var nextResult core.ParseResult
			var incErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("⚠️ 解析拦截器 [%s] 发生 panic 恢复: %v", interceptor.Name(), r)
						nextResult = result
					}
				}()
				nextResult, incErr = interceptor.InterceptParse(ctx, input, result)
			}()

			if incErr == nil && (nextResult.Confidence > result.Confidence || (result.Title == "" && nextResult.Title != "")) {
				result = nextResult
			}
		}
	}

	// 3. 可选大模型 AI 语义消歧回退（仅在置信度仍不足时启用）
	if result.Title == "" || result.Confidence < 0.4 {
		if p.aiFallback && p.aiParser != nil && ctx.Err() == nil {
			log.Printf("🤖 置信度 %.2f < 0.4，启用 AI 回退解析: %q", result.Confidence, truncateRunes(input, 120))
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
