import assert from 'node:assert/strict'
import { test } from 'node:test'
import { view } from './view.mjs'

function card(props = {}) {
  const sub = { id: 37, title_cn: '示例订阅', enabled: true }
  const { component, emitted } = view('components/SubscriptionCard', {}, { props: { sub, ...props }, inlineTemplate: true })
  const root = component({ $t: key => key }, [])
  function find(node, predicate) {
    if (!node || typeof node !== 'object') return
    if (predicate(node)) return node
    if (Array.isArray(node.children)) {
      for (const child of node.children) {
        const found = find(child, predicate)
        if (found) return found
      }
    }
  }
  return { root, emitted, find: predicate => find(root, predicate) }
}

test('标题链接使用原路由，普通激活只导航一次，修饰键保留浏览器行为', () => {
  const c = card()
  const title = c.find(node => node.type === 'a')
  assert.equal(title?.props.href, '/subscriptions/37')
  const event = { button: 0, stopPropagation() {}, preventDefault() { this.defaultPrevented = true } }
  title.props.onClick(event)
  assert.equal(event.defaultPrevented, true)
  assert.deepEqual(c.emitted, [['click']])
  const modified = { ...event, defaultPrevented: false, ctrlKey: true }
  title.props.onClick(modified)
  assert.equal(modified.defaultPrevented, false)
  assert.equal(c.emitted.length, 1)
})

test('批量标题报告开关状态，图标操作具名，等待删除的卡片禁用交互', () => {
  for (const selected of [false, true]) {
    const c = card({ batchDeleteMode: true, batchSelected: selected })
    assert.equal(c.find(n => n.type === 'a'), undefined)
    assert.equal(c.find(n => n.type === 'button' && n.props?.['aria-pressed'] === selected)?.props.type, 'button')
  }
  const normal = card()
  for (const label of ['detail.actions.suspend', 'common.delete']) {
    assert.ok(normal.find(n => n.type === 'button' && n.props?.['aria-label'] === label))
  }
  const pending = card({ pending: true })
  assert.equal(pending.root.props.inert, true)
  assert.equal(pending.find(n => n.type === 'a')?.props.href, undefined)
})
