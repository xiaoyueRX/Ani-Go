package v2

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"github.com/xiaoyueRX/Ani-Go/internal/event"
)

func newTestMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&database.Setting{}, &database.NotificationLog{}); err != nil {
		t.Fatalf("迁移数据库表失败: %v", err)
	}
	return db
}

type recordNotifier struct {
	name     string
	mu       sync.Mutex
	received []struct{ Title, Content string }
}

func newRecordNotifier(name string) *recordNotifier {
	return &recordNotifier{name: name}
}

func (r *recordNotifier) Name() string { return r.name }

func (r *recordNotifier) Send(ctx context.Context, title, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.received = append(r.received, struct{ Title, Content string }{Title: title, Content: message})
	return nil
}

func (r *recordNotifier) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.received)
}

func (r *recordNotifier) last() (string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.received) == 0 {
		return "", ""
	}
	l := r.received[len(r.received)-1]
	return l.Title, l.Content
}

// ------------------------------------------------------------
// 既有测试保留
// ------------------------------------------------------------

func TestOrganizedNotification_SeparatesSuccessAndFailure(t *testing.T) {
	msg := organizedNotification(map[string]interface{}{
		"success":    2,
		"failed":     1,
		"final_path": "/media/anime/a.mkv",
	})

	if msg.Title != "📁 文件整理结果" {
		t.Fatalf("标题 = %q", msg.Title)
	}
	if !strings.Contains(msg.Content, "成功: 2 个") || !strings.Contains(msg.Content, "失败: 1 个") {
		t.Fatalf("内容 = %q", msg.Content)
	}
}

func TestOrganizedNotification_AllFailuresUseAlert(t *testing.T) {
	msg := organizedNotification(map[string]interface{}{"success": 0, "failed": 3})

	if msg.Title != "🚨 文件整理失败" || strings.Contains(msg.Content, "成功提示") {
		t.Fatalf("通知 = %+v", msg)
	}
	if msg.Priority != 2 {
		t.Fatalf("优先级 = %d, 期望 2", msg.Priority)
	}
}

func TestOrganizedNotification_ZeroCountsSuppressed(t *testing.T) {
	msg := organizedNotification(map[string]interface{}{"success": 0, "failed": 0})
	if msg != nil {
		t.Fatalf("当 success=0 && failed=0 时应静默返回 nil，实际得到: %+v", msg)
	}
}

// ------------------------------------------------------------
// TemplateManager 核心功能测试
// ------------------------------------------------------------

func TestTemplateManager_SafeParsingAndFunctions(t *testing.T) {
	tm := NewTemplateManager()

	// 1. 安全校验语法错误，禁止 panic
	if err := tm.Add("bad.template", "{{.Title"); err == nil {
		t.Fatalf("未闭合标签应当返回错误")
	}
	if err := tm.ValidateTemplate("{{.Title | unknownFunc}}"); err == nil {
		t.Fatalf("未知函数应当校验失败")
	}

	// 2. 内置安全函数：default / upper / lower / truncate
	tmplStr := `标题: {{.Title | default "缺省标题" | upper}}
截断: {{truncate .AnimeTitle 6}}
小写: {{.Channel | lower}}`

	err := tm.Add("func.test", tmplStr)
	if err != nil {
		t.Fatalf("注册模板失败: %v", err)
	}

	res, err := tm.Render("func.test", map[string]any{
		"Title":      "",
		"AnimeTitle": "葬送的芙莉莲剧场版",
		"Channel":    "TELEGRAM",
	})
	if err != nil {
		t.Fatalf("渲染模板失败: %v", err)
	}

	if !strings.Contains(res, "标题: 缺省标题") {
		t.Errorf("default+upper 失败: %s", res)
	}
	if !strings.Contains(res, "截断: 葬送的...") {
		t.Errorf("truncate 失败: %s", res)
	}
	if !strings.Contains(res, "小写: telegram") {
		t.Errorf("lower 失败: %s", res)
	}
}

func TestTemplateManager_InheritanceAndFallback(t *testing.T) {
	tm := NewTemplateManager()

	// 1. 无任何自定义时，回退到内置默认模板
	res1, err := tm.RenderForChannel(core.EventDownloadCompleted, "Telegram", map[string]any{
		"Name":     "海贼王 - 1100",
		"Size":     "1.2 GB",
		"SavePath": "/data/anime",
	})
	if err != nil {
		t.Fatalf("内置默认模板渲染失败: %v", err)
	}
	if !strings.Contains(res1, "下载完成：海贼王 - 1100") || !strings.Contains(res1, "1.2 GB") {
		t.Errorf("默认模板未正确渲染: %s", res1)
	}

	// 2. 设置事件级默认模板 (eventType, "")
	err = tm.SetTemplate(core.EventDownloadCompleted, "", "【通用】下载完结：{{.Name}}")
	if err != nil {
		t.Fatalf("设置事件级默认模板失败: %v", err)
	}

	// 此时任意渠道未单独定制时回退到事件级自定义模板
	res2, err := tm.RenderForChannel(core.EventDownloadCompleted, "Telegram", map[string]any{"Name": "进击的巨人"})
	if err != nil {
		t.Fatalf("渲染事件级自定义模板失败: %v", err)
	}
	if !strings.Contains(res2, "【通用】下载完结：进击的巨人") {
		t.Errorf("未回退到事件级自定义模板: %s", res2)
	}

	// 3. 设置特定渠道模板 (eventType, channel)
	err = tm.SetTemplate(core.EventDownloadCompleted, "WeChat", "【微信公众号专用】{{.Name}}")
	if err != nil {
		t.Fatalf("设置渠道定制模板失败: %v", err)
	}

	// 微信渠道命中专属模板
	resWeChat, err := tm.RenderForChannel(core.EventDownloadCompleted, "WeChat", map[string]any{"Name": "进击的巨人"})
	if err != nil || !strings.Contains(resWeChat, "【微信公众号专用】进击的巨人") {
		t.Errorf("微信专属模板未命中: res=%s, err=%v", resWeChat, err)
	}

	// 其他渠道依然走事件级默认模板
	resDiscord, err := tm.RenderForChannel(core.EventDownloadCompleted, "Discord", map[string]any{"Name": "进击的巨人"})
	if err != nil || !strings.Contains(resDiscord, "【通用】下载完结：进击的巨人") {
		t.Errorf("其他渠道未回退事件级默认模板: res=%s, err=%v", resDiscord, err)
	}

	// 4. 删除渠道定制模板后回退事件级默认模板
	_ = tm.DeleteTemplate(core.EventDownloadCompleted, "WeChat")
	resWeChatAfter, _ := tm.RenderForChannel(core.EventDownloadCompleted, "WeChat", map[string]any{"Name": "进击的巨人"})
	if !strings.Contains(resWeChatAfter, "【通用】下载完结：进击的巨人") {
		t.Errorf("删除定制后应当回退事件级默认模板: %s", resWeChatAfter)
	}

	// 5. 删除事件级默认模板后回退内置系统默认模板
	_ = tm.DeleteTemplate(core.EventDownloadCompleted, "")
	resBuiltin, _ := tm.RenderForChannel(core.EventDownloadCompleted, "WeChat", map[string]any{"Name": "进击的巨人"})
	if !strings.Contains(resBuiltin, "下载完成：进击的巨人") {
		t.Errorf("删除事件级默认模板后应当回退内置系统模板: %s", resBuiltin)
	}
}

func TestTemplateManager_CRUDAndCatalog(t *testing.T) {
	tm := NewTemplateManager()

	_ = tm.SetTemplate("test.event", "default", "通用: {{.Title}}")
	_ = tm.SetTemplate("test.event", "DingTalk", "钉钉: {{.Title}}")

	catalog := tm.ListTemplates()
	if catalog.Defaults[core.EventDownloadCompleted] == "" {
		t.Fatalf("Catalog 中应包含内置系统默认模板")
	}

	customMap := catalog.Custom["test.event"]
	if customMap == nil || customMap["default"] != "通用: {{.Title}}" || customMap["DingTalk"] != "钉钉: {{.Title}}" {
		t.Fatalf("Catalog custom 结构不正确: %+v", customMap)
	}

	// 验证 Items 扁平列表
	var foundDingTalk bool
	for _, it := range catalog.Items {
		if it.EventType == "test.event" && it.Channel == "DingTalk" && it.IsCustom {
			foundDingTalk = true
			break
		}
	}
	if !foundDingTalk {
		t.Fatalf("Catalog.Items 未包含定制条目")
	}
}

func TestTemplateManager_PreviewTemplate(t *testing.T) {
	tm := NewTemplateManager()

	// 预览临时模板文本
	preview, err := tm.PreviewTemplate("test.event", "Telegram", "预览测试: {{.AnimeTitle}} 第 {{.Episode}} 集", nil)
	if err != nil {
		t.Fatalf("PreviewTemplate 失败: %v", err)
	}
	if !strings.Contains(preview, "葬送的芙莉莲") || !strings.Contains(preview, "第 1 集") {
		t.Errorf("示例数据未自动注入: %s", preview)
	}

	// 预览当前生效内置模板
	previewBuiltin, err := tm.PreviewTemplate(core.EventDownloadCompleted, "WeChat", "", nil)
	if err != nil {
		t.Fatalf("PreviewTemplate 内置模板失败: %v", err)
	}
	if !strings.Contains(previewBuiltin, "下载完成") {
		t.Errorf("内置默认模板未正确预览: %s", previewBuiltin)
	}
}

func TestTemplateManager_DBPersistenceAndReload(t *testing.T) {
	db := newTestMemoryDB(t)
	tm1 := NewTemplateManagerWithDB(db)

	err := tm1.SetTemplate(core.EventDownloadStarted, "Telegram", "【电报下载】{{.Name}}")
	if err != nil {
		t.Fatalf("SetTemplate 失败: %v", err)
	}

	// 验证数据库写回
	var s database.Setting
	if err := db.Where("key = ?", NotifyTemplatesSettingKey).First(&s).Error; err != nil {
		t.Fatalf("未在数据库中找到模板配置记录: %v", err)
	}
	if !strings.Contains(s.Value, "【电报下载】") {
		t.Fatalf("数据库中的值未包含定制模板: %s", s.Value)
	}

	// 新建实例从 DB 读取
	tm2 := NewTemplateManagerWithDB(db)
	res, err := tm2.RenderForChannel(core.EventDownloadStarted, "Telegram", map[string]any{"Name": "间谍过家家"})
	if err != nil || !strings.Contains(res, "【电报下载】间谍过家家") {
		t.Fatalf("从数据库加载的模板未正确渲染: res=%s, err=%v", res, err)
	}
}

func TestTemplateManager_Concurrency(t *testing.T) {
	tm := NewTemplateManager()
	var wg sync.WaitGroup

	// 并发写
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch := fmt.Sprintf("ch_%d", idx%3)
			_ = tm.SetTemplate("concurrent.event", ch, fmt.Sprintf("模板-%d-{{.Name}}", idx))
		}(i)
	}

	// 并发读
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch := fmt.Sprintf("ch_%d", idx%3)
			_, _ = tm.RenderForChannel("concurrent.event", ch, map[string]any{"Name": "并发番剧"})
			_ = tm.ListTemplates()
		}(i)
	}

	wg.Wait()
}

// ------------------------------------------------------------
// NotifyManager 模板分发与按渠道渲染集成测试
// ------------------------------------------------------------

func TestNotifyManager_StructuredDataAndPerChannelDispatch(t *testing.T) {
	bus := event.New()
	tgNotifier := newRecordNotifier("Telegram")
	wechatNotifier := newRecordNotifier("WeChat")

	mgr := NewNotifyManager([]Notifier{tgNotifier, wechatNotifier}, bus)
	mgr.Start()
	defer mgr.Stop()

	// 定制 Telegram 模板与微信模板
	tm := mgr.GetTemplateManager()
	_ = tm.SetTemplate(core.EventDownloadCompleted, "Telegram", "TG专供: {{.AnimeTitle}} (大小: {{.Size}})")
	_ = tm.SetTemplate(core.EventDownloadCompleted, "WeChat", "微信专供: {{.AnimeTitle}} 第 {{.Episode}} 集")

	// 模拟发布核心业务下载完成事件
	bus.Publish(core.Event{
		Type: core.EventDownloadCompleted,
		Payload: map[string]any{
			"anime_title": "鬼灭之刃",
			"episode":     5,
			"size":        int64(104857600), // 100MB
			"duration":    "30s",
		},
		Time: time.Now(),
	})

	// 等待异步处理
	time.Sleep(300 * time.Millisecond)

	if tgNotifier.count() == 0 || wechatNotifier.count() == 0 {
		t.Fatalf("两个渠道均应收到通知: tg=%d, wechat=%d", tgNotifier.count(), wechatNotifier.count())
	}

	_, tgContent := tgNotifier.last()
	if !strings.Contains(tgContent, "TG专供: 鬼灭之刃 (大小: 100.0 MB)") {
		t.Errorf("Telegram 渠道未按定制模板渲染: %s", tgContent)
	}

	_, wechatContent := wechatNotifier.last()
	if !strings.Contains(wechatContent, "微信专供: 鬼灭之刃 第 5 集") {
		t.Errorf("WeChat 渠道未按定制模板渲染: %s", wechatContent)
	}
}

func TestNotifyManager_CustomTemplateFailureFallback(t *testing.T) {
	bus := event.New()
	mockN := newRecordNotifier("Telegram")

	mgr := NewNotifyManager([]Notifier{mockN}, bus)
	mgr.Start()
	defer mgr.Stop()

	// 注入一个引发除以零或未知字段导致渲染失败的模板
	tm := mgr.GetTemplateManager()
	// 手动注册一个执行阶段必定报错的模板
	_ = tm.SetTemplate("download.started", "Telegram", `{{index . "missing" "deep"}}`)

	// 派发下载开始事件
	bus.Publish(core.Event{
		Type: core.EventDownloadStarted,
		Payload: map[string]any{
			"title": "测试番剧-01",
		},
		Time: time.Now(),
	})

	time.Sleep(300 * time.Millisecond)

	if mockN.count() == 0 {
		t.Fatalf("即使自定义模板执行失败，也应安全降级发送通知")
	}
	_, content := mockN.last()
	if !strings.Contains(content, "测试番剧-01") {
		t.Errorf("未安全降级到默认内容: %s", content)
	}
}

func TestNotifyManager_GetAndReloadTemplateManager(t *testing.T) {
	db := newTestMemoryDB(t)
	oldDB := database.DB
	database.DB = db
	defer func() { database.DB = oldDB }()

	mgr := NewNotifyManager(nil, nil)
	tm := mgr.GetTemplateManager()
	if tm == nil {
		t.Fatalf("GetTemplateManager 不应返回 nil")
	}

	// 通过 DB 写入配置后重载
	tm.SetDB(db)
	_ = tm.SetTemplate("custom.ev", "", "自定义模板内容")

	err := mgr.ReloadTemplates()
	if err != nil {
		t.Fatalf("ReloadTemplates 失败: %v", err)
	}
	if !mgr.GetTemplateManager().HasCustomTemplate("custom.ev", "") {
		t.Fatalf("重载后应包含 custom.ev 模板")
	}
}
