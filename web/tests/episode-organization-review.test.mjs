import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { createSSRApp } from 'vue'
import { parse } from 'vue/compiler-sfc'
import { renderToString } from 'vue/server-renderer'
import { createI18n } from 'vue-i18n'
import * as icons from 'lucide-vue-next'
import en from '../src/locales/en.ts'
import zh from '../src/locales/zh.ts'
import { view } from './view.mjs'

const source = readFileSync(new URL('../src/views/SubscriptionDetail.vue', import.meta.url), 'utf8')
function groupTemplate(node) {
  if (node.props?.some(prop => prop.type === 7 && prop.name === 'for' && prop.exp?.content.includes('groupedEpisodes'))) return node.loc.source
  return node.children?.map(groupTemplate).find(Boolean)
}
const template = groupTemplate(parse(source).descriptor.template.ast)
function review(overrides = {}) {
  return { id: 41, status: 'needs_review', review_kind: '', can_retry_organize: true, review_reason: 'The destination was unavailable', season: 1, number: 0,
    group_name: 'Harbor', manual_status: 'pending', ...overrides }
}
function setup(rows, request = {}, locale = 'en') {
  const { t } = createI18n({ legacy: false, locale, messages: { en, zh } }).global
  const { component } = view('views/SubscriptionDetail', {
    get: async () => ({ data: { subscription: { id: 2 }, episodes: rows } }), ...request,
  }, { route: { params: { id: 2 } }, translate: t })
  component.episodes.value = rows
  return component
}
async function renderGroups(component) {
  const app = createSSRApp({ template, setup: () => ({ ...component }) })
  for (const icon of ['RefreshCw', 'AlertTriangle']) app.component(icon, icons[icon])
  return renderToString(app)
}

for (const [locale, badge, retry, assign] of [
  ['zh', '待复核', '重新整理', '指定季集'],
  ['en', 'Needs review', 'Retry organization', 'Assign season and episode'],
]) {
  test(`折叠组及剧集行显示复核状态、原因与适用操作（${locale}）`, async () => {
    const c = setup([review(), review({ id: 42, status: 'organized', review_reason: 'stale reason' })], {}, locale)
    const html = await renderGroups(c)
    assert.ok(html.slice(0, html.indexOf('<table')).includes(badge))
    const rows = html.split('<tbody')[1].split('</tr>')
    assert.ok(rows[0].includes(badge))
    assert.ok(rows[0].includes('The destination was unavailable'))
    assert.ok(rows[0].includes(retry))
    assert.ok(!rows[0].includes(assign))
    assert.ok(!rows[1].includes('stale reason'))
    assert.ok(!rows[1].includes(retry))
    const ep = c.episodes.value[0]
    ep.can_assign_episode = true
    ep.review_kind = 'episode_identity'
    ep.can_retry_organize = false
    ep.source_file = 'Disc 1/Harbor Special.mkv'
    c.editAssignment(ep)
    const edit = await renderGroups(c)
    assert.ok(edit.includes('Disc 1/Harbor Special.mkv'))
    assert.match(edit, /type="number"/)
  })
}

test('普通重试只提交一次，不改变手动标记，重试后显示服务端进度', async () => {
  const calls = []
  const row = review()
  const done = Promise.withResolvers()
  const c = setup([row], {
    post: (...args) => { calls.push(args); return done.promise },
    get: async () => ({ data: { subscription: { id: 2 }, episodes: [{ ...row, status: 'renaming', review_reason: '' }] } }),
  })
  const ep = c.episodes.value[0]
  const first = c.retryOrganize(ep)
  await c.retryOrganize(ep)
  assert.equal(c.updatingEps.value.has(41), true)
  assert.match(await renderGroups(c), /<button\b[^>]*\bdisabled[^>]*>Retry organization<\/button>/)
  done.resolve({ data: { status: 'downloaded' } })
  await first
  await c.retryOrganize(c.episodes.value[0])
  assert.equal(calls.length, 1)
  assert.equal(calls[0][0], '/episodes/41/retry-organize')
  assert.equal(calls[0][1], undefined)
  assert.equal(c.episodes.value[0].status, 'renaming')
  assert.equal(c.episodes.value[0].manual_status, 'pending')
})

test('特别篇季号与小数集号如实显示，已有的零号整包记录仍可识别', async () => {
  const c = setup([review({ status: 'organized', season: 0, number: 1.25 }), review({ id: 44, status: 'organized', season: 1, number: 0 })])
  c.sub.value = { anime_type: 'TV' }
  const html = await renderGroups(c)
  assert.match(html, />S0E1\.25</)
  assert.match(html, />E0</)
})

for (const [locale, label] of [['zh', '无集号'], ['en', 'Unnumbered']]) {
  test(`电影的无集号记录显示中性标签，已编号记录保留季集（${locale}）`, async () => {
    const c = setup([
      review({ status: 'organized' }),
      review({ id: 42, review_kind: 'movie_selection', can_confirm_movie: true, source_file: 'Extras/Preview.mkv' }),
      review({ id: 43, status: 'organized', season: 2, number: 1.5 }),
    ], {}, locale)
    for (const type of ['movie', ' FILM ', '剧场版', '劇場版']) {
      c.sub.value = { anime_type: type }
      const rows = (await renderGroups(c)).split('<tbody')[1].split('</tr>').slice(0, 3)
      const numbers = rows.map(row => row.split('</td>')[1].match(/<span\b[^>]*>([^<]*)<\/span>/)[1].trim())
      assert.deepEqual(numbers, [label, label, 'S2E1.5'], type)
    }
  })
}

test('仅已绑定视频可指派，特别篇第零季与小数集号原样提交，越界值和空值不会触发请求', async () => {
  const ep = review({ review_kind: 'episode_identity', can_retry_organize: false, can_assign_episode: true, source_file: 'Extras/Harbor.mkv' })
  const calls = []
  const c = setup([ep], { post: async (url, body) => { calls.push([url, body]); return { data: { status: 'downloaded' } } } })
  c.editAssignment(ep)
  for (const [season, number] of [['',1],[-1,1],[0.5,1],[100,1],[0,''],[0,0],[0,2001]]) {
    Object.assign(c.assignment.value, { season, number })
    await c.retryWithAssignment(ep)
  }
  assert.equal(calls.length, 0)
  Object.assign(c.assignment.value, { season: 0, number: 2.5 })
  ep.can_assign_episode = false
  await c.retryWithAssignment(ep)
  assert.equal(calls.length, 0)
  ep.can_assign_episode = true
  await c.retryWithAssignment(ep)
  assert.deepEqual(JSON.parse(JSON.stringify(calls)), [['/episodes/41/retry-organize', { season: 0, number: 2.5 }]])
})

for (const [locale, film, retry, assign] of [
  ['zh', '确认为电影正片并整理', '重新整理', '指定季集'],
  ['en', 'Confirm as feature film and organize', 'Retry organization', 'Assign season and episode'],
]) {
  test(`按源文件确认电影版本，季集和未绑定来源不会显示无效操作（${locale}）`, async () => {
    const movie = { review_kind: 'movie_selection', can_retry_organize: false, can_confirm_movie: true }
    const items = [
      review({ ...movie, source_file: '1080p/Harbor.mkv' }),
      review({ ...movie, id: 42, source_file: '2160p/Harbor.mkv' }),
      review({ ...movie, id: 43, can_confirm_movie: false, review_reason: 'Source mismatch' }),
      review({ id: 44, review_kind: 'episode_identity', can_retry_organize: false, can_assign_episode: true, source_file: 'Disc/Episode.mkv' }),
    ]
    const calls = []
    const done = Promise.withResolvers()
    const c = setup(items, { post: (...args) => { calls.push(args); return done.promise } }, locale)
    const rows = (await renderGroups(c)).split('<tbody')[1].split('</tr>')
    for (const i of [0, 1]) {
      assert.ok(rows[i].includes(items[i].source_file))
      assert.ok(rows[i].includes(film))
      assert.ok(!rows[i].includes(retry) && !rows[i].includes(assign))
    }
    assert.ok(rows[2].includes('Source mismatch'))
    assert.ok(!rows[2].includes('<button'))
    assert.ok(rows[3].includes(assign) && !rows[3].includes(retry) && !rows[3].includes(film))
    for (const ep of c.episodes.value) await c.retryOrganize(ep)
    await c.retryOrganize(items[2], { confirm_movie: true })
    await c.retryOrganize(items[3], { confirm_movie: true })
    await c.retryOrganize(items[0], { season: 1, number: 1 })
    assert.equal(calls.length, 0)
    const confirmation = c.retryOrganize(items[1], { confirm_movie: true })
    await c.retryOrganize(items[1], { confirm_movie: true })
    assert.deepEqual(JSON.parse(JSON.stringify(calls)), [['/episodes/42/retry-organize', { confirm_movie: true }]])
    done.resolve({ data: { status: 'downloaded' } })
    await confirmation
    assert.equal(c.episodes.value[0].status, 'needs_review')
    assert.equal(c.episodes.value[1].status, 'downloaded')
  })
}

test('重试失败显示服务端原因；并发修改后的过期按钮随刷新移除', async () => {
  const row = review()
  let conflict = false
  const c = setup([row], {
    post: async () => { throw { response: { status: conflict ? 409 : 500, data: { error: 'Organizer unavailable' } } } },
    get: async () => ({ data: { subscription: { id: 2 }, episodes: [{ ...row, status: 'organized' }] } }),
  })
  await c.retryOrganize(c.episodes.value[0])
  assert.equal(c.episodes.value[0].status, 'needs_review')
  assert.match(await renderGroups(c), /role="alert"[^>]*>Organizer unavailable</)
  conflict = true
  await c.retryOrganize(c.episodes.value[0])
  assert.equal(c.updatingEps.value.size, 0)
  assert.equal(c.episodes.value[0].status, 'organized')
  assert.equal(c.actionError.value, 'Organizer unavailable')
})

test('核验源文件与确认独立授权；核验使用空请求体，刷新后仍需逐项人工确认', async () => {
  const film = review({ review_kind: 'movie_selection', can_confirm_movie: true, source_file: 'UHD/Harbor.mkv' })
  const episode = review({ id: 42, review_kind: 'episode_identity', can_assign_episode: true, source_file: 'Disc/Episode.mkv' })
  const oldSource = review({ id: 43, review_kind: 'movie_selection', can_confirm_movie: false, source_file: 'SD/Harbor.mkv' })
  const items = [film, episode, oldSource]
  const calls = []
  const c = setup(items, {
    post: async (...args) => { calls.push(args); return { data: { status: 'downloaded' } } },
    get: async () => ({ data: { subscription: { id: 2 }, episodes: items.map(row => ({
      ...row, status: 'needs_review', can_confirm_movie: row.review_kind === 'movie_selection',
    })) } }),
  }, 'zh')
  const rows = (await renderGroups(c)).split('<tbody')[1].split('</tr>')
  for (const row of rows.slice(0, 3)) {
    assert.ok(row.includes('重新核验源文件'))
    assert.ok(!row.includes('>重新整理</button>'))
  }
  assert.ok(rows[0].includes('确认为电影正片并整理'))
  assert.ok(rows[1].includes('指定季集'))
  assert.ok(!rows[2].includes('确认为电影正片并整理'))
  await c.retryOrganize(c.episodes.value[2])
  assert.equal(calls.length, 1)
  assert.equal(calls[0][0], '/episodes/43/retry-organize')
  assert.equal(calls[0][1], undefined)
  assert.equal(c.episodes.value[2].status, 'needs_review')
  assert.equal(c.episodes.value[2].review_kind, 'movie_selection')
  assert.ok((await renderGroups(c)).split('<tbody')[1].split('</tr>')[2].includes('确认为电影正片并整理'))
})
