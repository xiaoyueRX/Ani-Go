import assert from 'node:assert/strict'
import { test } from 'node:test'
import { view } from './view.mjs'

function subscriptions() {
  const calls = []
  const { component } = view('views/Subscriptions', {
    get: async () => ({ data: [] }),
    post: async (url, body) => { calls.push({ url, ids: [...body.ids], deleteFiles: body.delete_files }) },
  })
  component.subs.value = [{ id: 10, title_cn: 'Amber' }, { id: 20, title_cn: 'Violet' }]
  component.enterBatchDeleteMode()
  return { component, calls }
}

test('批量全选和取消只改变当前筛选中的订阅', async () => {
  const { component, calls } = subscriptions()
  component.filterText.value = 'Amber'
  component.toggleBatchSelect(10)
  component.filterText.value = 'Violet'
  component.toggleSelectAllBatch()
  assert.deepEqual([...component.batchDeleteSelected.value], [10, 20])

  component.filterText.value = 'Amber'
  component.toggleSelectAllBatch()
  assert.deepEqual([...component.batchDeleteSelected.value], [20])
  component.filterText.value = '未匹配'
  component.toggleSelectAllBatch()
  assert.deepEqual([...component.batchDeleteSelected.value], [20])

  component.openBatchDeleteModal()
  component.batchDeleteFilesChecked.value = false
  component.confirmBatchDeleteWithFiles()
  await component.executePendingDeletes()
  assert.deepEqual(calls, [{ url: '/subscriptions/batch-delete', ids: [20], deleteFiles: false }])
})

test('等待删除与回收站条目都不参与全选', () => {
  const { component } = subscriptions()
  component.scheduleDelete([10], false)
  component.toggleSelectAllBatch()
  assert.deepEqual([...component.batchDeleteSelected.value], [20])
  component.deletedSubs.value = [{ id: 30, title_cn: 'Recycled' }]
  component.filterType.value = 'recycle'
  component.toggleSelectAllBatch()
  assert.deepEqual([...component.batchDeleteSelected.value], [20])
  component.filterType.value = 'all'
  component.toggleSelectAllBatch()
  assert.deepEqual([...component.batchDeleteSelected.value], [])
})
