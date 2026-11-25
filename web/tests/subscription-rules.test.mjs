import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const require = createRequire(import.meta.url)

function loadRulesModule() {
  const source = readFileSync(new URL('../src/utils/subscriptionRules.ts', import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
  })
  const exports = {}
  runInNewContext(outputText, { exports, require, URL })
  return exports
}

const clone = v => JSON.parse(JSON.stringify(v))

test('订阅规则解析并兼容旧 keywords/exclude 写法，同时保留扩展字段', () => {
  const { parseRuleState, serializeRuleState } = loadRulesModule()

  const parsed = clone(parseRuleState(
    JSON.stringify({
      resolution: '1080p',
      keywords: ['简日', 'CHS', 'chs'],
      exclude: ['RAW'],
      source_url: 'https://mikanani.me/Home/Bangumi/123',
    }),
    JSON.stringify(['720p', 'raw']),
    JSON.stringify(['LoliHouse', 'ANi', 'anI']),
  ))

  assert.equal(parsed.resolution, '1080p')
  assert.deepEqual(parsed.includeKeywords, ['简日', 'CHS'])
  assert.deepEqual(parsed.excludeKeywords, ['RAW', '720p'])
  assert.deepEqual(parsed.allowedSubgroups, ['LoliHouse', 'ANi'])
  assert.deepEqual(parsed.extraFilterKeys, { source_url: 'https://mikanani.me/Home/Bangumi/123' })

  const serialized = serializeRuleState(parsed)
  assert.deepEqual(JSON.parse(serialized.filter_json), {
    source_url: 'https://mikanani.me/Home/Bangumi/123',
    resolution: '1080p',
    include_keywords: ['简日', 'CHS'],
  })
  assert.deepEqual(JSON.parse(serialized.excluded_keywords), ['RAW', '720p'])
  assert.deepEqual(JSON.parse(serialized.allowed_subgroups), ['LoliHouse', 'ANi'])
})

test('清空全部规则且无扩展键时返回空 filter_json 与空数组串', () => {
  const { serializeRuleState, summarizeRules } = loadRulesModule()
  const out = serializeRuleState({
    resolution: '',
    includeKeywords: [],
    excludeKeywords: [],
    allowedSubgroups: [],
    extraFilterKeys: {},
  })
  assert.equal(out.filter_json, '')
  assert.equal(out.excluded_keywords, '[]')
  assert.equal(out.allowed_subgroups, '[]')

  const summary = clone(summarizeRules({
    rss_url: 'https://mikanani.me/RSS/Bangumi?bangumiId=1',
    filter_json: '{"resolution":"2160p","include_keywords":["4K","简繁"]}',
    excluded_keywords: '["HEVC"]',
    allowed_subgroups: '["桜都字幕组"]',
    custom_path: '/media/Anime/New',
  }))
  assert.deepEqual(summary.map(s => s.kind), ['rss', 'resolution', 'include', 'exclude', 'subgroups', 'path'])
})

test('损坏的 JSON 输入不会抛出异常并可安全降级为空状态', () => {
  const { parseRuleState } = loadRulesModule()
  const parsed = clone(parseRuleState('{invalid-json', 'not-array', 'broken'))
  assert.equal(parsed.resolution, '')
  assert.deepEqual(parsed.includeKeywords, [])
  assert.deepEqual(parsed.excludeKeywords, [])
  assert.deepEqual(parsed.allowedSubgroups, [])
})

test('Mikan 字幕组选择按单选、多选与留空分支正确构造 RSS 与白名单', () => {
  const { stripMikanSubgroupParam, resolveMikanSelectionPayload } = loadRulesModule()
  const groups = [
    { name: 'ANi', rss_url: 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=583' },
    { name: 'LoliHouse', rss_url: 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=370' },
  ]

  assert.equal(
    stripMikanSubgroupParam('https://mikanani.me/RSS/Bangumi?bangumiId=3141&SubGroupId=583'),
    'https://mikanani.me/RSS/Bangumi?bangumiId=3141',
  )
  assert.equal(stripMikanSubgroupParam('not-a-url'), '')

  const one = clone(resolveMikanSelectionPayload(groups, ['LoliHouse']))
  assert.equal(one.subgroup_name, 'LoliHouse')
  assert.deepEqual(one.allowed_subgroups, ['LoliHouse'])
  assert.equal(one.rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=370')

  const many = clone(resolveMikanSelectionPayload(groups, ['ANi', 'LoliHouse']))
  assert.equal(many.subgroup_name, '')
  assert.deepEqual(many.subgroup_names, ['ANi', 'LoliHouse'])
  assert.deepEqual(many.allowed_subgroups, ['ANi', 'LoliHouse'])
  assert.equal(many.rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141')

  const none = clone(resolveMikanSelectionPayload(groups, []))
  assert.equal(none.subgroup_name, undefined)
  assert.equal(none.allowed_subgroups, undefined)
  assert.equal(none.rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141')
})
