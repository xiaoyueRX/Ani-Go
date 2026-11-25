import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import { createSSRApp, h } from 'vue'
import { compileScript, parse } from 'vue/compiler-sfc'
import { renderToString } from 'vue/server-renderer'
import ts from 'typescript'

const require = createRequire(import.meta.url)

function loadThemeModule(initialStorage = new Map(), initialDomTheme = 'anigo-dark') {
  const source = readFileSync(new URL('../src/composables/useTheme.ts', import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
  })
  let domTheme = initialDomTheme
  let metaColor = '#09090b'
  const document = {
    documentElement: {
      getAttribute: name => (name === 'data-theme' ? domTheme : null),
      setAttribute: (name, value) => { if (name === 'data-theme') domTheme = value },
    },
    querySelector: selector => (selector === 'meta[name="theme-color"]'
      ? { setAttribute: (_attr, value) => { metaColor = value } }
      : null),
  }
  const exports = {}
  runInNewContext(outputText, {
    exports,
    require: name => require(name),
    document,
    localStorage: {
      getItem: key => initialStorage.get(key) ?? null,
      setItem: (key, val) => initialStorage.set(key, String(val)),
    },
  })
  return {
    mod: exports,
    getDomTheme: () => domTheme,
    getMetaColor: () => metaColor,
  }
}

async function renderSfc(path, props = {}, slots = {}) {
  const source = readFileSync(new URL('../src/' + path + '.vue', import.meta.url), 'utf8')
  const { descriptor } = parse(source, { filename: path + '.vue' })
  const compiled = compileScript(descriptor, { id: path, inlineTemplate: true }).content
  const { outputText } = ts.transpileModule(compiled, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
  })
  const exports = {}
  runInNewContext(outputText, { exports, require: name => require(name) })
  const app = createSSRApp({
    render: () => h(exports.default, props, slots),
  })
  return renderToString(app)
}

test('useTheme 默认启用深色主题，非法存储值回退默认并支持持久化切换', () => {
  const storage = new Map([['ani-go-theme', 'unexpected-value']])
  const ctx = loadThemeModule(storage)
  assert.equal(ctx.mod.readStoredTheme(), 'anigo-dark')

  const { theme, isDark, toggleTheme, setTheme } = ctx.mod.useTheme()
  assert.equal(theme.value, 'anigo-dark')
  assert.equal(isDark.value, true)

  toggleTheme()
  assert.equal(theme.value, 'anigo-light')
  assert.equal(isDark.value, false)
  assert.equal(storage.get('ani-go-theme'), 'anigo-light')
  assert.equal(ctx.getDomTheme(), 'anigo-light')
  assert.equal(ctx.getMetaColor(), '#f5f3ee')

  setTheme('anigo-dark')
  assert.equal(theme.value, 'anigo-dark')
  assert.equal(storage.get('ani-go-theme'), 'anigo-dark')
  assert.equal(ctx.getDomTheme(), 'anigo-dark')
  assert.equal(ctx.getMetaColor(), '#09090b')
})

test('基础 UI 组件均可正确渲染标题、状态、无障碍属性与插槽', async () => {
  const headerHtml = await renderSfc(
    'components/ui/PageHeader',
    { title: '订阅管理', subtitle: '自动化追番列表', badge: 12 },
    { actions: () => h('button', { type: 'button' }, '新建') },
  )
  assert.match(headerHtml, /<h1[^>]*class="app-page-title[^"]*"[^>]*>订阅管理<\/h1>/)
  assert.match(headerHtml, /自动化追番列表/)
  assert.match(headerHtml, />12</)
  assert.match(headerHtml, />新建</)

  const surfaceHtml = await renderSfc(
    'components/ui/SurfaceCard',
    { title: '面板标题', description: '说明文本', muted: true },
    {
      headerActions: () => h('span', '操作'),
      default: () => h('p', '主体内容'),
      footer: () => h('span', '页脚内容'),
    },
  )
  assert.match(surfaceHtml, /app-surface-muted/)
  assert.match(surfaceHtml, /面板标题/)
  assert.match(surfaceHtml, /主体内容/)
  assert.match(surfaceHtml, /页脚内容/)

  const statButtonHtml = await renderSfc(
    'components/ui/StatCard',
    { label: '连载中', value: 8, tone: 'primary', clickable: true, active: true },
  )
  assert.match(statButtonHtml, /<button[^>]*type="button"[^>]*aria-pressed="true"/)
  assert.match(statButtonHtml, /连载中/)
  assert.match(statButtonHtml, /tabular-nums/)

  const statStaticHtml = await renderSfc(
    'components/ui/StatCard',
    { label: '累计完成', value: 24, clickable: false },
  )
  assert.doesNotMatch(statStaticHtml, /<button/)

  const tabsHtml = await renderSfc(
    'components/ui/SegmentedTabs',
    {
      modelValue: 'downloading',
      options: [
        { value: 'all', label: '全部', count: 5 },
        { value: 'downloading', label: '下载中', count: 2 },
      ],
    },
  )
  assert.match(tabsHtml, /role="tablist"/)
  assert.match(tabsHtml, /aria-selected="true"[^>]*data-active="true"/)
  assert.match(tabsHtml, /下载中/)

  const emptyHtml = await renderSfc(
    'components/ui/EmptyState',
    { title: '暂无记录', description: '可先添加订阅', hint: '支持关键词筛选' },
    { actions: () => h('button', '去发现') },
  )
  assert.match(emptyHtml, /app-empty-state/)
  assert.match(emptyHtml, /暂无记录/)
  assert.match(emptyHtml, /去发现/)

  const badgeHtml = await renderSfc(
    'components/ui/StatusBadge',
    { tone: 'success', dot: true },
    { default: () => '已入库' },
  )
  assert.match(badgeHtml, /text-success/)
  assert.match(badgeHtml, /bg-success/)
  assert.match(badgeHtml, /已入库/)
})
