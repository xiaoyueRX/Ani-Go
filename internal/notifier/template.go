package notifier

import (
	"bytes"
	"text/template"
)

// TemplateManager manages notification message templates
type TemplateManager struct {
	templates map[string]*template.Template
}

// NewTemplateManager initializes with default templates
func NewTemplateManager() *TemplateManager {
	tm := &TemplateManager{
		templates: make(map[string]*template.Template),
	}

	// Default templates mapping core.Event types to standard notification text
	tm.templates["download.completed"] = template.Must(template.New("download.completed").Parse("🎉 下载完成：{{.Name}}\n💾 大小：{{.Size}}"))
	tm.templates["download.failed"] = template.Must(template.New("download.failed").Parse("❌ 下载失败：{{.Name}}\n🚨 错误：{{.Error}}"))
	tm.templates["subscription.added"] = template.Must(template.New("subscription.added").Parse("➕ 新增订阅：{{.Title}}"))

	return tm
}

// Render processes a template with event payload data
func (tm *TemplateManager) Render(eventType string, data interface{}) (string, error) {
	tmpl, exists := tm.templates[eventType]
	if !exists {
		// Fallback to empty string if no template defined for the event
		return "", nil
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
