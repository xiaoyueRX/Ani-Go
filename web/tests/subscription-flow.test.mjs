import assert from 'node:assert/strict'
import test from 'node:test'
import { view } from './view.mjs'

test('新建订阅页支持 RSS 预览解析自动回填并提交完整规则', async () => {
  const pushed = []
  const posts = []
  const request = {
    get: async url => {
      if (url === '/subgroups') return { data: ['LoliHouse', 'ANi'] }
      return { data: [] }
    },
    post: async (url, body) => {
      posts.push([url, JSON.parse(JSON.stringify(body))])
      if (url === '/rss/preview') {
        return {
          data: {
            success: true,
            feed: { title: 'Test Feed' },
            total: 1,
            items: [{ title: '[ANi] 葬送的芙莉莲 - 01 [1080p]', parsed_title: '葬送的芙莉莲', season: 2, subgroup: 'ANi', resolution: '1080p', episode: 1 }],
          },
        }
      }
      if (url === '/subscriptions') return { data: { id: 88 } }
      return { data: {} }
    },
  }
  const { component: c } = view('views/SubscriptionForm', request, {
    router: { push: path => pushed.push(path) },
  })
  c.rssUrl.value = 'https://mikanani.me/RSS/Bangumi?bangumiId=3141'
  await c.handlePreviewRSS()
  assert.equal(c.titleCN.value, '葬送的芙莉莲')
  assert.equal(c.season.value, 2)
  assert.equal(c.subgroupName.value, 'ANi')
  assert.equal(JSON.parse(c.filterJson.value).resolution, '1080p')

  await c.handleSubmit()
  const subCall = posts.find(([url]) => url === '/subscriptions')
  assert.ok(subCall)
  assert.equal(subCall[1].rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141')
  assert.equal(subCall[1].season, 2)
  assert.equal(pushed.at(-1), '/subscriptions/88')
})

test('搜索与时间表新订阅后记住订阅 ID 并支持跳往详情', async () => {
  const pushed = []
  const searchReq = {
    get: async url => url === '/subscriptions' ? { data: [{ id: 12, title_cn: '旧订阅', bangumi_id: '900' }] } : { data: [] },
    post: async url => url === '/subscriptions' ? { data: { id: 35 } } : { data: {} },
  }
  const { component: s } = view('views/Search', searchReq, { router: { push: p => pushed.push(p) } })
  await s.loadSubscribedList()
  assert.equal(s.findSubscribedId({ title: '旧订阅', url: '', magnet: '', source: 'Mikan', bangumi_id: '900' }), 12)
  await s.saveSubscription({ title: '新番测试', url: 'https://mikan.example/1', magnet: '', source: 'Mikan', bangumi_id: '901' }, 'https://rss.example/1', 'ANi')
  assert.equal(s.findSubscribedId({ title: '新番测试', url: '', magnet: '', source: 'Mikan', bangumi_id: '901' }), 35)

  const schReq = {
    get: async () => ({ data: [] }),
    post: async url => url === '/subscriptions' ? { data: { id: 77 } } : { data: {} },
  }
  const { component: sch } = view('views/Schedule', schReq, { router: { push: p => pushed.push(p) } })
  sch.selectedItem.value = { title: '时间表番剧', url: '', source: 'BGM.tv', bangumi_id: '555', info_hash: '', cover_url: '' }
  sch.modalSelectedSubgroups.value = new Set(['LoliHouse'])
  await sch.subscribeFromModal()
  assert.equal(sch.resolveSubscribedId(sch.selectedItem.value), 77)
})

test('订阅列表 AI 解析支持携带预填参数进入规则新建页', () => {
  const pushed = []
  const { component: subs } = view('views/Subscriptions', { get: async () => ({ data: [] }) }, {
    router: { push: target => pushed.push(target) },
  })
  subs.aiResult.value = { title: '药屋少女的呢喃', season: 2, subgroup_pref: '桜都字幕组' }
  subs.adjustAiRules()
  assert.deepEqual(JSON.parse(JSON.stringify(pushed.at(-1))), {
    path: '/subscriptions/new',
    query: { title: '药屋少女的呢喃', season: '2', subgroup: '桜都字幕组' },
  })
})

test('订阅详情支持立即补全、状态筛选及含文件选项删除', async () => {
  const pushed = []
  const calls = []
  const request = {
    get: async () => ({
      data: {
        subscription: {
          id: 9, title_cn: '测试作品', title_en: '', title_jp: '', year: 2025, season: 1,
          bangumi_id: '', subgroup_name: 'ANi', metadata_id: '', metadata_provider: '', cover_url: '',
          description: '', anime_type: 'TV', total_episodes: 12, current_episodes: 1, stalled_episodes: 0,
          enabled: true, completed: false, rss_url: 'https://rss.example/9',
          filter_json: '{"resolution":"1080p"}', excluded_keywords: '["HEVC"]', allowed_subgroups: '["ANi"]',
          custom_path: '', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', stall_timeout_hours: 0,
        },
        episodes: [
          { id: 1, subscription_id: 9, season: 1, number: 1, title: 'E1', status: 'organized', original_name: '[ANi] Test - 01', final_path: '/a.mkv', file_size: 100, is_stalled: false, group_name: 'ANi', torrent_hash: '', torrent_url: '', download_started_at: '', created_at: '2026-01-01T00:00:00Z' },
          { id: 2, subscription_id: 9, season: 1, number: 2, title: 'E2', status: 'failed', original_name: '[ANi] Test - 02', final_path: '', file_size: 0, is_stalled: false, group_name: 'ANi', torrent_hash: '', torrent_url: '', download_started_at: '', created_at: '2026-01-01T00:00:00Z' },
        ],
      },
    }),
    post: async url => { calls.push(['POST', url]); return { data: {} } },
    delete: async (url, cfg) => { calls.push(['DELETE', url, cfg?.params?.delete_files]); return { data: {} } },
  }
  const { component: d } = view('views/SubscriptionDetail', request, {
    route: { params: { id: '9' } },
    router: { push: p => pushed.push(p) },
  })
  await d.fetchDetail()
  assert.equal(d.activeRuleBadges.value.length > 0, true)
  assert.equal(d.groupedEpisodes.value[0][1].length, 2)
  d.epStatusFilter.value = 'failed'
  assert.equal(d.groupedEpisodes.value[0][1].length, 1)
  assert.equal(d.groupedEpisodes.value[0][1][0].id, 2)

  await d.triggerSupplement()
  assert.deepEqual(calls.find(c => c[0] === 'POST'), ['POST', '/subscriptions/9/trigger-supplement'])

  d.deleteFiles.value = true
  await d.confirmDeleteSub()
  assert.deepEqual(calls.find(c => c[0] === 'DELETE'), ['DELETE', '/subscriptions/9', true])
  assert.equal(pushed.at(-1), '/subscriptions')
})

test('时间表与搜索页 Mikan 弹窗支持多选勾选后再点击确认提交', async () => {
  const mikanGroups = [
    { name: 'ANi', rss_url: 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=583' },
    { name: 'LoliHouse', rss_url: 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=370' },
  ]
  const schPosts = []
  const { component: sch } = view('views/Schedule', {
    get: async () => ({ data: [] }),
    post: async (url, body) => {
      schPosts.push([url, JSON.parse(JSON.stringify(body))])
      return { data: { id: 101 } }
    },
  })
  sch.selectedItem.value = { title: '芙莉莲', url: '', source: 'Mikan', bangumi_id: '3141', info_hash: '', cover_url: '' }
  sch.modalGroups.value = mikanGroups

  // 勾选仅不触发提交，点击确认才提交（多选分支）
  sch.toggleModalSubgroup('ANi')
  sch.toggleModalSubgroup('LoliHouse')
  assert.equal(schPosts.length, 0)
  await sch.subscribeFromModal()
  assert.equal(schPosts.length, 1)
  assert.equal(schPosts[0][1].rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141')
  assert.deepEqual(schPosts[0][1].allowed_subgroups, ['ANi', 'LoliHouse'])
  assert.deepEqual(schPosts[0][1].subgroup_names, ['ANi', 'LoliHouse'])

  // 仅勾选 1 个时保留专属 RSS
  schPosts.length = 0
  sch.modalSelectedSubgroups.value = new Set(['LoliHouse'])
  await sch.subscribeFromModal()
  assert.equal(schPosts[0][1].rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=370')
  assert.equal(schPosts[0][1].subgroup_name, 'LoliHouse')

  // 搜索页 Mikan 弹窗多选后再确认提交
  const searchPosts = []
  const { component: search } = view('views/Search', {
    get: async () => ({ data: [] }),
    post: async (url, body) => {
      searchPosts.push([url, JSON.parse(JSON.stringify(body))])
      return { data: { id: 102 } }
    },
  })
  search.selectedItem.value = { title: '芙莉莲', url: 'https://mikanani.me/Home/Bangumi/3141', magnet: '', source: 'Mikan', bangumi_id: '3141' }
  search.subgroups.value = mikanGroups
  search.showGroupModal.value = true
  search.toggleModalSubgroup('ANi')
  search.toggleModalSubgroup('LoliHouse')
  assert.equal(searchPosts.length, 0)
  await search.confirmGroupSubscribe()
  assert.equal(searchPosts.length, 1)
  assert.equal(searchPosts[0][1].rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141')
  assert.deepEqual(searchPosts[0][1].allowed_subgroups, ['ANi', 'LoliHouse'])
  assert.equal(search.showGroupModal.value, false)
})

test('未勾选直接点订阅时优先走默认字幕组，默认缺失时拦截并弹窗引导选择', async () => {
  const mikanGroups = [
    { name: 'ANi', rss_url: 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=583' },
    { name: 'LoliHouse', rss_url: 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=370' },
  ]
  const schPosts = []
  const { component: sch } = view('views/Schedule', {
    get: async () => ({ data: [] }),
    post: async (url, body) => {
      schPosts.push([url, JSON.parse(JSON.stringify(body))])
      return { data: { id: 201 } }
    },
  })
  sch.selectedItem.value = { title: '芙莉莲', url: '', source: 'Mikan', bangumi_id: '3141', info_hash: '', cover_url: '' }
  sch.modalGroups.value = mikanGroups

  // 1. 无默认且未勾选 -> 弹出 no_selection，不发 POST
  await sch.subscribeFromModal()
  assert.equal(sch.subgroupPromptMode.value, 'no_selection')
  assert.equal(schPosts.length, 0)

  // 2. 设置默认字幕组为番剧不存在的 桜都字幕组 -> 弹出 missing_default，不发 POST
  sch.defaultSubgroups.value = ['桜都字幕组']
  await sch.subscribeFromModal()
  assert.equal(sch.subgroupPromptMode.value, 'missing_default')
  assert.equal(schPosts.length, 0)

  // 3. 在提示框点击“不限字幕组订阅” -> 正常提交
  await sch.subscribeFromModal({ allowUnrestricted: true })
  assert.equal(sch.subgroupPromptMode.value, null)
  assert.equal(schPosts.length, 1)
  assert.equal(schPosts[0][1].rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141')

  // 4. 默认含 ANi -> 未勾选直接点订阅自动使用 ANi 专属 RSS 提交
  schPosts.length = 0
  sch.defaultSubgroups.value = ['桜都字幕组', 'ANi']
  await sch.subscribeFromModal()
  assert.equal(schPosts.length, 1)
  assert.equal(schPosts[0][1].subgroup_name, 'ANi')
  assert.equal(schPosts[0][1].rss_url, 'https://mikanani.me/RSS/Bangumi?bangumiId=3141&subgroupid=583')

  // 5. 搜索页同样拦截不存在的默认字幕组
  const searchPosts = []
  const { component: search } = view('views/Search', {
    get: async () => ({ data: [] }),
    post: async (url, body) => {
      searchPosts.push([url, JSON.parse(JSON.stringify(body))])
      return { data: { id: 202 } }
    },
  })
  search.defaultSubgroups.value = ['桜都字幕组']
  search.selectedItem.value = { title: '芙莉莲', url: 'https://mikanani.me/Home/Bangumi/3141', magnet: '', source: 'Mikan', bangumi_id: '3141' }
  search.subgroups.value = mikanGroups
  search.showGroupModal.value = true
  await search.confirmGroupSubscribe()
  assert.equal(search.searchSubgroupPromptMode.value, 'missing_default')
  assert.equal(searchPosts.length, 0)
})
