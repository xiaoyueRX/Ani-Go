import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import test from 'node:test'
import { nextTick, reactive } from 'vue'
import { view } from './view.mjs'

const { Image } = createRequire(import.meta.url)('lucide-vue-next')
const first = '/api/proxy/image?url=first'
const second = '/api/proxy/image?url=second'

function find(node, matches) {
  if (matches(node)) return node
  if (Array.isArray(node.children)) {
    for (const child of node.children) {
      const found = find(child, matches)
      if (found) return found
    }
  }
}

function card(url) {
  const sub = reactive({ title_cn: '合成订阅', cover_url: url, enabled: true })
  const { component: render } = view('components/SubscriptionCard', {}, { props: { sub }, inlineTemplate: true })
  const cache = []
  return {
    async changeCover(url) {
      sub.cover_url = url
      await nextTick()
    },
    cover() {
      const root = render({ $t: key => key }, cache)
      return {
        skeleton: Boolean(find(root, node => node.props?.class?.split(' ').includes('skeleton'))),
        icon: Boolean(find(root, node => node.type === Image)),
        image: find(root, node => node.type === 'img')?.props,
      }
    },
  }
}

test('没有封面的卡片仅显示图标，添加或移除封面时随之切换', async () => {
  const c = card('')
  const emptyCover = { skeleton: false, icon: true, image: undefined }
  assert.deepEqual(c.cover(), emptyCover)
  await c.changeCover(first)
  assert.equal(c.cover().skeleton, true)
  assert.equal(c.cover().icon, false)
  assert.equal(c.cover().image.src, first)
  c.cover().image.onLoad()
  assert.equal(c.cover().skeleton, false)
  await c.changeCover('')
  assert.deepEqual(c.cover(), emptyCover)
})

for (const failed of [false, true]) {
  test(`替换${failed ? '已显示错误占位图' : '已加载'}的封面时等新图片加载`, async () => {
    const c = card(first)
    const initial = c.cover()
    assert.equal(initial.skeleton, true)
    if (failed) {
      const target = { src: first }
      initial.image.onError({ target })
      assert.match(target.src, /^data:image\/svg\+xml,/)
      assert.equal(c.cover().skeleton, true)
    }
    initial.image.onLoad()
    assert.equal(c.cover().skeleton, false)
    await c.changeCover(second)
    const replacement = c.cover()
    assert.equal(replacement.image.src, second)
    assert.equal(replacement.skeleton, true)
    replacement.image.onLoad()
    assert.equal(c.cover().skeleton, false)
  })
}

test('只跳过离屏的装饰动画，保留图片、替代文本和卡片标题', () => {
  const sub = { title_cn: '合成订阅', cover_url: first, enabled: true }
  const { component: render } = view('components/SubscriptionCard', {}, { props: { sub }, inlineTemplate: true })
  const root = render({ $t: key => key }, [])
  const layer = find(root, node => node.props?.class?.split(' ').includes('[content-visibility:auto]'))
  assert.ok(layer, '骨架装饰层应允许跳过离屏渲染')
  assert.equal(layer.props['aria-hidden'], 'true')
  assert.ok(find(layer, node => node.props?.class?.split(' ').includes('skeleton')))
  assert.equal(find(layer, node => node.type === 'img'), undefined)
  const image = find(root, node => node.type === 'img')
  assert.equal(image.props.loading, 'lazy')
  assert.equal(image.props.alt, sub.title_cn)
  assert.equal(find(layer, node => node.type === 'h3'), undefined)
  assert.ok(find(root, node => node.type === 'h3'))
})
