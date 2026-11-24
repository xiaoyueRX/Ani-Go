import { isNavigationFailure, NavigationFailureType, type RouteLocationNormalized, type RouteRecordNormalized, type Router, type RouterHistory } from 'vue-router'

const recoveryKey = 'ani-go-route-asset-recovery'

function entryScript(doc: Document) {
  const src = doc.querySelector('script[type="module"][src^="/assets/index-"]')?.getAttribute('src')
  return src && /^\/assets\/index-[\w-]+\.js$/.test(src) ? src : null
}

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function localKey(value: unknown): value is string {
  return typeof value === 'string' && /^[\w.-]+(?:\/[\w.-]+)*$/.test(value)
    && !value.split('/').some(part => part === '.' || part === '..')
}

function asset(path: unknown, suffix: '.js' | '.css'): path is string {
  return typeof path === 'string' && path.endsWith(suffix) && /^assets\/[\w.-]+-[\w-]+\.(?:js|css)$/.test(path)
}

function staticAssets(manifest: unknown, sources: string[], original: string) {
  if (!object(manifest)) return
  const entry = manifest['index.html']
  if (!object(entry) || entry.isEntry !== true || entry.file !== original.slice(1)) return
  for (const source of sources) {
    const chunk = manifest[source]
    if (!object(chunk) || chunk.src !== source || chunk.isDynamicEntry !== true) return
  }

  const files = new Set<string>()
  const visited = new Set<string>()
  const queue = ['index.html', ...sources]
  while (queue.length) {
    const key = queue.pop()!
    if (visited.has(key)) continue
    visited.add(key)
    const chunk = Object.hasOwn(manifest, key) && manifest[key]
    if (!object(chunk) || !asset(chunk.file, '.js')) return
    const styles = chunk.css === undefined ? [] : chunk.css
    const imports = chunk.imports === undefined ? [] : chunk.imports
    if (!Array.isArray(styles) || !styles.every(path => asset(path, '.css'))) return
    if (!Array.isArray(imports) || !imports.every(localKey)) return
    files.add('/' + chunk.file)
    for (const css of styles) files.add('/' + css)
    // dynamicImports 是其它用户操作才会加载的模块，不是渲染当前页面的前置依赖。
    for (const dep of imports) queue.push(dep)
  }
  return files
}

// 首次恢复时才取清单。完整性校验绑定当前文档的构建，校验失败则沿用全量复验。
export function routeAssetScope(original: string, integrity?: string | null) {
  let manifest: Promise<unknown> | undefined
  return async (sources: unknown[]) => {
    if (!integrity || !/^sha256-[A-Za-z0-9+/]{43}=$/.test(integrity) || !('integrity' in Request.prototype)) return
    if (!sources.length || !sources.every((key): key is string => localKey(key) && key.startsWith('src/views/') && key.endsWith('.vue'))) return
    const request = manifest ??= fetch('/route-manifest.json', {
      cache: 'no-store', redirect: 'error', integrity, signal: AbortSignal.timeout(5000),
    }).then(response => {
      if (response.ok && response.headers.get('Content-Type')?.split(';')[0].trim().toLowerCase() === 'application/json') return response.json()
    }).catch(() => undefined)
    const files = staticAssets(await request, sources, original)
    if (!files && manifest === request) manifest = undefined
    return files
  }
}

type EntryState = 'changed' | 'same' | 'unavailable' | 'unverified'

interface RouteFailure {
  unavailable: boolean
  checking?: RouteLocationNormalized
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

// WebKit 会跨文档复用失败的 modulepreload，包含路由模块的相对依赖。
function trackFailedPreloads(original: string) {
  const assetScope = routeAssetScope(original, document.querySelector('meta[name="anigo-route-manifest"]')?.getAttribute('content'))
  const failed = new Map<string, Promise<boolean> | null>()
  document.addEventListener('error', ({ target }) => {
    if (!(target instanceof HTMLLinkElement)) return
    const url = new URL(target.href)
    if (url.origin !== location.origin || url.search || url.hash) return
    if (!/^\/assets\/[\w.-]+-[\w-]+\.(js|css)$/.test(url.pathname)) return
    const rel = url.pathname.endsWith('.js') ? 'modulepreload' : 'stylesheet'
    if (target.rel === rel && !failed.has(url.pathname)) failed.set(url.pathname, null)
  }, true)

  async function refetch(path: string) {
    try {
      const response = await fetch(path, { cache: 'reload', redirect: 'error', signal: AbortSignal.timeout(15000) })
      const type = response.headers.get('Content-Type')?.split(';')[0]?.trim().toLowerCase() ?? ''
      const valid = path.endsWith('.css') ? type === 'text/css' : /^(?:text|application)\/(?:java|ecma)script$/.test(type)
      if (!response.ok || !valid) {
        await response.body?.cancel()
        return false
      }
      await response.blob()
      return true
    } catch { return false }
  }

  return async (to: RouteLocationNormalized, current: () => boolean) => {
    if (!failed.size) return true
    const required = await assetScope(to.matched.map(record => record.meta?.sourceModule))
    if (!current()) return false
    const results = await Promise.all(Array.from(failed, ([path, pending]) => {
      if (required && !required.has(path)) return true
      if (pending) return pending
      const request = refetch(path).then(ok => {
        if (ok) failed.delete(path)
        else failed.set(path, null)
        return ok
      })
      failed.set(path, request)
      return request
    }))
    return results.every(ok => ok)
  }
}

export function installAssetRecovery(router: Router) {
  const original = entryScript(document)
  if (!original) return

  const restorePreloads = trackFailedPreloads(original)
  const failures = new WeakSet<Error>()
  const history = router.options.history
  let nextTraversal: Traversal | undefined
  let intent: { route: RouteLocationNormalized; pop?: Traversal; checked?: boolean } | undefined
  // 静态路由记录在各个 URL 参数间共享；同一标签的内存仅随路由种类增长。
  const routeFailures = new WeakMap<RouteRecordNormalized, RouteFailure>()
  let checking: Promise<EntryState> | undefined
  let lastEntry: EntryState = 'unverified'
  let checkedUntil = 0
  let checkedRoute: RouteRecordNormalized | undefined

  function checkEntry(record: RouteRecordNormalized) {
    if (checking) return checking
    // 新页面的模块也失败时要重新确认部署，先前页面的结果只用于限制它自身的重试频率。
    if (checkedRoute === record && Date.now() < checkedUntil) return Promise.resolve(lastEntry)
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
        checkedRoute = record
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
    const anotherAttempt = !!failedRoute.checking
    failedRoute.checking = to
    let entry = await checkEntry(record)
    if (entry === 'unavailable') {
      failedRoute.unavailable = true
      // 用户再次点击时旧请求尚未结束，新操作可以在旧连接失败后再确认一次。
      if (anotherAttempt && active === intent) entry = await checkEntry(record)
    }
    if (failedRoute.checking === to) delete failedRoute.checking
    if (active !== intent) return
    // 浏览器会记住失败的动态导入。只有用户再次打开停服时失败的页面才恢复同一版本。
    if (entry !== 'changed' && !(entry === 'same' && failedRoute.unavailable)) return
    const href = router.resolve(to.fullPath).href
    if (new URL(href, location.origin).origin !== location.origin) return
    try {
      if (sessionStorage.getItem(recoveryKey)) return
    } catch { return }
    if (entry === 'same' && (!await restorePreloads(to, () => active === intent) || active !== intent)) return
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
