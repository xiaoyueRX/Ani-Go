import test from 'node:test'
import assert from 'node:assert/strict'
import { view } from './view.mjs'

test('下载页按下载器给出的运行状态、完成状态与可用操作展示任务', async () => {
  const { component: c } = view('views/Downloads', { get: async () => ({ data: [
    { hash: 'qb', name: '已下载的暂停任务', status: 'pausedUP', display_status: 'paused', complete: true, can_resume: true, progress: 1 },
    { hash: 'aria', name: '下载中', status: 'active', display_status: 'downloading', complete: false, can_pause: true, progress: .99999 },
    { hash: 'tr', name: '做种', status: 'seeding', display_status: 'seeding', complete: true, can_pause: true, progress: 1 },
  ] }) })
  await c.fetchDownloads()
  assert.equal(c.downloadingCount.value, 1)
  assert.equal(c.completedCount.value, 2)
  c.currentFilter.value = 'completed'
  assert.deepEqual(Array.from(c.filteredTasks.value, t => t.hash), ['qb', 'tr'])
  c.currentFilter.value = 'paused'
  assert.deepEqual(Array.from(c.filteredTasks.value, t => t.hash), ['qb'])
  assert.equal(c.calcPercent(c.tasks.value[1]), 99)
})

test('操作后收到的旧轮询响应不能覆盖较新的队列', async () => {
  const calls = []
  const { component: c } = view('views/Downloads', { get: () => new Promise(resolve => calls.push(resolve)) })
  const old = c.fetchDownloads()
  const next = c.fetchDownloads()
  calls[1]({ data: [{ name: '新状态', hash: 'a', status: 'paused', display_status: 'paused' }] })
  await next
  calls[0]({ data: [{ name: '旧状态', hash: 'a', status: 'downloading', display_status: 'downloading' }] })
  await old
  assert.equal(c.tasks.value[0].name, '新状态')
})

test('同一下载命令与确认弹窗重复提交时只写一次', async () => {
  const calls = []
  const pending = []
  const send = async path => { calls.push(path); return new Promise(resolve => pending.push(resolve)) }
  const { component: c } = view('views/Downloads', {
    post: send, delete: send, get: async () => ({ data: [] }),
  })
  const task = { hash: 'abc', name: 'Example' }
  const first = c.pauseTask(task), again = c.pauseTask(task)
  assert.deepEqual(calls,['/downloads/abc/pause'])
  pending.shift()({ data: {} }); await Promise.all([first,again])
  c.openDeleteModal(task)
  const del = c.confirmDelete(), duplicate = c.confirmDelete()
  assert.equal(calls.filter(p => p==='/downloads/abc').length,1)
  pending.shift()({ data: {} }); await Promise.all([del,duplicate])
  c.addUrl.value = 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567'
  const add=c.submitNewDownload(), another=c.submitNewDownload()
  assert.equal(calls.filter(p => p==='/downloads').length,1)
  pending.shift()({ data: {} }); await Promise.all([add,another])
})
