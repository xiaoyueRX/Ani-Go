import assert from 'node:assert/strict'
import test from 'node:test'
import { activeDownloader, downloaderForms, downloaderSettings } from '../src/utils/setupDownloader.ts'

test('opening and finishing the wizard preserves existing downloader credentials', () => {
  const settings = {
    DEFAULT_DOWNLOADER: 'transmission',
    QB_HOST: 'http://qb:8080', QB_USER: 'old-user', QB_CATEGORY: 'my-category', QB_PASS: '',
    TR_HOST: 'http://tr:9091', TR_USER: 'transmission-user', TR_PASS: '',
    ARIA2_HOST: 'http://aria:6800/jsonrpc', ARIA2_SECRET: '',
  }
  const forms = downloaderForms(settings)
  assert.equal(activeDownloader(settings), 'transmission')
  assert.deepEqual(downloaderSettings('transmission', forms.transmission), {
    DEFAULT_DOWNLOADER: 'transmission', TR_HOST: settings.TR_HOST, TR_USER: settings.TR_USER,
  })
  assert.deepEqual(downloaderSettings('qbittorrent', forms.qbittorrent), {
    DEFAULT_DOWNLOADER: 'qbittorrent', QB_HOST: settings.QB_HOST, QB_USER: settings.QB_USER,
    QB_CATEGORY: settings.QB_CATEGORY,
  })
  assert.deepEqual(downloaderSettings('aria2', forms.aria2), {
    DEFAULT_DOWNLOADER: 'aria2', ARIA2_HOST: settings.ARIA2_HOST,
  })
  forms.transmission.password = 'new-password'
  assert.equal(downloaderSettings('transmission', forms.transmission).TR_PASS, 'new-password')
  assert.equal(downloaderSettings('qbittorrent', forms.qbittorrent).QB_PASS, undefined)
  assert.equal(activeDownloader({ DOWNLOADER_TYPE: 'aria2' }), 'aria2')
})
