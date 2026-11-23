import assert from 'node:assert/strict'
import { test } from 'node:test'
import { view } from './view.mjs'

function pending() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

test('设置后台刷新保留脏字段并忽略过时快照', async () => {
  const older = pending(), latest = pending()
  const replies = [Promise.resolve({ data: { BANGUMI_CLIENT_ID: 'old', TV_BASE_PATH: '/a' } }), older.promise, latest.promise]
  const writes = []
  const { component: c } = view('views/SettingsPage', {
    get: () => replies.shift(), put: async (_path, body) => { writes.push({ ...body.settings }) },
  })
  await c.fetchSettings()
  const first = c.fetchSettings(), second = c.fetchSettings()
  c.setVal('BANGUMI_CLIENT_ID', 'draft')
  latest.resolve({ data: { BANGUMI_CLIENT_ID: 'server', TV_BASE_PATH: '/b' } }); await second
  older.resolve({ data: { BANGUMI_CLIENT_ID: 'old', TV_BASE_PATH: '/a' } }); await first
  assert.equal(c.getVal('BANGUMI_CLIENT_ID'), 'draft')
  assert.equal(c.getVal('TV_BASE_PATH'), '/b')
  await c.saveAll()
  assert.deepEqual(writes, [{ BANGUMI_CLIENT_ID: 'draft' }])
})

test('保存期间重复操作只提交一次，后续输入留给下次保存', async () => {
  const write = pending(), writes = []
  const { component: c } = view('views/SettingsPage', {
    get: async path => ({ data: path === '/settings' ? { TV_BASE_PATH: '/a' } : { patterns: ['first'] } }),
    put: async (_path, body) => { writes.push({ ...body.settings }); await write.promise },
  })
  await c.fetchSettings(); await c.fetchCustomRegex()
  c.customRegexList.value[0] = 'second'
  const a = c.saveAll()
  c.customRegexList.value[0] = 'third'
  const b = c.saveAll()
  write.resolve()
  await Promise.all([a,b])
  assert.deepEqual(writes, [{ custom_regex_0: 'second' }])
  assert.equal(c.customRegexList.value[0], 'third')
})

test('详情页迟到的刷新无法把较新的剧集状态回滚', async () => {
  const older = pending(), newest = pending()
  const replies = [older.promise, newest.promise]
  const { component: c } = view('views/SubscriptionDetail', { get: () => replies.shift() }, { route: { params: { id: 1 } } })
  const a = c.fetchDetail(), b = c.fetchDetail()
  newest.resolve({ data: { subscription: { id: 1 }, episodes: [{ id: 7, status: 'organized' }] } }); await b
  older.resolve({ data: { subscription: { id: 1 }, episodes: [{ id: 7, status: 'pending' }] } }); await a
  assert.equal(c.episodes.value[0].status, 'organized')
})

test('批量操作固定选中集并阻止重入，失败的单集保留以便重试', async () => {
  const first = pending(), calls = []
  const episodes = [1,2,3].map(id => ({ id, status: 'pending' }))
  const { component: c } = view('views/SubscriptionDetail', {
    put: async path => {
      calls.push(path)
      if (path === '/episodes/1/status') await first.promise
      if (path === '/episodes/2/status') throw new Error('unavailable')
    },
    get: async () => ({ data: { subscription: { id: 1 }, episodes } }),
  }, { route: { params: { id: 1 } } })
  c.episodes.value = episodes
  c.toggleEpisodeSelect(1); c.toggleEpisodeSelect(2)
  const a = c.bulkUpdateStatus('completed')
  const b = c.bulkUpdateStatus('completed')
  c.toggleEpisodeSelect(3)
  first.resolve(); await Promise.all([a,b])
  assert.deepEqual(calls, ['/episodes/1/status','/episodes/2/status'])
  assert.deepEqual([...c.selectedEpisodes.value],[2])
})

test('通知筛选中迟到的上一页不能覆盖新一页', async () => {
  const older = pending(), latest = pending(), replies = [older.promise,latest.promise]
  const { component: c } = view('views/SettingsPage', { get: () => replies.shift() })
  const a = c.fetchNotifyLogs(); c.notifyFilterStatus.value = 'failed'; const b = c.fetchNotifyLogs()
  latest.resolve({ data: { logs: [{ status:'failed' }], total:1 } }); await b
  older.resolve({ data: { logs: [{ status:'success' }], total:10 } }); await a
  assert.equal(c.notifyLogs.value[0].status,'failed')
  assert.equal(c.notifyLogsTotal.value,1)
})

test('详情刷新遇到暂时故障保留快照，订阅被删除后清理旧剧集与操作', async () => {
  let status = 0
  const { component: c } = view('views/SubscriptionDetail', { get: async () => {
    if (status) throw { response: { status, data: { error: 'error' } } }
    return { data: { subscription: { id: 1 }, episodes: [{ id: 12 }] } }
  } }, { route: { params: { id: 1 } } })
  await c.fetchDetail(); c.toggleEpisodeSelect(12)
  status = 503; await c.fetchDetail()
  assert.equal(c.sub.value.id,1)
  status = 404; await c.fetchDetail()
  assert.equal(c.sub.value,null)
  assert.equal(c.episodes.value.length,0)
  assert.equal(c.selectedEpisodes.value.size,0)
})

test('单独保存正则只提交改动槽位，保留另一页签新增的规则', async () => {
  const server = ['rule-a','rule-b']
  const { component:c }=view('views/SettingsPage',{
    get:async()=>({data:{patterns:[...server]}}),
    put:async(_path,body)=>{ for(const [key,value] of Object.entries(body.settings)) server[Number(key.slice('custom_regex_'.length))] = value },
    post:async()=>({data:{compiled:[...server]}}),
  })
  await c.fetchCustomRegex()
  server[1]='b from another tab'
  c.customRegexList.value[0]='a from this tab'
  await c.saveCustomRegex()
  assert.equal(server[0],'a from this tab')
  assert.equal(server[1],'b from another tab')
})
