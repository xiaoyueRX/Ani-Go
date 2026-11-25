import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import * as vue from 'vue'
import { compileScript, parse } from 'vue/compiler-sfc'
import ts from 'typescript'

const require = createRequire(import.meta.url)
const read = name => readFileSync(new URL('../src/' + name, import.meta.url), 'utf8')
const layout = compileScript(parse(read('views/Layout.vue')).descriptor, { id: 'layout' }).content

function session(health, update = {}) {
  const calls = []
  const storage = new Map()
  const mounted = []
  const responses = {
    '/me': { username: 'test' },
    '/settings': { AUTO_CHECK_UPDATE: 'false' },
    '/version': { version: 'v0.6.0', changelog: ['旧版本说明'] },
    '/system/update/check': update,
  }
  const modules = {
    vue: { ...vue, onMounted: fn => mounted.push(fn) },
    'vue-router': { useRouter: () => ({}), useRoute: () => ({}) },
    'vue-i18n': { useI18n: () => ({}) },
    '../i18n': { setAppLocale: async () => {} },
    '../utils/request': { get: async path => {
      calls.push(path)
      assert.ok(Object.hasOwn(responses, path), path)
      return { data: responses[path] }
    } },
  }
  function load(source) {
    const exports = {}
    const { outputText } = ts.transpileModule(source, { compilerOptions: {
      module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true,
    } })
    runInNewContext(outputText, {
      exports, require: name => modules[name] ?? require(name), console,
      window: { addEventListener() {} },
      localStorage: { getItem: key => storage.get(key), setItem: (key, val) => storage.set(key, val) },
      fetch: async url => {
        calls.push(url)
        assert.equal(url, '/api/health')
        return typeof health === 'function' ? health() : { ok: true, json: async () => health }
      },
    })
    return exports
  }
  const version = load(read('composables/useVersion.ts'))
  modules['../composables/useVersion'] = version
  load(layout).default.setup({}, { expose() {} })
  return {
    version: version.useVersion(), calls, storage,
    mount: async () => {
      await Promise.all(mounted.map(fn => fn()))
      await new Promise(resolve => setImmediate(resolve))
    },
  }
}

test('直接打开布局且关闭自动更新时显示本地运行版本', async () => {
  const { version, calls, mount } = session({ status: 'ok', version: 'v0.7.0-rc.1' })
  await mount()
  assert.equal(version.currentVersion.value, 'v0.7.0-rc.1')
  assert.ok(!calls.includes('/system/update/check'))
})

test('等待健康响应或服务不可用时不显示具体版本', async () => {
  const { version, mount } = session(() => { throw new TypeError('offline') })
  assert.doesNotMatch(version.currentVersion.value, /\d+\.\d+/)
  await mount()
  assert.doesNotMatch(version.currentVersion.value, /\d+\.\d+/)
})

test('更新日志和更新结果保留各自的功能，不覆盖健康接口的版本', async () => {
  const { version, mount, storage } = session({ status: 'ok', version: 'v0.7.0' }, {
    current_version: 'v0.6.0', latest_version: 'v0.8.0', has_update: true,
    can_self_update: false, environment: 'docker', release_notes: '新版说明',
  })
  await mount()
  storage.set('ani-go-last-version', 'v0.5.0')
  await version.checkVersion()
  assert.equal(version.showChangelog.value, true)
  assert.equal(version.changelog.value[0], '旧版本说明')
  assert.equal(version.currentVersion.value, 'v0.7.0')
  const result = await version.checkGitHubUpdate()
  assert.equal(result.hasUpdate, true)
  assert.equal(result.latest, 'v0.8.0')
  assert.equal(version.canSelfUpdate.value, false)
  assert.equal(version.currentVersion.value, 'v0.7.0')
})

test('后端以 HTTP 200 报告上游不可用时，手动检查返回失败', async () => {
  const { mount, version } = session({ status: 'ok', version: 'v0.7.0' }, {
    current_version: 'v0.7.0', has_update: false, error: 'release server unavailable',
  })
  await mount()
  const result = await version.checkGitHubUpdate(true)
  assert.equal(result.checked, false)
  assert.equal(result.error, 'release server unavailable')
  assert.equal(version.currentVersion.value, 'v0.7.0')
})
