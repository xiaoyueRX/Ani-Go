import assert from 'node:assert/strict'
import test from 'node:test'
import { routeAssetScope } from '../src/router/assetRecovery.ts'

const original = '/assets/index-a1234567.js'
const integrity = 'sha256-' + 'A'.repeat(43) + '='
const layout = 'src/views/Layout.vue', search = 'src/views/Search.vue'
const files = {
  'index.html': { file: original.slice(1), isEntry: true, dynamicImports: [layout, search, 'src/views/SettingsPage.vue'], css: ['assets/index-b1234567.css'] },
  [layout]: { src: layout, isDynamicEntry: true, file: 'assets/Layout-c1234567.js', imports: ['index.html'] },
  [search]: { src: search, isDynamicEntry: true, file: 'assets/Search-d1234567.js', imports: ['_timer.js'], css: ['assets/Search-e1234567.css'] },
  '_timer.js': { file: 'assets/timer-f1234567.js', imports: ['_main.js'] },
  '_main.js': { file: 'assets/main-g1234567.js', imports: ['index.html', '_timer.js'] },
  'src/views/SettingsPage.vue': { file: 'assets/Settings-h1234567.js' },
}
const response = data => Response.json(data)

test('按入口及所有匹配视图读取静态依赖和样式，成功的清单只取一次', async t => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => response(files))
  const scope = routeAssetScope(original, integrity)
  assert.equal(fetch.mock.callCount(), 0)
  const [target, parent] = await Promise.all([scope([layout, search]), scope([layout])])
  assert.deepEqual(target, new Set([original, '/assets/index-b1234567.css', '/assets/Layout-c1234567.js',
    '/assets/Search-d1234567.js', '/assets/Search-e1234567.css', '/assets/timer-f1234567.js', '/assets/main-g1234567.js']))
  assert.deepEqual(parent, new Set([original, '/assets/index-b1234567.css', '/assets/Layout-c1234567.js']))
  assert.equal(fetch.mock.callCount(), 1)
  const [url, options] = fetch.mock.calls[0].arguments
  assert.equal(url, '/route-manifest.json')
  assert.equal(options.cache, 'no-store')
  assert.equal(options.redirect, 'error')
  assert.equal(options.integrity, integrity)
})

test('未绑定或外部来源的路由保守恢复，无需请求清单', async t => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => response(files))
  const scope = routeAssetScope(original, integrity)
  for (const sources of [[], [layout, undefined], ['https://other.test/route.vue'], ['../views/Settings.vue']]) {
    assert.equal(await scope(sources), undefined)
  }
  assert.equal(fetch.mock.callCount(), 0)
})

const bad = {
  '不可访问': () => { throw new Error('offline') },
  '不存在': () => new Response(null, { status: 404 }),
  '旧包返回 HTML': () => new Response('<html>', { headers: { 'Content-Type': 'text/html' } }),
  '换过入口': () => response({ ...files, 'index.html': { file: 'assets/index-oldhash.js' } }),
  '错误的源模块绑定': () => response({ ...files, [search]: { ...files[search], src: layout } }),
  '缺失组件': () => response({ ...files, [search]: undefined }),
  '缺失传递依赖': () => response({ ...files, '_main.js': undefined }),
  '外部脚本': () => response({ ...files, [search]: { file: 'https://other.test/assets/Search-newhash.js' } }),
  '目录穿越': () => response({ ...files, [search]: { file: 'assets/../api/Search-newhash.js' } }),
  '外部样式': () => response({ ...files, [search]: { ...files[search], css: ['//other.test/app.css'] } }),
  '接口路径': () => response({ ...files, [search]: { ...files[search], css: ['api/backup.css'] } }),
  '异常导入键': () => response({ ...files, [search]: { ...files[search], imports: ['__proto__'] } }),
  '损坏 JSON': () => new Response('{', { headers: { 'Content-Type': 'application/json' } }),
}
for (const [title, invalid] of Object.entries(bad)) {
  test('无可验证的完整依赖时保守降级，下次可重试：' + title, async t => {
    let attempts = 0
    t.mock.method(globalThis, 'fetch', async () => ++attempts === 1 ? invalid() : response(files))
    const scope = routeAssetScope(original, integrity)
    assert.equal(await scope([layout, search]), undefined)
    assert.ok((await scope([layout, search]))?.has('/assets/timer-f1234567.js'))
    assert.equal(attempts, 2)
  })
}

test('当前文档没有构建的完整性摘要时保守恢复，不请求清单', async t => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => response(files))
  for (const digest of [null, '', 'sha256-broken']) assert.equal(await routeAssetScope(original, digest)([search]), undefined)
  assert.equal(fetch.mock.callCount(), 0)
})
