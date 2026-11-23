import assert from 'node:assert/strict'
import { test } from 'node:test'
import { listEntry, listView, saveListView } from '../src/utils/subscriptionListHistory.ts'

function historyEntry(position) {
  return { state: { position }, replaceState(state) { this.state = state } }
}

test('同一条历史记录保留筛选与位置，新导航即使复用序号也获得独立状态', () => {
  const first = historyEntry(4)
  const id = listEntry(first)
  const saved = { search: '01', filter: 'active', sort: 'title', y: 1425, card: { id: 7, top: 88 } }
  saveListView(id, 'session-a', saved)
  assert.equal(listEntry(first), id)
  assert.deepEqual(listView(first.state.subscriptionListEntry, 'session-a'), saved)
  assert.deepEqual(Object.keys(first.state).sort(), ['position', 'subscriptionListEntry'])
  const next = listEntry(historyEntry(4))
  assert.notEqual(id, next)
  assert.equal(listView(next, 'session-a'), undefined)
})

test('退出或换账号丢弃上一会话的历史状态', () => {
  const id = listEntry(historyEntry(1))
  const saved = { search: '仅限原会话', filter: 'active', sort: 'progress', y: 2880 }
  saveListView(id, 'old', saved)
  assert.equal(listView(id, null), undefined)
  assert.equal(listView(id, 'old'), undefined)
  saveListView(id, 'old', saved)
  assert.equal(listView(id, 'new'), undefined)
  assert.equal(listView(id, 'old'), undefined)
})
