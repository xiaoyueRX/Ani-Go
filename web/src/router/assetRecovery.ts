import { isNavigationFailure, NavigationFailureType, type RouteLocationNormalized, type Router, type RouterHistory } from 'vue-router'

const recoveryKey = 'ani-go-route-asset-recovery'

function entryScript(doc: Document) {
  const src = doc.querySelector('script[type="module"][src^="/assets/index-"]')?.getAttribute('src')
  return src && /^\/assets\/index-[\w-]+\.js$/.test(src) ? src : null
}

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
  let intent: { route: RouteLocationNormalized; pop?: Traversal; recovering?: boolean } | undefined
  let checking: Promise<boolean> | undefined
  let changed = false
  let checkedUntil = 0

  function entryChanged() {
    if (checking) return checking
    if (Date.now() < checkedUntil) return Promise.resolve(changed)
    checking = fetch('/', { cache: 'no-store', redirect: 'error', signal: AbortSignal.timeout(5000) })
      .then(async response => {
        if (!response.ok || !response.headers.get('Content-Type')?.startsWith('text/html')) return false
        const next = entryScript(new DOMParser().parseFromString(await response.text(), 'text/html'))
        return next !== null && next !== original
      })
      .catch(() => false)
      .then(result => {
        changed = result
        checkedUntil = Date.now() + 5000
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
  router.afterEach((_to, _from, failure) => {
    if (!failure || isNavigationFailure(failure, NavigationFailureType.duplicated)) intent = undefined
    if (!failure) {
      try { sessionStorage.removeItem(recoveryKey) } catch { /* 浏览器可能禁用会话存储。 */ }
    }
  })
  router.onError(async (error, to) => {
    console.error(error)
    const active = intent
    if (!active || active.route !== to || !failures.has(error)) return
    if (!await entryChanged() || active !== intent || active.recovering) return
    const href = router.resolve(to.fullPath).href
    if (new URL(href, location.origin).origin !== location.origin) return
    try {
      if (sessionStorage.getItem(recoveryKey)) return
    } catch { return }
    active.recovering = true
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
