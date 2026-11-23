import assert from 'node:assert/strict'
import { test } from 'node:test'
import { view } from './view.mjs'

function subscriptionList(t) {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const sources = []
  class EventSource {
    closed = false
    constructor() { sources.push(this) }
    close() { this.closed = true }
    disconnect() { this.onerror?.() }
  }
  const { mounted, unmounted } = view('views/Subscriptions', { get: async () => ({ data: [] }) }, {
    globals: { document: new EventTarget(), EventSource, setTimeout, clearTimeout },
  })
  mounted.forEach(fn => fn())
  return { sources, leave: () => unmounted.forEach(fn => fn()) }
}

test('留在订阅页时断开的事件流会在五秒后重连一次', t => {
  const { sources, leave } = subscriptionList(t)
  assert.equal(sources.length, 1)
  sources[0].disconnect()
  assert.ok(sources[0].closed)
  t.mock.timers.tick(4999)
  assert.equal(sources.length, 1)
  t.mock.timers.tick(1)
  assert.equal(sources.length, 2)
  t.mock.timers.tick(5000)
  assert.equal(sources.length, 2)
  leave()
  assert.ok(sources[1].closed)
})

test('事件流断开后离开订阅页会取消尚未触发的重连', t => {
  const { sources, leave } = subscriptionList(t)
  sources[0].disconnect()
  t.mock.timers.tick(4999)
  leave()
  t.mock.timers.tick(10001)
  assert.equal(sources.length, 1, '离开后不应发起新的事件流连接')
})
