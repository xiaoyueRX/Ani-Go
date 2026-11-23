import assert from 'node:assert/strict'
import { test } from 'node:test'
import { view } from './view.mjs'

test('剧集删除限定为当前订阅', async () => {
  const calls = []
  const { component } = view('views/SubscriptionDetail', {
    post: async (path, body) => calls.push([path, body]),
    get: async () => ({ data: { episodes: [] } }),
  }, { route: { params: { id: 7 } } })
  component.toggleEpisodeSelect(72)
  await component.bulkDeleteEpisodes(true)
  assert.equal(calls[0][0], '/subscriptions/7/episodes/batch-delete')
  assert.deepEqual(JSON.parse(JSON.stringify(calls[0][1])), { ids: [72], delete_files: true })
})

test('撤销窗口能保留每个订阅的文件删除选择', async () => {
  const calls = []
  const { component } = view('views/Subscriptions', {
    post: async (_path, body) => calls.push(body),
  })
  component.scheduleDelete([1, 2], false)
  component.scheduleDelete([3], true)
  await component.executePendingDeletes()
  const flags = new Map(calls.flatMap(group => group.ids.map(id => [id, group.delete_files])))
  assert.deepEqual(flags, new Map([[1, false], [2, false], [3, true]]))
})

test('单项暂停与恢复成功后提示成功并刷新', async () => {
  let refreshes = 0
  const { component, notices } = view('views/Downloads', {
    post: async () => ({}), get: async () => { refreshes++; return { data: [] } },
  })
  await component.pauseTask({ hash: 'a' })
  await component.resumeTask({ hash: 'a' })
  assert.deepEqual(notices.map(item => item.type), ['success', 'success'])
  assert.equal(refreshes, 2)
})

test('向导先保存选中的下载器再更新密码', async () => {
  const calls = []
  let active = true
  const { component } = view('components/OnboardingModal', {
    post: async path => { calls.push(path); active = false },
    put: async (path, body) => {
      if (!active) throw new Error('登录已过期')
      calls.push([path, body.settings.DEFAULT_DOWNLOADER])
    },
  })
  component.isDefaultPassword.value = true
  component.newPassword.value = '新密码abc'
  component.dlType.value = 'transmission'
  await component.finishSetup()
  assert.deepEqual(calls, [['/settings', 'transmission'], '/user/change-password'])
})

test('快速订阅传递季度及原生字幕组数组', async () => {
  let payload
  const { component } = view('views/Subscriptions', {
    post: async (_path, body) => { payload = body }, get: async () => ({ data: [] }),
  })
  component.aiResult.value = { title: '续篇', season: 2, subgroup_pref: 'ANi' }
  await component.applyAiSubscribe()
  assert.equal(payload.season, 2)
  assert.deepEqual(Array.from(payload.allowed_subgroups), ['ANi'])
})

test('归并按钮提交应用请求并读取实际变更数量', async () => {
  let payload
  let refreshed = false
  const { component, notices } = view('views/Subscriptions', {
    post: async (_path, body) => {
      payload = body
      return { data: { data: { total_applied: 2 } } }
    },
    get: async () => { refreshed = true; return { data: [] } },
  })
  await component.triggerMergeCollections()
  assert.equal(payload.apply, true)
  assert.equal(refreshed, true)
  assert.equal(notices.at(-1).type, 'success')
})

test('智能搜索开关采用服务端可用能力', async () => {
  const { component, mounted } = view('views/Search', {
    get: async path => ({data: path === '/search/capabilities' ? { smart_search: true } : []}),
  })
  for (const initialize of mounted) await initialize()
  assert.equal(component.aiConfigured.value, true)
})

test('设置页面只保存修改过的字段，并显示服务端的间隔校验错误', async () => {
  const calls = []
  const message = 'RSS_INTERVAL 需要大于零的有效时长'
  const { component, notices } = view('views/SettingsPage', {
    get: async () => ({ data: { RSS_INTERVAL: '15m', MCP_TOKEN_CONFIGURED: 'true', QB_HOST: 'http://configured.example' } }),
    put: async (_path, body) => {
      calls.push(body.settings)
      throw { response: { status: 400, data: { error: message } } }
    },
  })
  await component.fetchSettings()
  component.settings.value.RSS_INTERVAL = '0s'
  await component.saveAll()
  assert.deepEqual(JSON.parse(JSON.stringify(calls)), [{ RSS_INTERVAL: '0s' }])
  assert.equal(component.error.value, message)
  assert.equal(component.saved.value, false)
  assert.equal(notices.at(-1).type, 'error')
})

test('应用启动时清理旧登录记录，已登录的浏览器也能保留用户名', () => {
  const storage = new Map([
    ['token', 'active-session'], ['remembered_user', JSON.stringify({ username: 'alice', password: '旧测试值' })],
  ])
  view('App', {}, { storage })
  assert.deepEqual([...storage.entries()], [['token', 'active-session'], ['remembered_username', 'alice']])
  const { component } = view('views/Login', {}, { storage })
  assert.equal(component.username.value, 'alice')
  assert.equal(component.remember.value, true)
  assert.equal(component.password.value, '')
})

test('记住用户名只保存账户名，取消选择后正常忘记', async () => {
  const storage = new Map()
  const { component } = view('views/Login', { post: async () => ({ data: { token: 'session' } }) }, { storage })
  component.username.value = 'alice'
  component.password.value = '本次输入的测试口令'
  component.remember.value = true
  await component.handleLogin()
  assert.deepEqual([...storage.entries()], [['token', 'session'], ['remembered_username', 'alice']])
  component.remember.value = false
  await component.handleLogin()
  assert.deepEqual([...storage.entries()], [['token', 'session']])
})

test('插件来源筛选使用服务端归属并兼容缺少来源的旧数据', () => {
  const { component } = view('views/SettingsPage', {})
  component.pluginList.value = [
    { id: 'project', origin: 'official' },
    { id: 'packaged', is_builtin: true, author: 'xiaoyue', origin: 'community' },
    { id: 'legacy', is_builtin: true },
  ]
  component.pluginSourceFilter.value = 'official'
  assert.deepEqual(Array.from(component.visiblePlugins.value, p => p.id), ['project'])
  component.pluginSourceFilter.value = 'community'
  assert.deepEqual(Array.from(component.visiblePlugins.value, p => p.id), ['packaged', 'legacy'])
  component.pluginSourceFilter.value = 'all'
  assert.equal(component.visiblePlugins.value.length, 3)
})

test('详情只在已知总集数时计算完成百分比', () => {
  const { component } = view('views/SubscriptionDetail', {})
  component.sub.value = { current_episodes: 2, total_episodes: 0 }
  assert.equal(component.progress.value, null)
  component.sub.value = { current_episodes: 0, total_episodes: 12 }
  assert.equal(component.progress.value, 0)
  component.sub.value = { current_episodes: 3, total_episodes: 12 }
  assert.equal(component.progress.value, 25)
})

test('订阅卡片隐藏无法计算的百分比', () => {
  const { component } = view('components/SubscriptionCard', {}, { props: { sub: { current_episodes: 2, total_episodes: 0 } } })
  assert.equal(component.progress.value, null)
})

test('单集的手动标记可清除，写入失败或成功均保留真实整理状态', async () => {
  const episode = { id: 27, status: 'organized', manual_status: '' }
  const calls = []
  let fail = false
  const { component } = view('views/SubscriptionDetail', {
    put: async (path, body) => {
      calls.push([path, body.status])
      if (fail) throw new Error('offline')
      return { data: { status: body.status, manual_status: body.status } }
    },
    get: async () => ({ data: { subscription: { id: 7 }, episodes: [episode] } }),
  }, { route: { params: { id: 7 } } })
  const select = { value: 'failed' }
  await component.setEpisodeMark(episode, { target: select })
  assert.equal(episode.manual_status, 'failed')
  fail = true
  select.value = 'completed'
  await component.setEpisodeMark(episode, { target: select })
  assert.equal(episode.manual_status, 'failed')
  assert.equal(select.value, 'failed')
  fail = false
  select.value = ''
  await component.setEpisodeMark(episode, { target: select })
  assert.equal(episode.manual_status, '')
  assert.equal(episode.status, 'organized')
  assert.deepEqual(calls, [['/episodes/27/status', 'failed'], ['/episodes/27/status', 'completed'], ['/episodes/27/status', '']])
})
