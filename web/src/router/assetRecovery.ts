import { isNavigationFailure, NavigationFailureType, type RouteLocationNormalized, type RouteRecordNormalized, type Router, type RouterHistory } from 'vue-router'

const recoveryKey = 'ani-go-route-asset-recovery'

function entryScript(doc: Document) {
  const src = doc.querySelector('script[type="module"][src^="/assets/index-"]')?.getAttribute('src')
  return src && /^\/assets\/index-[\w-]+\.js$/.test(src) ? src : null
}

type EntryState = 'changed' | 'same' | 'unavailable' | 'unverified'

interface Traversal {
  path: string
  position: number
  delta: number
}

// Vue Router 在报告错误前先撤销后退或前进。等浏览器回到起点后，再前往原历史项并加载新文档。
function revisit(history: RouterHistory, pop: Traversal, current: () => boolean, replace: () => void) {
  const resume = () => {
    if (!current() || history.state.position !== pop.position - pop.delta) return
    window.addEventListener('popstate', () => {
      if (current() && history.state.position === pop.position) replace()
    }, { once: true })
    history.go(pop.delta, false)
  }
  if (history.state.position === pop.position) window.addEventListener('popstate', resume, { once: true })
  else resume()
}

export function installAssetRecovery(router: Router) {
  const original = entryScript(document)
  if (!original) return

  const failures = new WeakSet<Error>()
  const history = router.options.history
  let nextTraversal: Traversal | undefined
  let intent: { route: RouteLocationNormalized; pop?: Traversal; checked?: boolean } | undefined
  // 静态路由记录在各个 URL 参数间共享；同一标签的内存仅随路由种类增长。
  const routeFailures = new WeakMap<RouteRecordNormalized, { unavailable: boolean }>()
  let checking: Promise<EntryState> | undefined
  let lastEntry: EntryState = 'unverified'
  let checkedUntil = 0

  function checkEntry() {
    if (checking) return checking
    if (Date.now() < checkedUntil) return Promise.resolve(lastEntry)
    checking = fetch('/', { cache: 'no-store', redirect: 'manual', signal: AbortSignal.timeout(5000) })
      .then(async (response): Promise<EntryState> => {
        if ([502, 503, 504].includes(response.status)) return 'unavailable'
        if (!response.ok || !response.headers.get('Content-Type')?.startsWith('text/html')) return 'unverified'
        const next = entryScript(new DOMParser().parseFromString(await response.text(), 'text/html'))
        if (!next) return 'unverified'
        return next === original ? 'same' : 'changed'
      })
      .catch((): EntryState => 'unavailable')
      .then(result => {
        lastEntry = result
        // 整站停服时，允许下一次主动导航立即重试。重复上报同一次导航仍只检查一次。
        checkedUntil = result === 'unavailable' ? 0 : Date.now() + 5000
        checking = undefined
        return result
      })
    return checking
  }

  window.addEventListener('vite:preloadError', event => {
    if (event.payload !== null && typeof event.payload === 'object') failures.add(event.payload)
  })
  history.listen((path, _from, { delta }) => {
    const position = history.state.position
    nextTraversal = delta && typeof position === 'number' ? { path, position, delta } : undefined
  })
  router.beforeEach(to => {
    intent = { route: to, pop: nextTraversal?.path === to.fullPath ? nextTraversal : undefined }
    nextTraversal = undefined
  })
  router.afterEach((to, _from, failure) => {
    if (!failure || isNavigationFailure(failure, NavigationFailureType.duplicated)) intent = undefined
    if (!failure) {
      const record = to.matched.at(-1)
      if (record) routeFailures.delete(record)
      try { sessionStorage.removeItem(recoveryKey) } catch { /* 浏览器可能禁用会话存储。 */ }
    }
  })
  router.onError(async (error, to) => {
    console.error(error)
    const active = intent
    if (!active || active.route !== to || active.checked || !failures.has(error)) return
    active.checked = true
    const record = to.matched.at(-1)
    if (!record) return
    const failedRoute = routeFailures.get(record) ?? { unavailable: false }
    routeFailures.set(record, failedRoute)
    const entry = await checkEntry()
    if (entry === 'unavailable') failedRoute.unavailable = true
    if (active !== intent) return
    // 浏览器会记住失败的动态导入。只有用户再次打开停服时失败的页面才恢复同一版本。
    if (entry !== 'changed' && !(entry === 'same' && failedRoute.unavailable)) return
    const href = router.resolve(to.fullPath).href
    if (new URL(href, location.origin).origin !== location.origin) return
    try {
      if (sessionStorage.getItem(recoveryKey)) return
    } catch { return }
    function load(replace: boolean) {
      // 新文档成功进入页面才解除标记，避免混合部署反复刷新。
      try { sessionStorage.setItem(recoveryKey, '1') } catch { return }
      if (replace) location.replace(href)
      else location.assign(href)
    }
    if (active.pop) revisit(history, active.pop, () => active === intent, () => load(true))
    else load(href === location.pathname + location.search + location.hash)
  })
}
