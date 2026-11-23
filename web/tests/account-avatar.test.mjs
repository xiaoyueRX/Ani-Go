import test from 'node:test'
import assert from 'node:assert/strict'
import { view } from './view.mjs'

test('rejecting an oversized avatar clears the picker and prior success feedback', async () => {
  const { component } = view('views/SettingsPage', {})
  component.avatarSuccess.value = 'previous upload succeeded'
  const target = { files: [{ size: 5 * 1024 * 1024 + 1 }], value: 'large.png' }
  await component.handleAvatarFileChange({ target })
  assert.equal(target.value, '')
  assert.equal(component.avatarSuccess.value, '')
  assert.ok(component.avatarError.value)
})

test('failed avatar reset preserves the preview and current URL', async () => {
  const request = { put: async () => { throw new Error('temporary failure') } }
  const { component } = view('views/SettingsPage', request)
  const current = '/api/user/avatar?v=avatar-42.png'
  component.setVal('USER_AVATAR_URL', current)
  await component.clearAvatar()
  assert.equal(component.getVal('USER_AVATAR_URL'), current)
  assert.ok(component.avatarError.value)
})
