package plugin

// 改变运行状态的入口持有两把锁；调用插件时临时释放状态锁，
// 其他变更继续串行，插件和正在退出的协程都可以查询其状态。
func (m *Manager) lockChange()   { m.serial.Lock(); m.mu.Lock() }
func (m *Manager) unlockChange() { m.mu.Unlock(); m.serial.Unlock() }
func (m *Manager) outsideStateLock(fn func()) {
	m.mu.Unlock()
	defer m.mu.Lock()
	fn()
}

// 超过同步退出期限的实例仍然拥有资源，待其真正释放后再启动同 ID 的新实例。
func (m *Manager) disposeScopeLocked(id string, scope PluginContext) {
	m.outsideStateLock(scope.Dispose)
	c, ok := scope.(*pluginContextImpl)
	if !ok {
		return
	}
	select {
	case <-c.finished:
		return
	default:
	}
	if m.draining[id] == c {
		return
	}
	m.draining[id] = c
	go func() {
		select {
		case <-c.finished:
		case <-m.shutdown:
			return
		}
		m.lockChange()
		defer m.unlockChange()
		if m.draining[id] != c {
			return
		}
		delete(m.draining, id)
		if m.stopDone == nil && m.activePlugins != nil {
			m.applyRuntimeLocked()
		}
	}()
}
