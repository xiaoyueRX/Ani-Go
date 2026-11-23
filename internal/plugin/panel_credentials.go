package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/core"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const panelCredentialPrefix = "PLUGIN_PANEL_CREDENTIAL_"
const PanelCredentialService core.ServiceType = "plugin-panel-credential"

var ErrPanelInactive = errors.New("插件面板未处于活动状态")
var ErrPanelCredentialReadOnly = errors.New("凭据由环境变量配置，请在部署环境中修改")

// PanelCredentialStorage 由内置插件在自己的实例中登记。社区清单不能指定存储键。
// 未声明数据库键时，凭据使用由插件 ID 和完整目标 URL 决定的默认位置。
type PanelCredentialStorage struct {
	EnvironmentKey   string
	legacySettingKey string
}

type PanelCredentialValue struct {
	Token      string `json:"-"`
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
}

// IsPanelCredentialSetting 标识只能通过所属面板访问的内部设置。
func IsPanelCredentialSetting(key string) bool {
	return strings.HasPrefix(strings.ToUpper(key), panelCredentialPrefix)
}

// ID 和完整接收地址共同标识授权对象，查询参数或路径变化也要重新授权。
func panelCredentialKey(p PluginInfo) string {
	digest := sha256.Sum256([]byte(p.ID + "\x00" + p.URL))
	return panelCredentialPrefix + hex.EncodeToString(digest[:])
}

func panelCredentialKeys(plugins []PluginInfo) map[string]bool {
	keys := make(map[string]bool)
	for _, p := range plugins {
		if p.Panel != nil && p.Panel.Credential != nil {
			keys[panelCredentialKey(p)] = true
		}
	}
	return keys
}

// PanelCredentialSettingKeys 返回清单声明的面板凭据键，供完整备份校验所属目标。
func PanelCredentialSettingKeys(plugins []PluginInfo) map[string]bool {
	return panelCredentialKeys(plugins)
}

func removeRetiredPanelCredentials(tx *gorm.DB, before map[string]bool, current []PluginInfo) error {
	for key := range panelCredentialKeys(current) {
		delete(before, key)
	}
	if len(before) == 0 {
		return nil
	}
	keys := make([]string, 0, len(before))
	for key := range before {
		keys = append(keys, key)
	}
	return tx.Where("key IN ?", keys).Delete(&database.Setting{}).Error
}

// StoredPanelCredential 只允许活动实例操作自己的凭据。nil 表示读取，空字符串表示删除。
// 在数据库操作期间持有生命周期读锁，防止已停用或删除的旧实例延迟写回密钥。
func (m *Manager) StoredPanelCredential(ctx context.Context, scope PluginContext, update *string) (string, error) {
	result, err := m.AccessPanelCredential(ctx, scope, update)
	return result.Token, err
}

// AccessPanelCredential 读取或更新当前实例的凭据，返回权威来源及可公开的配置状态。
func (m *Manager) AccessPanelCredential(ctx context.Context, scope PluginContext, update *string) (PanelCredentialValue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.restoring || scope == nil || m.pluginContexts[scope.PluginID()] != scope || m.pluginStates[scope.PluginID()] != StateActive || scope.Context().Err() != nil {
		return PanelCredentialValue{}, ErrPanelInactive
	}
	info, ok := m.runningWebhooks[scope.PluginID()]
	if !ok {
		for _, builtin := range m.builtInPlugins {
			if p := builtin.GetInfo(); p.ID == scope.PluginID() {
				info = p
				break
			}
		}
	}
	if info.Panel == nil || info.Panel.Credential == nil {
		return PanelCredentialValue{}, ErrPanelInactive
	}
	storage, _ := localPanelService[PanelCredentialStorage](scope, PanelCredentialService)
	if token := strings.TrimSpace(os.Getenv(storage.EnvironmentKey)); token != "" {
		if update != nil {
			return PanelCredentialValue{}, ErrPanelCredentialReadOnly
		}
		return PanelCredentialValue{Token: token, Configured: true, Source: "environment"}, nil
	}
	if database.DB == nil {
		return PanelCredentialValue{}, errors.New("插件凭据数据库未就绪")
	}
	key := storage.legacySettingKey
	if key == "" {
		key = panelCredentialKey(info)
	}
	db := database.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard})
	var token string
	var err error
	switch {
	case update == nil:
		var saved database.Setting
		err = db.Where("key = ?", key).Limit(1).Find(&saved).Error
		token = strings.TrimSpace(saved.Value)
	case *update == "":
		err = db.Where("key = ?", key).Delete(&database.Setting{}).Error
	default:
		token = *update
		err = db.Save(&database.Setting{Key: key, Value: token}).Error
	}
	if err != nil {
		return PanelCredentialValue{}, err
	}
	if token == "" {
		return PanelCredentialValue{Source: "none"}, nil
	}
	return PanelCredentialValue{Token: token, Configured: true, Source: "stored"}, nil
}
