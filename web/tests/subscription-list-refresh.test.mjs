import assert from 'node:assert/strict'
import { test } from 'node:test'
import { view } from './view.mjs'

const next = () => new Promise(resolve => setImmediate(resolve))
function held() {
  let resolve
  const promise = new Promise(yes => { resolve = yes })
  return { promise, resolve }
}

function list(get) {
  const doc = new EventTarget()
  doc.hidden = false
  let tick, stream
  class EventSource {
    constructor() { stream = this }
    close() {}
  }
  const { component, mounted, unmounted } = view('views/Subscriptions', { get }, { globals: {
    document: doc, EventSource, setInterval: fn => { tick = fn },
  } })
  mounted.forEach(fn => fn())
  return {
    component, tick: () => tick(), leave: () => unmounted.forEach(fn => fn()),
    hidden(value) { doc.hidden = value; doc.dispatchEvent(new Event('visibilitychange')) },
    added() { stream.onmessage({ data: JSON.stringify({ Type: 'supplement.progress', Payload: { title: 'Harbour', message: '已添加下载' } }) }) },
  }
}

test('后台列表保留补全消息，切回页面立即刷新订阅和回收站', async () => {
  const calls = []
  const l = list(async path => { calls.push(path); return { data: [] } })
  await next()
  calls.length = 0
  l.hidden(true)
  l.tick(); l.tick(); l.added()
  await next()
  assert.equal(l.component.taskCenter.value.logs[0].message, '已添加下载')
  assert.deepEqual(calls, [])
  l.hidden(false)
  await next()
  assert.deepEqual(calls, ['/subscriptions', '/subscriptions?deleted=true'])
  l.leave(); l.hidden(true); l.hidden(false)
  await next()
  assert.equal(calls.length, 2)
})

test('跨轮询的读取与补全消息合并，收到最后一次刷新时返回最新列表', async () => {
  const original = held(), fresh = held()
  let reads = 0
  const l = list(path => {
    if (path.includes('?')) return Promise.resolve({ data: [] })
    reads++
    return reads === 1 ? original.promise : fresh.promise
  })
  l.tick(); l.tick(); l.added()
  assert.equal(reads, 1, '慢响应返回前只能有一次列表请求')
  original.resolve({ data: [{ id: 1, title_cn: '旧标题' }] })
  await next()
  assert.equal(reads, 2, '合并的刷新仍须重新读取最新数据')
  fresh.resolve({ data: [{ id: 1, title_cn: '新标题' }] })
  await next()
  assert.equal(l.component.subs.value[0].title_cn, '新标题')
  l.leave()
})

test('有在途请求时离开列表不再执行排队的刷新', async () => {
  const response = held()
  let reads = 0
  const l = list(path => {
    if (path.includes('?')) return Promise.resolve({ data: [] })
    reads++
    return response.promise
  })
  l.tick()
  l.leave()
  response.resolve({ data: [] })
  await next()
  assert.equal(reads, 1)
})
