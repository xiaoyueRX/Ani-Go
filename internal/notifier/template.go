package notifier

import (
	"gorm.io/gorm"

	v2 "github.com/xiaoyueRX/Ani-Go/internal/notifier/v2"
)

// NotifyTemplatesSettingKey 保存在 database.Setting 表中的通知模板配置键名
const NotifyTemplatesSettingKey = v2.NotifyTemplatesSettingKey

// TemplateManager 复用 v2.TemplateManager，提供线程安全、无 panic 的多事件/多渠道模板管理能力
type TemplateManager = v2.TemplateManager

// TemplateContext 结构化通知模板上下文数据
type TemplateContext = v2.TemplateContext

// TemplateInfo 单个通知模板条目
type TemplateInfo = v2.TemplateInfo

// TemplateCatalog 通知模板全量目录
type TemplateCatalog = v2.TemplateCatalog

// NewTemplateManager 初始化并预装内置默认通知模板
func NewTemplateManager() *TemplateManager {
	return v2.NewTemplateManager()
}

// NewTemplateManagerWithDB 使用指定数据库初始化模板管理器并自动加载自定义模板
func NewTemplateManagerWithDB(db *gorm.DB) *TemplateManager {
	return v2.NewTemplateManagerWithDB(db)
}

// ValidateTemplate 校验 Go 模板语法是否合法
func ValidateTemplate(tmpl string) error {
	return v2.ValidateTemplate(tmpl)
}
