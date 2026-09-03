package notifier

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
)

// TemplateManager 管理通知模板
type TemplateManager struct {
	templates map[string]*template.Template
}

// NewTemplateManager 初始化并预装默认模板
func NewTemplateManager() *TemplateManager {
	tm := &TemplateManager{
		templates: make(map[string]*template.Template),
	}

	// 注册默认事件模板
	tm.registerDefaultTemplates()

	return tm
}

func (tm *TemplateManager) registerDefaultTemplates() {
	// 下载完成模板
	tm.Add(core.EventDownloadCompleted, "🎉 下载完成：{{.Name}}\n💾 大小：{{.Size}}\n📂 路径：{{.SavePath}}")
	
	// 下载失败模板
	tm.Add(core.EventDownloadFailed, "❌ 下载失败：{{.Name}}\n🚨 错误：{{.Error}}")
	
	// 文件整理完成
	tm.Add(core.EventFileOrganized, "📂 番剧归档完成：{{.AnimeTitle}}\n🎞️ 剧集：第 {{.Episode}} 集\n📍 新路径：{{.NewPath}}")
	
	// 新增订阅
	tm.Add(core.EventSubscriptionAdded, "➕ 新增订阅：{{.Title}}\n🔍 来源：{{.Source}}")
}

// Add 添加或更新模板
func (tm *TemplateManager) Add(id string, text string) {
	tmpl := template.Must(template.New(id).Parse(text))
	tm.templates[id] = tmpl
}

// Render 渲染指定模板
func (tm *TemplateManager) Render(templateID string, data interface{}) (string, error) {
	tmpl, ok := tm.templates[templateID]
	if !ok {
		return "", fmt.Errorf("未找到模板: %s", templateID)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("模板渲染错误: %w", err)
	}

	return buf.String(), nil
}
