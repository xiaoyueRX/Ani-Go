package core

import (
	"context"
	"testing"
)

type mockOrganizeInterceptor struct {
	name   string
	mutate func(input OrganizeInput) OrganizeOutput
}

func (m *mockOrganizeInterceptor) Name() string { return m.name }
func (m *mockOrganizeInterceptor) InterceptOrganize(ctx context.Context, input OrganizeInput) (OrganizeOutput, error) {
	if m.mutate != nil {
		return m.mutate(input), nil
	}
	return OrganizeOutput{FinalPath: input.ProposedPath}, nil
}

type mockFilterInterceptor struct {
	name  string
	allow bool
}

func (m *mockFilterInterceptor) Name() string { return m.name }
func (m *mockFilterInterceptor) InterceptFilter(ctx context.Context, input FilterInput) (FilterOutput, error) {
	if !m.allow {
		return FilterOutput{Allow: false, Reason: "mock filter blocked"}, nil
	}
	return FilterOutput{Allow: true}, nil
}

type mockParseInterceptor struct {
	name   string
	action string
	title  string
}

func (m *mockParseInterceptor) Name() string { return m.name }
func (m *mockParseInterceptor) InterceptParse(ctx context.Context, rawInput string, current ParseResult) (ParseResult, error) {
	current.Action = m.action
	current.Title = m.title
	current.Confidence = 0.95
	return current, nil
}

func TestWaterfallHookManager_ZeroOverhead(t *testing.T) {
	mgr := NewWaterfallHookManager()

	// 零钩子零开销验证
	path, cancelled, reason := mgr.ExecuteNamingHooks(context.Background(), NamingHookInput{
		RenderedPath: "test/path.mkv",
	})
	if path != "test/path.mkv" || cancelled || reason != "" {
		t.Fatalf("零钩子状态下应原样返回，得到 path=%s cancelled=%v reason=%s", path, cancelled, reason)
	}

	allow, filterReason := mgr.ExecuteFilterHooks(context.Background(), FilterHookInput{})
	if !allow || filterReason != "" {
		t.Fatalf("零钩子状态下应默认放行，得到 allow=%v reason=%s", allow, filterReason)
	}

	res, err := mgr.ExecuteParseInterceptors(context.Background(), "raw", ParseResult{Title: "init"})
	if err != nil || res.Title != "init" {
		t.Fatalf("零钩子状态下应原样返回，得到 res=%+v err=%v", res, err)
	}
}

func TestWaterfallHookManager_OrganizeInterceptor(t *testing.T) {
	mgr := NewWaterfallHookManager()

	interceptor1 := &mockOrganizeInterceptor{
		name: "cleaner",
		mutate: func(input OrganizeInput) OrganizeOutput {
			return OrganizeOutput{FinalPath: input.ProposedPath + "_cleaned"}
		},
	}
	interceptor2 := &mockOrganizeInterceptor{
		name: "canceller",
		mutate: func(input OrganizeInput) OrganizeOutput {
			return OrganizeOutput{Cancel: true, Reason: "test reject"}
		},
	}

	mgr.RegisterOrganizeInterceptor(PriorityNormal, interceptor1)

	path, cancelled, _ := mgr.ExecuteNamingHooks(context.Background(), NamingHookInput{
		RenderedPath: "season1/ep01",
	})
	if cancelled || path != "season1/ep01_cleaned" {
		t.Fatalf("期望拦截器修改路径为 season1/ep01_cleaned, 实际: %s", path)
	}

	// 注册一票否决拦截器
	mgr.RegisterOrganizeInterceptor(PriorityHigh, interceptor2)
	_, cancelled, reason := mgr.ExecuteNamingHooks(context.Background(), NamingHookInput{
		RenderedPath: "season1/ep01",
	})
	if !cancelled || reason != "test reject" {
		t.Fatalf("期望一票否决取消整理, 实际 cancelled=%v reason=%s", cancelled, reason)
	}

	// 注销 canceller
	mgr.UnregisterHooksByName("canceller")
	path, cancelled, _ = mgr.ExecuteNamingHooks(context.Background(), NamingHookInput{
		RenderedPath: "season1/ep01",
	})
	if cancelled || path != "season1/ep01_cleaned" {
		t.Fatalf("注销后应恢复放行并只执行 cleaner, 实际: %s, cancelled=%v", path, cancelled)
	}
}

func TestWaterfallHookManager_FilterInterceptor(t *testing.T) {
	mgr := NewWaterfallHookManager()

	mgr.RegisterFilterInterceptor(PriorityNormal, &mockFilterInterceptor{name: "blocker", allow: false})

	allowed, reason := mgr.ExecuteFilterHooks(context.Background(), FilterHookInput{
		Item: TorrentItem{Title: "Ad Torrent"},
	})
	if allowed || reason != "mock filter blocked" {
		t.Fatalf("期望过滤拦截器阻止, 实际 allowed=%v, reason=%s", allowed, reason)
	}

	mgr.UnregisterHooksByName("blocker")
	allowed, _ = mgr.ExecuteFilterHooks(context.Background(), FilterHookInput{
		Item: TorrentItem{Title: "Good Torrent"},
	})
	if !allowed {
		t.Fatalf("注销 blocker 后应放行")
	}
}

func TestWaterfallHookManager_ParseInterceptor(t *testing.T) {
	mgr := NewWaterfallHookManager()

	mgr.RegisterParseInterceptor(PriorityHigh, &mockParseInterceptor{
		name:   "ai-semantic",
		action: "subscribe",
		title:  "葬送的芙莉莲",
	})

	res, err := mgr.ExecuteParseInterceptors(context.Background(), "我想追芙莉莲", ParseResult{
		Action:     "unknown",
		Title:      "",
		Confidence: 0.2,
	})
	if err != nil {
		t.Fatalf("执行解析拦截器失败: %v", err)
	}
	if res.Action != "subscribe" || res.Title != "葬送的芙莉莲" || res.Confidence < 0.9 {
		t.Fatalf("期望解析拦截器增强结果, 实际: %+v", res)
	}
}

func TestWaterfallHookManager_PanicRecovery(t *testing.T) {
	mgr := NewWaterfallHookManager()

	// 注册一个引发 panic 的命名钩子
	mgr.RegisterNamingHook(PriorityHighest, "panicking-hook", func(ctx context.Context, input interface{}) (interface{}, error) {
		panic("fatal hook error inside plugin")
	})

	// 正常后续拦截器
	mgr.RegisterOrganizeInterceptor(PriorityNormal, &mockOrganizeInterceptor{
		name: "safe-interceptor",
		mutate: func(input OrganizeInput) OrganizeOutput {
			return OrganizeOutput{FinalPath: input.ProposedPath + "_safe"}
		},
	})

	// 执行不应崩溃，且后续钩子正常工作
	path, cancelled, _ := mgr.ExecuteNamingHooks(context.Background(), NamingHookInput{
		RenderedPath: "target",
	})
	if cancelled || path != "target_safe" {
		t.Fatalf("Panic 应被安全捕获并不影响后续链条执行, path=%s cancelled=%v", path, cancelled)
	}
}
