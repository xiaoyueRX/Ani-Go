<script setup lang="ts">
import { ref, onMounted, computed, watch, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { downloaderForms, downloaderProbe, type DownloaderType } from '../utils/setupDownloader'
import PluginPanel from '../components/PluginPanel.vue'
import UserAvatar from '../components/v2/UserAvatar.vue'
import ThemeAppearancePanel from '../components/v2/ThemeAppearancePanel.vue'
import {
  Check, Antenna, Download,
  Folder, Bell, Cpu,
  Timer, Lock, FileText,
  Eye, EyeOff, RefreshCw,
  Database, Upload, Trash2,
  Sparkles, Copy, Palette,
  Search, Zap, ExternalLink,
  Play, AlertCircle, Globe, X
} from 'lucide-vue-next'
import { currentVersion, useVersion } from '../composables/useVersion'

const { t, locale } = useI18n()
const router = useRouter()
const route = useRoute()

const settings = ref<Record<string, string>>({})
let savedSettings: Record<string, string> = {}
const savingSettings = ref(false)
let settingsRead = 0
const settingsReady = ref(false)
const loading = ref(true)
const error = ref('')
const saved = ref(false)
const activeTab = ref(route.query.tab ? String(route.query.tab) : 'paths')
const showPasswords = ref<Set<string>>(new Set())

watch(() => route.query.tab, (nextTab) => {
  if (typeof nextTab === 'string' && nextTab) {
    activeTab.value = nextTab
  }
})

// 镜像测速
const mirrorTesting = ref(false)
const mirrorResults = ref<{ domain: string; latency_ms: number; ok: boolean }[]>([])
const selectedMirror = ref('')

// 管理员头像上传与校验
const avatarUploading = ref(false)
const avatarFileInput = ref<HTMLInputElement | null>(null)
const avatarError = ref('')
const avatarSuccess = ref('')

// 账户密码修改
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const changingPassword = ref(false)
const passwordMsg = ref('')
const passwordError = ref('')

// 版本与更新检测
const { latestVersion, hasNewVersion, checkGitHubUpdate, updating, triggerUpdate, canSelfUpdate } = useVersion()
const checkingUpdate = ref(false)
const manualCheckMsg = ref('')
const manualCheckMsgType = ref<'success' | 'error'>('success')
const manualCheckResult = ref<string>('')

// 插件管理
const pluginLoading = ref(false)
const pluginList = ref<any[]>([])
const pluginListError = ref('')
let pluginListRequest = 0
const pendingPlugins = ref(new Set<string>())
const pluginSourceFilter = ref<'all' | 'official' | 'community'>('all')
const visiblePlugins = computed(() => pluginList.value.filter(plugin =>
  pluginSourceFilter.value === 'all' ||
  (plugin.origin === 'official' ? 'official' : 'community') === pluginSourceFilter.value
))

// 日志管理
const logs = ref<string[]>([])
const logLoading = ref(false)
const logFilter = ref('')
const autoRefreshLogs = ref(false)
const autoScrollBottom = ref(true)
const logContainerRef = ref<HTMLElement | null>(null)
let logTimer: any = null

const logLevels = [
  { key: 'all', label: '全部' },
  { key: 'error', label: '仅错误' },
  { key: 'warn', label: '仅警告' },
  { key: 'info', label: '成功与信息' },
  { key: 'plugin', label: '插件与任务' }
]
const currentLogLevel = ref('all')

const filteredLogs = computed(() => {
  let list = Array.isArray(logs.value) ? logs.value : []
  if (currentLogLevel.value === 'error') {
    list = list.filter(l => typeof l === 'string' && (l.includes('❌') || l.includes('ERROR') || l.includes('error') || l.includes('failed')))
  } else if (currentLogLevel.value === 'warn') {
    list = list.filter(l => typeof l === 'string' && (l.includes('⚠️') || l.includes('WARN') || l.includes('warn')))
  } else if (currentLogLevel.value === 'info') {
    list = list.filter(l => typeof l === 'string' && (l.includes('✅') || l.includes('ℹ️') || l.includes('SUCCESS') || l.includes('success')))
  } else if (currentLogLevel.value === 'plugin') {
    list = list.filter(l => typeof l === 'string' && (l.includes('🔗') || l.includes('🚀') || l.includes('插件') || l.includes('plugin')))
  }
  if (!logFilter.value.trim()) return list
  const q = logFilter.value.toLowerCase()
  return list.filter(l => typeof l === 'string' && l.toLowerCase().includes(q))
})

function pluginRunning(plugin: { enabled: boolean; status?: string }) {
  return plugin.enabled && (!plugin.status || plugin.status === 'active')
}

function isPluginActive(id: string): boolean {
  const p = pluginList.value.find(item => item.id === id)
  return p ? pluginRunning(p) : false
}

const tabGroups = computed(() => {
  const rawGroups = [
    {
      name: t('settings.groups.core'),
      tabs: [
        { key: 'appearance', label: locale.value === 'zh' ? '外观与主题' : 'Themes & Appearance', icon: Palette, ready: true },
        { key: 'paths', label: t('settings.tabs.paths'), icon: Folder, ready: true },
        { key: 'downloader', label: t('settings.tabs.downloader'), icon: Download, ready: true },
      ]
    },
    {
      name: t('settings.groups.sources'),
      tabs: [
        { key: 'sources', label: t('settings.tabs.sources'), icon: Globe, ready: true },
        { key: 'mikan', label: t('settings.tabs.mikan'), icon: Antenna, ready: true },
        { key: 'bangumi', label: t('settings.tabs.bangumi'), icon: Antenna, ready: true },
      ]
    },
    {
      name: t('settings.groups.extensions'),
      tabs: [
        { key: 'plugins', label: t('settings.tabs.plugins'), icon: Sparkles, ready: true },
        { key: 'ai', label: t('settings.tabs.ai'), icon: Cpu, ready: false },
        { key: 'customRegex', label: t('settings.sections.customRegex'), icon: FileText, ready: false },
        { key: 'notify', label: t('settings.tabs.notify'), icon: Bell, ready: false, pluginRequired: 'extended-notifiers' },
      ]
    },
    {
      name: t('settings.groups.system'),
      tabs: [
        { key: 'scheduler', label: t('settings.tabs.scheduler'), icon: Timer, ready: true },
        { key: 'logs', label: t('settings.tabs.logs'), icon: FileText, ready: true },
        { key: 'account', label: t('settings.tabs.account'), icon: Lock, ready: true },
        { key: 'backup', label: t('settings.tabs.backup'), icon: Database, ready: false },
      ]
    }
  ]

  return rawGroups.map(g => ({
    ...g,
    tabs: g.tabs.filter((tab: any) => !tab.pluginRequired || isPluginActive(tab.pluginRequired))
  }))
})

const allFlatTabs = computed(() => tabGroups.value.flatMap(g => g.tabs))

interface FieldDef {
  label: string
  key: string
  placeholder: string
  type?: 'text' | 'password' | 'select' | 'switch'
  hint?: string
  testDownloader?: DownloaderType | 'current'
  selectOptions?: { label: string; value: string }[]
}

interface SectionDef {
  title: string
  desc: string
  fields: FieldDef[]
}

const sectionsByTab = computed<Record<string, SectionDef[]>>(() => ({
  paths: [
    {
      title: t('settings.sections.pathsStorage'),
      desc: t('settings.sections.pathsStorageDesc'),
      fields: [
        { label: t('settings.fields.db'), key: 'DB_PATH', placeholder: './data/ani-go.db' },
        { label: t('settings.fields.media'), key: 'TV_BASE_PATH', placeholder: './TV/Media/番剧' },
        { label: t('settings.fields.movieMedia'), key: 'MOVIE_BASE_PATH', placeholder: './TV/Media/剧场版' },
        { label: t('settings.fields.ovaMedia'), key: 'OVA_BASE_PATH', placeholder: './TV/Media/OVA' },
        { label: t('settings.fields.hardlink'), key: 'USE_HARDLINK', placeholder: 'false', type: 'switch', hint: '开启后优先在同盘创建硬链接（零额外空间占用），跨盘时自动回退为复制。' },
        { label: t('settings.fields.tvTemplate'), key: 'TV_TEMPLATE', placeholder: '{title_cn} ({year})/Season {season}/{title_en} S{season:02}E{ep:02}{ext}' },
        { label: t('settings.fields.movieTemplate'), key: 'MOVIE_TEMPLATE', placeholder: '{title_cn} ({year})/{title_en}{ext}' },
        { label: t('settings.fields.otherTemplate'), key: 'OTHER_TEMPLATE', placeholder: '{title_cn}{year}/Specials/{title_en} S00E{ep:02}{ext}' },
      ]
    }
  ],
  downloader: [
    {
      title: t('settings.sections.engine'),
      desc: t('settings.sections.engineDesc'),
      fields: [
        {
          label: t('settings.fields.activeDownloader'), key: 'DEFAULT_DOWNLOADER', placeholder: 'qbittorrent', type: 'select', testDownloader: 'current',
          selectOptions: [
            { label: t('settings.options.qbRecommend'), value: 'qbittorrent' },
            { label: t('settings.options.transmission'), value: 'transmission' },
            { label: t('settings.options.aria2'), value: 'aria2' },
          ]
        }
      ]
    },
    {
      title: t('settings.sections.qb'),
      desc: t('settings.sections.qbDesc'),
      fields: [
        { label: t('settings.fields.qbHost'), key: 'QB_HOST', placeholder: 'http://localhost:8081', testDownloader: 'qbittorrent' },
        { label: t('settings.fields.qbCategory'), key: 'QB_CATEGORY', placeholder: 'ani-go' },
        { label: t('settings.fields.qbUser'), key: 'QB_USER', placeholder: 'admin' },
        { label: t('settings.fields.qbPass'), key: 'QB_PASS', placeholder: '••••••••', type: 'password' },
      ]
    },
    {
      title: t('settings.sections.tr'),
      desc: t('settings.sections.trDesc'),
      fields: [
        { label: t('settings.fields.trHost'), key: 'TR_HOST', placeholder: 'http://localhost:9091', testDownloader: 'transmission' },
        { label: t('settings.fields.trUser'), key: 'TR_USER', placeholder: 'Username' },
        { label: t('settings.fields.trPass'), key: 'TR_PASS', placeholder: '••••••••', type: 'password' },
      ]
    },
    {
      title: t('settings.sections.aria2'),
      desc: t('settings.sections.aria2Desc'),
      fields: [
        { label: t('settings.fields.aria2Host'), key: 'ARIA2_HOST', placeholder: 'http://localhost:6800/jsonrpc', testDownloader: 'aria2' },
        { label: t('settings.fields.aria2Secret'), key: 'ARIA2_SECRET', placeholder: 'Secret Token', type: 'password' },
      ]
    },
    {
      title: t('settings.sections.seedCleanup'),
      desc: t('settings.sections.seedCleanupDesc'),
      fields: [
        { label: t('settings.fields.seedCleanupEnabled'), key: 'SEED_CLEANUP_ENABLED', placeholder: 'false', type: 'switch' },
        { label: t('settings.fields.seedCleanupInterval'), key: 'SEED_CLEANUP_INTERVAL', placeholder: '1h' },
        { label: t('settings.fields.seedCleanupMinRatio'), key: 'SEED_CLEANUP_MIN_RATIO', placeholder: '1.0' },
        { label: t('settings.fields.seedCleanupMinSeedTime'), key: 'SEED_CLEANUP_MIN_SEED_TIME', placeholder: '48h' },
      ]
    },
    {
      title: t('settings.sections.diskGuard'),
      desc: t('settings.sections.diskGuardDesc'),
      fields: [
        { label: t('settings.fields.diskMinFreeGb'), key: 'DISK_MIN_FREE_GB', placeholder: '5.0' },
      ]
    }
  ],
  sources: [
    {
      title: t('settings.sections.sourcesAcgrip'),
      desc: t('settings.sections.sourcesAcgripDesc'),
      fields: [
        { label: t('settings.fields.acgripEnabled'), key: 'ACGRIP_ENABLED', placeholder: 'false', type: 'switch', hint: '国内直连友好站点，适合中文动漫与双语字幕组资源。' },
        { label: t('settings.fields.acgripDomain'), key: 'ACGRIP_DOMAIN', placeholder: 'acg.rip' },
      ]
    },
    {
      title: t('settings.sections.sourcesNyaa'),
      desc: t('settings.sections.sourcesNyaaDesc'),
      fields: [
        { label: t('settings.fields.nyaaEnabled'), key: 'NYAA_ENABLED', placeholder: 'false', type: 'switch', hint: '海外主流 BT 资源站，境内直连通常需代理或镜像反代域名。' },
        { label: t('settings.fields.nyaaDomain'), key: 'NYAA_DOMAIN', placeholder: 'nyaa.si' },
      ]
    },
    {
      title: t('settings.sections.sourcesTosho'),
      desc: t('settings.sections.sourcesToshoDesc'),
      fields: [
        { label: t('settings.fields.toshoEnabled'), key: 'ANIMETOSHO_ENABLED', placeholder: 'false', type: 'switch', hint: '自动提取多源镜像并附带磁力与种子元信息。' },
        { label: t('settings.fields.toshoDomain'), key: 'ANIMETOSHO_DOMAIN', placeholder: 'animetosho.org' },
      ]
    }
  ],
  mikan: [
    {
      title: t('settings.sections.mikanSync'),
      desc: t('settings.sections.mikanSyncDesc'),
      fields: [
        { label: t('settings.fields.mikanRss'), key: 'MIKAN_RSS_URL', placeholder: 'https://mikanani.me/RSS/MyBangumi?token=***' },
        {
          label: t('settings.fields.mikanRssMode'), key: 'MIKAN_RSS_MODE', placeholder: 'personal', type: 'select',
          selectOptions: [
            { label: t('settings.options.rssPersonal'), value: 'personal' },
            { label: t('settings.options.rssClassic'), value: 'classic' },
          ]
        },
        { label: t('settings.fields.mikanDomain'), key: 'MIKAN_DOMAIN', placeholder: 'mikanani.me' },
        { label: t('settings.fields.mikanProxy'), key: 'MIKAN_PROXY_DOMAIN', placeholder: 'mikanani.me' },
        { label: t('settings.fields.mikanMirrors'), key: 'MIKAN_MIRROR_DOMAINS', placeholder: 'mikanani.me,mikanime.tv' },
      ]
    }
  ],
  bangumi: [
    {
      title: t('settings.sections.metadataPrimary'),
      desc: t('settings.sections.metadataPrimaryDesc'),
      fields: [
        {
          label: t('settings.fields.metadataPrimary'), key: 'METADATA_PRIMARY', placeholder: 'bangumi', type: 'select',
          selectOptions: [
            { label: 'Bangumi (bgm.tv)', value: 'bangumi' },
            { label: 'TMDB (The Movie Database)', value: 'tmdb' },
          ]
        }
      ]
    },
    {
      title: t('settings.sections.tmdbMeta'),
      desc: t('settings.sections.tmdbMetaDesc'),
      fields: [
        { label: t('settings.fields.tmdbApiKey'), key: 'TMDB_API_KEY', placeholder: '32-char API Key / v3 Read Token', type: 'password' },
        { label: t('settings.fields.tmdbMirrors'), key: 'TMDB_MIRROR_DOMAINS', placeholder: 'api.themoviedb.org,api.tmdb.org' },
        { label: t('settings.fields.tmdbLanguage'), key: 'TMDB_LANGUAGE', placeholder: 'zh-CN' },
      ]
    },
    {
      title: t('settings.sections.bangumiOAuth'),
      desc: t('settings.sections.bangumiOAuthDesc'),
      fields: [
        { label: t('settings.fields.bangumiClientId'), key: 'BANGUMI_CLIENT_ID', placeholder: 'bgm... (Client ID)' },
        { label: t('settings.fields.bangumiClientSecret'), key: 'BANGUMI_CLIENT_SECRET', placeholder: 'Client Secret', type: 'password' },
      ]
    },
    {
      title: t('settings.sections.bangumiSync'),
      desc: t('settings.sections.bangumiSyncDesc'),
      fields: [
        { label: t('settings.fields.bangumiUsername'), key: 'BGMTV_USERNAME', placeholder: 'Bangumi username' },
        { label: t('settings.fields.bangumiToken'), key: 'BGMTV_USER_TOKEN', placeholder: 'Bearer Token', type: 'password' },
        { label: t('settings.fields.bangumiSyncInterval'), key: 'BGMTV_SYNC_INTERVAL', placeholder: '6h' },
        { label: t('settings.fields.bangumiDomain'), key: 'BGMTV_DOMAIN', placeholder: 'api.bgm.tv' },
        { label: t('settings.fields.bangumiMirrors'), key: 'BGMTV_MIRROR_DOMAINS', placeholder: 'api.bgm.tv,api.bangumi.tv,api.chii.in' },
      ]
    }
  ],
  scheduler: [
    {
      title: t('settings.sections.schedulerCycle'),
      desc: t('settings.sections.schedulerCycleDesc'),
      fields: [
        { label: t('settings.fields.rssInterval'), key: 'RSS_INTERVAL', placeholder: '15m' },
        { label: t('settings.fields.organizerInterval'), key: 'ORGANIZER_INTERVAL', placeholder: '2m' },
        { label: t('settings.fields.supplementInterval'), key: 'SUPPLEMENT_INTERVAL', placeholder: '12h' },
        { label: t('settings.fields.seedCleanupInterval'), key: 'SEED_CLEANUP_INTERVAL', placeholder: '1h' },
        { label: t('settings.fields.seedCleanupMinSeedTime'), key: 'SEED_CLEANUP_MIN_SEED_TIME', placeholder: '48h' },
        { label: t('settings.fields.seedCleanupMinRatio'), key: 'SEED_CLEANUP_MIN_RATIO', placeholder: '1.0' },
      ]
    }
  ],
}))

const allFields = computed(() => {
  const m: Record<string, FieldDef> = {}
  for (const sections of Object.values(sectionsByTab.value)) {
    for (const section of sections) {
      for (const f of section.fields) m[f.key] = f
    }
  }
  m['USER_AVATAR_URL'] = { label: '管理员头像', key: 'USER_AVATAR_URL', placeholder: '' }
  m['AUTO_CHECK_UPDATE'] = { label: '自动检测更新', key: 'AUTO_CHECK_UPDATE', placeholder: '' }
  return m
})

const currentTabSections = computed<SectionDef[]>(() => sectionsByTab.value[activeTab.value] || [])

function getVal(key: string): string {
  return settings.value[key] || ''
}

function setVal(key: string, val: string) {
  settings.value[key] = val
}

function isSwitchEnabled(key: string): boolean {
  return settings.value[key] === 'true'
}

function toggleSwitch(key: string) {
  setVal(key, isSwitchEnabled(key) ? 'false' : 'true')
}

function togglePassword(key: string) {
  if (showPasswords.value.has(key)) showPasswords.value.delete(key)
  else showPasswords.value.add(key)
}

function inputType(field: FieldDef): string {
  if (field.type !== 'password') return 'text'
  return showPasswords.value.has(field.key) ? 'text' : 'password'
}

function selectValue(field: FieldDef): string {
  return getVal(field.key) || field.placeholder || field.selectOptions?.[0]?.value || ''
}

async function fetchSettings() {
  if (savingSettings.value) return
  const sequence = ++settingsRead
  if (!settingsReady.value) loading.value = true
  try {
    const { data } = await request.get('/settings')
    if (sequence !== settingsRead) return
    const incoming = { ...data } as Record<string, string>
    incoming.AUTO_CHECK_UPDATE ??= localStorage.getItem('ani-go-auto-update') !== 'false' ? 'true' : 'false'
    const next = { ...incoming }
    for (const [key, value] of Object.entries(settings.value)) {
      if (value !== savedSettings[key]) next[key] = value
    }
    savedSettings = incoming
    settings.value = next
    settingsReady.value = true
  } catch (e: any) {
    if (sequence === settingsRead) error.value = e.response?.data?.error || '加载系统设置失败'
  } finally {
    if (sequence === settingsRead) loading.value = false
  }
}

async function saveAll() {
  if (!settingsReady.value || savingSettings.value) return
  savingSettings.value = true
  ++settingsRead
  error.value = ''
  saved.value = false
  const changed: Record<string, string> = {}
  for (const key of Object.keys(allFields.value)) {
    const val = settings.value[key]
    const field = allFields.value[key]
    if (field.type === 'password' && (val === '' || val === undefined)) continue
    if (val !== undefined && val !== (savedSettings[key] ?? '')) {
      changed[key] = val
    }
  }
  try {
    if (Object.keys(changed).length) {
      await request.put('/settings', { settings: changed })
      Object.assign(savedSettings, changed)
    }
    saved.value = true
    window.showToast?.(t('settings.updateSuccess'), 'success')
    setTimeout(() => { saved.value = false }, 2500)
  } catch (e: any) {
    error.value = e.response?.data?.error || '保存设置失败'
    window.showToast?.(error.value, 'error')
  } finally {
    savingSettings.value = false
  }
}

async function connectBangumi() {
  const newTab = window.open('', '_blank')
  try {
    const { data } = await request.get('/bangumi/auth/link')
    if (data.url) {
      if (newTab) newTab.location.href = data.url
      else window.open(data.url, '_blank')
    } else {
      newTab?.close()
      error.value = '授权链接为空'
    }
  } catch (e: any) {
    newTab?.close()
    error.value = e.response?.data?.error || '无法获取 Bangumi 授权链接'
  }
}

async function testMirrors() {
  mirrorTesting.value = true
  mirrorResults.value = []
  try {
    const endpoint = activeTab.value === 'bangumi' ? '/bgm/test-mirrors' : '/mikan/test-mirrors'
    const { data } = await request.post(endpoint, {}, { timeout: 15000 })
    mirrorResults.value = data || []
  } catch {
    error.value = '测速请求失败'
  } finally {
    mirrorTesting.value = false
  }
}

async function selectMirror(domain: string) {
  try {
    const isBgm = activeTab.value === 'bangumi'
    const endpoint = isBgm ? '/bgm/select-mirror' : '/mikan/select-mirror'
    const key = isBgm ? 'BGMTV_DOMAIN' : 'MIKAN_DOMAIN'
    await request.post(endpoint, { domain })
    setVal(key, domain)
    selectedMirror.value = domain
    window.showToast?.(`已切换节点为 ${domain}`, 'success')
  } catch {
    error.value = '切换节点失败'
  }
}

const testingDownloaderKey = ref<string | null>(null)
async function testDownloader(type: DownloaderType | 'current') {
  testingDownloaderKey.value = type
  try {
    let payload: { type: string } = { type: '' }
    if (type !== 'current') {
      const form = downloaderForms(settings.value)[type]
      if (type === 'aria2') form.secret = getVal('ARIA2_SECRET')
      else form.password = getVal(type === 'qbittorrent' ? 'QB_PASS' : 'TR_PASS')
      payload = downloaderProbe(type, form)
    }
    const { data } = await request.post('/downloader/test', payload)
    if (data.success) window.showToast?.(data.message || '下载器连接正常', 'success')
    else window.showToast?.(data.error || '下载器连接失败', 'error')
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '下载器测试请求失败', 'error')
  } finally {
    testingDownloaderKey.value = null
  }
}

async function fetchPlugins(afterChange = false) {
  const currentRequest = ++pluginListRequest
  pluginLoading.value = true
  try {
    const { data } = await request.get('/plugins')
    if (currentRequest !== pluginListRequest) return false
    if (!Array.isArray(data)) throw new Error()
    pluginList.value = data
    pluginListError.value = ''
    return true
  } catch {
    if (currentRequest === pluginListRequest) {
      pluginListError.value = afterChange ? 'settings.plugins.refreshFailed' : 'settings.plugins.listFailed'
    }
    return false
  } finally {
    if (currentRequest === pluginListRequest) pluginLoading.value = false
  }
}

async function reloadPlugins() {
  pluginLoading.value = true
  try {
    await request.post('/plugins/reload')
    if (await fetchPlugins(true)) window.showToast?.('插件已重载', 'success')
  } catch (e: any) {
    error.value = e.response?.data?.error || t('settings.plugins.reloadFailed')
  } finally {
    pluginLoading.value = false
  }
}

async function togglePlugin(p: { id: string; enabled: boolean; status?: string; error?: string }) {
  if (pendingPlugins.value.has(p.id)) return
  pendingPlugins.value.add(p.id)
  try {
    const { data } = await request.post('/plugins/toggle', { id: p.id, enabled: !p.enabled })
    p.enabled = data.enabled
    p.status = data.status
    p.error = data.error
    await fetchPlugins(true)
  } catch (e: any) {
    error.value = e.response?.data?.error || t('settings.plugins.toggleFailed')
  } finally {
    pendingPlugins.value.delete(p.id)
  }
}

async function deletePlugin(id: string) {
  if (pendingPlugins.value.has(id) || !confirm(t('settings.plugins.deleteConfirm'))) return
  pendingPlugins.value.add(id)
  try {
    await request.delete(`/plugins/${encodeURIComponent(id)}`)
    await fetchPlugins(true)
  } catch (e: any) {
    error.value = e.response?.data?.error || t('settings.plugins.deleteFailed')
  } finally {
    pendingPlugins.value.delete(id)
  }
}

async function fetchLogs() {
  if (logLoading.value) return
  logLoading.value = true
  try {
    const { data } = await request.get('/logs?lines=300')
    if (data && Array.isArray(data.lines)) logs.value = data.lines
    else if (Array.isArray(data)) logs.value = data
    else logs.value = []
    if (autoScrollBottom.value) {
      setTimeout(() => {
        if (logContainerRef.value) logContainerRef.value.scrollTop = logContainerRef.value.scrollHeight
      }, 50)
    }
  } catch (e) {
    console.error('获取系统日志失败:', e)
  } finally {
    logLoading.value = false
  }
}

function copyAllLogs() {
  const content = (Array.isArray(logs.value) ? logs.value : []).join('\n')
  navigator.clipboard.writeText(content)
  window.showToast?.('已复制全部日志', 'success')
}

watch(() => activeTab.value === 'logs' && autoRefreshLogs.value, (val) => {
  if (val) logTimer = setInterval(() => fetchLogs(), 3000)
  else if (logTimer) { clearInterval(logTimer); logTimer = null }
})

function triggerAvatarSelect() {
  avatarFileInput.value?.click()
}

function isValidAvatarUrl(raw: string): boolean {
  const value = raw.trim()
  if (!value) return false
  return /^(?:\/|https?:\/\/|data:image\/)/i.test(value)
}

async function handleAvatarFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return

  avatarError.value = ''
  avatarSuccess.value = ''
  target.value = ''
  if (file.size > 5 * 1024 * 1024) {
    avatarError.value = '图片大小不能超过 5 MiB'
    return
  }

  avatarUploading.value = true
  const formData = new FormData()
  formData.append('avatar', file)

  try {
    const { data } = await request.post('/user/avatar', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
    if (data.avatar_url) {
      setVal('USER_AVATAR_URL', data.avatar_url)
      savedSettings['USER_AVATAR_URL'] = data.avatar_url
      avatarSuccess.value = '头像上传成功，已实时生效'
      setTimeout(() => { avatarSuccess.value = '' }, 3000)
      window.dispatchEvent(new CustomEvent('avatar-updated', { detail: data.avatar_url }))
    }
  } catch (err: any) {
    avatarError.value = err?.response?.data?.error || '头像上传失败'
  } finally {
    avatarUploading.value = false
  }
}

async function saveAvatarUrl() {
  avatarError.value = ''
  avatarSuccess.value = ''
  const url = getVal('USER_AVATAR_URL').trim()
  if (!url) {
    avatarError.value = '请输入以 http(s)://、/ 或 data:image/ 开头的图片地址'
    return
  }
  if (!isValidAvatarUrl(url)) {
    avatarError.value = '无效的头像链接：仅支持 https://、http://、/ 或 data:image/ 地址'
    return
  }
  try {
    await request.put('/settings', { settings: { USER_AVATAR_URL: url } })
    savedSettings['USER_AVATAR_URL'] = url
    avatarSuccess.value = '头像链接已应用'
    setTimeout(() => { avatarSuccess.value = '' }, 3000)
    window.dispatchEvent(new CustomEvent('avatar-updated', { detail: url }))
  } catch (err: any) {
    avatarError.value = err?.response?.data?.error || '保存头像链接失败'
  }
}

async function clearAvatar() {
  avatarError.value = ''
  avatarSuccess.value = ''
  try {
    await request.put('/settings', { settings: { USER_AVATAR_URL: '' } })
    setVal('USER_AVATAR_URL', '')
    savedSettings['USER_AVATAR_URL'] = ''
    window.dispatchEvent(new CustomEvent('avatar-updated', { detail: '' }))
    avatarSuccess.value = '头像已重置为默认'
    setTimeout(() => { avatarSuccess.value = '' }, 2500)
  } catch {
    avatarError.value = '重置头像失败'
  }
}

async function changePassword() {
  passwordMsg.value = ''
  passwordError.value = ''
  if (newPassword.value.length < 6) { passwordError.value = '新密码不能少于 6 位'; return }
  if (newPassword.value !== confirmPassword.value) { passwordError.value = '两次输入密码不一致'; return }
  changingPassword.value = true
  try {
    await request.post('/user/change-password', { old_password: oldPassword.value, new_password: newPassword.value })
    passwordMsg.value = '密码修改成功，即将重新登录...'
    oldPassword.value = ''; newPassword.value = ''; confirmPassword.value = ''
    localStorage.removeItem('token')
    setTimeout(() => { router.push('/login') }, 1500)
  } catch (e: any) {
    passwordError.value = e?.response?.data?.error || '修改失败'
  } finally {
    changingPassword.value = false
  }
}

async function triggerManualCheckUpdate() {
  checkingUpdate.value = true
  manualCheckMsg.value = ''
  manualCheckResult.value = ''
  try {
    const res = await checkGitHubUpdate(true)
    if (res?.hasUpdate) {
      manualCheckResult.value = 'update'
      manualCheckMsgType.value = 'success'
      manualCheckMsg.value = `发现新版本：${res.latest}`
    } else if (res?.error) {
      manualCheckResult.value = 'error'
      manualCheckMsgType.value = 'error'
      manualCheckMsg.value = `检查更新失败: ${res.error}`
    } else {
      manualCheckResult.value = 'latest'
      manualCheckMsgType.value = 'success'
      manualCheckMsg.value = currentVersion.value ? `当前已是最新版本 (${currentVersion.value})` : '当前已是最新版本'
    }
  } catch (e: any) {
    manualCheckMsgType.value = 'error'
    manualCheckMsg.value = `检查更新失败: ${e?.message || '网络连接异常'}`
  } finally {
    checkingUpdate.value = false
  }
}

function toggleAutoCheckUpdate() {
  const next = getVal('AUTO_CHECK_UPDATE') !== 'false' ? 'false' : 'true'
  setVal('AUTO_CHECK_UPDATE', next)
  localStorage.setItem('ani-go-auto-update', next)
  window.showToast?.(next === 'true' ? '已开启自动检测更新' : '已关闭自动检测更新', 'info')
}

function handleGlobalKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    saveAll()
  }
}

watch(activeTab, (newTab) => {
  if (newTab === 'logs') fetchLogs()
})

onMounted(() => {
  fetchSettings()
  fetchPlugins()
  if (activeTab.value === 'logs') fetchLogs()
  window.addEventListener('keydown', handleGlobalKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  if (logTimer) clearInterval(logTimer)
})
</script>

<template>
  <div class="space-y-5 pb-16">
    <!-- 顶部 Preferences 操作栏 -->
    <section class="v2-panel p-4 sm:p-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div class="flex items-center gap-3 min-w-0">
        <div>
          <div class="flex items-center gap-2.5 flex-wrap">
            <h1 class="text-lg sm:text-xl font-bold tracking-tight text-[var(--v2-ink)]">{{ $t('settings.title') }}</h1>
            <span v-if="currentVersion" class="v2-pill font-mono text-[11px]">{{ currentVersion }}</span>
          </div>
          <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.subtitle') }}</p>
        </div>
      </div>

      <button
        type="button"
        @click="saveAll"
        class="v2-btn v2-btn-primary shrink-0"
        :disabled="loading || !settingsReady || savingSettings"
      >
        <Check :size="15" />
        <span>{{ $t('settings.saveAll') }}</span>
        <kbd class="hidden md:inline-block font-mono text-[10px] px-1.5 py-0.5 rounded bg-black/20">{{ $t('settings.ctrlS') }}</kbd>
      </button>
    </section>

    <div v-if="saved" class="v2-panel border-emerald-500/35 bg-emerald-500/10 text-emerald-400 px-4 py-3 flex items-center justify-between text-xs font-medium">
      <span>{{ $t('settings.updateSuccess') }} — {{ $t('settings.updateSuccessDesc') }}</span>
      <button type="button" @click="saved = false"><X :size="14" /></button>
    </div>

    <div v-if="error" class="v2-panel border-red-500/35 bg-red-500/10 text-red-400 px-4 py-3 flex items-center justify-between text-xs font-medium">
      <span>{{ error }}</span>
      <button type="button" @click="error = ''"><X :size="14" /></button>
    </div>

    <div v-if="loading" class="flex justify-center py-24">
      <span class="loading loading-spinner loading-lg text-[var(--v2-accent)]"></span>
    </div>

    <!-- Linear 风格双栏 Preferences -->
    <div v-else class="flex flex-col lg:flex-row gap-6 items-start">
      <!-- 移动端横向标签条 -->
      <div class="lg:hidden w-full overflow-x-auto no-scrollbar flex items-center gap-1.5 v2-panel p-1.5 sticky top-14 z-20">
        <button
          v-for="tab in allFlatTabs"
          :key="tab.key"
          type="button"
          @click="activeTab = tab.key"
          class="px-3 py-1.5 rounded-lg text-xs font-semibold whitespace-nowrap flex items-center gap-1.5 shrink-0 transition-colors cursor-pointer"
          :class="activeTab === tab.key ? 'bg-[var(--v2-accent)] text-white' : 'text-[var(--v2-ink-muted)]'"
        >
          <component :is="tab.icon" :size="13" />
          <span>{{ tab.label }}</span>
        </button>
      </div>

      <!-- 桌面端左侧粘性分类栏 -->
      <aside class="hidden lg:block w-60 shrink-0 lg:sticky lg:top-20 v2-panel p-3 space-y-4">
        <div v-for="group in tabGroups" :key="group.name" class="space-y-1">
          <div class="px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wider text-[var(--v2-ink-subtle)]">
            {{ group.name }}
          </div>
          <button
            v-for="tab in group.tabs"
            :key="tab.key"
            type="button"
            @click="activeTab = tab.key"
            class="w-full flex items-center justify-between px-2.5 py-2 rounded-lg text-[13px] transition-colors cursor-pointer"
            :class="activeTab === tab.key
              ? 'bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] font-semibold'
              : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] hover:bg-[var(--v2-surface-2)] font-medium'"
          >
            <div class="flex items-center gap-2 min-w-0">
              <component :is="tab.icon" :size="15" class="shrink-0" />
              <span class="truncate">{{ tab.label }}</span>
            </div>
            <span v-if="!tab.ready" class="text-[9px] font-mono px-1.5 py-0.5 rounded bg-[var(--v2-surface-2)] text-[var(--v2-ink-subtle)]">v2</span>
            <span v-else-if="activeTab === tab.key" class="w-1.5 h-1.5 rounded-full bg-[var(--v2-accent)] shrink-0"></span>
          </button>
        </div>
      </aside>

      <!-- 右侧分组面板 -->
      <div class="flex-1 min-w-0 w-full space-y-5">
        <!-- Mikan / Bangumi 镜像测速与授权卡片 -->
        <div v-if="activeTab === 'mikan' || activeTab === 'bangumi'" class="v2-panel p-5 sm:p-6 space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.mikan.mirrorAudit') }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">实测当前环境至镜像站点的握手延迟并一键应用</p>
            </div>
            <div class="flex items-center gap-2">
              <button v-if="activeTab === 'bangumi'" type="button" class="v2-btn v2-btn-sm" @click="connectBangumi">
                <ExternalLink :size="13" />
                <span>Bangumi OAuth</span>
              </button>
              <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" :disabled="mirrorTesting" @click="testMirrors">
                <RefreshCw :size="13" :class="{ 'animate-spin': mirrorTesting }" />
                <span>{{ mirrorTesting ? '测速中...' : '开始镜像测速' }}</span>
              </button>
            </div>
          </div>

          <div v-if="mirrorResults.length > 0" class="grid sm:grid-cols-3 gap-2.5">
            <button
              v-for="m in mirrorResults"
              :key="m.domain"
              type="button"
              @click="selectMirror(m.domain)"
              class="v2-panel-muted p-3 text-left flex items-center justify-between hover:border-[var(--v2-accent)]/50 transition-colors cursor-pointer"
              :class="{ 'border-[var(--v2-accent)]': selectedMirror === m.domain }"
            >
              <span class="text-xs font-mono font-semibold truncate">{{ m.domain }}</span>
              <span class="text-xs font-mono" :class="m.ok ? 'text-emerald-400' : 'text-red-400'">
                {{ m.ok ? `${m.latency_ms}ms` : '不可达' }}
              </span>
            </button>
          </div>
        </div>

        <!-- 通用分组渲染（paths, downloader, sources, mikan, bangumi, scheduler） -->
        <template v-if="currentTabSections.length > 0">
          <div v-for="section in currentTabSections" :key="section.title" class="v2-panel p-5 sm:p-6 space-y-5">
            <div class="border-b border-[var(--v2-border)] pb-3">
              <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ section.title }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ section.desc }}</p>
            </div>

            <div class="grid gap-4">
              <div v-for="field in section.fields" :key="field.key" class="space-y-1.5">
                <!-- 布尔 Switch 行（严格读取 val === 'true'，彻底修复旧 <select> 假开启 bug） -->
                <div v-if="field.type === 'switch'" class="v2-panel-muted p-3.5 flex items-center justify-between gap-4">
                  <div class="min-w-0">
                    <div class="text-xs font-semibold text-[var(--v2-ink)]">{{ field.label }}</div>
                    <p v-if="field.hint" class="text-[11px] text-[var(--v2-ink-muted)] mt-0.5">{{ field.hint }}</p>
                    <p class="text-[10px] font-mono text-[var(--v2-ink-subtle)] mt-0.5">
                      {{ field.key }} = {{ isSwitchEnabled(field.key) ? 'true' : 'false' }}
                    </p>
                  </div>
                  <button
                    type="button"
                    role="switch"
                    class="v2-switch"
                    :aria-checked="isSwitchEnabled(field.key)"
                    :aria-label="field.label"
                    @click="toggleSwitch(field.key)"
                  >
                    <span class="v2-switch-thumb"></span>
                  </button>
                </div>

                <!-- 下拉或输入框 -->
                <template v-else>
                  <div class="flex items-center justify-between gap-2">
                    <label class="text-xs font-semibold text-[var(--v2-ink-muted)]">{{ field.label }}</label>
                    <button
                      v-if="field.testDownloader"
                      type="button"
                      class="text-xs font-semibold text-[var(--v2-accent)] hover:underline flex items-center gap-1 cursor-pointer bg-transparent border-0 p-0"
                      :disabled="testingDownloaderKey === field.testDownloader"
                      @click="testDownloader(field.testDownloader)"
                    >
                      <Play :size="11" />
                      <span>{{ testingDownloaderKey === field.testDownloader ? $t('settings.testing') : $t('settings.testNow') }}</span>
                    </button>
                  </div>

                  <select
                    v-if="field.type === 'select'"
                    :value="selectValue(field)"
                    @change="(e: Event) => setVal(field.key, (e.target as HTMLSelectElement).value)"
                    class="v2-input cursor-pointer"
                  >
                    <option v-for="opt in field.selectOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                  </select>

                  <div v-else class="relative">
                    <input
                      :type="inputType(field)"
                      :value="getVal(field.key)"
                      @input="(e: Event) => setVal(field.key, (e.target as HTMLInputElement).value)"
                      :placeholder="field.placeholder"
                      autocomplete="off"
                      class="v2-input font-mono text-xs pr-9"
                    />
                    <button
                      v-if="field.type === 'password'"
                      type="button"
                      @click="togglePassword(field.key)"
                      class="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)] hover:text-[var(--v2-ink)]"
                    >
                      <EyeOff v-if="showPasswords.has(field.key)" :size="14" />
                      <Eye v-else :size="14" />
                    </button>
                  </div>
                </template>
              </div>
            </div>
          </div>
        </template>

        <!-- 插件生态面板（保留 <h4> + checkbox + PluginPanel 契约） -->
        <div v-else-if="activeTab === 'plugins'" class="v2-panel p-5 sm:p-6 space-y-5">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.plugins.title') }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.plugins.desc') }}</p>
            </div>
            <div class="flex items-center gap-2">
              <select v-model="pluginSourceFilter" class="v2-input h-8 w-32 text-xs cursor-pointer">
                <option value="all">{{ $t('settings.plugins.allSources') }}</option>
                <option value="official">{{ $t('settings.plugins.official') }}</option>
                <option value="community">{{ $t('settings.plugins.community') }}</option>
              </select>
              <button type="button" class="v2-btn v2-btn-sm" @click="reloadPlugins" :disabled="pluginLoading || pendingPlugins.size > 0">
                <RefreshCw :size="13" :class="{ 'animate-spin': pluginLoading }" />
                <span>{{ $t('settings.plugins.reload') }}</span>
              </button>
            </div>
          </div>

          <div v-if="pluginListError" role="alert" class="text-xs text-red-400">{{ $t(pluginListError) }}</div>
          <div v-else-if="!visiblePlugins.length" class="py-8 text-center text-xs text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.noPluginsFromSource') }}</div>
          <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div
              v-for="plugin in visiblePlugins"
              :key="plugin.id"
              class="v2-panel-muted p-4 flex flex-col justify-between gap-3"
              :class="{ 'border-[var(--v2-accent)]/35': plugin.enabled }"
            >
              <div class="space-y-2">
                <div class="flex items-start justify-between gap-2">
                  <div class="min-w-0">
                    <div class="flex items-center gap-2 flex-wrap">
                      <h4 class="font-bold text-sm text-[var(--v2-ink)]">{{ plugin.name }}</h4>
                      <span class="v2-pill font-mono text-[10px]">v{{ plugin.version || '1.0' }}</span>
                    </div>
                    <div class="flex items-center gap-1.5 mt-1 text-[10px]">
                      <span class="v2-pill h-5">{{ plugin.origin === 'official' ? $t('settings.plugins.official') : $t('settings.plugins.community') }}</span>
                      <span v-if="pluginRunning(plugin)" class="text-emerald-400 font-semibold">{{ $t('settings.plugins.statusRunning') }}</span>
                      <span v-else class="text-[var(--v2-ink-subtle)]">{{ $t('settings.plugins.statusStopped') }}</span>
                    </div>
                  </div>
                  <div class="flex items-center gap-1.5 shrink-0">
                    <input
                      type="checkbox"
                      class="toggle"
                      :checked="plugin.enabled"
                      :disabled="pluginLoading || pendingPlugins.has(plugin.id)"
                      @click.prevent="togglePlugin(plugin)"
                    />
                    <button
                      v-if="!plugin.is_builtin"
                      type="button"
                      :disabled="pluginLoading || pendingPlugins.has(plugin.id)"
                      :aria-label="$t('settings.plugins.deleteNamed', { name: plugin.name })"
                      @click="deletePlugin(plugin.id)"
                      class="p-1 text-red-400 hover:bg-red-500/10 rounded"
                    >
                      <Trash2 :size="13" />
                    </button>
                  </div>
                </div>
                <p class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">{{ plugin.description || $t('settings.plugins.noDesc') }}</p>
                <div v-if="plugin.status === 'error' || plugin.error" class="p-2 rounded-lg bg-red-500/10 border border-red-500/25 text-red-400 text-[11px] font-mono">
                  <AlertCircle :size="12" class="inline mr-1" />{{ plugin.error || $t('settings.plugins.statusError') }}
                </div>
              </div>
              <fieldset v-if="pluginRunning(plugin) && plugin.panel" class="min-w-0 pt-2 border-t border-[var(--v2-border)]" :disabled="pluginLoading || pendingPlugins.has(plugin.id)">
                <PluginPanel :key="JSON.stringify([plugin.url, plugin.panel])" :plugin="plugin" />
              </fieldset>
            </div>
          </div>
        </div>

        <!-- 实时运行日志控制台 -->
        <div v-else-if="activeTab === 'logs'" class="v2-panel p-5 sm:p-6 space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-bold flex items-center gap-2 text-[var(--v2-ink)]">
                <span>{{ $t('settings.logs.consoleTitle') }}</span>
                <span class="v2-pill font-mono text-[10px]">{{ filteredLogs.length }} / {{ logs.length }}</span>
              </h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.logs.consoleDesc') }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <div class="relative">
                <Search :size="12" class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
                <input v-model="logFilter" type="text" :placeholder="$t('settings.logs.searchPlaceholder')" class="v2-input pl-7 h-8 w-44 text-xs font-mono" />
              </div>
              <label class="flex items-center gap-1.5 text-xs cursor-pointer border border-[var(--v2-border)] px-2.5 h-8 rounded-lg">
                <input type="checkbox" v-model="autoRefreshLogs" class="checkbox checkbox-primary checkbox-xs" />
                <span>{{ $t('settings.logs.autoRefresh') }}</span>
              </label>
              <button type="button" class="v2-btn v2-btn-sm" @click="copyAllLogs"><Copy :size="12" /><span>复制</span></button>
              <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="fetchLogs" :disabled="logLoading">
                <RefreshCw :size="12" :class="{ 'animate-spin': logLoading }" />
                <span>{{ $t('common.refresh') }}</span>
              </button>
            </div>
          </div>

          <div class="flex items-center gap-1.5 flex-wrap">
            <button
              v-for="lvl in logLevels"
              :key="lvl.key"
              type="button"
              @click="currentLogLevel = lvl.key"
              class="h-7 px-2.5 rounded-md text-xs font-semibold cursor-pointer transition-colors"
              :class="currentLogLevel === lvl.key ? 'bg-[var(--v2-accent)] text-white' : 'bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)]'"
            >
              {{ lvl.label }}
            </button>
          </div>

          <div ref="logContainerRef" class="bg-[#09090b] text-zinc-300 rounded-xl p-4 font-mono text-[11px] leading-relaxed max-h-[540px] overflow-y-auto space-y-1 border border-[var(--v2-border)]">
            <div
              v-for="(line, i) in filteredLogs"
              :key="i"
              class="py-0.5 px-1.5 rounded break-all"
              :class="{
                'text-red-400 bg-red-500/10': line.includes('❌') || line.includes('ERROR') || line.includes('error') || line.includes('failed'),
                'text-amber-300 bg-amber-500/10': line.includes('⚠️') || line.includes('WARN') || line.includes('warn'),
                'text-emerald-400': line.includes('✅') || line.includes('SUCCESS'),
              }"
            >
              {{ line }}
            </div>
            <div v-if="filteredLogs.length === 0" class="text-center py-14 text-[var(--v2-ink-subtle)]">
              {{ logLoading ? '正在加载日志...' : '暂无运行日志' }}
            </div>
          </div>
        </div>

        <!-- 账户与安全（彻底修复默认头像破图 & 浏览器自动填充 "admin" 串位问题） -->
        <div v-else-if="activeTab === 'account'" class="space-y-5">
          <!-- 管理员头像卡片 -->
          <div class="v2-panel p-5 sm:p-6 space-y-5">
            <div>
              <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.account.avatarTitle') }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.account.avatarDesc') }}</p>
            </div>

            <div class="v2-panel-muted p-4 sm:p-5 flex flex-col sm:flex-row items-center sm:items-start gap-5">
              <div class="shrink-0 cursor-pointer" @click="triggerAvatarSelect" title="点击上传头像">
                <UserAvatar :src="getVal('USER_AVATAR_URL')" username="admin" size="lg" rounded="full" />
              </div>

              <div class="flex-1 w-full space-y-3.5">
                <div class="flex items-center gap-2.5 flex-wrap">
                  <input
                    ref="avatarFileInput"
                    type="file"
                    accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
                    class="hidden"
                    @change="handleAvatarFileChange"
                  />
                  <button type="button" @click="triggerAvatarSelect" :disabled="avatarUploading" class="v2-btn v2-btn-primary v2-btn-sm">
                    <RefreshCw v-if="avatarUploading" :size="13" class="animate-spin" />
                    <Upload v-else :size="13" />
                    <span>{{ avatarUploading ? $t('common.loading') : $t('settings.account.upload') }}</span>
                  </button>
                  <button v-if="getVal('USER_AVATAR_URL')" type="button" @click="clearAvatar" class="v2-btn v2-btn-sm text-red-400">
                    <Trash2 :size="13" />
                    <span>{{ $t('settings.account.resetAvatar') }}</span>
                  </button>
                  <span class="text-[11px] text-[var(--v2-ink-subtle)]">支持 JPG, PNG, WebP, GIF, SVG（最大 5 MiB）</span>
                </div>

                <div class="space-y-1.5">
                  <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.account.avatarUrlLabel') }}</label>
                  <div class="flex gap-2">
                    <input
                      type="url"
                      name="anigo_custom_avatar_url"
                      autocomplete="off"
                      data-lpignore="true"
                      data-1p-ignore="true"
                      :value="getVal('USER_AVATAR_URL')"
                      @input="(e: Event) => setVal('USER_AVATAR_URL', (e.target as HTMLInputElement).value)"
                      placeholder="https://example.com/avatar.png"
                      class="v2-input font-mono text-xs flex-1"
                    />
                    <button type="button" @click="saveAvatarUrl" class="v2-btn v2-btn-sm shrink-0">应用链接</button>
                  </div>
                </div>

                <p v-if="avatarError" class="text-xs text-red-400 bg-red-500/10 border border-red-500/25 p-2.5 rounded-lg">{{ avatarError }}</p>
                <p v-if="avatarSuccess" class="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/25 p-2.5 rounded-lg">{{ avatarSuccess }}</p>
              </div>
            </div>
          </div>

          <!-- 修改密码（独立 form autocomplete="off" + 隐藏 username 隔离浏览器自动填充） -->
          <div class="v2-panel p-5 sm:p-6 space-y-4">
            <div>
              <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.account.title') }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.account.desc') }}</p>
            </div>

            <form autocomplete="off" @submit.prevent="changePassword" class="max-w-md space-y-3.5">
              <input type="text" name="username" value="admin" autocomplete="username" readonly tabindex="-1" class="hidden" />

              <div>
                <label class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1">{{ $t('settings.account.oldPass') }}</label>
                <input v-model="oldPassword" type="password" autocomplete="current-password" :placeholder="$t('settings.account.oldPassPlaceholder')" class="v2-input" />
              </div>
              <div>
                <label class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1">{{ $t('settings.account.newPass') }}</label>
                <input v-model="newPassword" type="password" autocomplete="new-password" :placeholder="$t('settings.account.newPassPlaceholder')" class="v2-input" />
              </div>
              <div>
                <label class="block text-xs font-semibold text-[var(--v2-ink-muted)] mb-1">{{ $t('settings.account.confirmPass') }}</label>
                <input v-model="confirmPassword" type="password" autocomplete="new-password" :placeholder="$t('settings.account.confirmPassPlaceholder')" class="v2-input" />
              </div>

              <button type="submit" :disabled="changingPassword" class="v2-btn v2-btn-primary">
                <Lock :size="14" />
                <span>{{ changingPassword ? $t('settings.account.submitting') : $t('settings.account.submit') }}</span>
              </button>

              <p v-if="passwordMsg" class="text-xs text-emerald-400 bg-emerald-500/10 border border-emerald-500/25 p-2.5 rounded-lg">{{ passwordMsg }}</p>
              <p v-if="passwordError" class="text-xs text-red-400 bg-red-500/10 border border-red-500/25 p-2.5 rounded-lg">{{ passwordError }}</p>
            </form>
          </div>

          <!-- 系统版本与在线更新 -->
          <div class="v2-panel p-5 sm:p-6 space-y-4">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-bold flex items-center gap-2 text-[var(--v2-ink)]">
                  <Zap class="text-[var(--v2-accent)]" :size="16" />
                  <span>系统版本与在线更新</span>
                </h3>
                <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">检测 Ani-Go GitHub Releases 最新版本</p>
              </div>
              <span v-if="currentVersion" class="v2-pill font-mono text-xs">{{ currentVersion }}</span>
            </div>

            <div class="v2-panel-muted p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div class="text-xs space-y-1">
                <div>
                  <span class="font-semibold">当前版本：</span>
                  <span class="font-mono text-[var(--v2-accent)]">{{ currentVersion || '—' }}</span>
                </div>
                <div class="text-[11px] text-[var(--v2-ink-muted)]">
                  仓库：<a href="https://github.com/xiaoyueRX/Ani-Go/releases" target="_blank" rel="noopener noreferrer" class="text-[var(--v2-accent)] hover:underline">github.com/xiaoyueRX/Ani-Go</a>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <button v-if="hasNewVersion && canSelfUpdate" type="button" @click="triggerUpdate" :disabled="updating" class="v2-btn v2-btn-primary v2-btn-sm">
                  <Download :size="13" />
                  <span>{{ updating ? '正在升级...' : `升级至 ${latestVersion}` }}</span>
                </button>
                <button type="button" @click="triggerManualCheckUpdate" :disabled="checkingUpdate" class="v2-btn v2-btn-sm">
                  <RefreshCw :size="13" :class="{ 'animate-spin': checkingUpdate }" />
                  <span>{{ checkingUpdate ? '正在检测...' : '检查新版本' }}</span>
                </button>
              </div>
            </div>

            <p v-if="manualCheckMsg" class="text-xs p-2.5 rounded-lg border" :class="manualCheckMsgType === 'success' ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/25' : 'bg-red-500/10 text-red-400 border-red-500/25'">
              {{ manualCheckMsg }}
            </p>

            <div class="v2-panel-muted p-3.5 flex items-center justify-between gap-4">
              <div>
                <div class="text-xs font-semibold">自动检测最新版本 (AUTO_CHECK_UPDATE)</div>
                <p class="text-[11px] text-[var(--v2-ink-muted)] mt-0.5">开启后在页面打开时静默探测 GitHub Releases</p>
              </div>
              <button
                type="button"
                role="switch"
                class="v2-switch"
                :aria-checked="getVal('AUTO_CHECK_UPDATE') !== 'false'"
                @click="toggleAutoCheckUpdate"
              >
                <span class="v2-switch-thumb"></span>
              </button>
            </div>
          </div>
        </div>

        <!-- 外观与多套主题工坊 -->
        <ThemeAppearancePanel v-else-if="activeTab === 'appearance'" />

        <!-- v2 框架待迁移分组占位卡片（ai, customRegex, notify, backup） -->
        <div v-else class="v2-panel p-8 sm:p-10 text-center space-y-3">
          <div class="w-12 h-12 rounded-2xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center mx-auto">
            <Sparkles :size="22" />
          </div>
          <h3 class="text-base font-bold text-[var(--v2-ink)]">
            {{ allFlatTabs.find(t => t.key === activeTab)?.label }} — {{ locale === 'zh' ? 'v2 框架待迁移模块' : 'v2 Pending Migration' }}
          </h3>
          <p class="text-xs text-[var(--v2-ink-muted)] max-w-md mx-auto leading-relaxed">
            {{ locale === 'zh'
              ? '当前为全新 v2「编辑部放映厅」整体骨架预览阶段。核心设置（存储路径、下载器、多源搜索站、Mikan、Bangumi、调度周期、日志、账户头像与密码）已完整接线；待您确认整体视觉方向后，将把本分组（AI/正则测试/多渠道通知流水/备份迁移）一并迁入。'
              : 'This section is queued for migration once the v2 visual framework is approved.' }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
</style>
