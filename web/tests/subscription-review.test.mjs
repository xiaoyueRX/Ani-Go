import assert from 'node:assert/strict'
import test from 'node:test'
import { view } from './view.mjs'

const release = episode => ({
  title: '[CityLab] Harbor Story - ' + episode + ' [1080p]', source: 'Nyaa',
  url: 'https://index.example/' + episode + '.torrent', info_hash: episode,
})
const releases = [release('01'), release('02')]
const parse = { title:'Harbor Story', subgroup:'CityLab', season:1, resolution:'1080p', episode:1 }
async function setup(items, handler) {
  const c = view('components/SubscriptionReviewDialog', { post: handler }, { props: { items } }).component
  await c.prepare()
  return c
}

test('单集及同季多集只生成一条待确认的作品订阅', async () => {
  const created = []
  const c = await setup(releases, async (url, body) => {
    if (url.endsWith('/test')) return { data: parse }
    created.push(JSON.parse(JSON.stringify(body))); return { data: {} }
  })
  assert.equal(created.length,0)
  assert.equal(c.rows.value.length,1)
  assert.equal(c.rows.value[0].items.length,2)
  assert.equal(c.rows.value[0].title,'Harbor Story')
  await c.submit()
  assert.equal(created.length,1)
  assert.equal(created[0].season,1)
  assert.equal(created[0].title_cn,'Harbor Story')
  assert.equal(created[0].subgroup_name,'CityLab')
  assert.equal(JSON.parse(created[0].filter_json).resolution,'1080p')
  assert.equal(created[0].rss_url,undefined)
})

test('无法确认作品时需手填；重试不会重新发送已成功项目', async () => {
  const attempts = []
  let offline=true
  const c=await setup([releases[0],{ title:'Ambiguous release', url:'https://site.example/item' }], async (url,body)=>{
    if (url.endsWith('/test')) {
      if (body.title==='Ambiguous release') throw Error('unavailable')
      return {data:parse}
    }
    attempts.push(body.title_cn)
    if(body.title_cn==='My series' && offline) throw Error('offline')
    return {data:{}}
  })
  assert.equal(c.rows.value[1].title,'')
  await c.submit()
  assert.equal(attempts.length,0)
  c.rows.value[1].title='My series'
  await c.submit()
  assert.equal(c.rows.value.length,1)
  assert.ok(c.rows.value[0].error)
  offline=false
  await c.submit()
  assert.deepEqual(attempts,['Harbor Story','My series','My series'])
})

test('普通结果先打开审阅，有作品 ID 时迟到的字幕组不会覆盖新作品', async () => {
  const calls=[]
  const pending=[]
  const c=view('views/Search',{post:async (...args)=>calls.push(args),get:()=>new Promise(resolve=>pending.push(resolve))}).component
  await c.openSubscribe(releases[0])
  assert.equal(calls.length,0)
  assert.equal(c.reviewItems.value[0].title,releases[0].title)
  const series = (title,id) => ({title,source:'Mikan',bangumi_id:id})
  const first=c.openSubscribe(series('A','1'))
  const second=c.openSubscribe(series('B','2'))
  pending[1]({data:[{name:'B group'}]});await second
  pending[0]({data:[{name:'A group'}]});await first
  assert.equal(c.subgroups.value[0].name,'B group')
})

test('混合字幕组与画质留给用户选择，不同季保留单独订阅', async () => {
  const c = await setup([...releases, release('S02E01')], async (_path, body) => ({
    data: body.title === releases[0].title ? parse
      : { ...parse, season: body.title.includes('S02') ? 2 : 1, subgroup: 'Another group', resolution: '720p' },
  }))
  assert.equal(c.rows.value.length, 2)
  assert.equal(c.rows.value[0].subgroup, '')
  assert.equal(c.rows.value[0].resolution, '')
  assert.equal(c.rows.value[1].season, 2)
  assert.equal(c.rows.value[1].subgroup, 'Another group')
})

test('解析回执缺少作品名时仍可手工确认', async () => {
  const c = await setup([releases[0]], async () => ({ data: { episode: 2 } }))
  assert.equal(c.rows.value[0].title, '')
  assert.equal(c.loading.value, false)
})

test('批量确认绑定打开时的登录，换会话后不继续提交后续作品', async () => {
  const storage = new Map([['token', 'original']])
  const calls = []
  const items = ['Harbor', 'Island'].map(name => ({ title: name + ' - 01', source: 'Nyaa' }))
  const c = view('components/SubscriptionReviewDialog', { post: async (url, body, config) => {
    if (url.endsWith('/test')) return { data: { ...parse, title: body.title.split(' - ')[0] } }
    calls.push(config.confirmedSession)
    storage.set('token', 'replacement')
    return { data: {} }
  } }, { storage, props: { items } }).component
  await c.prepare()
  await c.submit()
  assert.deepEqual(calls, ['original'])
  assert.equal(c.saved.value, 0)
})
