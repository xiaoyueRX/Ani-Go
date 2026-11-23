import assert from 'node:assert/strict'
import test from 'node:test'
import { view } from './view.mjs'

const settings = {
  DEFAULT_DOWNLOADER: 'qbittorrent',
  QB_HOST: 'http://qb:8080', QB_USER: 'qb-user', QB_PASS: '', QB_CATEGORY: 'anime',
  TR_HOST: 'http://transmission:9091', TR_USER: 'tr-user', TR_PASS: '',
  ARIA2_HOST: 'http://aria2:6800/jsonrpc', ARIA2_SECRET: '',
}

async function wizard(request, saved = settings) {
  const { component, mounted } = view('components/OnboardingModal', {
    get: async path => ({ data: path === '/settings' ? saved : {} }),
    ...request,
  })
  await Promise.all(mounted.map(start => start()))
  return component
}

test('switching downloader tests and saves only the selected profile and keeps its masked credential', async () => {
  let probe, saved
  const c = await wizard({
    post: async (path, body) => { assert.equal(path, '/downloader/test'); probe = body; return { data: { success: true } } },
    put: async (path, body) => { assert.equal(path, '/settings'); saved = body.settings },
  })
  for (const [type, hostKey, userKey, secretKey] of [
    ['transmission', 'TR_HOST', 'TR_USER', 'TR_PASS'],
    ['aria2', 'ARIA2_HOST', null, 'ARIA2_SECRET'],
    ['qbittorrent', 'QB_HOST', 'QB_USER', 'QB_PASS'],
  ]) {
    c.dlType.value = type
    await c.testDownloader()
    assert.equal(probe.type, type)
    assert.equal(probe.host, settings[hostKey])
    assert.equal(probe.username, userKey ? settings[userKey] : undefined)
    assert.equal(probe.use_saved_credentials, true)
    assert.equal(probe.secret, undefined)
    assert.equal(probe.password, undefined)
    await c.finishSetup()
    assert.equal(saved.DEFAULT_DOWNLOADER, type)
    assert.equal(saved[hostKey], settings[hostKey])
    if (userKey) assert.equal(saved[userKey], settings[userKey])
    assert.equal(saved[secretKey], undefined)
    for (const key of ['QB_HOST', 'TR_HOST', 'ARIA2_HOST'].filter(key => key !== hostKey)) assert.equal(saved[key], undefined)
  }
})

test('a connection result from a previously selected downloader does not appear on the new form', async () => {
  let complete
  const c = await wizard({ post: () => new Promise(resolve => { complete = resolve }) })
  const pending = c.testDownloader()
  c.dlType.value = 'aria2'
  complete({ data: { success: true } })
  await pending
  assert.equal(c.dlTestResult.value, null)
  assert.equal(c.dlTesting.value, false)
})

test('typed passwords and tokens remain in their own drafts and only the chosen one is submitted', async () => {
  let probe, saved
  const c = await wizard({
    post: async (_path, body) => { probe = body; return { data: { success: true } } },
    put: async (_path, body) => { saved = body.settings },
  })
  c.downloader.value.password = 'typed-qb-password'
  c.dlType.value = 'transmission'
  assert.equal(c.downloader.value.password, '')
  Object.assign(c.downloader.value, { host: 'http://edited:9091', username: 'edited-user', password: 'typed-tr-password' })
  c.dlType.value = 'aria2'
  assert.equal(c.downloader.value.secret, '')
  c.downloader.value.secret = 'typed-token'
  await c.testDownloader()
  assert.deepEqual({ ...probe }, { type: 'aria2', host: settings.ARIA2_HOST, secret: 'typed-token' })
  c.dlType.value = 'qbittorrent'
  assert.equal(c.downloader.value.password, 'typed-qb-password')
  c.dlType.value = 'transmission'
  await c.testDownloader()
  assert.deepEqual({ ...probe }, { type: 'transmission', host: 'http://edited:9091', username: 'edited-user', password: 'typed-tr-password' })
  await c.finishSetup()
  assert.equal(saved.TR_PASS, 'typed-tr-password')
  assert.equal(saved.QB_PASS, undefined)
  assert.equal(saved.ARIA2_SECRET, undefined)
})

for (const [type, hostKey, userKey, credentialKey] of [
  ['qbittorrent', 'QB_HOST', 'QB_USER', 'QB_PASS'],
  ['transmission', 'TR_HOST', 'TR_USER', 'TR_PASS'],
  ['aria2', 'ARIA2_HOST', null, 'ARIA2_SECRET'],
]) {
  test(`settings tests the ${type} address with the saved credential or the user's current input`, async () => {
    let payload
    const { component: c } = view('views/SettingsPage', {
      get: async () => ({ data: { ...settings } }),
      post: async (path, body) => {
        assert.equal(path, '/downloader/test')
        payload = { ...body }
        return { data: { success: true } }
      },
    })
    await c.fetchSettings()
    const expected = { type, host: settings[hostKey] }
    if (userKey) expected.username = settings[userKey]
    if (type === 'qbittorrent') expected.category = settings.QB_CATEGORY
    await c.testDownloader(type)
    assert.deepEqual(payload, { ...expected, use_saved_credentials: true })

    // The server checks the requested endpoint and account before reusing a saved credential.
    expected.host = 'http://new-host:9000'
    c.setVal(hostKey, expected.host)
    if (userKey) {
      expected.username = 'new-user'
      c.setVal(userKey, expected.username)
    }
    await c.testDownloader(type)
    assert.deepEqual(payload, { ...expected, use_saved_credentials: true })

    c.setVal(credentialKey, 'entered-for-new-endpoint')
    await c.testDownloader(type)
    assert.deepEqual(payload, { ...expected, [type === 'aria2' ? 'secret' : 'password']: 'entered-for-new-endpoint' })
    await c.testDownloader('current')
    assert.ok(!payload.type && !payload.host && !payload.password && !payload.secret)
  })
}

test('a successful settings probe replaces the connection error without reporting a settings save', async () => {
  const { component: c, notices } = view('views/SettingsPage', {
    post: async (_path, body) => ({ data: body.secret === 'correct'
      ? { success: true, message: 'Connected' }
      : { success: false, error: 'Authentication failed' } }),
  })
  c.setVal('ARIA2_SECRET', 'incorrect')
  await c.testDownloader('aria2')
  assert.deepEqual(notices.at(-1), { message: 'Authentication failed', type: 'error' })

  c.setVal('ARIA2_SECRET', 'correct')
  await c.testDownloader('aria2')
  assert.deepEqual(notices.at(-1), { message: 'Connected', type: 'success' })
  assert.deepEqual({ error: c.error.value, saved: c.saved.value }, { error: '', saved: false })
})
