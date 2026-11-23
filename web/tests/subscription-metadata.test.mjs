import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { createSSRApp } from 'vue'
import { parse } from 'vue/compiler-sfc'
import { renderToString } from 'vue/server-renderer'
import { createI18n } from 'vue-i18n'
import en from '../src/locales/en.ts'
import zh from '../src/locales/zh.ts'

// 直接渲染详情页里的元数据栏，使用生产模板和中英文词条。
const source = readFileSync(new URL('../src/views/SubscriptionDetail.vue', import.meta.url), 'utf8')
const { descriptor } = parse(source)
function findMetadataField(node) {
  if (node.children?.some(child => child.tag === 'p' && child.loc.source.includes('detail.stats.metadata'))) return node
  for (const child of node.children ?? []) {
    const field = findMetadataField(child)
    if (field) return field
  }
}
const template = findMetadataField(descriptor.template.ast).loc.source

for (const [locale, label, missing] of [
  ['zh', '元数据提供方', '未记录'],
  ['en', 'Metadata provider', 'Not recorded'],
]) {
  test(`订阅详情呈现实际元数据提供方（${locale}）`, async () => {
    const { t } = createI18n({ legacy: false, locale, messages: { zh, en } }).global
    for (const provider of [undefined, '', '  ', 'BGM.tv', 'TMDB', 'yuc.wiki']) {
      const sub = { source_name: 'Mikan', metadata_provider: provider }
      const html = await renderToString(createSSRApp({ template, setup: () => ({ sub, t }) }))
      const values = [...html.matchAll(/<p\b[^>]*>(.*?)<\/p>/gs)].map(match => match[1].trim())
      assert.deepEqual(values, [label, provider?.trim() || missing])
    }
  })
}
