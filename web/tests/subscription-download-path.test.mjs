import assert from 'node:assert/strict'
import test from 'node:test'
import { view } from './view.mjs'

test('clearing a saved subscription download directory explicitly restores the downloader default', () => {
  const { component, emitted } = view('components/SubscriptionEditForm', {}, {
    props: { sub: { title_cn: 'Example', custom_path: '/downloads/series' } },
  })
  component.customPath.value = ''
  component.handleSubmit()
  assert.equal(emitted[0][0], 'save')
  assert.equal(emitted[0][1].custom_path, '')
})
