import assert from 'node:assert/strict'
import test from 'node:test'
import { view } from './view.mjs'

const input = 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567'
const item = { title: 'Fixture episode', magnet: input }
const action = { id: 'queue', label: 'Submit', placement: 'search_result', input: { format: 'magnet' }, result: 'message' }
const plugin = { id: 'sample', name: 'My provider', url: 'https://example.org/cloud', enabled: true, status: 'active', panel: { actions: [action] } }

test('搜索结果只显示运行中且主动声明入口的磁力动作', async () => {
  const data = [plugin, { ...plugin, id: 'off', enabled: false }, { ...plugin, id: 'pending', status: 'pending' },
    { ...plugin, id: 'legacy', panel: { actions: [{ ...action, placement: undefined }] } }]
  const { component: c } = view('views/Search', { get: async () => ({ data }) })
  await c.loadPluginTargets()
  assert.equal(c.pluginTargets.value.length, 1)
  c.choosePlugin(item)
  assert.equal(c.pluginSelection.value.targets[0].id, 'sample')
  data.splice(0)
  await c.loadPluginTargets()
  assert.equal(c.pluginTargets.value.length, 0)
  assert.equal(c.pluginSelection.value.targets.length, 1)
})

test('磁力只发送给确认的插件一次，未确认交付时保留反馈', async () => {
  const calls = []
  let resolve
  const props = { item, targets: [{ id: plugin.id, name: plugin.name, url: plugin.url, action: action.id, label: action.label, official: false }] }
  const { component: c } = view('components/SearchPluginDialog', { post(path, body) {
    calls.push([path, { ...body }]); return new Promise(done => { resolve = done })
  } }, { props, translationExists: () => true, translate: key => 'translated:' + key })
  const first = c.send()
  await c.send()
  assert.deepEqual(calls, [['/plugins/sample/panel/actions/queue', { input, placement: 'search_result', expected_url: plugin.url }]])
  resolve({ data: {} })
  await first
  assert.equal(c.error.value, 'settings.pluginPanel.submitFailed')
  assert.equal(c.message.value, '')
  const retry = c.send()
  resolve({ data: { message: 'community.literal' } })
  await retry
  assert.equal(c.message.value, 'community.literal')
  await c.send()
  assert.equal(calls.length, 2)
})

test('备用磁力仅用于云端，直接下载继续提交原种子网址', async () => {
  const posts = []
  const { component: c } = view('views/Search', {
    get: async () => ({ data: [plugin] }),
    post: async (path, body) => { posts.push([path, { ...body }]); return { data: {} } },
  })
  await c.loadPluginTargets()
  const result = { title: 'with alternative', url: 'https://example.org/one.torrent', magnet: '', alternate_magnet: input }
  c.choosePlugin(result)
  assert.equal(c.pluginSelection.value.item.magnet, input)
  await c.directDownload(result)
  assert.equal(posts[0][1].magnet, '')
  assert.equal(posts[0][1].url, result.url)
})

test('插件确认在会话切换后不会向新账户投递旧选择', async () => {
  const storage = new Map([['token', 'original']])
  const calls = []
  const props = { item, targets: [{ id: plugin.id, name: plugin.name, url: plugin.url, action: action.id, label: action.label, official: false }] }
  const c = view('components/SearchPluginDialog', { post: async (...args) => { calls.push(args); return { data: { message: 'done' } } } }, { storage, props }).component
  storage.set('token', 'replacement')
  await c.send()
  assert.equal(calls.length, 0)
  assert.equal(c.message.value, '')
})
