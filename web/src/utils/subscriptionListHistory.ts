export type ListFilter = 'all' | 'active' | 'completed' | 'stalled' | 'recycle'
export type ListSort = 'created_at' | 'title' | 'progress' | 'year'

interface ListView {
  search: string
  filter: ListFilter
  sort: ListSort
  y: number
  card?: { id: number; top: number }
}

// 历史条目仅保存随机编号。搜索内容和滚动位置属于当前页面会话。
const views = new Map<string, ListView>()
let session: string | null = null

export function listEntry(history: History | undefined): string | undefined {
  if (!history) return
  if (typeof history.state?.subscriptionListEntry === 'string') return history.state.subscriptionListEntry
  const id = crypto.getRandomValues(new Uint32Array(2)).join('-')
  history.replaceState({ ...history.state, subscriptionListEntry: id }, '')
  return id
}

export function listView(entry: string | undefined, token: string | null): ListView | undefined {
  if (token !== session) {
    views.clear()
    session = token
  }
  return token && entry ? views.get(entry) : undefined
}

export function saveListView(entry: string, token: string, view: ListView) {
  listView(entry, token)
  views.delete(entry)
  views.set(entry, view)
  if (views.size > 32) views.delete(views.keys().next().value!)
}
