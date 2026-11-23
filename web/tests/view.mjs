import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { runInNewContext } from 'node:vm'
import { compileScript, parse } from 'vue/compiler-sfc'
import ts from 'typescript'
import * as vue from 'vue'

const require = createRequire(import.meta.url)

// 在隔离上下文执行 Vue 组件，保留响应式状态，只替代浏览器和外部 HTTP。
export function view(path, request, { route = {}, storage = new Map(), props = {}, router = { push() {}, replace() {} }, translate = key => key, translationExists = () => false, globals = {} } = {}) {
  const source = readFileSync(new URL('../src/' + path + '.vue', import.meta.url), 'utf8')
  const { descriptor } = parse(source, { filename: path + '.vue' })
  const script = compileScript(descriptor, { id: path }).content
  const { outputText } = ts.transpileModule(script, { compilerOptions: {
    module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true,
  } })
  const mounted = []
  const notices = []
  const window = { showToast: (message, type) => notices.push({ message, type }), location: {} }
  const modules = {
    vue: { ...vue, onMounted: fn => mounted.push(fn), onUnmounted() {} },
    'vue-router': { useRouter: () => router, useRoute: () => ({ params: {}, query: {}, ...route }) },
    'vue-i18n': { useI18n: () => ({ t: translate, te: translationExists, locale: vue.ref('en') }) },
    '../utils/request': request,
    '../utils/setupDownloader': require('../src/utils/setupDownloader.ts'),
    '../utils/dialogSession': require('../src/utils/dialogSession.ts'),
    '../utils/loginRedirect': require('../src/utils/loginRedirect.ts'),
    './utils/loginRedirect': require('../src/utils/loginRedirect.ts'),
    '../composables/useVersion': { currentVersion: vue.ref(''), useVersion: () => ({}) },
  }
  const exports = {}
  runInNewContext(outputText, {
    ...globals, exports, console, window, AbortController, URL, crypto: globalThis.crypto,
    require: name => modules[name] ?? (name.endsWith('.vue') ? {} : require(name)),
    confirm: () => true, alert() {}, setTimeout() {}, clearTimeout() {}, setInterval() {}, clearInterval() {},
    localStorage: { getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key) },
  })
  const emitted = []
  const component = exports.default.setup(props, { expose() {}, emit: (...args) => emitted.push(args) })
  return { component, notices, mounted, emitted }
}
