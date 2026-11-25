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
  const attrs = new Map([['data-theme', initialDomTheme]])
  const cssVars = new Map()
  let metaColor = '#09090b'
  const document = {
    documentElement: {
      getAttribute: name => attrs.get(name) ?? null,
      setAttribute: (name, value) => attrs.set(name, String(value)),
      style: {
        setProperty: (k, v) => cssVars.set(k, String(v)),
      },
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
    getDomTheme: () => attrs.get('data-theme'),
    getAttr: name => attrs.get(name),
    getCssVar: name => cssVars.get(name),
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

test('官方预设主题包含多套风格并支持即时切换结构与配色变量', () => {
  const storage = new Map()
  const ctx = loadThemeModule(storage)
  const { builtinThemes, activePackId, activeStyleId, selectTheme, selectStyle } = ctx.mod.useTheme()

  assert.equal(builtinThemes.length, 3)
  for (const pack of builtinThemes) {
    assert.ok(pack.styles.length >= 3, pack.id)
    assert.ok(pack.styles.some(s => s.mode === 'anigo-dark'), `${pack.id} 应含暗色`)
    assert.ok(pack.styles.some(s => s.mode === 'anigo-light'), `${pack.id} 应含亮色`)
  }

  selectTheme('studio-grid', 'carbon-amber')
  assert.equal(activePackId.value, 'studio-grid')
  assert.equal(activeStyleId.value, 'carbon-amber')
  assert.equal(ctx.getAttr('data-theme-pack'), 'studio-grid')
  assert.equal(ctx.getAttr('data-theme-style'), 'carbon-amber')
  assert.equal(ctx.getDomTheme(), 'anigo-dark')
  assert.equal(ctx.getCssVar('--v2-accent'), '#f59e0b')
  assert.equal(ctx.getCssVar('--v2-radius-panel'), '0.375rem')
  assert.equal(ctx.getMetaColor(), '#0a0b0d')

  selectStyle('blueprint-light')
  assert.equal(activeStyleId.value, 'blueprint-light')
  assert.equal(ctx.getDomTheme(), 'anigo-light')
  assert.equal(ctx.getCssVar('--v2-accent'), '#2563eb')
  assert.equal(ctx.getMetaColor(), '#f1f5f9')
})

test('支持导入与导出多色彩第三方主题，拒绝非法 CSS 注入并在删除后回退默认', () => {
  const storage = new Map()
  const ctx = loadThemeModule(storage)
  const {
    importCustomTheme,
    selectTheme,
    exportThemeJson,
    removeCustomTheme,
    customThemes,
    activePackId,
    activeStyleId,
    validateThemePack,
  } = { ...ctx.mod.useTheme(), validateThemePack: ctx.mod.validateThemePack }

  const validJson = JSON.stringify(ctx.mod.THIRD_PARTY_THEME_TEMPLATE)
  const imported = importCustomTheme(validJson)
  assert.equal(imported.ok, true)
  assert.equal(customThemes.value.length, 1)

  selectTheme('neo-tokyo-custom', 'neon-magenta')
  assert.equal(activePackId.value, 'neo-tokyo-custom')
  assert.equal(activeStyleId.value, 'neon-magenta')
  assert.equal(ctx.getCssVar('--v2-accent'), '#e879f9')

  const exported = exportThemeJson('neo-tokyo-custom')
  assert.match(exported, /"schema": "anigo-theme-v1"/)
  assert.match(exported, /neon-magenta/)

  removeCustomTheme('neo-tokyo-custom')
  assert.equal(customThemes.value.length, 0)
  assert.equal(activePackId.value, 'cinema')
  assert.equal(activeStyleId.value, 'obsidian-vermilion')

  const malicious = validateThemePack({
    schema: 'anigo-theme-v1',
    id: 'bad-theme',
    name: '恶意主题',
    styles: [{
      id: 'evil',
      name: 'Evil',
      mode: 'anigo-dark',
      colors: { bg: '#000; background: url(https://evil)', surface: '#111', ink: '#fff', accent: '#f00' },
    }],
  })
  assert.equal(malicious.ok, false)

  const conflict = validateThemePack({
    schema: 'anigo-theme-v1',
    id: 'cinema',
    name: 'Conflict',
    styles: [{
      id: 'c1',
      name: 'C1',
      mode: 'anigo-dark',
      colors: { bg: '#000000', surface: '#111111', ink: '#ffffff', accent: '#ff5c28' },
    }],
  })
  assert.equal(conflict.ok, false)
})

test('主题配置支持服务端设置双向同步，并安全忽略损坏的服务端自定义包', () => {
  const storage = new Map()
  const ctx = loadThemeModule(storage)
  const {
    importCustomTheme,
    selectTheme,
    getServerSettingsPayload,
    hydrateFromServerSettings,
    customThemes,
    activePackId,
    activeStyleId,
  } = ctx.mod.useTheme()

  const imported = importCustomTheme(JSON.stringify(ctx.mod.THIRD_PARTY_THEME_TEMPLATE))
  assert.equal(imported.ok, true)
  selectTheme('neo-tokyo-custom', 'silver-daylight')

  const payload = getServerSettingsPayload()
  assert.equal(payload.UI_THEME_PACK, 'neo-tokyo-custom')
  assert.equal(payload.UI_THEME_STYLE, 'silver-daylight')
  assert.match(payload.UI_CUSTOM_THEMES, /"neo-tokyo-custom"/)

  // 切到另一台设备（清空上下文，只注入服务端 settings）
  const freshCtx = loadThemeModule(new Map())
  const fresh = freshCtx.mod.useTheme()
  fresh.hydrateFromServerSettings({
    UI_THEME_PACK: 'neo-tokyo-custom',
    UI_THEME_STYLE: 'silver-daylight',
    UI_CUSTOM_THEMES: payload.UI_CUSTOM_THEMES,
  })
  assert.equal(fresh.activePackId.value, 'neo-tokyo-custom')
  assert.equal(fresh.activeStyleId.value, 'silver-daylight')
  assert.equal(fresh.customThemes.value.length, 1)

  // 注入恶意/不存在的服务端包时保持当前合法主题且不接受非法内容
  fresh.hydrateFromServerSettings({
    UI_THEME_PACK: 'nonexistent-pack',
    UI_THEME_STYLE: 'unknown',
    UI_CUSTOM_THEMES: '[{"schema":"bad","id":"x"}]',
  })
  assert.equal(fresh.activePackId.value, 'neo-tokyo-custom')
  assert.equal(fresh.customThemes.value.length, 1)
  assert.equal(activePackId.value, 'neo-tokyo-custom')
  assert.equal(activeStyleId.value, 'silver-daylight')
  assert.equal(customThemes.value.length, 1)
})
