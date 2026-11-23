import test from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { view } from './view.mjs'
const require = createRequire(import.meta.url)

function loadWith(file, deps, globals = {}) {
  const source = readFileSync(new URL('../src/' + file, import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true } })
  const exports = {}
  runInNewContext(outputText, { exports, require: name => deps[name] ?? require(name), ...globals })
  return exports
}

test('登出状态打开下载深链接会保留原始路径、查询与片段', () => {
  let guard
  loadWith('router/index.ts', { 'vue-router': {
    createWebHistory() {}, createRouter: () => ({ beforeEach: fn => { guard = fn } }),
  } }, { localStorage: { getItem: () => null } })
  const next = guard({ path: '/downloads', fullPath: '/downloads?from=home#tasks' })
  assert.equal(next.name, 'login')
  assert.equal(next.query.redirect, '/downloads?from=home#tasks')
})

test('登录成功返回原站点已注册页面，普通登录及外部目标返回主页', async () => {
  for (const [candidate, expected] of [['/downloads?from=home#tasks', '/downloads?from=home#tasks'], ['//outside.example', '/'], ['/missing', '/'], ['/login', '/'], [null, '/']]) {
    let redirected
    const router = {
      resolve: path => ({ fullPath: path, name: path.startsWith('/downloads') ? 'downloads' : path === '/login' ? 'login' : undefined }),
      replace: target => { redirected = target },
    }
    const { component: c } = view('views/Login', { post: async () => ({ data: { token: 'new' } }) }, { route: { query: { redirect: candidate } }, router })
    c.username.value = 'admin'; c.password.value = 'test'
    await c.handleLogin()
    assert.equal(redirected, expected)
  }
})

test('登录失败显示服务端返回的错误', async () => {
  const { component: c } = view('views/Login', { post: async () => { throw { response: { data: { error: '用户名或密码错误' } } } } })
  c.username.value = 'a'; c.password.value = 'wrong'
  await c.handleLogin()
  assert.equal(c.error.value, '用户名或密码错误')
})

test('过期响应不能登出已更新的会话，当前会话过期保留返回页面', async () => {
  let rejectResponse
  const storage = new Map([['token', 'new']])
  let target
  const location = { pathname: '/downloads', search: '?from=home', hash: '#tasks', replace: url => { target = url } }
  loadWith('utils/request.ts', { axios: { create: () => ({ interceptors: {
    request: { use() {} }, response: { use(_resolve, reject) { rejectResponse = reject } },
  } }) } }, { window: { location }, localStorage: { getItem: key => storage.get(key) ?? null, removeItem: key => storage.delete(key) } })
  const unauthorized = token => ({ response: { status: 401 }, config: { headers: { Authorization: 'Bearer ' + token } } })
  await assert.rejects(rejectResponse(unauthorized('old')))
  assert.equal(storage.get('token'), 'new')
  assert.equal(target, undefined)
  await assert.rejects(rejectResponse(unauthorized('new')))
  assert.equal(storage.has('token'), false)
  assert.equal(target, '/login?redirect=' + encodeURIComponent('/downloads?from=home#tasks'))
})

test('时间表收到旧凭据的验证 401 后保留新会话，新凭据会重新验证', async () => {
  const storage = new Map([['token', 'old']])
  let finish
  const calls = []
  const module = loadWith('composables/useAuth.ts', { vue: require('vue'), 'vue-router': { useRouter: () => ({}) } }, {
    localStorage: { getItem: key => storage.get(key) ?? null, removeItem: key => storage.delete(key), setItem: (k,v) => storage.set(k,v) },
    fetch: (_path, args) => {
      calls.push(args.headers.Authorization)
      if (args.headers.Authorization === 'Bearer old') return new Promise(resolve => { finish = resolve })
      return Promise.resolve({ ok: true })
    },
  })
  const c = module.useAuth()
  const old = c.initAuth()
  storage.set('token','new')
  const fresh = c.initAuth()
  finish({ ok: false, status: 401 })
  await Promise.all([old,fresh])
  assert.equal(storage.get('token'), 'new')
  assert.equal(c.isAuthenticated.value, true)
  assert.deepEqual(calls, ['Bearer old','Bearer new'])
})

test('确认型请求只使用最初会话，其他请求照常应用当前令牌', () => {
  let beforeSend
  let token = 'old'
  loadWith('utils/request.ts', { axios: { CanceledError: Error, create: () => ({ interceptors: {
    request: { use(fn) { beforeSend = fn } }, response: { use() {} },
  } }) } }, { localStorage: { getItem: () => token } })
  assert.equal(beforeSend({ confirmedSession: 'old', headers: {} }).headers.Authorization, 'Bearer old')
  token = 'replacement'
  assert.throws(() => beforeSend({ confirmedSession: 'old', headers: {} }))
  assert.equal(beforeSend({ headers: {} }).headers.Authorization, 'Bearer replacement')
})
