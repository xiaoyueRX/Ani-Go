import assert from 'node:assert/strict'
import test from 'node:test'
import { view } from './view.mjs'
import { nextTick } from 'vue'

const finish = () => new Promise(resolve => setImmediate(resolve))
const ep = (id, status, torrent_hash) => ({ id, status, torrent_hash })
test('详情只核对当前订阅的已绑定任务，断线清除过时结论，原任务恢复后可重新核对', async () => {
  let state = 'missing'
  let checks = 0
  const episodes = [ep(1,'downloading','ABC'),ep(2,'downloaded','abc'),ep(3,'organized','old')]
  const request = {get: async url => {
    if (url === '/subscriptions/1') return {data:{ subscription:{id:1}, episodes }}
    assert.equal(url,'/subscriptions/1/download-tasks')
    checks++
    if (state==='offline') throw new Error('offline')
    return {data:{tasks:{abc:state}}}
  }}
  const c = view('views/SubscriptionDetail', request, {route:{params:{id:1}}}).component
  await c.fetchDetail(); await nextTick(); await finish()
  assert.equal(checks,1)
  assert.equal(c.episodeTaskState(episodes[0]),'missing')
  assert.equal(c.episodeTaskState(episodes[1]),'missing')
  assert.equal(c.episodeTaskState(episodes[2]),undefined)
  await c.fetchDetail(); await nextTick(); await finish()
  assert.equal(checks,1)
  state='offline'
  await c.checkDownloadTasks()
  assert.equal(c.episodeTaskState(episodes[0]),undefined)
  assert.equal(c.taskCheckFailed.value,true)
  state='present'
  await c.checkDownloadTasks()
  assert.equal(c.episodeTaskState(episodes[0]),'present')
  assert.equal(c.taskCheckFailed.value,false)
})

test('任务集合变更后忽略迟到的旧请求，缺少结果的响应不会标记任务丢失', async () => {
  let finishOld
  let checks = 0
  let entries = [ep(1, 'downloading', 'oldtask')]
  const request = {get: async url => {
    if (url === '/subscriptions/7') return {data:{subscription:{id:7}, episodes:entries}}
    if (++checks === 1) return new Promise(resolve => { finishOld = resolve })
    return {data:{}}
  }}
  const c = view('views/SubscriptionDetail',request,{route:{params:{id:7}}}).component
  await c.fetchDetail(); await nextTick()
  entries = [ep(2, 'downloading', 'newtask')]
  await c.fetchDetail(); await nextTick(); await finish()
  finishOld({data:{tasks:{oldtask:'missing'}}}); await finish()
  assert.equal(c.taskCheckFailed.value,true)
  assert.equal(c.episodeTaskState(entries[0]),undefined)
  assert.equal(c.hasMissingTask.value,false)
})
