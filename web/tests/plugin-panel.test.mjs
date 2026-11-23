import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { createSSRApp } from 'vue'
import { parse } from 'vue/compiler-sfc'
import { renderToString } from 'vue/server-renderer'
import { createI18n } from 'vue-i18n'
import zh from '../src/locales/zh.ts'
import en from '../src/locales/en.ts'
import { view } from './view.mjs'

const endpoint = '/plugins/cloud-115/panel'
const panel = 'components/PluginPanel'
const magnet = 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567'
const submit = {
  id: 'submit', label: 'settings.cloud115.submit', result: 'message',
  input: { key: 'magnet', label: 'settings.cloud115.magnet', format: 'magnet', placeholder: 'magnet:?xt=...' },
}
const list = { id: 'tasks', label: 'settings.cloud115.tasks', result: 'tasks' }
const official = {
  id: 'cloud-115', is_builtin: true, origin: 'official',
  panel: {
    credential: { label: 'settings.cloud115.accessToken', required: true },
    actions: [submit, list],
  },
}

async function mount(request, plugin = official) {
  const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh, en } })
  const { mounted, component } = view(panel, request, { props: { plugin }, translate: i18n.global.t, translationExists: i18n.global.te })
  for (const init of mounted) await init()
  const template = parse(readFileSync(new URL('../src/' + panel + '.vue', import.meta.url), 'utf8')).descriptor.template
  component.markup = async (locale = 'zh') => {
    i18n.global.locale.value = locale
    return renderToString(createSSRApp({
      template: template.content,
      setup: () => ({ ...component, plugin, t: i18n.global.t }),
    }).use(i18n))
  }
  return component
}

test('只有运行中的声明式插件挂载通用面板，挂起显示依赖缺失', () => {
  const source = readFileSync(new URL('../src/views/SettingsPage.vue', import.meta.url), 'utf8')
  assert.match(source, /<PluginPanel\b[^>]*v-if="plugin.panel"/)
  assert.doesNotMatch(source, /Cloud115Panel/)
  const { component } = view('views/SettingsPage', {})
  const p = { ...official, enabled: true, status: 'active' }
  assert.equal(component.pluginRunning(p), true)
  p.status = 'suspended'
  assert.equal(component.pluginRunning(p), false)
  assert.match(source, /plugin.missing_dependencies/)
})

test('115 使用通用凭据、动作与分页接口，保存后清空密钥并防重复请求', async () => {
  let configured = false
  const calls = []
  let accept
  const c = await mount({
    get: async path => {
      calls.push(path)
      return { data: { configured, source: configured ? 'stored' : 'none' } }
    },
    put: async (path, body) => {
      assert.equal(path, endpoint + '/credential')
      assert.deepEqual({ ...body }, { access_token: 'test-secret' })
      configured = true
      return { data: { configured, source: 'stored' } }
    },
    delete: async path => {
      assert.equal(path, endpoint + '/credential')
      configured = false
      return { data: { configured, source: 'none' } }
    },
    post: async (path, body) => {
      calls.push([path, { ...body }])
      if (path.endsWith('/submit')) return await new Promise(resolve => { accept = resolve })
      const task = { info_hash: 'abc', name: 'Task ' + body.page, status: 'downloading', progress: 32 }
      return { data: { page: body.page, page_count: 2, tasks: [task] } }
    },
  })
  assert.match(await c.markup(), /未配置/)
  assert.doesNotMatch(await c.markup(), /type="text"/)
  c.tokenInput.value = ' test-secret '
  await c.updateToken('save')
  assert.equal(c.tokenInput.value, '')
  assert.match(await c.markup(), /凭据已保存/)
  assert.doesNotMatch(await c.markup(), /test-secret/)
  c.inputs.value.submit = 'invalid'
  await c.execute(submit)
  assert.match(await c.markup(), /有效的 BitTorrent/)
  assert.deepEqual(calls, [endpoint + '/credential'])
  c.inputs.value.submit = ' ' + magnet + ' '
  const pending = c.execute(submit)
  await c.execute(submit)
  assert.deepEqual(calls.slice(1), [[endpoint + '/actions/submit', { input: magnet }]])
  accept({ data: { message: 'settings.cloud115.submitted' } })
  await pending
  assert.equal(c.inputs.value.submit, '')
  assert.match(await c.markup('en'), /Submitted to 115/)
  assert.equal(calls.length, 2)
  await c.execute(list)
  assert.match(await c.markup('en'), /Downloading · 32%/)
  await c.execute(list, 2)
  assert.deepEqual(calls.slice(2), [[endpoint + '/actions/tasks', { page: 1 }], [endpoint + '/actions/tasks', { page: 2 }]])
  assert.match(await c.markup(), /Task 2/)
  await c.updateToken('clear')
  assert.doesNotMatch(await c.markup(), /Task 2/)
})

test('115 环境授权只读，五种任务状态和请求错误有中英文反馈', async () => {
  let failure = 0
  const statuses = ['pending', 'downloading', 'completed', 'failed', 'unknown']
  const c = await mount({
    get: async () => ({ data: { configured: true, source: 'environment' } }),
    put: () => assert.fail(),
    delete: () => assert.fail(),
    post: async () => {
      if (failure) throw { isAxiosError: true, response: { status: failure, data: { error: 'private-upstream-secret' } } }
      const tasks = statuses.map(status => ({ info_hash: status, name: status, status, progress: 0 }))
      return { data: { page: 1, page_count: 1, tasks } }
    },
  })
  assert.doesNotMatch(await c.markup(), /type="password"/)
  c.tokenInput.value = 'unused'
  await c.updateToken('save')
  await c.updateToken('clear')
  await c.execute(list)
  const zhView = await c.markup()
  const enView = await c.markup('en')
  for (const text of ['等待中', '下载中', '已完成', '失败', '状态未知']) assert.ok(zhView.includes(text + ' · 0%'))
  for (const text of ['Pending', 'Downloading', 'Completed', 'Failed', 'Unknown status']) assert.ok(enView.includes(text + ' · 0%'))
  c.inputs.value.submit = magnet
  for (const [code, label] of [[400, '有效的 BitTorrent'], [422, '检查凭据与权限'], [429, '请求过于频繁'], [502, '未确认']]) {
    failure = code
    await c.execute(submit)
    const html = await c.markup()
    assert.ok(html.includes(label))
    assert.equal(c.inputs.value.submit, magnet)
    assert.doesNotMatch(html, /private-upstream-secret|已提交/)
  }
})

test('社区插件按声明渲染多个动作，可无凭据运行，标签、响应与 URL 按文本呈现', async () => {
  const community = {
    id: 'community', origin: 'community', url: 'https://example.org/tools',
    panel: { actions: [
      { id: 'ping', label: 'Ping', result: 'message' },
      { id: 'query', label: 'settings.cloud115.submit', input: { key: 'search', label: 'Search terms' }, result: 'message' },
      { id: 'lookup', label: 'List', result: 'tasks' },
    ] },
  }
  let requests = []
  const req = {
    get: async url => {
      requests.push(url)
      return { data: { configured: false, source: 'none' } }
    },
    post: async (url, data) => {
      requests.push([url, { ...data }])
      return { data: { message: 'settings.cloud115.submitted' } }
    },
  }
  let c = await mount(req, community)
  assert.deepEqual(requests, [])
  let html = await c.markup()
  assert.match(html, /Ping/)
  assert.match(html, /Search terms/)
  assert.match(html, /settings.cloud115.submit/)
  assert.doesNotMatch(html, /115 云端任务|type="password"/)
  c.inputs.value.query = 'plain input'
  await c.execute(community.panel.actions[1])
  assert.deepEqual(requests, [['/plugins/community/panel/actions/query', { input: 'plain input' }]])
  assert.match(await c.markup(), /settings.cloud115.submitted/)
  community.panel.credential = { label: 'API key', required: false }
  requests = []
  c = await mount(req, community)
  html = await c.markup()
  assert.match(html, /https:\/\/example.org\/tools/)
  assert.match(html, /type="password"/)
  assert.match(html, /可选/)
  await c.execute(community.panel.actions[0])
  assert.deepEqual(requests, ['/plugins/community/panel/credential', ['/plugins/community/panel/actions/ping', {}]])
})

test('启用确认之前不挂载 115 操作区，重复切换只发送一次请求，失败后允许重试', async () => {
  const plugin = { id: 'cloud-115', enabled: false }
  let calls = 0
  let succeed
  let fail
  const { component } = view('views/SettingsPage', {
    get: async () => ({ data: [] }),
    post: async (path, body) => {
      assert.equal(path, '/plugins/toggle')
      assert.deepEqual({ ...body }, { id: 'cloud-115', enabled: true })
      calls++
      return await new Promise((resolve, reject) => { succeed = resolve; fail = reject })
    },
  })
  const first = component.togglePlugin(plugin)
  assert.equal(plugin.enabled, false)
  await component.togglePlugin(plugin)
  assert.equal(calls, 1)
  fail(new Error('toggle failed'))
  await first
  assert.equal(plugin.enabled, false)
  const second = component.togglePlugin(plugin)
  assert.equal(calls, 2)
  assert.equal(plugin.enabled, false)
  succeed({ data: { enabled: true, status: 'active' } })
  await second
  assert.equal(plugin.enabled, true)
})

test('切换依赖服务后从列表获取挂起插件及缺失服务', async () => {
  const provider = { id: 'provider', enabled: true, status: 'active' }
  const waiting = { id: 'consumer', enabled: true, status: 'suspended', missing_dependencies: ['media-index'] }
  const { component } = view('views/SettingsPage', {
    post: async () => ({ data: { enabled: false, status: 'inactive' } }),
    get: async path => {
      assert.equal(path, '/plugins')
      return { data: [provider, waiting] }
    },
  })
  await component.togglePlugin(provider)
  assert.equal(provider.enabled, false)
  const consumer = component.pluginList.value.find(p => p.id === waiting.id)
  assert.equal(component.pluginRunning(consumer), false)
  assert.deepEqual([...consumer.missing_dependencies], ['media-index'])
})

test('删除请求保留被选插件的完整 ID', async () => {
  let deleted
  const { component } = view('views/SettingsPage', {
    delete: async path => { deleted = path },
    get: async () => ({ data: [] }),
  })
  await component.deletePlugin('team?name#1')
  assert.equal(deleted, '/plugins/team%3Fname%231')
})

test('导入中途失败后显示已落库插件，重试只发送剩余项', async () => {
  const saved = []
  const attempts = []
  let fail = true
  const i18n = createI18n({ legacy: false, locale: 'en', messages: { en } })
  const { component: c } = view('views/SettingsPage', {
    post: async (_path, item) => {
      attempts.push({ ...item })
      if (item.name === 'second' && fail) throw new Error('second failed')
      saved.push({ ...item })
    },
    get: async () => ({ data: [...saved] }),
  }, { translate: (key, params) => i18n.global.t(key, { ...params }) })
  c.openAddPluginModal()
  c.pluginModalTab.value = 'json'
  c.pluginJsonText.value = JSON.stringify([{ name: 'first' }, { name: 'second' }, { id: 'third-id', name: 'third' }])
  await c.submitPluginForm()
  assert.deepEqual([...c.pluginList.value].map(p => p.name), ['first'])
  assert.deepEqual(JSON.parse(c.pluginJsonText.value).map(p => p.name), ['second', 'third'])
  assert.match(c.pluginJsonError.value, /1 of 3/)
  assert.match(c.pluginJsonError.value, /second failed/)
  assert.equal(c.showPluginModal.value, true)
  fail = false
  await c.submitPluginForm()
  assert.equal(c.showPluginModal.value, false)
  assert.deepEqual(attempts.map(p => p.name), ['first', 'second', 'second', 'third'])
  assert.ok(attempts[1].id)
  assert.equal(attempts[1].id, attempts[2].id)
  assert.equal(attempts[3].id, 'third-id')
})

test('空数组或非对象清单保留在编辑器；没有可导出社区配置时不下载', async () => {
  let writes = 0
  const { component: c } = view('views/SettingsPage', {
    post: async () => { writes++ },
    get: async () => ({ data: [] }),
  })
  for (const json of ['[]', 'null', '[{"name":"ok"},false]']) {
    c.openAddPluginModal()
    c.pluginModalTab.value = 'json'
    c.pluginJsonText.value = json
    await c.submitPluginForm()
    assert.equal(c.showPluginModal.value, true)
    assert.ok(c.pluginJsonError.value)
    assert.equal(writes, 0)
  }
  c.pluginList.value = [{ ...official }]
  c.exportPluginsJSON()
})

test('导入确认后清单刷新失败不会同时显示成功和空插件列表', async () => {
  const { component: c } = view('views/SettingsPage', {
    post: async () => ({}),
    get: async () => { throw new Error('offline') },
  })
  c.openAddPluginModal()
  c.pluginModalTab.value = 'json'
  c.pluginJsonText.value = '{"id":"community","name":"ok"}'
  await c.submitPluginForm()
  assert.equal(c.showPluginModal.value, false)
  assert.equal(c.saved.value, false)
  assert.match(c.pluginListError.value, /refreshFailed/)
})

test('新的文件选择顺序覆盖旧读取，文件读取失败显示错误', async () => {
  let done
  const { component: c } = view('views/SettingsPage', {})
  c.openAddPluginModal()
  const select = text => c.handlePluginFileImport({ target: { files: [{ text }], value: 'file' } })
  const older = select(() => new Promise(resolve => { done = resolve }))
  await select(async () => 'newer')
  done('older')
  await older
  assert.equal(c.pluginJsonText.value, 'newer')
  await select(async () => { throw new Error('file unavailable') })
  assert.match(c.pluginJsonError.value, /fileReadFailed/)
})

test('使用凭据写入确认的返回值，随后读取离线仍显示已保存状态', async () => {
  let reads = 0
  const c = await mount({
    get: async () => {
      if (++reads > 1) throw new Error('offline')
      return { data: { configured: false, source: 'none' } }
    },
    put: async () => ({ data: { configured: true, source: 'stored' } }),
    delete: async () => ({ data: { configured: false, source: 'none' } }),
  })
  c.tokenInput.value = 'sample'
  await c.updateToken('save')
  assert.equal(c.config.value?.source, 'stored')
  assert.equal(c.error.value, '')
  await c.updateToken('clear')
  assert.equal(c.config.value?.source, 'none')
  assert.equal(reads, 1)
})

test('消息动作收到不完整的成功响应时保留输入并提示请求未确认', async () => {
  const c = await mount({
    get: async () => ({ data: { configured: true, source: 'stored' } }),
    post: async () => ({ data: {} }),
  })
  c.inputs.value.submit = magnet
  await c.execute(submit)
  assert.equal(c.inputs.value.submit, magnet)
  assert.equal(c.notice.value, '')
  assert.equal(c.error.value, 'submitFailed')
})

test('同一插件的删除和启停进行中时，其他点击不会再次提交', async () => {
  const calls = []
  let settle
  const defer = () => new Promise(resolve => { settle = resolve })
  const p = { id: 'test', enabled: false }
  const { component: c } = view('views/SettingsPage', {
    post: async () => { calls.push('toggle'); return await defer() },
    delete: async () => { calls.push('delete'); return await defer() },
    get: async () => ({ data: [p] }),
  })
  const pending = c.togglePlugin(p)
  const competing = c.deletePlugin(p.id)
  assert.deepEqual(calls, ['toggle'])
  settle({ data: { enabled: true, status: 'active' } })
  await Promise.all([pending, competing])
  const firstDelete = c.deletePlugin(p.id)
  const secondDelete = c.deletePlugin(p.id)
  const toggle = c.togglePlugin(p)
  assert.deepEqual(calls, ['toggle', 'delete'])
  settle({})
  await Promise.all([firstDelete, secondDelete, toggle])
})

test('官方面板翻译任何已知声明文案，未知标签与社区文本保持原样', async () => {
  const action = { id: 'clear', label: 'common.refresh', result: 'message' }
  const plugin = { id: 'future-builtin', origin: 'official', is_builtin: true, panel: { actions: [action] } }
  const c = await mount({ post: async () => ({ data: { message: 'Plain text from provider' } }) }, plugin)
  assert.match(await c.markup('en'), /Refresh/)
  assert.doesNotMatch(await c.markup('en'), /common.refresh/)
  await c.execute(action)
  assert.match(await c.markup('en'), /Plain text from provider/)
  const d = await mount({}, { ...plugin, origin: 'community', is_builtin: false })
  assert.match(await d.markup('en'), /common.refresh/)
})
