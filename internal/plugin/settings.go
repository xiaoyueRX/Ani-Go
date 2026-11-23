package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	pluginStatusKey  = "plugin_status"
	customPluginsKey = "custom_plugins"
)

type configurationChange uint8

const (
	statusChanged configurationChange = 1 << iota
	customChanged
)

var ErrReloadAfterRestore = errors.New("备份已恢复，插件配置重载失败")

// RestoreConfiguration 串行执行恢复与重载，区分恢复失败和提交后的重载失败。
func (m *Manager) RestoreConfiguration(restore func() error) error {
	m.lockChange()
	defer m.unlockChange()
	if m.stopDone != nil {
		return errors.New("插件管理器已关闭")
	}
	if restore == nil {
		return errors.New("缺少恢复操作")
	}
	m.restoring = true
	defer func() { m.restoring = false }()
	if err := restore(); err != nil {
		return err
	}
	if err := m.updateConfigurationLocked(0, nil); err != nil {
		// 数据库已变更；旧实例的事件投递和凭据授权不可继续使用。
		m.configurationPending = true
		m.retireInstancesLocked()
		return fmt.Errorf("%w: %w", ErrReloadAfterRestore, err)
	}
	return nil
}

type pluginConfiguration struct {
	status map[string]bool
	custom []PluginInfo
}

// 同一事务读取完整配置、迁移旧 ID 并保存本次操作，提交后才切换运行状态。
// 首次读取失败时没有可信开关；后续失败则继续使用此前已经提交的配置。
func (m *Manager) updateConfigurationLocked(changes configurationChange, edit func(*pluginConfiguration) error) error {
	if m.stopDone != nil {
		return errors.New("插件管理器已关闭")
	}
	config := pluginConfiguration{status: make(map[string]bool), custom: []PluginInfo{}}
	if database.DB == nil {
		if m.activePlugins != nil {
			config.status = m.activePlugins
		}
		config.custom = m.customPlugins
		if edit != nil {
			if err := edit(&config); err != nil {
				return err
			}
		}
	} else {
		err := database.DB.Transaction(func(tx *gorm.DB) error {
			if err := config.read(tx); err != nil {
				return err
			}
			before := panelCredentialKeys(config.custom)
			rekeyed := m.normalizeCustomPlugins(config.custom, config.status)
			if edit != nil {
				if err := edit(&config); err != nil {
					return err
				}
			}
			if rekeyed {
				changes |= statusChanged | customChanged
			}
			if changes == 0 {
				return nil
			}
			if err := removeRetiredPanelCredentials(tx, before, config.custom); err != nil {
				return err
			}
			return config.save(tx, changes)
		})
		if err != nil {
			return err
		}
	}
	m.activePlugins, m.customPlugins = config.status, config.custom
	m.configurationPending = false
	m.applyRuntimeLocked()
	return nil
}

func (c *pluginConfiguration) read(tx *gorm.DB) error {
	status := make(map[string]*bool)
	if err := readPluginSetting(tx, pluginStatusKey, &status); err != nil {
		return err
	}
	if status == nil {
		return fmt.Errorf("%s 应为开关对象", pluginStatusKey)
	}
	for id, enabled := range status {
		if enabled == nil {
			return fmt.Errorf("%s 中的开关必须为布尔值", pluginStatusKey)
		}
		c.status[id] = *enabled
	}
	if err := readPluginSetting(tx, customPluginsKey, &c.custom); err != nil {
		return err
	}
	if c.custom == nil {
		return fmt.Errorf("%s 应为插件数组", customPluginsKey)
	}
	for _, p := range c.custom {
		if strings.TrimSpace(p.ID) == "" {
			return fmt.Errorf("%s 中的插件缺少 ID", customPluginsKey)
		}
	}
	return nil
}

func readPluginSetting(tx *gorm.DB, key string, out any) error {
	var setting database.Setting
	err := tx.Where("key = ?", key).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取插件配置 %s 失败: %w", key, err)
	}
	if err := json.Unmarshal([]byte(setting.Value), out); err != nil {
		return fmt.Errorf("插件配置 %s 的格式无效", key)
	}
	return nil
}

func (c *pluginConfiguration) save(tx *gorm.DB, changes configurationChange) error {
	var rows []database.Setting
	for _, field := range []struct {
		key   string
		flag  configurationChange
		value any
	}{
		{pluginStatusKey, statusChanged, c.status}, {customPluginsKey, customChanged, c.custom},
	} {
		if changes&field.flag == 0 {
			continue
		}
		data, err := json.Marshal(field.value)
		if err != nil {
			return err
		}
		rows = append(rows, database.Setting{Key: field.key, Value: string(data)})
	}
	// 清单可能含 Webhook 密钥，避免数据库错误日志展开绑定值。
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(logger.Silent)}).Save(&rows).Error
}
