import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { createSSRApp, nextTick, reactive } from 'vue'
import { parse } from 'vue/compiler-sfc'
import { renderToString } from 'vue/server-renderer'
import { createI18n } from 'vue-i18n'
import * as icons from 'lucide-vue-next'
import en from '../src/locales/en.ts'
import zh from '../src/locales/zh.ts'
import { view } from './view.mjs'

const source = readFileSync(new URL('../src/views/SubscriptionDetail.vue', import.meta.url), 'utf8')
function findTemplate(node, matches) {
  if (matches(node)) return node.loc.source
  return node.children?.map(child => findTemplate(child, matches)).find(Boolean)
}
const tree = parse(source).descriptor.template.ast
const template = findTemplate(tree, node => node.props?.some(prop => prop.name === 'for' && prop.exp?.content.includes('groupedEpisodes')))
const resultTemplate = findTemplate(tree, node => node.tag === 'details' && node.props.some(prop => prop.exp?.content === 'recheckResults.length'))
function review(overrides = {}) {
  return { id: 41, status: 'needs_review', review_kind: '', can_retry_organize: true, review_reason: 'The destination was unavailable', season: 1, number: 0,
    group_name: 'Harbor', manual_status: 'pending', ...overrides }
}
function setup(rows, request = {}, locale = 'en', params = { id: 2 }, globals = {}) {
  const { t } = createI18n({ legacy: false, locale, messages: { en, zh } }).global
  const { component } = view('views/SubscriptionDetail', {
    get: async () => ({ data: { subscription: { id: 2 }, episodes: rows } }), ...request,
  }, { route: { params }, globals, translate: (key, values) => values ? t(key, { ...values }) : t(key) })
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

test('批量核验按项排队、跳过人工确认并保留失败；重复点击只重试尚可核验的选项', async () => {
  const items = [
    review({ id: 1, original_name: 'A.mkv' }), review({ id: 2, original_name: 'B.mkv' }),
    review({ id: 3, review_kind: 'episode_identity', can_assign_episode: true, original_name: 'C.mkv' }),
    review({ id: 4, review_kind: 'movie_selection', can_confirm_movie: true, original_name: 'D.mkv' }),
    review({ id: 5, status: 'organized', can_retry_organize: false, original_name: 'E.mkv' }),
    review({ id: 6, can_retry_organize: false, original_name: 'F.mkv' }), review({ id: 7 }),
  ]
  const calls = []
  let reads = 0
  const first = Promise.withResolvers()
  const c = setup(items, {
    post: async (url, body) => {
      calls.push([url, body])
      if (calls.length === 1) await first.promise
      if (calls.length === 2) throw { response: { status: 409, data: { error: 'another session updated B' } } }
      const row = items.find(row => url === `/episodes/${row.id}/retry-organize`)
      row.status = 'downloaded'
      return { data: { status: row.status } }
    },
    get: async () => { reads++; return { data: { subscription: { id: 2 }, episodes: items } } },
  }, 'zh')
  for (const row of items.slice(0, 6)) c.toggleEpisodeSelect(row.id)
  const batch = c.bulkRecheck()
  await c.bulkRecheck()
  await c.retryOrganize(items[0])
  assert.deepEqual(calls, [['/episodes/1/retry-organize', undefined]])
  first.resolve()
  await batch
  assert.deepEqual(calls, [['/episodes/1/retry-organize', undefined], ['/episodes/2/retry-organize', undefined]])
  assert.equal(reads, 1)
  assert.deepEqual([...c.selectedEpisodes.value], [2, 3, 4, 5, 6])
  const results = () => renderToString(createSSRApp({ template: resultTemplate, setup: () => ({ ...c }) }))
  const html = await results()
  for (const expected of ['A.mkv', '已排队', 'B.mkv', 'another session updated B', 'C.mkv', '指定季集', 'D.mkv', '确认正片', 'E.mkv', 'F.mkv', '已跳过']) assert.ok(html.includes(expected), expected)
  await c.bulkRecheck()
  assert.deepEqual(calls.slice(2), [['/episodes/2/retry-organize', undefined]])
  assert.equal(reads, 2)
  assert.deepEqual([...c.selectedEpisodes.value], [3, 4, 5, 6])
  await c.bulkRecheck()
  assert.equal(calls.length, 3)
  assert.equal(reads, 2)
})

test('刷新后已删除的文件仍有可追溯的请求失败原因，并显示英文核验结果', async () => {
  const gone = review({ original_name: 'Removed edition.mkv' })
  const c = setup([gone], {
    post: async () => { throw { response: { status: 404, data: { error: 'Episode no longer exists' } } } },
    get: async () => ({ data: { subscription: { id: 2 }, episodes: [] } }),
  })
  c.toggleEpisodeSelect(gone.id)
  await c.bulkRecheck()
  assert.equal(c.selectedEpisodes.value.size, 0)
  assert.equal(c.episodes.value.length, 0)
  const html = await renderToString(createSSRApp({ template: resultTemplate, setup: () => ({ ...c }) }))
  assert.ok(html.includes('Recheck results: 0 queued, 1 failed, 0 skipped'), html)
  for (const label of ['Removed edition.mkv', 'Request failed', 'Episode no longer exists']) assert.ok(html.includes(label), label)
})


const checkTemplate = findTemplate(tree, node => node.tag === 'details' && node.props.some(prop => prop.exp?.content === 'fileReport'))

test('主动核对所选条目只读展示每个文件的结果，零字节和未设卷标均有说明', async () => {
  const rows = [review({ id: 31, status: 'organized', final_path: '/private/nas/Show/E01.mkv' }), review({ id: 32, status: 'organized', final_path: 'D:\\private\\E02.mkv' }), review({ id: 33, status: 'needs_review', original_name: 'E03.mkv' })]
  const requests = []
  const c = setup(rows, { post: async (url, body) => {
    requests.push([url, Array.from(body.ids)])
    return { data: { volume_guarded: false, results: [{ id: 31, status: 'size_matches' }, { id: 32, status: 'size_mismatch', expected_size: 351, actual_size: 0 }, { id: 33, status: 'not_applicable' }] } }
  } }, 'zh')
  c.toggleSelectAll(rows)
  assert.deepEqual(requests, [])
  const before = JSON.stringify(c.episodes.value)
  await c.verifyFiles('selected')
  assert.deepEqual(requests, [['/subscriptions/2/verify-files', [31, 32, 33]]])
  assert.equal(JSON.stringify(c.episodes.value), before)
  assert.deepEqual([...c.selectedEpisodes.value], [31, 32, 33])
  const html = await renderToString(createSSRApp({ template: checkTemplate, setup: () => ({ ...c }) }))
  for (const word of ['E01.mkv', 'E02.mkv', 'E03.mkv', '大小与记录一致', '351', '0 字节', '尚未整理', '当前媒体库路径']) assert.ok(html.includes(word), word)
  assert.ok(!html.includes('private'), html)
  c.toggleEpisodeSelect(31)
  await nextTick()
  assert.equal(c.fileReport.value, null)
})

test('全订阅核对仅提交已整理文件，连续分批，报错后仍保留其他批次结果', async () => {
  const rows = Array.from({length: 103}, (_,i) => review({ id: i+1, status: i===102?'needs_review':'organized', final_path: '/library/ep-'+i+'.mkv' }))
  const calls = []
  const c = setup(rows, { post: async (url,body) => {
    calls.push(Array.from(body.ids))
    if (calls.length===2) throw { response: { status: 404, data: { error: '/private/error' } } }
    return {data: { volume_guarded: true, results: body.ids.map(id=>({id,status:'present'})) }}
  } })
  await c.verifyFiles('organized')
  assert.deepEqual(calls.map(batch=>batch.length), [100, 2])
  assert.deepEqual(calls[1], [101, 102])
  const html = await renderToString(createSSRApp({ template: checkTemplate, setup: () => ({ ...c }) }))
  assert.ok(html.includes('ep-100.mkv') && html.includes('ep-101.mkv') && html.includes('Request failed'))
  assert.ok(!html.includes('ep-102.mkv') && !html.includes('/private/error'))
})

test('切换订阅或勾选时晚到的核对结果不会出现在当前页面', async () => {
  const rows = [review({id:21, status:'organized'}), review({id:22, status:'organized'})]
  for (const changeRoute of [false, true]) {
    const params = reactive({id:2}), pending = Promise.withResolvers()
    const c = setup(rows, { post: ()=>pending.promise }, 'en', params)
    c.toggleEpisodeSelect(21)
    const first = c.verifyFiles('selected')
    if (changeRoute) params.id=9
    else c.toggleEpisodeSelect(22)
    await nextTick()
    pending.resolve({data:{volume_guarded:true,results:[{id:21,status:'size_matches'}]}})
    await first
    assert.equal(c.fileReport.value,null)
  }
})


test('文件缺失、空文件和卷身份未核实各自显示对应原因', async () => {
  const rows = [11, 12, 13].map(id => review({ id, status: 'organized', title: `Episode ${id}` }))
  const c = setup(rows, { post: async () => ({ data: { volume_guarded: true, results: [
    { id: 11, status: 'missing' }, { id: 12, status: 'empty' }, { id: 13, status: 'unverified', reason: 'volume_unavailable' },
  ] } }) })
  await c.verifyFiles('organized')
  const html = await renderToString(createSSRApp({ template: checkTemplate, setup: () => ({ ...c }) }))
  for (const word of ['File missing at current path', 'Empty file', 'Could not confirm media volume']) assert.ok(html.includes(word), word)
  assert.ok(!html.includes('No volume ID is configured'))
})

function renderFileReport(c) {
  return renderToString(createSSRApp({ template: checkTemplate, setup: () => ({ ...c }) }))
}

test('只有缺失版本可补回，未标记媒体卷须先确认，单次成功只复验这一个版本', async () => {
  const calls = [], prompts = []
  const pending = Promise.withResolvers()
  let approved = false
  const rows = [11, 12, 13, 14, 15].map(id => review({ id, status: 'organized', final_path: '/library/edition-' + id + '.mkv' }))
  const states = ['missing', 'size_matches', 'size_mismatch', 'empty', 'unverified']
  const c = setup(rows, { post: async (url, body) => {
    calls.push([url, JSON.parse(JSON.stringify(body))])
    if (url.endsWith('/restore-file')) return pending.promise
    return { data: { volume_guarded: false, results: body.ids.map(id => ({ id, status: body.ids.length === 1 ? 'size_matches' : states[id - 11] })) } }
  } }, 'zh', { id: 2 }, { confirm: msg => { prompts.push(msg); return approved } })
  await c.verifyFiles('organized')
  const report = c.fileReport.value
  const html = await renderFileReport(c)
  assert.equal((html.match(/从下载原件补回/g) || []).length, 1)
  for (const word of ['硬链接', '同一', '原件', '父目录']) assert.ok(html.includes(word), word)
  for (const sibling of report.rows.slice(1)) await c.restoreFile(sibling)
  await c.restoreFile(report.rows[0])
  assert.equal(prompts.length, 1)
  assert.match(prompts[0], /正确的媒体库/)
  assert.equal(calls.length, 1)
  approved = true
  const first = c.restoreFile(report.rows[0])
  await c.restoreFile(report.rows[0])
  assert.equal(prompts.length, 2)
  assert.equal(calls.length, 2)
  pending.resolve({ data: { restored: true } })
  await first
  assert.deepEqual(calls.slice(1), [
    ['/subscriptions/2/episodes/11/restore-file', { confirm_unmarked_volume: true }],
    ['/subscriptions/2/verify-files', { ids: [11] }],
  ])
  assert.deepEqual(Array.from(report.rows, row => row.status), ['size_matches', ...states.slice(1)])
  assert.ok(!(await renderFileReport(c)).includes('从下载原件补回'))
})

test('已标记卷的补回失败在对应行显示服务端原因，保留其它版本和再次补回入口', async () => {
  const calls = []
  const c = setup([41, 42].map(id => review({ id, status: 'organized', original_name: id + '.mkv' })), { post: async (url, body) => {
    if (url.endsWith('/verify-files')) return { data: { volume_guarded: true, results: body.ids.map(id => ({ id, status: 'missing' })) } }
    calls.push([url, body.confirm_unmarked_volume])
    throw { response: { data: { error: '下载原件已变化' } } }
  } }, 'en', { id: 2 }, { confirm: () => assert.fail('Marked media volume requested unmarked confirmation') })
  await c.verifyFiles('organized')
  await c.restoreFile(c.fileReport.value.rows[0])
  const html = await renderFileReport(c)
  const [failed, sibling] = html.split('<li>').slice(1)
  assert.ok(failed.includes('下载原件已变化') && failed.includes('Restore from download'))
  assert.ok(sibling.includes('42.mkv') && sibling.includes('Restore from download') && !sibling.includes('下载原件已变化'))
  assert.deepEqual(calls, [['/subscriptions/2/episodes/41/restore-file', false]])
})

test('离开核对范围时晚到的补回成功不会复写其他报告', async () => {
  const pending = Promise.withResolvers(), calls = []
  const ep = review({ status: 'organized' })
  const c = setup([ep], { post: async url => {
    calls.push(url)
    if (url.endsWith('/restore-file')) return pending.promise
    return { data: { volume_guarded: true, results: [{ id: ep.id, status: 'missing' }] } }
  } })
  await c.verifyFiles('organized')
  const first = c.restoreFile(c.fileReport.value.rows[0])
  c.toggleEpisodeSelect(ep.id)
  await nextTick()
  assert.equal(c.fileReport.value, null)
  pending.resolve({ data: { restored: true } })
  await first
  assert.equal(c.fileReport.value, null)
  assert.equal(calls.length, 2)
})

test('已补回后重新核对失败会区分两步结果，用户不能再次提交同一次补回', async () => {
  const calls = []
  const ep = review({ status: 'organized' })
  const c = setup([ep], { post: async url => {
    calls.push(url)
    if (calls.length === 1) return { data: { volume_guarded: true, results: [{ id: ep.id, status: 'missing' }] } }
    if (url.endsWith('/restore-file')) return { data: { restored: true } }
    throw new Error('Connection closed')
  } })
  await c.verifyFiles('organized')
  const row = c.fileReport.value.rows[0]
  await c.restoreFile(row)
  const html = await renderFileReport(c)
  assert.ok(html.includes('File restored, but the follow-up check failed.'))
  assert.ok(!html.includes('Restore from download'))
  await c.restoreFile(row)
  assert.equal(calls.length, 3)
})
