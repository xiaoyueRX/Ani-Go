import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { createSSRApp } from 'vue'
import { parse } from 'vue/compiler-sfc'
import { renderToString } from 'vue/server-renderer'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ChevronLeft, Plus, Search } from 'lucide-vue-next'
import en from '../src/locales/en.ts'
import zh from '../src/locales/zh.ts'

test('搜索页首次打开就有可访问的手动添加订阅链接', async () => {
  const source = readFileSync(new URL('../src/views/Search.vue', import.meta.url), 'utf8')
  const root = parse(source).descriptor.template.ast.children.find(node => node.tag)
  const header = root.children.find(node => node.tag)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/subscriptions/new', component: {} }] })
  for (const [locale, label] of [['zh', '手动添加订阅'], ['en', 'Add subscription manually']]) {
    const app = createSSRApp({ template: header.loc.source, components: { ChevronLeft, Plus, Search }, setup: () => ({ results: [] }) })
    app.use(router).use(createI18n({ legacy: false, locale, messages: { zh, en } }))
    const html = await renderToString(app)
    const link = html.match(/<a\b[^>]*href="\/subscriptions\/new"[^>]*>([\s\S]*?)<\/a>/)?.[1]
    assert.ok(link?.includes(label), locale)
  }
})
