import assert from 'node:assert/strict'
import test from 'node:test'
import { view } from './view.mjs'

const result = (name) => ({ title: name, url: 'https://release.example/' + name, magnet: '', source: 'Nyaa' })
const one = result('one')
const two = result('two')

test('后发搜索接管结果、错误和加载状态，清空输入后早先响应不能回填', async () => {
  const pending = new Map()
  const { component: c } = view('views/Search', { get: (_path, options) => new Promise((resolve, reject) => pending.set(options.params.q, { resolve, reject })) })
  c.query.value = 'first'; const first = c.handleSearch()
  c.query.value = 'second'; const second = c.handleSearch()
  pending.get('first').reject(new Error('old error')); await first
  assert.equal(c.loading.value, true)
  assert.equal(c.error.value, '')
  pending.get('second').resolve({ data: [two] }); await second
  assert.equal(c.results.value[0].title, 'two')
  c.query.value = 'third'; const third = c.handleSearch()
  c.clearQuery()
  pending.get('third').resolve({ data: [one] }); await third
  assert.equal(c.results.value.length, 0)
  assert.equal(c.loading.value, false)
})

test('批量下载报告部分失败并仅保留失败项，重复触发不会再次入队', async () => {
  let finish
  let firstTry = true
  const calls = []
  const { component: c, notices } = view('views/Search', { async post(_path, body) {
    calls.push(body.title)
    if (body.title === 'one') return new Promise(resolve => { finish = resolve })
    if (firstTry) throw new Error('unavailable')
    return {}
  } })
  c.selectedItems.value = [one, two]
  c.batchMode.value = true
  const a = c.submitBatch('download')
  const b = c.submitBatch('download')
  assert.equal(calls.length, 1)
  finish({})
  await Promise.all([a,b])
  assert.deepEqual(calls, ['one','two'])
  assert.equal(notices.at(-1).type, 'error')
  assert.equal(c.batchMode.value, true)
  assert.equal(c.selectedItems.value[0].title, 'two')
  assert.equal(c.selectedItems.value.length, 1)
  firstTry = false
  await c.submitBatch('download')
  assert.deepEqual(calls, ['one','two','two'])
  assert.equal(c.batchMode.value, false)
  assert.equal(notices.at(-1).type, 'success')
})

test('筛选后全选只切换当前可见的资源并保留其他选择', () => {
  const { component: c } = view('views/Search', {})
  c.results.value = [one, two, { ...result('index'), url: 'https://mikan.example/Home/Bangumi/12' }]
  c.selectedItems.value = [one]
  c.inResultFilter.value = 'two'
  c.toggleSelectAll()
  assert.deepEqual([...c.selectedItems.value].map(x=>x.title), ['one','two'])
  c.toggleSelectAll()
  assert.deepEqual([...c.selectedItems.value].map(x=>x.title), ['one'])
  c.inResultFilter.value = ''
  c.toggleSelectAll()
  assert.deepEqual([...c.selectedItems.value].map(x=>x.title), ['one','two'])
})

test('新搜索清理历史选择，系列索引无法作为本地直接下载', async () => {
  const calls = []
  const { component: c } = view('views/Search', { get: async () => ({ data:[two] }), post: async (...args)=>calls.push(args) })
  c.results.value=[one]; c.selectedItems.value=[one]; c.batchMode.value=true; c.query.value='two'
  await c.handleSearch()
  assert.equal(c.selectedItems.value.length,0)
  assert.equal(c.batchMode.value,false)
  await c.directDownload({ ...result('series'), url:'https://mikan.example/Home/Bangumi/12' })
  assert.equal(calls.length,0)
})

test('缺少发布日期或 Go 零值时不显示虚构年份，未来时间不显示为刚刚', () => {
  const { component: c } = view('views/Search', {})
  assert.equal(c.formatPubDate('0001-01-01T00:00:00Z'), '')
  assert.equal(c.formatPubDate('unparseable'), '')
  assert.equal(c.formatPubDate(new Date(Date.now() - 1500).toISOString()), 'search.time.now')
  assert.notEqual(c.formatPubDate(new Date(Date.now() + 86400000).toISOString()), 'search.time.now')
})

test('复制磁力失败时给出错误且清理临时输入', async () => {
  const element = { select() {}, remove() { this.removed = true } }
  const { component: c, notices } = view('views/Search', {}, { globals: {
    navigator: { clipboard: { writeText: async () => { throw new Error('denied') } } },
    document: { createElement: () => element, body: { appendChild() {}, removeChild: el => el.remove() }, execCommand: () => false },
  } })
  await c.copyMagnet({ ...one, magnet: 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567' })
  assert.equal(notices.at(-1).type, 'error')
  assert.equal(element.removed, true)
})

test('来源页优先打开资源详情，只有普通网页地址才能显示外链', () => {
  const { component: c } = view('views/Search', {})
  assert.equal(c.sourceLink({ ...one, episode_url: 'https://release.example/detail/1' }), 'https://release.example/detail/1')
  assert.equal(c.sourceLink({ ...one, url: 'magnet:?xt=urn:btih:sample' }), '')
  assert.equal(c.sourceLink({ ...one, episode_url: 'malformed' }), one.url)
})
