package parser

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockAIChatClient struct {
	chatFunc        func(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	isAvailableFunc func(ctx context.Context) bool
}

func (m *mockAIChatClient) Chat(ctx context.Context, sys, usr string) (string, error) {
	if m.chatFunc != nil {
		return m.chatFunc(ctx, sys, usr)
	}
	return "", errors.New("not implemented")
}

func (m *mockAIChatClient) IsAvailable(ctx context.Context) bool {
	if m.isAvailableFunc != nil {
		return m.isAvailableFunc(ctx)
	}
	return true
}

// TestAIParser_NormalParse 测试标准 AI 格式化输出与字段清洗
func TestAIParser_NormalParse(t *testing.T) {
	client := &mockAIChatClient{
		chatFunc: func(ctx context.Context, sys, usr string) (string, error) {
			return `{"action":"subscribe","title":"葬送的芙莉莲","season":1,"resolution":"1080p","subgroup_pref":"澄空","keywords":["魔法"],"confidence":0.95}`, nil
		},
	}

	p := NewAIParser(client)
	res, err := p.Parse(context.Background(), "请帮我追一下澄空学园的芙莉莲 1080p")
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}

	if res.Title != "葬送的芙莉莲" || res.Action != "subscribe" || res.Resolution != "1080p" || res.SubgroupPref != "澄空" {
		t.Errorf("解析字段不符合预期: %+v", res)
	}
	if res.Confidence < 0.9 {
		t.Errorf("置信度期望 >= 0.9, 实际: %f", res.Confidence)
	}
}

// TestAIParser_MalformedJSON 测试大模型输出非标准 JSON（如 Markdown 代码块与杂项前缀）
func TestAIParser_MalformedJSON(t *testing.T) {
	client := &mockAIChatClient{
		chatFunc: func(ctx context.Context, sys, usr string) (string, error) {
			return "```json\n{\"action\":\"unsubscribe\",\"title\":\"进击的巨人\",\"season\":2,\"resolution\":\"\",\"subgroup_pref\":\"\",\"keywords\":[],\"confidence\":0.88}\n```", nil
		},
	}

	p := NewAIParser(client)
	res, err := p.Parse(context.Background(), "退订 进击的巨人第二季")
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}

	if res.Action != "unsubscribe" || res.Title != "进击的巨人" || res.Season != 2 {
		t.Errorf("Markdown 包裹的 JSON 解析失败: %+v", res)
	}
}

// TestAIParser_InvalidFieldsNormalization 测试大模型输出极端非法季号、超界浮点与空标题
func TestAIParser_InvalidFieldsNormalization(t *testing.T) {
	client := &mockAIChatClient{
		chatFunc: func(ctx context.Context, sys, usr string) (string, error) {
			// 输出负数季号与越界置信度
			return `{"action":"UNKNOWN_ACTION","title":"  ","season":-5,"resolution":"4K","subgroup_pref":"","keywords":[],"confidence":15.0}`, nil
		},
	}

	p := NewAIParser(client)
	res, err := p.Parse(context.Background(), "未知指令")
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}

	// 负数季号应被归一化为 1，空标题置信度降为 0.1
	if res.Season != 1 {
		t.Errorf("Season 应被归一化为 1, 实际: %d", res.Season)
	}
	if res.Confidence > 0.5 {
		t.Errorf("空标题置信度不应过高, 实际: %f", res.Confidence)
	}
	if res.Title != "未知指令" {
		t.Errorf("空标题应回退为原始安全输入, 实际: %q", res.Title)
	}
}

// TestAIParser_DailyQuotaGuard 测试每日限额配额耗尽与零点自动重置
func TestAIParser_DailyQuotaGuard(t *testing.T) {
	callCount := 0
	client := &mockAIChatClient{
		chatFunc: func(ctx context.Context, sys, usr string) (string, error) {
			callCount++
			return `{"action":"subscribe","title":"测试番剧","season":1,"confidence":0.9}`, nil
		},
	}

	guard := NewDailyQuotaGuard(3) // 限额 3 次
	p := NewAIParserWithQuota(client, guard)

	// 前 3 次调用应成功
	for i := 1; i <= 3; i++ {
		_, err := p.Parse(context.Background(), "追番 测试")
		if err != nil {
			t.Fatalf("第 %d 次调用失败: %v", i, err)
		}
	}
	if callCount != 3 {
		t.Fatalf("期望已调用 3 次，实际为 %d", callCount)
	}

	// 第 4 次调用应触发配额熔断
	_, err := p.Parse(context.Background(), "追番 测试")
	if err == nil {
		t.Fatalf("第 4 次调用期望配额超额错误，实际返回 nil")
	}
	if !errors.Is(err, ErrAIQuotaExceeded) {
		t.Errorf("期望错误包含 ErrAIQuotaExceeded, 实际: %v", err)
	}
	if callCount != 3 {
		t.Errorf("配额耗尽后不应再发起网络调用，实际调用次数为: %d", callCount)
	}

	// 模拟跨天重置
	guard.mu.Lock()
	guard.currentDate = time.Now().Add(-24 * time.Hour).Format("2006-01-02")
	guard.mu.Unlock()

	// 跨天后首次调用应成功自增
	_, err = p.Parse(context.Background(), "追番 测试")
	if err != nil {
		t.Fatalf("跨天重置后调用失败: %v", err)
	}
	if callCount != 4 {
		t.Errorf("跨天后期望成功调用第 4 次，实际调用次数: %d", callCount)
	}
}

// TestAIParser_PromptBounds 测试超长输入被安全截断
func TestAIParser_PromptBounds(t *testing.T) {
	var capturedUserPrompt string
	client := &mockAIChatClient{
		chatFunc: func(ctx context.Context, sys, usr string) (string, error) {
			capturedUserPrompt = usr
			return `{"action":"subscribe","title":"长标题番剧","season":1,"confidence":0.8}`, nil
		},
	}

	p := NewAIParser(client)
	hugeInput := "追番 " + strings.Repeat("超长标题字符", 1000)

	_, err := p.Parse(context.Background(), hugeInput)
	if err != nil {
		t.Fatalf("超长输入解析失败: %v", err)
	}

	// 确认送入大模型的 prompt 没有膨胀到数十KB（已从 18KB 截断至安全范围）
	if len(capturedUserPrompt) > 3000 {
		t.Errorf("送入大模型的 Prompt 未被有效截断，长度为 %d 字节", len(capturedUserPrompt))
	}
}

// TestDailyQuotaGuard_Concurrency 测试高并发下 CheckAndIncrement 的竞态安全
func TestDailyQuotaGuard_Concurrency(t *testing.T) {
	const limit = 50
	const workers = 20
	const opsPerWorker = 10

	guard := NewDailyQuotaGuard(limit)
	var wg sync.WaitGroup
	var successCount int64
	var mu sync.Mutex

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				_, ok := guard.CheckAndIncrement()
				if ok {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	if successCount != limit {
		t.Errorf("并发自增成功次数期望为 %d, 实际为 %d", limit, successCount)
	}
	if guard.GetCurrentCount() != limit {
		t.Errorf("当前计数期望为 %d, 实际为 %d", limit, guard.GetCurrentCount())
	}
}
