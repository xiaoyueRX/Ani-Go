import assert from 'node:assert/strict'
import { test } from 'node:test'
import { installAssetRecovery } from '../src/router/assetRecovery.ts'

function pending() {
  let resolve
  const promise = new Promise(yes => { resolve = yes })
  return { promise, resolve }
}

function browser(t, currentPath = '/subscriptions') {
  const old = '/assets/index-aaaaaaaa.js', next = '/assets/index-bbbbbbbb.js'
  const doc = entry => ({ querySelector: () => entry ? { getAttribute: () => entry } : null })
  const hooks = {}
  const calls = [], loads = [], jumps = []
  const stored = new Map()
  const storage = {
    getItem: key => stored.get(key) ?? null,
    setItem: (key, value) => stored.set(key, value),
    removeItem: key => stored.delete(key),
  }
  const window = new EventTarget()
  const location = Object.assign(new URL(currentPath, 'https://example.test'), {
    assign: href => loads.push(['assign', href]), replace: href => loads.push(['replace', href]),
  })
  let page = pending()
  function finish(result) {
    page.resolve(result)
    page = pending()
  }
  const globals = {
    window, document: doc(old), location, sessionStorage: storage,
    fetch: (...args) => { calls.push(args); return page.promise },
    DOMParser: class { parseFromString(entry) { return doc(entry) } },
  }
  for (const [key, value] of Object.entries(globals)) {
    const previous = Object.getOwnPropertyDescriptor(globalThis, key)
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value })
    t.after(() => previous ? Object.defineProperty(globalThis, key, previous) : delete globalThis[key])
  }
  t.mock.method(console, 'error', () => {})
  const history = {
    state: { position: 3 },
    listen: fn => { hooks.pop = fn },
    go: (...args) => jumps.push(args),
  }
  const router = {
    options: { history },
    beforeEach: fn => { hooks.before = fn },
    afterEach: fn => { hooks.after = fn },
    onError: fn => { hooks.error = fn },
    resolve: href => ({ href }),
  }
  installAssetRecovery(router)
  function begin(fullPath) {
    const to = { fullPath }
    hooks.before?.(to)
    return to
  }
  function fail(to, preload = true) {
    const err = new Error('不依赖浏览器的错误文字')
    if (preload) window.dispatchEvent(Object.assign(new Event('vite:preloadError'), { payload: err }))
    hooks.error?.(err, to)
  }
  return {
    old, begin, fail, hooks, calls, loads, window, history, jumps,
    reload: () => installAssetRecovery(router),
    respond(entry = next, status = 200) { finish(new Response(entry, { status, headers: { 'Content-Type': 'text/html' } })) },
    offline() { finish(Promise.reject(new TypeError('offline'))) },
  }
}

const settle = () => new Promise(resolve => setImmediate(resolve))
test('仅路由资源失败且入口已换时直接打开完整目的地址', async t => {
  const b = browser(t), to = b.begin('/settings?tab=plugins#cloud')
  b.fail(to, false)
  assert.equal(b.calls.length, 0)
  b.fail(to)
  await settle()
  assert.equal(b.calls.length, 1)
  assert.equal(b.calls[0][0], '/') // v0.7.0 会将 /index.html 重定向至首页。
  assert.equal(b.calls[0][1].cache, 'no-store')
  b.respond()
  await settle()
  assert.deepEqual(b.loads, [['assign', to.fullPath]])
})

test('多个未完成的跳转共用入口校验，只有最新目标可以打开', async t => {
  const b = browser(t)
  b.fail(b.begin('/downloads'))
  const last = b.begin('/settings?tab=plugins')
  b.fail(last)
  b.fail(last)
  await settle()
  assert.equal(b.calls.length, 1)
  b.respond()
  await settle()
  assert.deepEqual(b.loads, [['assign', last.fullPath]])
})

test('等待新版入口时已成功导航会使此前的恢复失效', async t => {
  const b = browser(t), first = b.begin('/downloads')
  b.fail(first)
  await settle()
  assert.equal(b.calls.length, 1)
  const after = b.begin('/schedule')
  b.hooks.after?.(after, first)
  b.respond()
  await settle()
  assert.deepEqual(b.loads, [])
})

for (const kind of ['same', 'offline', 'foreign']) {
  test('入口未确认更换时不整页跳转或密集探测：' + kind, async t => {
    const b = browser(t), to = b.begin('/downloads')
    b.fail(to)
    await settle()
    assert.equal(b.calls.length, 1)
    if (kind === 'offline') b.offline()
    else b.respond(kind === 'same' ? b.old : 'https://other.test/assets/index-cccccccc.js')
    await settle()
    b.fail(to)
    await settle()
    assert.equal(b.calls.length, 1)
    assert.deepEqual(b.loads, [])
  })
}

test('初次直达的深链保留当前历史项，异常重载只能尝试一次', async t => {
  const path = '/settings?tab=plugins#local'
  const b = browser(t, path)
  b.fail(b.begin(path))
  b.respond()
  await settle()
  assert.deepEqual(b.loads, [['replace', path]])
  b.reload()
  b.fail(b.begin(path))
  b.respond()
  await settle()
  assert.deepEqual(b.loads, [['replace', path]])
})

for (const correctionAlreadyFinished of [true, false]) {
  test('后退失败时保留已有历史项，浏览器已回到起点：' + correctionAlreadyFinished, async t => {
    const b = browser(t)
    b.history.state.position = 2
    b.hooks.pop?.('/settings', '/subscriptions', { delta: -1 })
    const to = b.begin('/settings')
    b.fail(to)
    if (correctionAlreadyFinished) b.history.state.position = 3
    b.respond()
    await settle()
    if (!correctionAlreadyFinished) {
      assert.deepEqual(b.jumps, [])
      b.history.state.position = 3
      b.window.dispatchEvent(new Event('popstate'))
    }
    assert.deepEqual(b.jumps, [[-1, false]])
    assert.deepEqual(b.loads, [])
    b.history.state.position = 2
    b.window.dispatchEvent(new Event('popstate'))
    assert.deepEqual(b.loads, [['replace', '/settings']])
  })
}

test('预加载抛出非对象时保留原始错误，忽略非标准负载', async t => {
  const b = browser(t), to = b.begin('/downloads')
  b.window.dispatchEvent(Object.assign(new Event('vite:preloadError'), { payload: 'offline' }))
  b.hooks.error?.('offline', to)
  await settle()
  assert.equal(b.calls.length, 0)
})

for (const unavailable of ['offline', '503']) {
  test('整站暂不可用后，仅再次打开相同目标才恢复已失败的模块：' + unavailable, async t => {
    const b = browser(t), path = '/settings?tab=plugins#local'
    const first = b.begin(path)
    b.fail(first)
    if (unavailable === 'offline') b.offline()
    else b.respond(null, 503)
    await settle()
    b.window.dispatchEvent(new Event('online'))
    b.fail(first)
    await settle()
    assert.equal(b.calls.length, 1)
    assert.deepEqual(b.loads, [])
    b.fail(b.begin(path))
    assert.equal(b.calls.length, 2)
    b.respond(b.old)
    await settle()
    assert.deepEqual(b.loads, [['assign', path]])
  })
}

test('整站仍不可用时反复点击保留当前文档，改往别处会取消原目标重试', async t => {
  const b = browser(t), path = '/settings'
  b.fail(b.begin(path))
  b.offline()
  await settle()
  b.fail(b.begin(path))
  b.respond(null, 503)
  await settle()
  assert.deepEqual(b.loads, [])
  b.fail(b.begin(path))
  b.begin('/downloads')
  b.respond(b.old)
  await settle()
  assert.deepEqual(b.loads, [])
  b.fail(b.begin(path))
  b.respond(b.old)
  await settle()
  assert.deepEqual(b.loads, [])
})

test('仅模块反复出错或首页返回无效响应，重复用户导航也不刷新', async t => {
  const b = browser(t), path = '/downloads'
  let now = 100000
  t.mock.method(Date, 'now', () => now)
  b.fail(b.begin(path))
  b.respond(null, 404)
  await settle()
  now += 6000
  b.fail(b.begin(path))
  assert.equal(b.calls.length, 2)
  b.respond(b.old)
  await settle()
  b.fail(b.begin(path))
  await settle()
  assert.equal(b.calls.length, 2)
  assert.deepEqual(b.loads, [])
})
