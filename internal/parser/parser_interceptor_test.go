package parser

import (
	"context"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

type mockCustomInterceptor struct {
	name string
}

func (m *mockCustomInterceptor) Name() string { return m.name }
func (m *mockCustomInterceptor) InterceptParse(ctx context.Context, input string, current core.ParseResult) (core.ParseResult, error) {
	if input == "特殊暗号：我想追新番" {
		return core.ParseResult{
			Action:     "subscribe",
			Title:      "间谍过家家",
			Season:     2,
			Confidence: 0.95,
			RawInput:   input,
		}, nil
	}
	return current, nil
}

func TestCompositeParser_Interceptor(t *testing.T) {
	cp := NewCompositeParser(nil)
	cp.AddInterceptor(&mockCustomInterceptor{name: "secret-code-interceptor"})

	res, err := cp.Parse(context.Background(), "特殊暗号：我想追新番")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if res.Action != "subscribe" || res.Title != "间谍过家家" || res.Season != 2 || res.Confidence < 0.9 {
		t.Fatalf("拦截器增强结果不符合预期: %+v", res)
	}

	// 验证未匹配时正常回退到正则
	res2, err := cp.Parse(context.Background(), "追番 进击的巨人 第4季 1080p")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res2.Title != "进击的巨人" || res2.Season != 4 {
		t.Fatalf("常规输入应由正则解析: %+v", res2)
	}
}
