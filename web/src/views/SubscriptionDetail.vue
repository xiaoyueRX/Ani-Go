<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listView } from '../utils/subscriptionListHistory'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import {
  ChevronLeft, RefreshCw, AlertTriangle,
  Image, Edit3, Pause, Play,
  History, Download, Check,
  MoreVertical, FileText, X,
  Trash2,
  Shield, FileCheck2
} from 'lucide-vue-next'
import SubscriptionEditForm from '../components/SubscriptionEditForm.vue'
import { summarizeRules } from '../utils/subscriptionRules'

interface Subscription {
  id: number; title_cn: string; title_en: string; title_jp: string
  year: number; season: number; bangumi_id: string; subgroup_name: string
  metadata_id: string; metadata_provider: string; cover_url: string
  description: string; anime_type: string
  total_episodes: number; current_episodes: number; stalled_episodes: number
  enabled: boolean; completed: boolean
  rss_url?: string
  filter_json: string; custom_path: string
  created_at: string; updated_at: string
  avatar_url?: string
  excluded_keywords?: string
  skip_bulk_update?: boolean
  allowed_subgroups?: string
  stall_timeout_hours: number
}

interface Episode {
  id: number; subscription_id: number; season: number; number: number
  title: string; status: string; manual_status?: string; torrent_hash: string; torrent_url: string
  original_name: string; final_path: string; file_size: number
  is_stalled: boolean; group_name?: string
  review_reason?: string; review_kind?: string; source_file?: string
  can_retry_organize?: boolean; can_assign_episode?: boolean; can_confirm_movie?: boolean
  download_started_at: string; created_at: string
}

type ReviewCorrection = { season: number; number: number } | { confirm_movie: true }
interface RecheckResult {
  id: number
  name: string
  status: 'queued' | 'failed' | 'skipped'
  reason?: string
}

type FileStatus = 'size_matches' | 'present' | 'empty' | 'size_mismatch' | 'missing' | 'unverified' | 'not_applicable'
interface FileVerification {
  id: number
  status: FileStatus
  reason?: 'path' | 'library_unavailable' | 'volume_unavailable' | 'file_type' | 'access'
  expected_size?: number
  actual_size?: number
}
type FileReportRow = Omit<FileVerification, 'status'> & {
  name: string; status: FileStatus | 'request_failed'; restoring?: boolean; restoreError?: string
}

const route = useRoute()
const router = useRouter()
function returnToList() {
  if (listView(window.history.state?.subscriptionListOrigin, localStorage.getItem('token'))) router.back()
  else router.push('/subscriptions')
}
const id = Number(route.params.id)

const { t, locale } = useI18n({ useScope: 'global' })

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

const sub = ref<Subscription | null>(null)
const episodes = ref<Episode[]>([])
const loading = ref(true)
type TaskState = 'present' | 'missing' | 'unknown'
function boundTaskKey(ep: Episode) {
  if (ep.status !== 'downloading' && ep.status !== 'downloaded') return ''
  const key = ep.torrent_hash?.trim().toLowerCase() || ''
  return key.startsWith('url:') ? '' : key
}
const taskScope = computed(() => [...new Set(episodes.value.map(boundTaskKey).filter(Boolean))].sort().join(','))
const taskStates = ref<Record<string, TaskState>>({})
const taskCheckFailed = ref(false)
const checkingTasks = ref(false)
const checkedTasksAt = ref(0)
const hasMissingTask = computed(() => Object.values(taskStates.value).includes('missing'))
let taskRequest: AbortController | undefined
function episodeTaskState(ep: Episode) {
  return taskStates.value[boundTaskKey(ep)]
}
async function checkDownloadTasks() {
  taskRequest?.abort()
  const controller = new AbortController()
  taskRequest = controller
  taskStates.value = {}
  taskCheckFailed.value = false
  checkedTasksAt.value = 0
  checkingTasks.value = !!taskScope.value
  if (!checkingTasks.value) return
  try {
    const { data } = await request.get(`/subscriptions/${id}/download-tasks`, { signal: controller.signal })
    if (!controller.signal.aborted) {
      if (!data?.tasks || typeof data.tasks !== 'object' || Array.isArray(data.tasks)) {
        taskCheckFailed.value = true
        return
      }
      taskStates.value = data.tasks
      checkedTasksAt.value = Date.now()
    }
  } catch {
    if (!controller.signal.aborted) taskCheckFailed.value = true
  } finally {
    if (!controller.signal.aborted) checkingTasks.value = false
  }
}
// 同一批任务只在打开详情时核对一次，定时刷新剧集列表不重复读取下载器。
watch(taskScope, () => { void checkDownloadTasks() })
const error = ref('')
const actionError = ref('')
let detailRead = 0
const showEdit = ref(false)
const editDialog = ref<HTMLDialogElement | null>(null)
const updatingEps = ref<Set<number>>(new Set())
const selectedEpisodes = ref<Set<number>>(new Set())
const showBulkActions = ref(false)
const bulkActionLoading = ref(false)
const fileReport = ref<{ rows: FileReportRow[]; checkedAt: number; volumeGuarded: boolean | null } | null>(null)
const verifyingFiles = ref(false)
const hasOrganizedFiles = computed(() => episodes.value.some(ep => ep.status === 'organized'))
const fileScope = computed(() => [route.params.id, ...selectedEpisodes.value].join(','))
let verificationRequest: AbortController | undefined
let verifiedScope = ''
function clearFileReport() {
  verificationRequest?.abort()
  verificationRequest = undefined
  verifyingFiles.value = false
  fileReport.value = null
}
watch(fileScope, scope => {
  if (scope !== verifiedScope) clearFileReport()
})
const recheckResults = ref<RecheckResult[]>([])
const recheckCounts = computed(() => {
  const counts = { queued: 0, failed: 0, skipped: 0 }
  for (const result of recheckResults.value) counts[result.status]++
  return counts
})
const reviewErrors = ref<Record<number, string>>({})
const assignment = ref<{ id: number; sourceFile: string; season: number | ''; number: number | '' } | null>(null)
const validAssignment = computed(() => {
  if (!assignment.value) return false
  const { season, number } = assignment.value
  return typeof season === 'number' && Number.isInteger(season) && season >= 0 && season <= 99 &&
    typeof number === 'number' && number > 0 && number <= 2000
})

function reviewConfirmation(ep: Episode) {
  if (ep.status !== 'needs_review' || !ep.source_file) return
  if (ep.review_kind === 'episode_identity' && ep.can_assign_episode) return 'assign'
  if (ep.review_kind === 'movie_selection' && ep.can_confirm_movie) return 'movie'
}

function editAssignment(ep: Episode) {
  if (reviewConfirmation(ep) !== 'assign' || !ep.source_file || updatingEps.value.has(ep.id) || bulkActionLoading.value) return
  assignment.value = { id: ep.id, sourceFile: ep.source_file, season: ep.season, number: ep.number > 0 ? ep.number : '' }
}

async function retryWithAssignment(ep: Episode) {
  const draft = assignment.value
  if (!ep.can_assign_episode || !draft || ep.id !== draft.id || ep.source_file !== draft.sourceFile || !validAssignment.value) return
  await retryOrganize(ep, { season: Number(draft.season), number: Number(draft.number) })
}

async function queueOrganization(ep: Episode, correction?: ReviewCorrection) {
  delete reviewErrors.value[ep.id]
  const { data } = await request.post(`/episodes/${ep.id}/retry-organize`, correction)
  const current = episodes.value.find(row => row.id === ep.id)
  if (current) {
    current.status = data.status
    current.review_reason = ''
    if (correction && 'season' in correction) Object.assign(current, correction)
  }
  if (assignment.value?.id === ep.id) assignment.value = null
}

async function retryOrganize(ep: Episode, correction?: ReviewCorrection) {
  if (ep.status !== 'needs_review' || updatingEps.value.has(ep.id) || bulkActionLoading.value) return
  if (correction) {
    if (reviewConfirmation(ep) !== ('confirm_movie' in correction ? 'movie' : 'assign')) return
  } else if (!ep.can_retry_organize) return
  updatingEps.value.add(ep.id)
  actionError.value = ''
  try {
    await queueOrganization(ep, correction)
    await fetchDetail()
  } catch (e: any) {
    const message = e.response?.data?.error || t(!correction && ep.review_kind ? 'detail.review.recheckFailed' : 'detail.review.failed')
    if ([404, 409].includes(e.response?.status)) await fetchDetail()
    if (episodes.value.some(row => row.id === ep.id && row.status === 'needs_review')) reviewErrors.value[ep.id] = message
    else actionError.value = message
  } finally {
    updatingEps.value.delete(ep.id)
  }
}

async function setEpisodeMark(ep: Episode, event: Event) {
  const select = event.target as HTMLSelectElement
  const mark = select.value
  select.value = ep.manual_status || ''
  if (updatingEps.value.has(ep.id) || bulkActionLoading.value || mark === select.value) return
  updatingEps.value.add(ep.id)
  actionError.value = ''
  try {
    const { data } = await request.put(`/episodes/${ep.id}/status`, { status: mark })
    ep.manual_status = data.manual_status
    await fetchDetail()
  } catch (e: any) {
    actionError.value = e.response?.data?.error || t('detail.error.update')
  } finally {
    updatingEps.value.delete(ep.id)
  }
}

function openEditDialog() {
  showEdit.value = true
  editDialog.value?.showModal()
}
function closeEditDialog() {
  showEdit.value = false
  editDialog.value?.close()
}

const supplementing = ref(false)
const showDeleteModal = ref(false)
const deleteFiles = ref(false)
const deletingSub = ref(false)

async function triggerSupplement() {
  if (supplementing.value) return
  supplementing.value = true
  actionError.value = ''
  try {
    await request.post(`/subscriptions/${id}/trigger-supplement`)
    window.showToast?.(t('detail.actions.supplementTriggered'), 'success')
    await fetchDetail()
  } catch (e: any) {
    actionError.value = e.response?.data?.error || t('detail.actions.supplementFailed')
  } finally {
    supplementing.value = false
  }
}

function openDeleteModal() {
  deleteFiles.value = false
  showDeleteModal.value = true
}

async function confirmDeleteSub() {
  if (deletingSub.value) return
  deletingSub.value = true
  actionError.value = ''
  try {
    await request.delete(`/subscriptions/${id}`, { params: { delete_files: deleteFiles.value } })
    showDeleteModal.value = false
    window.showToast?.(t('detail.deleteModal.success'), 'success')
    router.push('/subscriptions')
  } catch (e: any) {
    actionError.value = e.response?.data?.error || t('detail.deleteModal.failed')
    showDeleteModal.value = false
  } finally {
    deletingSub.value = false
  }
}

async function fetchDetail() {
  const sequence = ++detailRead
  loading.value = true
  try {
    const { data } = await request.get(`/subscriptions/${id}`)
    if (sequence !== detailRead) return
    error.value = ''
    sub.value = data.subscription
    episodes.value = data.episodes || []
    const visible = new Set(episodes.value.map(ep => ep.id))
    selectedEpisodes.value = new Set([...selectedEpisodes.value].filter(id => visible.has(id)))
    showBulkActions.value = selectedEpisodes.value.size > 0
  } catch (e: any) {
    if (sequence !== detailRead) return
    error.value = e.response?.data?.error || t('detail.error.load')
    if ([401, 403, 404].includes(e.response?.status)) {
      sub.value = null
      episodes.value = []
      clearSelection()
    }
  } finally {
    if (sequence === detailRead) loading.value = false
  }
}

async function handleSaveEdit(updated: Record<string, any>) {
  try {
    const { data } = await request.put(`/subscriptions/${id}`, updated)
    ++detailRead
    loading.value = false
    sub.value = data
    closeEditDialog()
    return true
  } catch (e: any) {
    alert(e.response?.data?.error || t('detail.error.update'))
    return false
  }
}

const statusCfg = computed<Record<string, { label: string; icon: any; cls: string }>>(() => ({
  pending: { label: t('detail.logs.status.pending'), icon: History, cls: 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-[var(--v2-border)]' },
  submitting: { label: t('detail.logs.status.submitting'), icon: Download, cls: 'bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] border-[var(--v2-accent)]/25' },
  retrying: { label: t('detail.logs.status.retrying'), icon: RefreshCw, cls: 'bg-amber-500/12 text-amber-400 border-amber-500/25' },
  downloaded: { label: t('detail.logs.status.downloaded'), icon: Check, cls: 'bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] border-[var(--v2-accent)]/25' },
  renaming: { label: t('detail.logs.status.organizing'), icon: FileText, cls: 'bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] border-[var(--v2-accent)]/25' },
  needs_review: { label: t('detail.logs.status.needsReview'), icon: AlertTriangle, cls: 'bg-amber-500/12 text-amber-400 border-amber-500/25' },
  downloading: { label: t('detail.logs.status.active'), icon: Download, cls: 'bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] border-[var(--v2-accent)]/25' },
  completed: { label: t('detail.logs.status.finished'), icon: Check, cls: 'bg-emerald-500/12 text-emerald-400 border-emerald-500/25' },
  organized: { label: t('detail.logs.status.organized'), icon: Check, cls: 'bg-emerald-500/12 text-emerald-400 border-emerald-500/25' },
  failed: { label: t('detail.logs.status.failed'), icon: AlertTriangle, cls: 'bg-red-500/12 text-red-400 border-red-500/25' },
}))

function formatSize(bytes: number): string {
  if (!bytes) return '-'
  if (bytes > 1e9) return (bytes / 1e9).toFixed(2) + ' GB'
  if (bytes > 1e6) return (bytes / 1e6).toFixed(1) + ' MB'
  return (bytes / 1e3).toFixed(0) + ' KB'
}

function episodeLabel(ep: Episode): string {
  if (ep.number === 0 && ['movie', 'film', '剧场版', '劇場版'].includes(sub.value?.anime_type?.trim().toLowerCase() ?? '')) {
    return t('detail.logs.unnumbered')
  }
  const season = ep.season === 0 || ep.season > 1 ? `S${ep.season}` : ''
  return `${season}E${ep.number}`
}

type EpFilterStatus = 'all' | 'needs_review' | 'downloading' | 'organized' | 'failed'
const epStatusFilter = ref<EpFilterStatus>('all')
const epSearchQuery = ref('')

const activeRuleBadges = computed(() => (sub.value ? summarizeRules(sub.value) : []))

const epFilterCounts = computed(() => {
  const counts: Record<EpFilterStatus, number> = {
    all: episodes.value.length,
    needs_review: 0,
    downloading: 0,
    organized: 0,
    failed: 0,
  }
  for (const ep of episodes.value) {
    if (ep.status === 'needs_review') counts.needs_review++
    else if (ep.status === 'failed') counts.failed++
    else if (ep.status === 'organized' || ep.status === 'completed') counts.organized++
    else counts.downloading++
  }
  return counts
})

const filteredEpisodes = computed(() => {
  let list = episodes.value
  if (epStatusFilter.value === 'needs_review') {
    list = list.filter(ep => ep.status === 'needs_review')
  } else if (epStatusFilter.value === 'failed') {
    list = list.filter(ep => ep.status === 'failed')
  } else if (epStatusFilter.value === 'organized') {
    list = list.filter(ep => ep.status === 'organized' || ep.status === 'completed')
  } else if (epStatusFilter.value === 'downloading') {
    list = list.filter(ep => !['needs_review', 'failed', 'organized', 'completed'].includes(ep.status))
  }
  const q = epSearchQuery.value.trim().toLowerCase()
  if (q) {
    list = list.filter(ep =>
      (ep.original_name || '').toLowerCase().includes(q)
      || (ep.title || '').toLowerCase().includes(q)
      || (ep.group_name || '').toLowerCase().includes(q)
      || episodeLabel(ep).toLowerCase().includes(q),
    )
  }
  return list
})

// 按字幕组分组的剧集列表
const groupedEpisodes = computed(() => {
  const groups: Record<string, Episode[]> = {}
  for (const ep of filteredEpisodes.value) {
    const key = ep.group_name || t('detail.logs.ungrouped')
    if (!groups[key]) groups[key] = []
    groups[key].push(ep)
  }
  const sorted = Object.entries(groups).sort(([a], [b]) => {
    if (a === t('detail.logs.ungrouped')) return 1
    if (b === t('detail.logs.ungrouped')) return -1
    return a.localeCompare(b)
  })
  return sorted
})

// 原生 Vue 折叠状态（替代原失效的 DaisyUI collapse-arrow）
const expandedGroups = ref<Record<string, boolean>>({})
function isGroupExpanded(groupName: string, idx: number): boolean {
  if (groupName in expandedGroups.value) return expandedGroups.value[groupName]
  return idx === 0
}
function toggleGroup(groupName: string, idx: number) {
  expandedGroups.value[groupName] = !isGroupExpanded(groupName, idx)
}

const progress = computed(() => {
  if (!sub.value || sub.value.total_episodes <= 0) return null
  return Math.min(100, (sub.value.current_episodes / sub.value.total_episodes) * 100)
})

function toggleEpisodeSelect(epId: number) {
  if (bulkActionLoading.value) return
  if (selectedEpisodes.value.has(epId)) {
    selectedEpisodes.value.delete(epId)
  } else {
    selectedEpisodes.value.add(epId)
  }
  showBulkActions.value = selectedEpisodes.value.size > 0
}

function toggleSelectAll(groupEps: Episode[]) {
  if (bulkActionLoading.value) return
  const allSelected = groupEps.every(ep => selectedEpisodes.value.has(ep.id))
  if (allSelected) {
    groupEps.forEach(ep => selectedEpisodes.value.delete(ep.id))
  } else {
    groupEps.forEach(ep => selectedEpisodes.value.add(ep.id))
  }
  showBulkActions.value = selectedEpisodes.value.size > 0
}

function clearSelection() {
  selectedEpisodes.value.clear()
  showBulkActions.value = false
}

function checkFileIds(ids: number[], signal?: AbortSignal) {
  return request.post<{ results: FileVerification[]; volume_guarded: boolean }>(
    `/subscriptions/${id}/verify-files`, { ids }, { signal },
  )
}

async function verifyFiles(mode: 'organized' | 'selected') {
  if (verifyingFiles.value || bulkActionLoading.value || updatingEps.value.size) return
  const items = episodes.value.filter(ep => mode === 'organized' ? ep.status === 'organized' : selectedEpisodes.value.has(ep.id))
  if (!items.length) return
  const controller = new AbortController()
  verificationRequest = controller
  verifiedScope = fileScope.value
  verifyingFiles.value = true
  fileReport.value = null
  const rows: FileReportRow[] = []
  let volumeGuarded: boolean | null = null
  try {
    for (let start = 0; start < items.length; start += 100) {
      if (controller.signal.aborted || fileScope.value !== verifiedScope) return
      const batch = items.slice(start, start + 100)
      let results: FileVerification[] = []
      try {
        const { data } = await checkFileIds(batch.map(ep => ep.id), controller.signal)
        results = data.results ?? []
        if (typeof data.volume_guarded === 'boolean') volumeGuarded = volumeGuarded !== false && data.volume_guarded
      } catch {
        if (controller.signal.aborted) return
      }
      for (const [index, ep] of batch.entries()) {
        const name = (ep.final_path || ep.source_file || ep.original_name || ep.title || '').split(/[\\/]/).pop() || episodeLabel(ep)
        const result = results[index]
        rows.push(result?.id === ep.id ? { ...result, name } : { id: ep.id, name, status: 'request_failed' })
      }
    }
    if (!controller.signal.aborted && fileScope.value === verifiedScope) {
      fileReport.value = { rows, checkedAt: Date.now(), volumeGuarded }
    }
  } finally {
    if (verificationRequest === controller) {
      verificationRequest = undefined
      verifyingFiles.value = false
    }
  }
}

async function restoreFile(row: FileReportRow) {
  const report = fileReport.value
  if (!report?.rows.includes(row) || fileScope.value !== verifiedScope || row.status !== 'missing' ||
      updatingEps.value.has(row.id) || verifyingFiles.value || bulkActionLoading.value) return
  const unmarked = report.volumeGuarded !== true
  if (unmarked && !confirm(t('detail.files.confirmVolume', { name: row.name }))) return

  updatingEps.value.add(row.id)
  row.restoring = true
  delete row.restoreError
  let restored = false
  try {
    const { data } = await request.post<{ restored: boolean }>(`/subscriptions/${id}/episodes/${row.id}/restore-file`,
      { confirm_unmarked_volume: unmarked })
    if (data?.restored !== true) throw new Error()
    restored = true
    if (fileReport.value !== report || fileScope.value !== verifiedScope) return
    const { data: checked } = await checkFileIds([row.id])
    const result = checked.results?.[0]
    if (result?.id !== row.id) throw new Error()
    Object.assign(row, result)
    if (checked.volume_guarded === false) report.volumeGuarded = false
  } catch (e: any) {
    if (restored) {
      row.status = 'request_failed'
      row.restoreError = t('detail.files.restoredCheckFailed')
    } else {
      row.restoreError = e.response?.data?.error || t('detail.files.restoreFailed')
    }
  } finally {
    row.restoring = false
    updatingEps.value.delete(row.id)
  }
}

function recheckSkipReason(ep: Episode) {
  if (ep.status !== 'needs_review') return t('detail.bulk.notInReview')
  if (ep.review_kind === 'episode_identity') return t('detail.bulk.needsAssignment')
  if (ep.review_kind === 'movie_selection') return t('detail.bulk.needsConfirmation')
  if (ep.review_kind || !ep.can_retry_organize) return t('detail.bulk.recheckUnavailable')
}

async function bulkRecheck() {
  if (!selectedEpisodes.value.size || bulkActionLoading.value || updatingEps.value.size) return
  const selected = episodes.value.filter(ep => selectedEpisodes.value.has(ep.id))
  bulkActionLoading.value = true
  actionError.value = ''
  const results: RecheckResult[] = []
  try {
    for (const ep of selected) {
      const item = { id: ep.id, name: ep.source_file || ep.original_name || ep.title || episodeLabel(ep) }
      const reason = recheckSkipReason(ep)
      if (reason) {
        results.push({ ...item, status: 'skipped', reason })
        continue
      }
      try {
        await queueOrganization(ep)
        results.push({ ...item, status: 'queued' })
        selectedEpisodes.value.delete(ep.id)
      } catch (e: any) {
        results.push({ ...item, status: 'failed', reason: e.response?.data?.error || t('detail.review.recheckFailed') })
      }
    }
    recheckResults.value = results
    showBulkActions.value = selectedEpisodes.value.size > 0
    if (results.some(result => result.status !== 'skipped')) await fetchDetail()
  } finally {
    bulkActionLoading.value = false
  }
}

async function bulkUpdateStatus(newStatus: string) {
  if (!selectedEpisodes.value.size || bulkActionLoading.value || updatingEps.value.size) return
  const ids = [...selectedEpisodes.value]
  bulkActionLoading.value = true
  actionError.value = ''
  try {
    const failures = new Set<number>()
    for (const epId of ids) {
      try {
        await request.put(`/episodes/${epId}/status`, { status: newStatus })
      } catch {
        failures.add(epId)
      }
    }
    selectedEpisodes.value = failures
    showBulkActions.value = failures.size > 0
    if (failures.size) actionError.value = t('detail.error.partialUpdate', { count: failures.size, total: ids.length })
    await fetchDetail()
  } finally {
    bulkActionLoading.value = false
  }
}

async function bulkDeleteEpisodes(deleteFiles: boolean) {
  if (!selectedEpisodes.value.size || bulkActionLoading.value || updatingEps.value.size) return
  if (!confirm(t(deleteFiles ? 'detail.bulk.confirmFiles' : 'detail.bulk.confirmRecords', { count: selectedEpisodes.value.size }))) return

  bulkActionLoading.value = true
  try {
    const ids = Array.from(selectedEpisodes.value)
    await request.post(`/subscriptions/${id}/episodes/batch-delete`, { ids, delete_files: deleteFiles })
    await fetchDetail()
    clearSelection()
  } catch (e: any) {
    await fetchDetail()
    actionError.value = e.response?.data?.error || t('detail.error.update')
  } finally {
    bulkActionLoading.value = false
  }
}

async function addToBlacklist() {
  if (!selectedEpisodes.value.size || !sub.value || bulkActionLoading.value || updatingEps.value.size) return

  const selectedEps = episodes.value.filter(ep => selectedEpisodes.value.has(ep.id))
  const keywords = new Set<string>()

  selectedEps.forEach(ep => {
    const name = ep.original_name || ep.title
    const parts = name.split(/[\s.\-_[\]()]+/)
    parts.forEach(p => {
      if (p.length > 3 && !/^\d+$/.test(p)) {
        keywords.add(p.toLowerCase())
      }
    })
  })

  let currentExcluded: string[] = []
  if (sub.value.excluded_keywords) {
    try {
      const parsed = JSON.parse(sub.value.excluded_keywords)
      if (Array.isArray(parsed)) currentExcluded = parsed
    } catch {
      currentExcluded = []
    }
  }
  const merged = Array.from(new Set([...currentExcluded, ...keywords]))

  try {
    bulkActionLoading.value = true
    if (!await handleSaveEdit({ excluded_keywords: JSON.stringify(merged) })) return
    window.showToast?.(t('detail.bulk.blocked', { count: merged.length - new Set(currentExcluded).size }), 'success')
    clearSelection()
  } catch (e: any) {
    actionError.value = e.response?.data?.error || t('detail.error.update')
  } finally {
    bulkActionLoading.value = false
  }
}

function refreshVisibleDetail() {
  if (!document.hidden && !loading.value && !showEdit.value && !bulkActionLoading.value && !updatingEps.value.size) {
    void fetchDetail()
  }
}

let refreshTimer: number
onMounted(() => {
  void fetchDetail()
  refreshTimer = window.setInterval(refreshVisibleDetail, 30_000)
  window.addEventListener('focus', refreshVisibleDetail)
  document.addEventListener('visibilitychange', refreshVisibleDetail)
})
onUnmounted(() => {
  taskRequest?.abort()
  clearFileReport()
  ++detailRead
  window.clearInterval(refreshTimer)
  window.removeEventListener('focus', refreshVisibleDetail)
  document.removeEventListener('visibilitychange', refreshVisibleDetail)
})
</script>

<template>
  <div class="space-y-6 pb-24">
    <!-- 顶部操作导航 -->
    <div class="flex items-center justify-between gap-3">
      <button type="button" class="v2-btn v2-btn-sm" @click="returnToList">
        <ChevronLeft :size="15" />
        <span>{{ t('detail.back') }}</span>
      </button>

      <div class="flex items-center gap-2">
        <button type="button" class="v2-btn v2-btn-sm px-2.5" :aria-label="t('common.refresh')" @click="fetchDetail" :disabled="loading">
          <RefreshCw :size="15" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <div v-if="actionError" role="alert" class="v2-panel border-red-500/35 bg-red-500/10 text-red-400 px-4 py-3 text-xs font-medium break-words">
      {{ actionError }}
    </div>

    <div v-if="loading && !sub" class="flex justify-center py-28">
      <span class="loading loading-spinner loading-lg text-[var(--v2-accent)]"></span>
    </div>

    <div v-if="error" role="alert" class="max-w-3xl mx-auto">
      <div class="v2-panel border-red-500/35 bg-red-500/10 text-red-400 p-4 flex items-start gap-3">
        <AlertTriangle :size="19" class="shrink-0 mt-0.5" />
        <div class="flex-1 min-w-0">
          <h3 class="font-bold text-sm">{{ t('detail.error.load') }}</h3>
          <p class="text-xs opacity-85 mt-0.5">{{ error }}</p>
        </div>
      </div>
    </div>

    <template v-if="sub">
      <!-- 放映厅作品档案主卡 Hero -->
      <section class="relative v2-panel overflow-hidden">
        <div class="absolute inset-x-0 top-0 h-52 z-0 overflow-hidden pointer-events-none">
          <img v-if="sub.cover_url" :src="proxyImage(sub.cover_url)" class="w-full h-full object-cover blur-3xl opacity-15 scale-125" />
          <div class="absolute inset-0 bg-gradient-to-b from-transparent to-[var(--v2-surface)]"></div>
        </div>

        <div class="p-5 sm:p-7 relative z-10 flex flex-col lg:flex-row gap-6 lg:gap-8">
          <!-- 左侧 2:3 海报 -->
          <div class="w-full lg:w-52 xl:w-60 shrink-0 flex justify-center lg:justify-start">
            <div class="w-full max-w-[210px] lg:max-w-none aspect-[2/3] rounded-xl bg-[var(--v2-surface-2)] overflow-hidden shadow-lg border border-[var(--v2-border-strong)] relative">
              <img v-if="sub.cover_url" :src="proxyImage(sub.cover_url)" class="w-full h-full object-cover" />
              <div v-else class="w-full h-full flex items-center justify-center text-[var(--v2-ink-subtle)]">
                <Image :size="44" />
              </div>
              <div class="absolute top-2.5 left-2.5">
                <span class="bg-black/75 backdrop-blur-sm text-white text-[11px] font-mono font-semibold px-2 py-0.5 rounded-md border border-white/15">
                  #{{ sub.id }}
                </span>
              </div>
            </div>
          </div>

          <!-- 右侧元信息与控制 -->
          <div class="flex-1 min-w-0 space-y-5">
            <div class="space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <span v-if="sub.anime_type && sub.anime_type !== t('detail.type.unknown')" class="v2-pill text-xs border-[var(--v2-accent)]/30 text-[var(--v2-accent)]">
                  {{ sub.anime_type }}
                </span>
                <span v-if="sub.completed" class="badge badge-success badge-xs">
                  {{ t('detail.status.finished') }}
                </span>
                <span v-if="!sub.enabled" class="badge badge-warning badge-xs">
                  {{ t('detail.status.paused') }}
                </span>
              </div>

              <div class="space-y-1">
                <h1 class="text-2xl sm:text-3xl font-bold tracking-tight text-[var(--v2-ink)] leading-snug">{{ sub.title_cn }}</h1>
                <p v-if="sub.title_jp" class="text-sm text-[var(--v2-ink-muted)]">{{ sub.title_jp }}</p>
                <p v-if="sub.title_en" class="text-xs text-[var(--v2-ink-subtle)]">{{ sub.title_en }}</p>
              </div>
            </div>

            <!-- 元数据摘要网格（metadata_provider 块保持仅两个 <p> 子节点契约） -->
            <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 p-4 v2-panel-muted">
              <div>
                <p class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.stats.releaseYear') }}</p>
                <p class="text-base font-semibold font-mono tabular-nums mt-0.5">{{ sub.year || 'N/A' }}</p>
              </div>
              <div>
                <p class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.stats.season') }}</p>
                <p class="text-base font-semibold font-mono tabular-nums mt-0.5">{{ sub.season || '1' }}</p>
              </div>
              <div class="col-span-2 sm:col-span-1">
                <p class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.stats.metadata') }}</p>
                <p class="text-base font-semibold mt-0.5 truncate">{{ sub.metadata_provider?.trim() || t('detail.stats.metadataNotRecorded') }}</p>
              </div>
            </div>

            <!-- 进度条 -->
            <div class="space-y-2">
              <div class="flex items-end justify-between">
                <div>
                  <p class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.progress.title') }}</p>
                  <p class="text-xl font-bold font-mono tabular-nums mt-0.5">
                    {{ sub.current_episodes }}
                    <span class="text-[var(--v2-ink-subtle)] mx-0.5">/</span>
                    {{ sub.total_episodes || '?' }}
                  </p>
                </div>
                <div v-if="progress !== null" class="text-right">
                  <p class="text-lg font-bold font-mono tabular-nums text-[var(--v2-accent)]">{{ Math.round(progress) }}%</p>
                </div>
              </div>
              <div v-if="progress !== null" class="h-2 w-full bg-[var(--v2-surface-3)] rounded-full overflow-hidden">
                <div class="h-full rounded-full bg-[var(--v2-accent)] transition-all duration-500" :style="{ width: `${progress}%` }"></div>
              </div>
            </div>

            <!-- 操作按钮 -->
            <div class="flex flex-wrap gap-2.5 pt-1">
              <button type="button" class="v2-btn v2-btn-primary px-5" @click="openEditDialog">
                <Edit3 :size="15" />
                <span>{{ t('detail.actions.modify') }}</span>
              </button>
              <button
                type="button"
                class="v2-btn px-4"
                :disabled="supplementing"
                @click="triggerSupplement"
              >
                <RefreshCw :size="15" :class="{ 'animate-spin': supplementing }" />
                <span>{{ supplementing ? t('detail.actions.supplementing') : t('detail.actions.supplement') }}</span>
              </button>
              <button
                type="button"
                class="v2-btn px-4"
                :class="sub.enabled ? 'border-amber-500/35 text-amber-400 hover:bg-amber-500/10' : 'border-emerald-500/35 text-emerald-400 hover:bg-emerald-500/10'"
                @click="handleSaveEdit({ enabled: !sub.enabled })"
              >
                <component :is="sub.enabled ? Pause : Play" :size="15" />
                <span>{{ sub.enabled ? t('detail.actions.suspend') : t('detail.actions.resume') }}</span>
              </button>
              <button
                type="button"
                class="v2-btn px-4 border-red-500/35 text-red-400 hover:bg-red-500/10"
                @click="openDeleteModal"
              >
                <Trash2 :size="15" />
                <span>{{ t('detail.actions.delete') }}</span>
              </button>
            </div>

            <!-- 作品简介与订阅字幕组 -->
            <div v-if="sub.description" class="pt-3 border-t border-[var(--v2-border)] space-y-1.5">
              <h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--v2-ink-subtle)]">{{ t('detail.synopsis') }}</h3>
              <p class="text-xs sm:text-sm text-[var(--v2-ink-muted)] leading-relaxed">{{ sub.description }}</p>
            </div>

            <div class="flex flex-wrap items-center gap-2 pt-1">
              <span class="text-xs font-semibold text-[var(--v2-ink-subtle)]">{{ t('detail.stats.subgroup') }}:</span>
              <span v-if="sub.subgroup_name" class="v2-pill text-xs text-[var(--v2-ink)]">{{ sub.subgroup_name }}</span>
              <span v-else class="v2-pill text-xs">{{ t('card.generic') }}</span>
            </div>

            <!-- 生效规则摘要栏 -->
            <div v-if="activeRuleBadges.length > 0" class="flex flex-wrap items-center gap-1.5 pt-2 border-t border-[var(--v2-border)]">
              <span class="text-[11px] font-semibold text-[var(--v2-ink-subtle)] mr-1">{{ t('detail.rulesSummary') }}:</span>
              <span
                v-for="badge in activeRuleBadges"
                :key="badge.kind"
                class="v2-pill text-[11px] font-mono max-w-xs truncate"
                :title="`${t('detail.ruleKinds.' + badge.kind)}: ${badge.value}`"
              >
                {{ t('detail.ruleKinds.' + badge.kind) }}: {{ badge.value }}
              </span>
            </div>
          </div>
        </div>
      </section>

      <!-- 停滞告警条 -->
      <Transition name="fade">
        <div v-if="sub.stalled_episodes > 0" class="v2-panel border-amber-500/35 bg-amber-500/10 text-amber-400 p-4 sm:p-5 flex items-start gap-4">
          <div class="w-10 h-10 rounded-xl bg-amber-500/15 flex items-center justify-center shrink-0">
            <AlertTriangle :size="20" />
          </div>
          <div class="flex-1 min-w-0">
            <h3 class="text-sm font-bold">{{ t('detail.warning.title') }}</h3>
            <p class="text-xs opacity-90 mt-1 leading-relaxed">
              {{ t('detail.warning.desc', { count: sub.stalled_episodes }) }}
              <span v-if="sub.stall_timeout_hours > 0" class="font-mono opacity-75 ml-1">({{ sub.stall_timeout_hours }}h)</span>
            </p>
          </div>
        </div>
      </Transition>

      <!-- 剧集与核验区域 -->
      <div class="space-y-4">
        <!-- 悬浮批量操作栏 -->
        <Transition name="slide">
          <fieldset
            v-if="showBulkActions"
            :disabled="bulkActionLoading || updatingEps.size > 0"
            class="fixed bottom-16 lg:bottom-6 left-3 right-3 sm:left-auto sm:right-6 z-50 v2-glass border border-[var(--v2-accent)]/40 rounded-2xl shadow-2xl p-3 sm:p-4 flex flex-col sm:flex-row gap-2 sm:gap-3 max-h-[55dvh] overflow-y-auto max-w-2xl"
          >
            <span class="text-xs font-bold text-[var(--v2-accent)] self-center px-2 whitespace-nowrap shrink-0">
              {{ t('detail.bulk.selected', { count: selectedEpisodes.size }) }}
            </span>
            <div class="flex flex-wrap items-center justify-center gap-1.5">
              <button type="button" class="v2-btn v2-btn-sm" :disabled="verifyingFiles" @click="verifyFiles('selected')">
                <FileCheck2 :size="14" /> {{ t('detail.files.selected') }}
              </button>
              <button type="button" class="v2-btn v2-btn-sm text-amber-400 border-amber-500/35" @click="bulkRecheck">
                <RefreshCw :size="14" /> {{ t('detail.bulk.recheck') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="bulkUpdateStatus('pending')">
                <History :size="14" /> {{ t('detail.logs.status.pending') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="bulkUpdateStatus('downloading')">
                <Download :size="14" /> {{ t('detail.logs.status.active') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="bulkUpdateStatus('completed')">
                <Check :size="14" /> {{ t('detail.logs.status.finished') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm text-amber-400" @click="bulkUpdateStatus('failed')">
                <AlertTriangle :size="14" /> {{ t('detail.logs.status.failed') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="bulkUpdateStatus('')">
                <X :size="14" /> {{ t('detail.clearMarks') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm text-red-400" @click="bulkDeleteEpisodes(false)">
                <Trash2 :size="14" /> {{ t('detail.bulk.deleteRecords') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm text-red-400" @click="bulkDeleteEpisodes(true)">
                <Trash2 :size="14" /> {{ t('detail.bulk.deleteFiles') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm text-amber-400" @click="addToBlacklist">
                <Shield :size="14" /> {{ t('detail.bulk.blacklist') }}
              </button>
              <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" @click="clearSelection">
                <X :size="14" /> {{ t('detail.bulk.cancel') }}
              </button>
            </div>
          </fieldset>
        </Transition>

        <!-- 标题行与文件/任务核验操作 -->
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-1">
          <div class="flex items-center gap-3">
            <h2 class="text-base sm:text-lg font-bold tracking-tight text-[var(--v2-ink)]">{{ t('detail.logs.title') }}</h2>
            <span class="v2-pill font-mono text-[11px]">{{ t('detail.logs.count', { count: episodes.length }) }}</span>
          </div>

          <div v-if="hasOrganizedFiles || taskScope" class="flex flex-wrap items-center gap-2">
            <button v-if="taskScope" class="v2-btn v2-btn-sm" type="button" @click="checkDownloadTasks" :disabled="checkingTasks">
              <RefreshCw :size="14" :class="{ 'animate-spin': checkingTasks }" />
              <span>{{ t(checkingTasks ? 'detail.tasks.checking' : 'detail.tasks.check') }}</span>
            </button>
            <span v-if="checkedTasksAt" class="text-xs text-[var(--v2-ink-muted)] font-mono">
              {{ t('detail.tasks.checkedAt', { time: new Date(checkedTasksAt).toLocaleTimeString(locale) }) }}
            </span>
            <button v-if="hasOrganizedFiles" type="button" class="v2-btn v2-btn-sm" :disabled="verifyingFiles || bulkActionLoading || updatingEps.size > 0" @click="verifyFiles('organized')">
              <FileCheck2 :size="14" />
              <span>{{ t('detail.files.all') }}</span>
            </button>
          </div>
        </div>

        <p v-if="taskCheckFailed" role="status" class="text-xs text-amber-400 px-1">{{ t('detail.tasks.failed') }}</p>
        <p v-if="hasMissingTask" role="status" class="text-xs text-[var(--v2-ink-muted)] px-1">{{ t('detail.tasks.restoreHelp') }}</p>
        <p v-if="verifyingFiles" role="status" class="text-xs text-[var(--v2-ink-muted)] px-1">{{ t('detail.files.checking') }}</p>

        <!-- 媒体库文件校验报告（<ul><li> 结构供单测直接渲染解析） -->
        <details v-if="fileReport" open class="v2-panel p-4 text-xs sm:text-sm space-y-2">
          <summary class="cursor-pointer font-semibold text-[var(--v2-ink)]" aria-live="polite">
            {{ t('detail.files.result', { count: fileReport.rows.length, time: new Date(fileReport.checkedAt).toLocaleTimeString(locale) }) }}
          </summary>
          <p v-if="fileReport.volumeGuarded === false" class="mt-2 text-xs text-[var(--v2-ink-muted)]">{{ t('detail.files.currentPath') }}</p>
          <p v-if="fileReport.rows.some(row => row.status === 'missing')" class="mt-2 text-xs text-[var(--v2-ink-muted)]">{{ t('detail.files.restoreHelp') }}</p>
          <ul class="mt-2 max-h-60 space-y-1.5 overflow-y-auto break-words">
            <li v-for="row in fileReport.rows" :key="row.id">
              <span class="font-medium">{{ row.name }}</span> · {{ t(`detail.files.${row.status}`) }}
              <template v-if="row.status === 'size_mismatch'"> · {{ t('detail.files.sizes', { expected: row.expected_size?.toLocaleString(locale), actual: row.actual_size?.toLocaleString(locale) }) }}</template>
              <template v-if="row.reason"> · {{ t(`detail.files.reason.${row.reason}`) }}</template>
              <button
                v-if="row.status === 'missing'"
                type="button"
                class="btn btn-outline btn-xs ms-2"
                :disabled="verifyingFiles || bulkActionLoading || updatingEps.has(row.id)"
                @click="restoreFile(row)"
              >
                {{ t(row.restoring ? 'detail.files.restoring' : 'detail.files.restore') }}
              </button>
              <span v-if="row.restoreError" role="alert" class="block text-error">{{ row.restoreError }}</span>
            </li>
          </ul>
        </details>

        <!-- 批量复核结果详情 -->
        <details v-if="recheckResults.length" open class="v2-panel p-4 text-xs sm:text-sm space-y-2">
          <summary class="cursor-pointer font-semibold text-[var(--v2-ink)]">
            <span aria-live="polite">{{ t('detail.bulk.recheckResults', recheckCounts) }}</span>
          </summary>
          <ul class="mt-2 max-h-60 space-y-1 overflow-y-auto break-words">
            <li v-for="result in recheckResults" :key="result.id">
              <span class="font-medium">{{ result.name }}</span> · {{ t(`detail.bulk.${result.status}`) }}<template v-if="result.reason"> · {{ result.reason }}</template>
            </li>
          </ul>
        </details>

        <!-- 剧集状态筛选与关键词过滤 -->
        <div v-if="episodes.length > 0" class="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 px-1">
          <div class="flex flex-wrap items-center gap-1.5">
            <button
              v-for="tab in ([
                { key: 'all', label: t('detail.filter.all') },
                { key: 'needs_review', label: t('detail.filter.needsReview') },
                { key: 'downloading', label: t('detail.filter.downloading') },
                { key: 'organized', label: t('detail.filter.organized') },
                { key: 'failed', label: t('detail.filter.failed') },
              ] as const)"
              :key="tab.key"
              type="button"
              class="v2-pill text-[11px] cursor-pointer transition-colors"
              :class="epStatusFilter === tab.key ? 'bg-[var(--v2-accent)] text-white border-[var(--v2-accent)] font-semibold' : 'hover:text-[var(--v2-ink)]'"
              @click="epStatusFilter = tab.key"
            >
              <span>{{ tab.label }}</span>
              <span class="font-mono opacity-80 ml-1">{{ epFilterCounts[tab.key] }}</span>
            </button>
          </div>
          <input
            v-model="epSearchQuery"
            type="search"
            class="v2-input h-8 sm:w-56 text-xs"
            :placeholder="t('detail.filter.searchPlaceholder')"
          />
        </div>

        <!-- 无剧集空状态 -->
        <div v-if="episodes.length === 0" class="v2-panel py-16 flex flex-col items-center justify-center gap-3 text-center">
          <div class="w-12 h-12 rounded-2xl bg-[var(--v2-surface-2)] text-[var(--v2-ink-subtle)] flex items-center justify-center">
            <FileText :size="24" />
          </div>
          <p class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.logs.empty') }}</p>
        </div>

        <div v-else-if="filteredEpisodes.length === 0" class="v2-panel py-12 flex flex-col items-center justify-center gap-3 text-center">
          <p class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.filter.noMatch') }}</p>
          <button type="button" class="v2-btn v2-btn-sm" @click="epStatusFilter = 'all'; epSearchQuery = ''">
            {{ t('detail.filter.reset') }}
          </button>
        </div>

        <!-- 按字幕组分组的原生可折叠剧集面板（仅使用 RefreshCw / AlertTriangle，兼容单测） -->
        <div v-else class="space-y-4">
          <div
            v-for="([groupName, groupEps], gIdx) in groupedEpisodes"
            :key="groupName"
            class="v2-panel overflow-hidden"
          >
            <!-- 组头（点击可折叠/展开，复选框阻止冒泡） -->
            <div
              class="px-4 sm:px-5 py-3.5 flex items-center justify-between gap-3 cursor-pointer select-none hover:bg-[var(--v2-surface-2)]/60 transition-colors"
              :class="{ 'border-b border-[var(--v2-border)]': isGroupExpanded(groupName, gIdx) }"
              @click="toggleGroup(groupName, gIdx)"
            >
              <div class="flex items-center gap-3 flex-wrap min-w-0">
                <label class="cursor-pointer flex items-center gap-2.5" @click.stop>
                  <input
                    type="checkbox"
                    class="checkbox"
                    :disabled="bulkActionLoading"
                    :checked="groupEps.every(ep => selectedEpisodes.has(ep.id))"
                    @change="toggleSelectAll(groupEps)"
                  />
                  <span class="text-sm sm:text-base font-bold text-[var(--v2-ink)] truncate">{{ groupName }}</span>
                </label>
                <span class="v2-pill font-mono text-[10px]">{{ groupEps.length }} {{ t('detail.logs.episodes') }}</span>
                <span v-if="groupEps.some(ep => ep.status === 'needs_review')" class="badge badge-warning badge-xs">
                  {{ t('detail.logs.status.needsReview') }}
                </span>
              </div>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                class="shrink-0 text-[var(--v2-ink-muted)] transition-transform duration-200"
                :class="{ 'rotate-180': isGroupExpanded(groupName, gIdx) }"
              >
                <polyline points="6 9 12 15 18 9" />
              </svg>
            </div>

            <!-- 剧集表格 -->
            <div v-show="isGroupExpanded(groupName, gIdx)" class="overflow-x-auto">
              <table class="w-full text-left border-collapse">
                <thead>
                  <tr class="border-b border-[var(--v2-border)] bg-[var(--v2-surface-2)]/70 text-[11px] font-semibold uppercase tracking-wider text-[var(--v2-ink-subtle)]">
                    <th class="py-3 pl-4 sm:pl-5 w-10">
                      <input
                        type="checkbox"
                        class="checkbox"
                        :disabled="bulkActionLoading"
                        :checked="groupEps.every(ep => selectedEpisodes.has(ep.id))"
                        @change="toggleSelectAll(groupEps)"
                      />
                    </th>
                    <th class="py-3 px-3">{{ t('detail.logs.table.id') }}</th>
                    <th class="py-3 px-3">{{ t('detail.logs.table.name') }}</th>
                    <th class="py-3 px-3">{{ t('detail.logs.table.status') }}</th>
                    <th class="py-3 px-3">{{ t('detail.logs.table.integrity') }}</th>
                    <th class="py-3 px-3">{{ t('detail.logs.table.capacity') }}</th>
                    <th class="py-3 pl-3 pr-4 sm:pr-5">{{ t('detail.logs.table.timestamp') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--v2-border)] text-xs">
                  <tr
                    v-for="ep in groupEps"
                    :key="ep.id"
                    class="hover:bg-[var(--v2-surface-2)]/45 transition-colors"
                    :class="{ 'bg-[var(--v2-accent)]/6': selectedEpisodes.has(ep.id) }"
                  >
                    <td class="py-3 pl-4 sm:pl-5">
                      <label class="cursor-pointer">
                        <input
                          type="checkbox"
                          class="checkbox"
                          :disabled="bulkActionLoading"
                          :checked="selectedEpisodes.has(ep.id)"
                          @change="toggleEpisodeSelect(ep.id)"
                        />
                      </label>
                    </td>
                    <td class="py-3 px-3 whitespace-nowrap">
                      <span class="font-mono font-semibold text-xs px-2 py-0.5 rounded bg-[var(--v2-surface-2)] border border-[var(--v2-border)] text-[var(--v2-ink)]">
                        {{ episodeLabel(ep) }}
                      </span>
                    </td>
                    <td class="py-3 px-3 max-w-xs sm:max-w-md">
                      <div class="font-medium text-[var(--v2-ink)] truncate" :title="ep.original_name || ep.title">
                        {{ ep.original_name || ep.title || 'Untitled Stream' }}
                      </div>
                    </td>
                    <td class="py-3 px-3 whitespace-nowrap">
                      <span class="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium" :class="statusCfg[ep.status]?.cls">
                        <component :is="statusCfg[ep.status]?.icon || MoreVertical" :size="13" />
                        {{ statusCfg[ep.status]?.label || ep.status }}
                      </span>
                      <label class="mt-1.5 flex items-center gap-1.5 text-[11px] text-[var(--v2-ink-muted)]">
                        <span>{{ t('detail.manualMark') }}</span>
                        <select
                          class="select select-bordered select-xs w-auto min-w-20"
                          :value="ep.manual_status || ''"
                          :disabled="updatingEps.has(ep.id) || bulkActionLoading"
                          @change="setEpisodeMark(ep, $event)"
                        >
                          <option value="">{{ t('detail.noMark') }}</option>
                          <option v-for="mark in ['pending', 'downloading', 'completed', 'failed']" :key="mark" :value="mark">
                            {{ statusCfg[mark]?.label }}
                          </option>
                        </select>
                      </label>
                    </td>
                    <td class="py-3 px-3">
                      <div v-if="ep.status === 'needs_review'" class="min-w-48 max-w-sm space-y-1.5">
                        <p class="text-xs whitespace-normal break-words text-[var(--v2-ink)]">{{ ep.review_reason || t('detail.review.noReason') }}</p>
                        <p v-if="ep.source_file" class="text-[11px] break-all text-[var(--v2-ink-muted)]"><span class="font-semibold">{{ t('detail.review.source') }}</span> {{ ep.source_file }}</p>
                        <p v-if="reviewErrors[ep.id]" role="alert" class="text-xs text-red-400 break-words">{{ reviewErrors[ep.id] }}</p>
                        <button v-if="ep.can_retry_organize" type="button" class="btn btn-outline btn-warning btn-xs" :disabled="updatingEps.has(ep.id) || bulkActionLoading" @click="retryOrganize(ep)">{{ t(ep.review_kind ? 'detail.review.recheck' : 'detail.review.retry') }}</button>
                        <form
                          v-if="reviewConfirmation(ep) === 'assign' && assignment && assignment.id === ep.id && assignment.sourceFile === ep.source_file"
                          @submit.prevent="retryWithAssignment(ep)"
                        >
                          <fieldset class="space-y-2" :disabled="updatingEps.has(ep.id) || bulkActionLoading">
                            <div class="flex gap-2">
                              <label class="text-xs">{{ t('detail.review.season') }}
                                <input v-model.number="assignment.season" type="number" min="0" max="99" step="1" required class="input input-bordered input-xs block w-20">
                              </label>
                              <label class="text-xs">{{ t('detail.review.episode') }}
                                <input v-model.number="assignment.number" type="number" min="0" max="2000" step="any" required class="input input-bordered input-xs block w-20">
                              </label>
                            </div>
                            <div class="flex flex-wrap gap-2">
                              <button type="submit" class="btn btn-outline btn-warning btn-xs" :disabled="!validAssignment">{{ t('detail.review.assignAndRetry') }}</button>
                              <button type="button" class="btn btn-ghost btn-xs" @click="assignment = null">{{ t('detail.bulk.cancel') }}</button>
                            </div>
                          </fieldset>
                        </form>
                        <div v-else class="flex flex-wrap gap-2">
                          <button v-if="reviewConfirmation(ep) === 'assign'" type="button" class="btn btn-outline btn-warning btn-xs" :disabled="updatingEps.has(ep.id) || bulkActionLoading" @click="editAssignment(ep)">{{ t('detail.review.assign') }}</button>
                          <button v-else-if="reviewConfirmation(ep) === 'movie'" type="button" class="btn btn-outline btn-warning btn-xs h-auto min-h-6 whitespace-normal" :disabled="updatingEps.has(ep.id) || bulkActionLoading" @click="retryOrganize(ep, { confirm_movie: true })">{{ t('detail.review.confirmMovie') }}</button>
                          <p v-else-if="ep.review_kind === 'episode_identity' || ep.review_kind === 'movie_selection'" class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.review.missingSource') }}</p>
                        </div>
                      </div>
                      <div v-else-if="episodeTaskState(ep) === 'missing'" class="min-w-32 text-xs text-amber-400">
                        <span>{{ t('detail.tasks.missing') }}</span>
                        <code class="block select-all break-all text-[var(--v2-ink-muted)]">{{ ep.torrent_hash }}</code>
                      </div>
                      <span v-else-if="boundTaskKey(ep) && episodeTaskState(ep) !== 'present'" class="text-xs text-[var(--v2-ink-muted)]">
                        {{ t(checkingTasks ? 'detail.tasks.checking' : 'detail.tasks.unknown') }}
                      </span>
                      <span v-else-if="ep.is_stalled" class="inline-flex items-center gap-1 text-amber-400 text-xs font-medium">
                        <AlertTriangle :size="12" /> {{ t('detail.logs.integrity.stalled') }}
                      </span>
                      <span v-else-if="episodeTaskState(ep) === 'present'" class="text-xs text-[var(--v2-ink-muted)]">{{ t('detail.tasks.present') }}</span>
                      <span v-else class="text-[11px] font-mono text-[var(--v2-ink-subtle)]">{{ t('detail.logs.integrity.verified') }}</span>
                    </td>
                    <td class="py-3 px-3 whitespace-nowrap font-mono text-[var(--v2-ink-muted)] tabular-nums">
                      {{ formatSize(ep.file_size) }}
                    </td>
                    <td class="py-3 pl-3 pr-4 sm:pr-5 whitespace-nowrap font-mono text-[11px] text-[var(--v2-ink-subtle)]">
                      {{ new Date(ep.created_at).toLocaleString(locale, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      <!-- 编辑订阅弹窗 -->
      <dialog ref="editDialog" class="modal" @click.self="closeEditDialog" @close="showEdit = false">
        <div class="modal-box max-w-2xl">
          <div class="flex items-center justify-between mb-6 pb-3 border-b border-[var(--v2-border)]">
            <div>
              <h3 class="text-lg font-bold text-[var(--v2-ink)]">{{ t('detail.edit.title') }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ t('detail.edit.subtitle') }}</p>
            </div>
            <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm px-2" @click="closeEditDialog">
              <X :size="16" />
            </button>
          </div>

          <SubscriptionEditForm
            v-if="showEdit"
            :sub="sub"
            @save="handleSaveEdit"
            @cancel="closeEditDialog"
          />
        </div>
      </dialog>

      <!-- 删除订阅确认弹窗 -->
      <dialog :class="['modal', { 'modal-open': showDeleteModal }]">
        <div class="modal-box max-w-md space-y-4">
          <div class="flex items-center justify-between border-b border-[var(--v2-border)] pb-3">
            <h3 class="text-base font-bold text-[var(--v2-ink)]">{{ t('detail.deleteModal.title') }}</h3>
            <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm px-2" @click="showDeleteModal = false">
              <X :size="15" />
            </button>
          </div>
          <p class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">
            {{ t('detail.deleteModal.desc', { title: sub.title_cn }) }}
          </p>
          <label class="flex items-center gap-2.5 p-3 rounded-xl bg-[var(--v2-surface-2)] border border-[var(--v2-border)] cursor-pointer text-xs text-[var(--v2-ink)]">
            <input v-model="deleteFiles" type="checkbox" class="checkbox checkbox-sm checkbox-error" />
            <span>{{ t('detail.deleteModal.deleteFilesLabel') }}</span>
          </label>
          <div class="flex justify-end gap-2 pt-1">
            <button type="button" class="v2-btn v2-btn-ghost v2-btn-sm" :disabled="deletingSub" @click="showDeleteModal = false">
              {{ t('common.cancel') }}
            </button>
            <button type="button" class="v2-btn v2-btn-sm bg-red-500 text-white border-red-500 hover:bg-red-600" :disabled="deletingSub" @click="confirmDeleteSub">
              <Trash2 :size="14" />
              <span>{{ deletingSub ? t('detail.deleteModal.deleting') : t('detail.deleteModal.confirm') }}</span>
            </button>
          </div>
        </div>
        <div class="modal-backdrop" @click="showDeleteModal = false"></div>
      </dialog>
    </template>
  </div>
</template>

<style scoped>
.modal-box {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.modal-box::-webkit-scrollbar {
  display: none;
}
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.25s ease, transform 0.25s ease;
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
  transform: translateY(10px);
}
.slide-enter-active, .slide-leave-active {
  transition: all 0.22s ease;
}
.slide-enter-from {
  opacity: 0;
  transform: translateY(14px) scale(0.97);
}
.slide-leave-to {
  opacity: 0;
  transform: translateY(10px) scale(0.97);
}
</style>
