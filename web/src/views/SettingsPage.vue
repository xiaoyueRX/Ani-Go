<script setup lang="ts">
import { ref, onMounted, computed, watch, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  Check, Antenna, Download, 
  Folder, Bell, Cpu, 
  Settings, Timer, Lock, 
  FileText, Eye, EyeOff,
  RefreshCw, User, Database, 
  RotateCcw, Upload, Trash2, 
  Shield, Sparkles, Plus, 
  Copy, Search, Camera
} from 'lucide-vue-next'
import { CURRENT_VERSION, currentVersion } from '../composables/useVersion'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()

// 全局配置数据
const settings = ref<Record<string, string>>({})
const loading = ref(true)
const error = ref('')
const saved = ref(false)
const activeTab = ref(route.query.tab ? String(route.query.tab) : 'paths')
const showPasswords = ref<Set<string>>(new Set())

// 镜像测速
const mirrorTesting = ref(false)
const mirrorResults = ref<{ domain: string; latency_ms: number; ok: boolean }[]>([])
const selectedMirror = ref('')

// 管理员头像上传与预览
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

// 备份管理
const backupLoading = ref(false)
const backupList = ref<{ name: string; size: number; mod_time: string }[]>([])
const creatingBackup = ref(false)
const restoringBackup = ref(false)
const deletingBackup = ref<string | null>(null)
const showBackupEpisodes = ref(false)

// 插件管理
const pluginLoading = ref(false)
const pluginList = ref<any[]>([])
const showPluginModal = ref(false)
const pluginModalTab = ref<'webhook' | 'json'>('webhook')
const pluginForm = ref({
  id: '',
  name: '',
  description: '',
  version: '1.0.0',
  author: 'User',
  author_url: '',
  type: 'webhook',
  url: '',
  secret: '',
  events: ['download.completed', 'file.organized'] as string[]
})
const pluginJsonText = ref('')
const pluginJsonError = ref('')
const pluginSaving = ref(false)

const availablePluginEvents = [
  { id: 'subscription.added', label: '订阅添加 (subscription.added)' },
  { id: 'download.started', label: '下载开始 (download.started)' },
  { id: 'download.completed', label: '下载完成 (download.completed)' },
  { id: 'file.organized', label: '文件整理完毕 (file.organized)' },
  { id: 'episode.missing', label: '剧集缺失检测 (episode.missing)' },
  { id: 'error', label: '系统异常报警 (error)' }
]

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
  { key: 'error', label: '仅错误 (❌)' },
  { key: 'warn', label: '仅警告 (⚠️)' },
  { key: 'info', label: '成功与信息 (✅/ℹ️)' },
  { key: 'plugin', label: '插件与任务 (🔗/🚀)' }
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

// AI 模型列表
const modelLoading = ref(false)
const modelOptions = ref<{ label: string; value: string }[]>([])

// 导航分组结构
const tabGroups = computed(() => [
  {
    name: t('settings.groups.core'),
    tabs: [
      { key: 'paths', label: t('settings.tabs.paths'), icon: Folder },
      { key: 'downloader', label: t('settings.tabs.downloader'), icon: Download },
    ]
  },
  {
    name: t('settings.groups.sources'),
    tabs: [
      { key: 'mikan', label: t('settings.tabs.mikan'), icon: Antenna },
      { key: 'bangumi', label: t('settings.tabs.bangumi'), icon: Antenna },
    ]
  },
  {
    name: t('settings.groups.extensions'),
    tabs: [
      { key: 'ai', label: t('settings.tabs.ai'), icon: Cpu },
      { key: 'notify', label: t('settings.tabs.notify'), icon: Bell },
      { key: 'plugins', label: t('settings.tabs.plugins'), icon: Sparkles },
    ]
  },
  {
    name: t('settings.groups.system'),
    tabs: [
      { key: 'backup', label: t('settings.tabs.backup'), icon: Database },
      { key: 'logs', label: t('settings.tabs.logs'), icon: FileText },
      { key: 'account', label: t('settings.tabs.account'), icon: Lock },
    ]
  }
])

const allFlatTabs = computed(() => tabGroups.value.flatMap(g => g.tabs))

// 扁平化标签页定义与字段映射
interface FieldDef {
  label: string
  key: string
  placeholder: string
  type?: string
  hint?: string
  testChannel?: string
  selectOptions?: { label: string; value: string }[]
}

const tabs = computed(() => [
  { key: 'paths', label: t('settings.tabs.paths'), icon: Folder, sections: [
    { title: t('settings.sections.pathsStorage'), desc: t('settings.sections.pathsStorageDesc'), fields: [
      { label: t('settings.fields.db'), key: 'DB_PATH', placeholder: './data/ani-go.db' },
      { label: t('settings.fields.tv'), key: 'TV_BASE_PATH', placeholder: '/data/media/anime' },
      { label: t('settings.fields.movie'), key: 'MOVIE_BASE_PATH', placeholder: '/data/media/movies' },
    ]}
  ]},
  { key: 'downloader', label: t('settings.tabs.downloader'), icon: Download, sections: [
    { title: t('settings.sections.engine'), desc: t('settings.sections.engineDesc'), fields: [
      { label: t('settings.fields.activeDownloader'), key: 'DEFAULT_DOWNLOADER', placeholder: 'qbittorrent', type: 'select', selectOptions: [
        { label: t('settings.options.qbRecommend'), value: 'qbittorrent' },
        { label: t('settings.options.transmission'), value: 'transmission' },
        { label: t('settings.options.aria2'), value: 'aria2' },
      ]},
    ]},
    { title: t('settings.sections.qb'), desc: t('settings.sections.qbDesc'), fields: [
      { label: t('settings.fields.qbHost'), key: 'QB_HOST', placeholder: 'http://localhost:8081' },
      { label: t('settings.fields.qbCategory'), key: 'QB_CATEGORY', placeholder: 'ani-go' },
      { label: t('settings.fields.qbUser'), key: 'QB_USER', placeholder: 'admin' },
      { label: t('settings.fields.qbPass'), key: 'QB_PASS', placeholder: '••••••••', type: 'password' },
    ]},
    { title: t('settings.sections.tr'), desc: t('settings.sections.trDesc'), fields: [
      { label: t('settings.fields.trHost'), key: 'TR_HOST', placeholder: 'http://localhost:9091' },
      { label: t('settings.fields.trUser'), key: 'TR_USER', placeholder: 'Username' },
      { label: t('settings.fields.trPass'), key: 'TR_PASS', placeholder: '••••••••', type: 'password' },
    ]},
    { title: t('settings.sections.aria2'), desc: t('settings.sections.aria2Desc'), fields: [
      { label: t('settings.fields.aria2Host'), key: 'ARIA2_HOST', placeholder: 'http://localhost:6800/jsonrpc' },
      { label: t('settings.fields.aria2Secret'), key: 'ARIA2_SECRET', placeholder: 'Secret Token', type: 'password' },
    ]},
    { title: t('settings.sections.seedCleanup'), desc: t('settings.sections.seedCleanupDesc'), fields: [
      { label: t('settings.fields.seedCleanupEnabled'), key: 'SEED_CLEANUP_ENABLED', placeholder: '', type: 'select', selectOptions: [
        { label: t('settings.options.seedDeleteTaskOnly'), value: 'true' },
        { label: t('settings.options.disabled'), value: 'false' },
      ]},
      { label: t('settings.fields.seedCleanupInterval'), key: 'SEED_CLEANUP_INTERVAL', placeholder: '1h' },
      { label: t('settings.fields.seedCleanupMinRatio'), key: 'SEED_CLEANUP_MIN_RATIO', placeholder: '1.0' },
    ]}
  ]},
  { key: 'mikan', label: t('settings.tabs.mikan'), icon: Antenna, sections: [
    { title: t('settings.sections.mikanSync'), desc: t('settings.sections.mikanSyncDesc'), fields: [
      { label: t('settings.fields.mikanRss'), key: 'MIKAN_RSS_URL', placeholder: 'https://mikanani.me/RSS/MyBangumi?token=***' },
      { label: t('settings.fields.mikanRssMode'), key: 'MIKAN_RSS_MODE', placeholder: '', type: 'select', selectOptions: [
        { label: t('settings.options.rssPersonal'), value: 'personal' },
        { label: t('settings.options.rssClassic'), value: 'classic' },
      ]},
      { label: t('settings.fields.mikanDomain'), key: 'MIKAN_DOMAIN', placeholder: 'mikanani.me' },
      { label: t('settings.fields.mikanProxy'), key: 'MIKAN_PROXY_DOMAIN', placeholder: 'mikanani.me' },
      { label: t('settings.fields.mikanMirrors'), key: 'MIKAN_MIRROR_DOMAINS', placeholder: 'mikanani.me,mikanime.tv' },
    ]}
  ]},
  { key: 'bangumi', label: t('settings.tabs.bangumi'), icon: Antenna, sections: [
    { title: t('settings.sections.bangumiOAuth'), desc: t('settings.sections.bangumiOAuthDesc'), fields: [
      { label: t('settings.fields.bangumiClientId'), key: 'BANGUMI_CLIENT_ID', placeholder: 'bgm... (Client ID)' },
      { label: t('settings.fields.bangumiClientSecret'), key: 'BANGUMI_CLIENT_SECRET', placeholder: 'Client Secret', type: 'password' },
    ]},
    { title: t('settings.sections.bangumiSync'), desc: t('settings.sections.bangumiSyncDesc'), fields: [
      { label: t('settings.fields.bangumiUsername'), key: 'BGMTV_USERNAME', placeholder: 'Bangumi username' },
      { label: t('settings.fields.bangumiToken'), key: 'BGMTV_USER_TOKEN', placeholder: 'Bearer Token', type: 'password' },
      { label: t('settings.fields.bangumiSyncInterval'), key: 'BGMTV_SYNC_INTERVAL', placeholder: '6h' },
      { label: t('settings.fields.bangumiDomain'), key: 'BGMTV_DOMAIN', placeholder: 'api.bgm.tv' },
      { label: t('settings.fields.bangumiMirrors'), key: 'BGMTV_MIRROR_DOMAINS', placeholder: 'api.bgm.tv,api.bangumi.tv,api.chii.in' },
    ]}
  ]},
  { key: 'ai', label: t('settings.tabs.ai'), icon: Cpu, sections: [
    { title: t('settings.sections.aiModel'), desc: t('settings.sections.aiModelDesc'), fields: [
      { label: t('settings.fields.aiProtocol'), key: 'AI_PROTOCOL', placeholder: 'openai', type: 'select', selectOptions: [
        { label: t('settings.options.aiOpenai'), value: 'openai' },
        { label: t('settings.options.aiGoogle'), value: 'google' },
        { label: t('settings.options.aiAnthropic'), value: 'anthropic' },
        { label: t('settings.options.aiOllama'), value: 'ollama' },
      ]},
      { label: t('settings.fields.aiEndpoint'), key: 'AI_ENDPOINT', placeholder: 'https://api.openai.com/v1' },
      { label: t('settings.fields.aiKey'), key: 'AI_API_KEY', placeholder: 'sk-...', type: 'password' },
      { label: t('settings.fields.aiModel'), key: 'AI_MODEL', placeholder: 'gpt-4o-mini / deepseek-chat' },
      { label: t('settings.fields.aiSmartSearch'), key: 'AI_SMART_SEARCH', placeholder: '', type: 'select', selectOptions: [
        { label: t('settings.options.enabled'), value: 'true' },
        { label: t('settings.options.disabled'), value: 'false' },
      ]},
    ]}
  ]},
  { key: 'notify', label: t('settings.tabs.notify'), icon: Bell, sections: [
    { title: t('settings.sections.notifyTg'), desc: t('settings.sections.notifyTgDesc'), fields: [
      { label: t('settings.fields.tgToken'), key: 'TELEGRAM_BOT_TOKEN', placeholder: '123456:ABC...', type: 'password', testChannel: 'Telegram' },
      { label: t('settings.fields.tgChatId'), key: 'TELEGRAM_CHAT_ID', placeholder: '123456789' },
    ]},
    { title: t('settings.sections.notifyWebhook'), desc: t('settings.sections.notifyWebhookDesc'), fields: [
      { label: t('settings.fields.dingtalkWebhook'), key: 'DINGTALK_WEBHOOK', placeholder: 'https://oapi.dingtalk.com/...', testChannel: 'DingTalk' },
      { label: t('settings.fields.dingtalkSecret'), key: 'DINGTALK_SECRET', placeholder: 'SEC...', type: 'password' },
      { label: t('settings.fields.wecomWebhook'), key: 'WECOM_WEBHOOK', placeholder: 'https://qyapi.weixin.qq.com/...', testChannel: 'WeCom' },
      { label: t('settings.fields.feishuWebhook'), key: 'FEISHU_WEBHOOK', placeholder: 'https://open.feishu.cn/...', testChannel: 'Feishu' },
    ]},
    { title: t('settings.sections.notifyQQ'), desc: t('settings.sections.notifyQQDesc'), fields: [
      { label: t('settings.fields.qqHost'), key: 'ONEBOT_HOST', placeholder: 'http://localhost:5700', testChannel: 'OneBot' },
      { label: t('settings.fields.qqToken'), key: 'ONEBOT_TOKEN', placeholder: 'Access Token', type: 'password' },
      { label: t('settings.fields.qqUserId'), key: 'ONEBOT_USER_ID', placeholder: '10001' },
      { label: t('settings.fields.qqGroupId'), key: 'ONEBOT_GROUP_ID', placeholder: '20002' },
    ]}
  ]},
  { key: 'plugins', label: t('settings.tabs.plugins'), icon: Sparkles, sections: [] },
  { key: 'backup', label: t('settings.tabs.backup'), icon: Database, sections: [
    { title: t('settings.sections.backupCron'), desc: t('settings.sections.backupCronDesc'), fields: [
      { label: t('settings.fields.backupPath'), key: 'BACKUP_PATH', placeholder: './data/backups' },
      { label: t('settings.fields.backupCron'), key: 'BACKUP_CRON', placeholder: '0 0 * * *' },
      { label: t('settings.fields.backupKeepCount'), key: 'BACKUP_KEEP_COUNT', placeholder: '7' },
    ]}
  ]},
  { key: 'logs', label: t('settings.tabs.logs'), icon: FileText, sections: [] },
  { key: 'account', label: t('settings.tabs.account'), icon: Lock, sections: [] },
])

const allFields = computed(() => {
  const m: Record<string, FieldDef> = {}
  for (const tab of tabs.value) {
    for (const section of tab.sections) {
      for (const f of section.fields) m[f.key] = f
    }
  }
  m['USER_AVATAR_URL'] = { label: '管理员头像', key: 'USER_AVATAR_URL', placeholder: '' }
  return m
})

function getVal(key: string): string { return settings.value[key] || '' }
function setVal(key: string, val: string) { settings.value[key] = val }
function isConfigured(key: string): boolean {
  const val = settings.value[key]
  const field = allFields.value[key]
  if (field?.type === 'password') return val !== undefined
  return val !== undefined && val.length > 0
}

function togglePassword(key: string) {
  if (showPasswords.value.has(key)) showPasswords.value.delete(key)
  else showPasswords.value.add(key)
}

function inputType(field: FieldDef): string {
  if (field.type !== 'password') return 'text'
  return showPasswords.value.has(field.key) ? 'text' : 'password'
}

function proxyImage(url: string | undefined): string {
  if (!url) return ''
  if (url.startsWith('/api/') || url.startsWith('data:') || url.includes('api/proxy/image')) return url
  let target = url
  if (url.startsWith('//')) target = 'https:' + url
  return `/api/proxy/image?url=${encodeURIComponent(target)}`
}

// 基础网络请求
async function fetchSettings() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.get('/settings')
    settings.value = (data as Record<string, string>) || {}
  } catch (e: any) {
    error.value = e.response?.data?.error || '加载系统设置失败'
  } finally {
    loading.value = false
  }
}

async function saveAll() {
  error.value = ''
  saved.value = false
  const changed: Record<string, string> = {}
  for (const key of Object.keys(allFields.value)) {
    const val = settings.value[key]
    const field = allFields.value[key]
    if (field.type === 'password' && (val === '' || val === undefined)) {
      continue
    }
    if (val !== undefined && val !== '') {
      changed[key] = val
    }
  }
  try {
    await request.put('/settings', { settings: changed })
    saved.value = true
    setTimeout(() => { saved.value = false }, 3000)
  } catch (e: any) {
    error.value = e.response?.data?.error || '保存设置失败'
  }
}

// Bangumi 授权
async function connectBangumi() {
  const newTab = window.open('', '_blank')
  try {
    const { data } = await request.get("/bangumi/auth/link")
    if (data.url) {
      if (newTab) newTab.location.href = data.url
      else window.open(data.url, '_blank')
    } else {
      if (newTab) newTab.close()
      error.value = "授权链接为空"
    }
  } catch (e: any) {
    if (newTab) newTab.close()
    error.value = e.response?.data?.error || "无法获取 Bangumi 授权链接"
  }
}

// 测速
async function testMirrors() {
  mirrorTesting.value = true
  mirrorResults.value = []
  try {
    const endpoint = activeTab.value === 'bangumi' ? '/bgm/test-mirrors' : '/mikan/test-mirrors'
    const { data } = await request.post(endpoint, {}, { timeout: 15000 })
    mirrorResults.value = data || []
  } catch (e: any) {
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
    saved.value = true
    setTimeout(() => { saved.value = false }, 2000)
  } catch (e: any) {
    error.value = '切换节点失败'
  }
}

// AI 模型拉取
async function fetchAIModels() {
  const protocol = getVal('AI_PROTOCOL')
  const endpoint = getVal('AI_ENDPOINT')
  const apiKey = getVal('AI_API_KEY')
  if (!endpoint) {
    error.value = '请先填写 API 端点地址'
    return
  }
  modelLoading.value = true
  modelOptions.value = []
  try {
    const { data } = await request.post('/ai/models', { protocol, endpoint, apiKey }, { timeout: 15000 })
    if (data.success && data.models && data.models.length > 0) {
      modelOptions.value = data.models.map((m: string) => ({ label: m, value: m }))
      if (!getVal('AI_MODEL')) setVal('AI_MODEL', data.models[0])
      saved.value = true
      setTimeout(() => { saved.value = false }, 2000)
    } else {
      error.value = data.error || '未获取到可用模型'
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || '拉取模型列表失败'
  } finally {
    modelLoading.value = false
  }
}

// 发送通知自检
async function sendTestNotify(channel: string) {
  try {
    const { data } = await request.post('/notify/test', {
      channel,
      title: 'Ani-Go 联通测试',
      message: `来自 ${channel} 的自检通知，收到说明配置成功。`
    })
    if (data.success) {
      saved.value = true
      setTimeout(() => { saved.value = false }, 3000)
    } else {
      error.value = data.error || '测试发送失败'
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || '发送测试请求失败'
  }
}

// 插件管理逻辑
async function fetchPlugins() {
  pluginLoading.value = true
  try {
    const { data } = await request.get('/plugins')
    pluginList.value = data || []
  } catch (e: any) {
    error.value = '获取插件列表失败'
  } finally {
    pluginLoading.value = false
  }
}

async function reloadPlugins() {
  pluginLoading.value = true
  try {
    await request.post('/plugins/reload')
    await fetchPlugins()
    saved.value = true
    setTimeout(() => { saved.value = false }, 2500)
  } catch (e: any) {
    error.value = '重新加载插件失败'
  } finally {
    pluginLoading.value = false
  }
}

async function togglePlugin(p: any) {
  const targetState = !p.enabled
  try {
    await request.post('/plugins/toggle', { id: p.id, enabled: targetState })
    p.enabled = targetState
    saved.value = true
    setTimeout(() => { saved.value = false }, 2000)
  } catch (e: any) {
    error.value = e.response?.data?.error || '切换插件状态失败'
  }
}

async function deletePlugin(id: string) {
  if (!confirm('确定要删除此自定义插件吗？')) return
  try {
    await request.delete(`/plugins/${id}`)
    await fetchPlugins()
    saved.value = true
    setTimeout(() => { saved.value = false }, 2000)
  } catch (e: any) {
    error.value = e.response?.data?.error || '删除插件失败'
  }
}

function openAddPluginModal() {
  pluginForm.value = {
    id: '',
    name: '',
    description: '',
    version: '1.0.0',
    author: 'User',
    author_url: '',
    type: 'webhook',
    url: '',
    secret: '',
    events: ['download.completed', 'file.organized']
  }
  pluginJsonText.value = JSON.stringify({
    name: "示例 Webhook 插件",
    description: "接收 Ani-Go 下载与整理事件",
    version: "1.0.0",
    type: "webhook",
    url: "https://your-server.com/webhook",
    events: ["download.completed", "file.organized"]
  }, null, 2)
  pluginJsonError.value = ''
  showPluginModal.value = true
}

async function submitPluginForm() {
  pluginSaving.value = true
  pluginJsonError.value = ''
  try {
    if (pluginModalTab.value === 'webhook') {
      if (!pluginForm.value.name.trim()) throw new Error('插件名称不能为空')
      if (!pluginForm.value.url.trim()) throw new Error('Webhook URL 不能为空')
      if (pluginForm.value.events.length === 0) throw new Error('请至少选择一个监听事件')
      await request.post('/plugins/save', pluginForm.value)
    } else {
      let parsed: any
      try {
        parsed = JSON.parse(pluginJsonText.value)
      } catch (err: any) {
        throw new Error('JSON 格式错误: ' + err.message)
      }
      if (Array.isArray(parsed)) {
        for (const item of parsed) await request.post('/plugins/save', item)
      } else {
        await request.post('/plugins/save', parsed)
      }
    }
    showPluginModal.value = false
    await fetchPlugins()
    saved.value = true
    setTimeout(() => { saved.value = false }, 2500)
  } catch (e: any) {
    pluginJsonError.value = e.message || e.response?.data?.error || '保存插件失败'
  } finally {
    pluginSaving.value = false
  }
}

function exportPluginsJSON() {
  const customOnly = pluginList.value.filter(p => !p.is_builtin)
  const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(customOnly.length ? customOnly : pluginList.value, null, 2))
  const dlAnchor = document.createElement('a')
  dlAnchor.setAttribute("href", dataStr)
  dlAnchor.setAttribute("download", `anigo-plugins-${new Date().toISOString().slice(0, 10)}.json`)
  dlAnchor.click()
}

function handlePluginFileImport(e: any) {
  const file = e.target.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = (evt) => {
    try {
      pluginJsonText.value = evt.target?.result as string
      pluginModalTab.value = 'json'
    } catch (err) {
      pluginJsonError.value = '读取文件失败'
    }
  }
  reader.readAsText(file)
}

// 备份逻辑
async function fetchBackupList() {
  backupLoading.value = true
  try {
    const { data } = await request.get('/backup/list')
    backupList.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || '获取备份列表失败'
  } finally {
    backupLoading.value = false
  }
}

async function createBackup() {
  creatingBackup.value = true
  try {
    await request.post('/backup/create', { include_episodes: showBackupEpisodes.value })
    saved.value = true
    setTimeout(() => { saved.value = false }, 2500)
    await fetchBackupList()
  } catch (e: any) {
    error.value = e.response?.data?.error || '创建备份失败'
  } finally {
    creatingBackup.value = false
  }
}

async function restoreBackup(name: string) {
  if (!confirm(`确定要从备份 "${name}" 恢复吗？这将覆盖当前设置和订阅数据。建议恢复后重启服务。`)) return
  restoringBackup.value = true
  try {
    await request.post('/backup/restore', { name })
    alert('恢复成功，建议重启服务生效')
  } catch (e: any) {
    error.value = e.response?.data?.error || '恢复备份失败'
  } finally {
    restoringBackup.value = false
  }
}

async function deleteBackup(name: string) {
  if (!confirm(`确定要删除备份 "${name}" 吗？此操作不可撤销。`)) return
  deletingBackup.value = name
  try {
    await request.delete(`/backup/${name}`)
    await fetchBackupList()
  } catch (e: any) {
    error.value = e.response?.data?.error || '删除备份失败'
  } finally {
    deletingBackup.value = null
  }
}

async function downloadBackup(name: string) {
  try {
    const response = await fetch(`/api/backup/download/${name}`, {
      headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
    })
    if (!response.ok) throw new Error('下载失败')
    const blob = await response.blob()
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = name
    a.click()
    window.URL.revokeObjectURL(url)
  } catch (e: any) {
    error.value = e.message || '下载备份失败'
  }
}

function formatBackupSize(bytes: number): string {
  if (bytes > 1e6) return (bytes / 1e6).toFixed(1) + ' MB'
  if (bytes > 1e3) return (bytes / 1e3).toFixed(0) + ' KB'
  return bytes + ' B'
}

function formatBackupTime(timeStr: string): string {
  return new Date(timeStr).toLocaleString('zh-CN', { 
    year: 'numeric', month: 'short', day: 'numeric', 
    hour: '2-digit', minute: '2-digit' 
  })
}

// 系统日志逻辑
async function fetchLogs() {
  logLoading.value = true
  try {
    const { data } = await request.get('/logs?lines=300')
    if (data && Array.isArray(data.lines)) {
      logs.value = data.lines
    } else if (Array.isArray(data)) {
      logs.value = data
    } else {
      logs.value = []
    }
    if (autoScrollBottom.value) {
      setTimeout(() => {
        if (logContainerRef.value) {
          logContainerRef.value.scrollTop = logContainerRef.value.scrollHeight
        }
      }, 50)
    }
  } catch (e: any) {
    console.error('获取系统日志失败:', e)
  } finally {
    logLoading.value = false
  }
}

function copyAllLogs() {
  const content = (Array.isArray(logs.value) ? logs.value : []).join('\n')
  navigator.clipboard.writeText(content)
  saved.value = true
  setTimeout(() => { saved.value = false }, 2000)
}

watch(autoRefreshLogs, (val) => {
  if (val) {
    logTimer = setInterval(() => fetchLogs(), 3000)
  } else if (logTimer) {
    clearInterval(logTimer)
    logTimer = null
  }
})

// 头像上传逻辑
function triggerAvatarSelect() {
  avatarFileInput.value?.click()
}

async function handleAvatarFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return

  if (file.size > 5 * 1024 * 1024) {
    avatarError.value = '图片大小不能超过 5MB'
    return
  }

  avatarUploading.value = true
  avatarError.value = ''
  avatarSuccess.value = ''

  const formData = new FormData()
  formData.append('avatar', file)

  try {
    const { data } = await request.post('/user/avatar', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
    if (data.avatar_url) {
      setVal('USER_AVATAR_URL', data.avatar_url)
      avatarSuccess.value = '头像上传成功！已实时生效'
      setTimeout(() => { avatarSuccess.value = '' }, 3000)
      window.dispatchEvent(new CustomEvent('avatar-updated', { detail: data.avatar_url }))
    }
  } catch (err: any) {
    avatarError.value = err?.response?.data?.error || '头像上传失败'
  } finally {
    avatarUploading.value = false
    if (target) target.value = ''
  }
}

async function saveAvatarUrl() {
  const url = getVal('USER_AVATAR_URL')
  if (!url) return
  try {
    await request.put('/settings', { settings: { USER_AVATAR_URL: url } })
    avatarSuccess.value = '头像链接已应用！'
    setTimeout(() => { avatarSuccess.value = '' }, 3000)
    window.dispatchEvent(new CustomEvent('avatar-updated', { detail: url }))
  } catch (err: any) {
    avatarError.value = err?.response?.data?.error || '保存头像链接失败'
  }
}

async function clearAvatar() {
  setVal('USER_AVATAR_URL', '')
  try {
    await request.put('/settings', { settings: { USER_AVATAR_URL: '' } })
    window.dispatchEvent(new CustomEvent('avatar-updated', { detail: '' }))
    avatarSuccess.value = '头像已重置为默认'
    setTimeout(() => { avatarSuccess.value = '' }, 2500)
  } catch (err: any) {
    avatarError.value = '重置头像失败'
  }
}

// 修改密码
async function changePassword() {
  passwordMsg.value = ''
  passwordError.value = ''
  if (newPassword.value.length < 6) { passwordError.value = '新密码不能少于6位'; return }
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

// 快捷键 Ctrl+S / Cmd+S
function handleGlobalKeydown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    saveAll()
  }
}

watch(activeTab, (newTab) => {
  if (newTab === 'logs') {
    fetchLogs()
  }
})

onMounted(() => {
  fetchSettings()
  fetchPlugins()
  fetchBackupList()
  fetchLogs()
  window.addEventListener('keydown', handleGlobalKeydown)
  window.addEventListener('focus', () => {
    if (activeTab.value === 'bangumi') fetchSettings()
  })
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  if (logTimer) clearInterval(logTimer)
})
</script>

<template>
  <div class="space-y-6 pb-24 max-w-7xl mx-auto animate-in fade-in duration-300">
    
    <!-- 顶部标题与保存操作栏 -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-base-100 p-6 sm:p-7 rounded-3xl border border-base-200/80 shadow-sm">
      <div class="space-y-1">
        <div class="flex items-center gap-3">
          <div class="w-10 h-10 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shadow-inner">
            <Settings :size="20" />
          </div>
          <h1 class="text-2xl sm:text-3xl font-black tracking-tight italic">{{ $t('settings.title') }}</h1>
          <span class="badge badge-neutral text-xs font-mono font-bold">{{ currentVersion }}</span>
        </div>
        <p class="text-xs text-base-content/60 font-medium">{{ $t('settings.subtitle') }}</p>
      </div>

      <div class="flex items-center gap-3">
        <button 
          @click="saveAll" 
          class="btn btn-primary rounded-xl px-7 gap-2 shadow-lg shadow-primary/25 hover:scale-[1.02] active:scale-95 transition-all"
          :disabled="loading"
        >
          <Check :size="18" />
          <span class="text-xs font-black uppercase tracking-wider">{{ $t('settings.saveAll') }}</span>
          <kbd class="hidden md:inline-block kbd kbd-xs bg-primary-content/20 text-primary-content font-mono border-0 ml-1">{{ $t('settings.ctrlS') }}</kbd>
        </button>
      </div>
    </div>

    <!-- 顶部状态提示条 -->
    <Transition name="fade">
      <div v-if="saved" class="alert bg-success/15 border border-success/30 text-success rounded-2xl p-4 shadow-sm flex items-center justify-between">
        <div class="flex items-center gap-3">
          <Check :size="18" class="shrink-0" />
          <div>
            <h4 class="font-black text-xs">{{ $t('settings.updateSuccess') }}</h4>
            <p class="text-[11px] opacity-80">{{ $t('settings.updateSuccessDesc') }}</p>
          </div>
        </div>
        <button @click="saved = false" class="btn btn-ghost btn-xs btn-circle">✕</button>
      </div>
    </Transition>
    <Transition name="fade">
      <div v-if="error" class="alert bg-error/15 border border-error/30 text-error rounded-2xl p-4 shadow-sm flex items-center justify-between">
        <div class="flex items-center gap-3">
          <Shield :size="18" class="shrink-0" />
          <div>
            <h4 class="font-black text-xs">{{ $t('settings.errorTitle') }}</h4>
            <p class="text-[11px] opacity-80">{{ error }}</p>
          </div>
        </div>
        <button @click="error = ''" class="btn btn-ghost btn-xs btn-circle">✕</button>
      </div>
    </Transition>

    <!-- 主布局：左侧导航 + 右侧内容面板 -->
    <div v-if="loading" class="flex justify-center py-32">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <div v-else class="flex flex-col lg:flex-row gap-6 items-start w-full">
      
      <!-- 移动端与平板端专属：横向滑动切换标签栏 (Mobile & Tablet Horizontal Tab Strip) -->
      <div class="lg:hidden w-full overflow-x-auto no-scrollbar py-1 flex items-center gap-2 bg-base-100/90 backdrop-blur-md p-2 rounded-2xl border border-base-200/80 sticky top-16 z-20 shadow-sm">
        <button
          v-for="tab in allFlatTabs"
          :key="tab.key"
          @click="activeTab = tab.key"
          class="px-3.5 py-2 rounded-xl text-xs font-bold transition-all whitespace-nowrap flex items-center gap-1.5 shrink-0"
          :class="activeTab === tab.key 
            ? 'bg-primary text-primary-content shadow-sm font-black' 
            : 'bg-base-200/60 text-base-content/60 hover:text-base-content hover:bg-base-200'"
        >
          <component :is="tab.icon" :size="14" />
          <span>{{ tab.label }}</span>
        </button>
      </div>

      <!-- 左侧分类侧边栏 Navigation Sidebar (桌面端常驻) -->
      <aside class="hidden lg:block w-64 shrink-0 lg:sticky lg:top-24 space-y-4 bg-base-100 p-4 rounded-3xl border border-base-200/80 shadow-sm">
        <div v-for="group in tabGroups" :key="group.name" class="space-y-1">
          <div class="px-3 py-1 text-[10px] font-black uppercase tracking-wider text-base-content/40">
            {{ group.name }}
          </div>
          <button
            v-for="tab in group.tabs"
            :key="tab.key"
            @click="activeTab = tab.key"
            class="w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl text-xs font-bold transition-all group"
            :class="activeTab === tab.key 
              ? 'bg-primary text-primary-content shadow-md shadow-primary/20 font-black' 
              : 'text-base-content/70 hover:text-base-content hover:bg-base-200/60'"
          >
            <div class="flex items-center gap-2.5">
              <component :is="tab.icon" :size="16" class="group-hover:scale-110 transition-transform" />
              <span>{{ tab.label }}</span>
            </div>
            <span v-if="activeTab === tab.key" class="w-1.5 h-1.5 rounded-full bg-primary-content"></span>
          </button>
        </div>
      </aside>

      <!-- 右侧配置主面板 Right Content Area -->
      <main class="flex-1 min-w-0 w-full space-y-6">

        <!-- 1. Mikan 镜像测速卡片 (在 Mikan 或 Bangumi Tab 顶部展示) -->
        <div v-if="activeTab === 'mikan' || activeTab === 'bangumi'" class="w-full bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-5 block">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-xl bg-primary/10 text-primary flex items-center justify-center">
                <Timer :size="20" />
              </div>
              <div>
                <h3 class="text-base font-black tracking-tight">{{ $t('settings.mikan.mirrorAudit') }}</h3>
                <p class="text-[11px] opacity-50">{{ $t('settings.mikan.mirrorAuditDesc') }}</p>
              </div>
            </div>
            <button 
              @click="testMirrors" 
              :disabled="mirrorTesting"
              class="btn btn-primary btn-sm rounded-xl px-5 gap-2"
            >
              <RefreshCw v-if="mirrorTesting" :size="14" class="animate-spin" />
              <Timer v-else :size="14" />
              <span class="text-xs font-bold">{{ mirrorTesting ? $t('settings.mikan.running') : $t('settings.mikan.runDiagnostics') }}</span>
            </button>
          </div>

          <!-- 测速结果列表 -->
          <div v-if="mirrorResults.length > 0" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2">
            <div 
              v-for="r in mirrorResults" 
              :key="r.domain"
              @click="r.ok && selectMirror(r.domain)"
              class="flex items-center justify-between p-3.5 rounded-2xl border transition-all"
              :class="r.ok 
                ? 'bg-base-200/30 border-base-300 hover:border-primary/50 cursor-pointer active:scale-95' 
                : 'bg-error/5 border-error/20 opacity-60 cursor-not-allowed'"
            >
              <div class="space-y-0.5">
                <div class="flex items-center gap-2">
                  <span class="w-2 h-2 rounded-full" :class="r.ok ? (r.latency_ms < 500 ? 'bg-success' : 'bg-warning') : 'bg-error'"></span>
                  <span class="text-xs font-mono font-bold">{{ r.domain }}</span>
                </div>
                <span v-if="(activeTab === 'bangumi' ? getVal('BGMTV_DOMAIN') : getVal('MIKAN_DOMAIN')) === r.domain" class="text-[10px] font-bold text-primary block pl-4">
                  {{ $t('settings.mikan.currentRoute') }}
                </span>
              </div>
              <div class="text-right">
                <span v-if="r.ok" class="text-sm font-mono font-black" :class="r.latency_ms < 500 ? 'text-success' : 'text-warning'">
                  {{ r.latency_ms }}<span class="text-[10px] opacity-60">ms</span>
                </span>
                <span v-else class="text-xs text-error font-bold">{{ $t('settings.mikan.unreachable') }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 2. 连接 Bangumi 账号 独立全宽操作卡片 -->
        <div v-if="activeTab === 'bangumi'" class="w-full bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-4 block">
          <div class="w-full flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
            <div class="flex items-center gap-4 flex-1 min-w-0">
              <div class="w-12 h-12 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
                <Antenna :size="24" class="shrink-0" />
              </div>
              <div class="flex-1 min-w-0 space-y-1 text-left">
                <div class="flex items-center gap-2.5 flex-wrap">
                  <h3 class="text-base font-black tracking-tight whitespace-nowrap break-keep text-left">{{ $t('settings.bangumi.connectTitle') }}</h3>
                  <span v-if="getVal('BGMTV_USERNAME')" class="badge badge-success badge-sm font-bold text-[10px] shrink-0 whitespace-nowrap">
                    {{ $t('settings.bangumi.bound') }}: {{ getVal('BGMTV_USERNAME') }}
                  </span>
                  <span v-else class="badge badge-ghost badge-sm text-[10px] shrink-0 whitespace-nowrap">{{ $t('settings.bangumi.unconnected') }}</span>
                </div>
                <p class="text-xs opacity-60 leading-normal text-left break-words">
                  {{ $t('settings.bangumi.connectDesc') }}
                </p>
              </div>
            </div>

            <button 
              @click="connectBangumi" 
              class="btn btn-primary rounded-xl gap-2 px-6 shadow-md hover:scale-[1.02] active:scale-95 transition-all shrink-0 whitespace-nowrap self-start sm:self-auto"
            >
              <Antenna :size="16" class="shrink-0" />
              <span class="text-xs font-black whitespace-nowrap">{{ $t('settings.bangumi.connectNow') }}</span>
            </button>
          </div>

          <div class="w-full text-xs bg-base-200/50 p-4 rounded-2xl border border-base-300/40 space-y-1.5 text-base-content/75 block text-left">
            <p class="font-bold text-primary">{{ $t('settings.bangumi.tipsTitle') }}</p>
            <p>{{ $t('settings.bangumi.tip1Token') }} <a href="https://next.bgm.tv/demo/access-token" target="_blank" class="link link-primary font-bold">{{ $t('settings.bangumi.tip1Link') }}</a> {{ $t('settings.bangumi.tip1Suffix') }}</p>
            <p>{{ $t('settings.bangumi.tip2OAuth') }} <a href="https://bgm.tv/dev/app" target="_blank" class="link link-primary font-bold">{{ $t('settings.bangumi.tip2Link') }}</a> {{ $t('settings.bangumi.tip2Suffix') }}</p>
          </div>
        </div>

        <!-- 3. 标准化表单渲染区块 (适用常规 Sections，垂直排列) -->
        <div 
          v-for="section in tabs.find(t => t.key === activeTab)?.sections" 
          :key="section.title" 
          class="w-full bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-5 block"
        >
          <div class="space-y-1 text-left">
            <h3 class="text-base font-black tracking-tight whitespace-nowrap break-keep">{{ section.title }}</h3>
            <p class="text-xs opacity-50 leading-normal">{{ section.desc }}</p>
          </div>

          <!-- 表单字段响应式网格 -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4 w-full">
            <div 
              v-for="field in section.fields" 
              :key="field.key" 
              class="space-y-1.5"
              :class="{ 'md:col-span-2': field.placeholder.includes('http') || field.label.includes('列表') || field.label.includes('说明') }"
            >
              <div class="flex items-center justify-between">
                <label class="text-xs font-bold text-base-content/75 flex items-center gap-1.5">
                  <span>{{ field.label }}</span>
                  <span v-if="isConfigured(field.key)" class="w-1.5 h-1.5 rounded-full bg-success"></span>
                </label>
                <span v-if="field.testChannel" class="text-[10px] text-primary cursor-pointer hover:underline font-bold" @click="sendTestNotify(field.testChannel)">
                  {{ $t('settings.notify.sendTest') }}
                </span>
              </div>

              <!-- 下拉选择控件 -->
              <div v-if="field.type === 'select'" class="relative">
                <select 
                  :value="getVal(field.key) || (field.key === 'AI_MODEL' ? (modelOptions[0]?.value || '') : field.selectOptions?.[0]?.value)"
                  @change="(e: any) => setVal(field.key, e.target.value)"
                  class="select select-bordered w-full rounded-xl text-xs font-semibold focus:border-primary"
                >
                  <option v-for="opt in (field.key === 'AI_MODEL' && modelOptions.length ? modelOptions : field.selectOptions)" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
              </div>

              <!-- AI 模型专用输入+拉取组合控件 -->
              <div v-else-if="field.key === 'AI_MODEL'" class="flex gap-2">
                <input 
                  type="text" 
                  :value="getVal(field.key)" 
                  @input="(e: Event) => setVal(field.key, (e.target as HTMLInputElement).value)"
                  :placeholder="field.placeholder" 
                  class="input input-bordered flex-1 rounded-xl text-xs font-mono font-medium focus:border-primary" 
                />
                <button 
                  @click="fetchAIModels" 
                  :disabled="modelLoading || !getVal('AI_ENDPOINT')"
                  class="btn btn-outline btn-sm rounded-xl gap-1.5 h-10 px-4"
                  :title="$t('settings.ai.fetchTooltip')"
                >
                  <RefreshCw v-if="modelLoading" :size="14" class="animate-spin" />
                  <span class="text-xs font-bold">{{ modelLoading ? $t('settings.ai.loadingModels') : $t('settings.ai.fetchModels') }}</span>
                </button>
              </div>

              <!-- 普通文本与密码输入控件 -->
              <div v-else class="relative">
                <input 
                  :type="inputType(field)" 
                  :value="getVal(field.key)"
                  @input="(e: Event) => setVal(field.key, (e.target as HTMLInputElement).value)"
                  :placeholder="field.placeholder"
                  class="input input-bordered w-full rounded-xl text-xs font-mono font-medium pr-10 focus:border-primary" 
                />
                <button 
                  v-if="field.type === 'password' && getVal(field.key)" 
                  type="button"
                  class="absolute right-3 top-1/2 -translate-y-1/2 opacity-40 hover:opacity-100 transition-opacity"
                  @click="togglePassword(field.key)"
                >
                  <component :is="showPasswords.has(field.key) ? EyeOff : Eye" :size="16" />
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- 4. 插件生态与自动化面板 (Plugins Tab) -->
        <div v-if="activeTab === 'plugins'" class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h3 class="text-base font-black tracking-tight">{{ $t('settings.plugins.title') }}</h3>
              <p class="text-xs opacity-50 mt-0.5">{{ $t('settings.plugins.desc') }}</p>
            </div>
            <div class="flex items-center gap-2 flex-wrap">
              <button class="btn btn-ghost btn-sm rounded-xl gap-1.5 border border-base-300" @click="exportPluginsJSON">
                <Download :size="14" />
                <span class="text-xs font-bold">{{ $t('settings.plugins.exportConfig') }}</span>
              </button>
              <button class="btn btn-ghost btn-sm rounded-xl gap-1.5 border border-base-300" @click="reloadPlugins" :disabled="pluginLoading">
                <RefreshCw :size="14" :class="{ 'animate-spin': pluginLoading }" />
                <span class="text-xs font-bold">{{ $t('settings.plugins.reload') }}</span>
              </button>
              <button class="btn btn-primary btn-sm rounded-xl gap-1.5 shadow-md" @click="openAddPluginModal">
                <Plus :size="14" />
                <span class="text-xs font-black">{{ $t('settings.plugins.add') }}</span>
              </button>
            </div>
          </div>

          <!-- 插件列表网格 -->
          <div v-if="pluginLoading && pluginList.length === 0" class="flex justify-center py-12">
            <span class="loading loading-spinner text-primary"></span>
          </div>
          <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div 
              v-for="plugin in pluginList" 
              :key="plugin.id"
              class="p-5 rounded-2xl border bg-base-200/30 flex flex-col justify-between gap-4 transition-all"
              :class="plugin.enabled ? 'border-primary/30 shadow-sm' : 'border-base-300/40 opacity-70'"
            >
              <div>
                <div class="flex items-start justify-between gap-3 mb-2">
                  <div class="flex items-center gap-3">
                    <div class="w-10 h-10 rounded-xl flex items-center justify-center shadow-inner"
                      :class="plugin.is_builtin ? 'bg-primary/10 text-primary' : 'bg-secondary/10 text-secondary'">
                      <Sparkles v-if="plugin.is_builtin" :size="18" />
                      <Antenna v-else :size="18" />
                    </div>
                    <div>
                      <div class="flex items-center gap-2">
                        <h4 class="font-black text-sm">{{ plugin.name }}</h4>
                        <span class="badge badge-neutral badge-xs font-mono">v{{ plugin.version || '1.0' }}</span>
                      </div>
                      <div class="flex items-center gap-1.5 mt-0.5">
                        <span class="badge badge-xs text-[9px]" :class="plugin.is_builtin ? 'badge-primary' : 'badge-secondary'">
                          {{ plugin.is_builtin ? $t('settings.plugins.builtin') : $t('settings.plugins.webhookType') }}
                        </span>
                        <span v-if="plugin.author" class="text-[10px] opacity-40">by {{ plugin.author }}</span>
                      </div>
                    </div>
                  </div>

                  <!-- 启用切换与删除 -->
                  <div class="flex items-center gap-2">
                    <input 
                      type="checkbox" 
                      class="toggle toggle-primary toggle-sm"
                      :checked="plugin.enabled"
                      @change="togglePlugin(plugin)" 
                      :title="plugin.enabled ? $t('settings.plugins.disableTip') : $t('settings.plugins.enableTip')"
                    />
                    <button 
                      v-if="!plugin.is_builtin" 
                      @click="deletePlugin(plugin.id)" 
                      class="btn btn-ghost btn-xs btn-circle text-error/50 hover:text-error"
                    >
                      <Trash2 :size="13" />
                    </button>
                  </div>
                </div>

                <p class="text-xs text-base-content/70 leading-relaxed font-medium">
                  {{ plugin.description || $t('settings.plugins.noDesc') }}
                </p>
              </div>

              <!-- 底部事件与目标 URL -->
              <div class="space-y-2 pt-2 border-t border-base-content/5 text-xs">
                <div v-if="plugin.url" class="space-y-0.5">
                  <span class="text-[9px] font-black uppercase tracking-wider opacity-40">{{ $t('settings.plugins.targetWebhook') }}</span>
                  <div class="bg-base-300/60 px-2.5 py-1 rounded-lg font-mono text-[11px] truncate select-all">
                    {{ plugin.url }}
                  </div>
                </div>

                <div v-if="plugin.events && plugin.events.length" class="space-y-1">
                  <span class="text-[9px] font-black uppercase tracking-wider opacity-40">{{ $t('settings.plugins.listenEvents') }}</span>
                  <div class="flex flex-wrap gap-1">
                    <span v-for="ev in plugin.events" :key="ev" class="px-2 py-0.5 rounded-md bg-base-300 text-[10px] font-mono font-bold opacity-75">
                      {{ ev }}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 5. 数据归档与恢复 (Backup Tab) -->
        <div v-if="activeTab === 'backup'" class="space-y-6">
          <div class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-4">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div>
                <h3 class="text-base font-black tracking-tight">{{ $t('settings.backup.title') }}</h3>
                <p class="text-xs opacity-50 mt-0.5">{{ $t('settings.backup.desc') }}</p>
              </div>
              <div class="flex items-center gap-3">
                <label class="flex items-center gap-2 cursor-pointer select-none">
                  <input type="checkbox" v-model="showBackupEpisodes" class="checkbox checkbox-primary checkbox-sm rounded-md" />
                  <span class="text-xs font-bold">{{ $t('settings.backup.includeEpisodes') }}</span>
                </label>
                <button 
                  @click="createBackup" 
                  :disabled="creatingBackup"
                  class="btn btn-primary btn-sm rounded-xl px-5 gap-1.5 shadow-md"
                >
                  <Database :size="14" />
                  <span class="text-xs font-bold">{{ creatingBackup ? $t('settings.backup.creating') : $t('settings.backup.createNow') }}</span>
                </button>
              </div>
            </div>

            <!-- 历史备份文件表格 -->
            <div class="overflow-x-auto border border-base-200/80 rounded-2xl">
              <table class="table table-sm w-full">
                <thead>
                  <tr class="bg-base-200/50 text-xs text-base-content/60">
                    <th>{{ $t('settings.backup.tableFile') }}</th>
                    <th>{{ $t('settings.backup.tableSize') }}</th>
                    <th>{{ $t('settings.backup.tableTime') }}</th>
                    <th class="text-right">{{ $t('settings.backup.tableAction') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-if="backupList.length === 0">
                    <td colspan="4" class="text-center py-8 opacity-40 text-xs">{{ $t('settings.backup.empty') }}</td>
                  </tr>
                  <tr v-for="b in backupList" :key="b.name" class="hover:bg-base-200/30 text-xs">
                    <td class="font-mono font-bold">{{ b.name }}</td>
                    <td class="font-mono">{{ formatBackupSize(b.size) }}</td>
                    <td class="opacity-70">{{ formatBackupTime(b.mod_time) }}</td>
                    <td class="text-right">
                      <div class="flex items-center justify-end gap-1.5">
                        <button @click="downloadBackup(b.name)" class="btn btn-ghost btn-xs rounded-lg" :title="$t('settings.backup.download')">
                          <Download :size="13" />
                        </button>
                        <button @click="restoreBackup(b.name)" class="btn btn-ghost btn-xs text-warning rounded-lg" :title="$t('settings.backup.restore')">
                          <RotateCcw :size="13" />
                        </button>
                        <button @click="deleteBackup(b.name)" class="btn btn-ghost btn-xs text-error rounded-lg" :title="$t('settings.backup.delete')">
                          <Trash2 :size="13" />
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <!-- 6. 系统运行日志 (Logs Tab - 独占控制台) -->
        <div v-if="activeTab === 'logs'" class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h3 class="text-base font-black tracking-tight flex items-center gap-2">
                <span>{{ $t('settings.logs.consoleTitle') }}</span>
                <span class="badge badge-neutral badge-xs font-mono">{{ $t('settings.logs.lineCount', { filtered: filteredLogs.length, total: logs.length }) }}</span>
              </h3>
              <p class="text-xs opacity-50 mt-0.5">{{ $t('settings.logs.consoleDesc') }}</p>
            </div>

            <div class="flex items-center gap-2.5 flex-wrap">
              <!-- 实时搜索输入框 -->
              <div class="relative">
                <input 
                  v-model="logFilter" 
                  type="text" 
                  :placeholder="$t('settings.logs.searchPlaceholder')" 
                  class="input input-bordered input-sm rounded-xl pl-8 text-xs font-mono w-44 focus:w-56 transition-all"
                />
                <Search :size="13" class="absolute left-2.5 top-1/2 -translate-y-1/2 opacity-40" />
              </div>

              <!-- 自动轮询开关 -->
              <label class="flex items-center gap-1.5 cursor-pointer text-xs font-bold border border-base-300/60 px-3 py-1.5 rounded-xl hover:bg-base-200/40 transition-colors">
                <input type="checkbox" v-model="autoRefreshLogs" class="checkbox checkbox-primary checkbox-xs rounded" />
                <span>{{ $t('settings.logs.autoRefresh') }}</span>
              </label>

              <!-- 自动滚底开关 -->
              <label class="flex items-center gap-1.5 cursor-pointer text-xs font-bold border border-base-300/60 px-3 py-1.5 rounded-xl hover:bg-base-200/40 transition-colors">
                <input type="checkbox" v-model="autoScrollBottom" class="checkbox checkbox-primary checkbox-xs rounded" />
                <span>锁定底部</span>
              </label>

              <!-- 复制按钮 -->
              <button @click="copyAllLogs" class="btn btn-ghost btn-sm rounded-xl gap-1 border border-base-300/60" title="复制所有日志">
                <Copy :size="13" />
                <span class="text-xs font-bold">复制</span>
              </button>

              <!-- 手动刷新按钮 -->
              <button @click="fetchLogs" :disabled="logLoading" class="btn btn-primary btn-sm rounded-xl gap-1 shadow-sm">
                <RefreshCw :size="13" :class="{ 'animate-spin': logLoading }" />
                <span class="text-xs font-bold">{{ $t('common.refresh') }}</span>
              </button>
            </div>
          </div>

          <!-- 快捷过滤标签栏 -->
          <div class="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs">
            <span class="text-[11px] font-bold opacity-40 mr-1">级别筛选:</span>
            <button 
              v-for="lvl in logLevels" 
              :key="lvl.key"
              @click="currentLogLevel = lvl.key"
              class="btn btn-xs rounded-lg font-bold transition-all"
              :class="currentLogLevel === lvl.key ? 'btn-primary shadow-sm' : 'btn-ghost border border-base-300/60 opacity-70'"
            >
              {{ lvl.label }}
            </button>
          </div>

          <!-- 终端风格滚动窗口 -->
          <div 
            ref="logContainerRef"
            class="bg-[#12161f] text-gray-300 rounded-2xl p-4 font-mono text-[11px] leading-relaxed max-h-[580px] overflow-y-auto space-y-1 select-text border border-white/5 shadow-inner"
          >
            <div 
              v-for="(line, i) in filteredLogs" 
              :key="i"
              class="py-0.5 px-1.5 rounded hover:bg-white/5 transition-colors break-all font-mono"
              :class="{
                'text-rose-400 font-bold bg-rose-500/10': line.includes('❌') || line.includes('ERROR') || line.includes('error') || line.includes('failed'),
                'text-amber-300 font-bold bg-amber-500/10': line.includes('⚠️') || line.includes('WARN') || line.includes('warn'),
                'text-emerald-400': line.includes('✅') || line.includes('SUCCESS') || line.includes('success'),
                'text-cyan-400': line.includes('🔌') || line.includes('🚀') || line.includes('🔗')
              }"
            >
              {{ line }}
            </div>
            <div v-if="filteredLogs.length === 0" class="text-center py-16 opacity-30 text-xs flex flex-col items-center gap-2">
              <FileText :size="24" />
              <span>{{ logFilter || currentLogLevel !== 'all' ? '未找到符合筛选条件的日志' : (logLoading ? '正在加载日志...' : '暂无运行日志') }}</span>
            </div>
          </div>
        </div>

        <!-- 7. 账户安全 (Account Tab) -->
        <div v-if="activeTab === 'account'" class="space-y-6">
          
          <!-- 管理员头像管理卡片 -->
          <div class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-6">
            <div>
              <h3 class="text-base font-black tracking-tight">{{ $t('settings.account.avatarTitle') }}</h3>
              <p class="text-xs opacity-50 mt-0.5">{{ $t('settings.account.avatarDesc') }}</p>
            </div>

            <div class="flex flex-col sm:flex-row items-center sm:items-start gap-6 p-5 rounded-2xl bg-base-200/30 border border-base-200/60">
              <!-- 头像圆形预览与悬浮更换 -->
              <div class="relative group cursor-pointer shrink-0" @click="triggerAvatarSelect" title="点击更换头像">
                <div class="w-24 h-24 rounded-full overflow-hidden border-2 border-primary/20 shadow-md bg-base-100 flex items-center justify-center">
                  <img 
                    v-if="getVal('USER_AVATAR_URL')" 
                    :src="proxyImage(getVal('USER_AVATAR_URL'))" 
                    alt="Avatar" 
                    class="w-full h-full object-cover"
                  />
                  <div v-else class="w-full h-full bg-primary/10 flex items-center justify-center text-primary">
                    <User :size="40" />
                  </div>
                </div>
                <!-- 悬浮蒙层 -->
                <div class="absolute inset-0 rounded-full bg-black/50 text-white opacity-0 group-hover:opacity-100 flex flex-col items-center justify-center transition-all duration-200 text-[11px] font-bold gap-1 shadow-inner">
                  <Camera :size="22" />
                  <span>更换图片</span>
                </div>
              </div>

              <!-- 上传按钮与配置输入区 -->
              <div class="space-y-4 flex-1 w-full">
                <div class="flex items-center gap-3 flex-wrap">
                  <input 
                    ref="avatarFileInput" 
                    type="file" 
                    accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml" 
                    class="hidden" 
                    @change="handleAvatarFileChange" 
                  />
                  <button 
                    type="button" 
                    @click="triggerAvatarSelect" 
                    :disabled="avatarUploading"
                    class="btn btn-primary btn-sm rounded-xl gap-2 shadow-md shadow-primary/20"
                  >
                    <RefreshCw v-if="avatarUploading" :size="14" class="animate-spin" />
                    <Upload v-else :size="14" />
                    <span class="text-xs font-bold">{{ avatarUploading ? $t('common.loading') : $t('settings.account.upload') }}</span>
                  </button>
                  <button 
                    v-if="getVal('USER_AVATAR_URL')" 
                    type="button" 
                    @click="clearAvatar" 
                    class="btn btn-ghost btn-sm rounded-xl text-error/70 hover:text-error text-xs font-bold gap-1"
                  >
                    <Trash2 :size="13" />
                    <span>{{ $t('settings.account.resetAvatar') }}</span>
                  </button>
                  <span class="text-[11px] opacity-40">支持 JPG, PNG, WebP, GIF, SVG，最大 5MB</span>
                </div>

                <div class="space-y-1.5">
                  <label class="text-[11px] font-black uppercase tracking-wider opacity-60">{{ $t('settings.account.avatarUrlLabel') }}</label>
                  <div class="relative">
                    <input 
                      type="text" 
                      :value="getVal('USER_AVATAR_URL')" 
                      @input="(e: Event) => setVal('USER_AVATAR_URL', (e.target as HTMLInputElement).value)"
                      placeholder="https://example.com/avatar.png" 
                      class="input input-bordered input-sm w-full rounded-xl text-xs font-mono pr-20" 
                    />
                    <button 
                      type="button" 
                      @click="saveAvatarUrl"
                      class="absolute right-1 top-1/2 -translate-y-1/2 btn btn-ghost btn-xs rounded-lg text-primary font-bold hover:bg-primary/10"
                    >
                      应用链接
                    </button>
                  </div>
                </div>

                <p v-if="avatarError" class="text-xs text-error font-bold bg-error/10 p-3 rounded-xl border border-error/20">{{ avatarError }}</p>
                <p v-if="avatarSuccess" class="text-xs text-success font-bold bg-success/10 p-3 rounded-xl border border-success/20">{{ avatarSuccess }}</p>
              </div>
            </div>
          </div>

          <!-- 修改管理员密码卡片 -->
          <div class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-5">
            <div>
              <h3 class="text-base font-black tracking-tight">{{ $t('settings.account.title') }}</h3>
              <p class="text-xs opacity-50 mt-0.5">{{ $t('settings.account.desc') }}</p>
            </div>

            <div class="max-w-md space-y-4">
              <div class="space-y-1">
                <label class="text-xs font-bold text-base-content/75">{{ $t('settings.account.oldPass') }}</label>
                <input v-model="oldPassword" type="password" :placeholder="$t('settings.account.oldPassPlaceholder')" class="input input-bordered w-full rounded-xl text-xs" />
              </div>
              <div class="space-y-1">
                <label class="text-xs font-bold text-base-content/75">{{ $t('settings.account.newPass') }}</label>
                <input v-model="newPassword" type="password" :placeholder="$t('settings.account.newPassPlaceholder')" class="input input-bordered w-full rounded-xl text-xs" />
              </div>
              <div class="space-y-1">
                <label class="text-xs font-bold text-base-content/75">{{ $t('settings.account.confirmPass') }}</label>
                <input v-model="confirmPassword" type="password" :placeholder="$t('settings.account.confirmPassPlaceholder')" class="input input-bordered w-full rounded-xl text-xs" />
              </div>

              <div class="pt-2">
                <button 
                  @click="changePassword" 
                  :disabled="changingPassword"
                  class="btn btn-primary rounded-xl px-7 gap-2 shadow-md"
                >
                  <Lock :size="14" />
                  <span class="text-xs font-black">{{ changingPassword ? $t('settings.account.submitting') : $t('settings.account.submit') }}</span>
                </button>
              </div>

              <p v-if="passwordMsg" class="text-success text-xs font-bold bg-success/10 p-3 rounded-xl border border-success/20">{{ passwordMsg }}</p>
              <p v-if="passwordError" class="text-error text-xs font-bold bg-error/10 p-3 rounded-xl border border-error/20">{{ passwordError }}</p>
            </div>
          </div>

        </div>

      </main>
    </div>

    <!-- 导入 / 添加插件弹窗 Modal -->
    <div v-if="showPluginModal" class="modal modal-open z-50 animate-in fade-in duration-200">
      <div class="modal-box max-w-2xl bg-base-100 rounded-3xl p-6 sm:p-7 border border-base-200/80 shadow-2xl space-y-5">
        <div class="flex items-center justify-between">
          <div class="space-y-0.5">
            <h3 class="text-lg font-black tracking-tight flex items-center gap-2">
              <Sparkles class="text-primary" :size="20" />
              添加 / 导入插件扩展
            </h3>
            <p class="text-xs opacity-50">支持快速创建 Webhook 推送或直接导入 JSON 规则包</p>
          </div>
          <button @click="showPluginModal = false" class="btn btn-ghost btn-sm btn-circle">✕</button>
        </div>

        <!-- 模式切换标签 -->
        <div class="flex rounded-2xl bg-base-200 p-1">
          <button class="flex-1 py-1.5 rounded-xl text-xs font-black transition-all"
            :class="pluginModalTab === 'webhook' ? 'bg-primary text-primary-content shadow-sm' : 'opacity-60 hover:opacity-100'"
            @click="pluginModalTab = 'webhook'">
            快捷创建 Webhook
          </button>
          <button class="flex-1 py-1.5 rounded-xl text-xs font-black transition-all"
            :class="pluginModalTab === 'json' ? 'bg-primary text-primary-content shadow-sm' : 'opacity-60 hover:opacity-100'"
            @click="pluginModalTab = 'json'">
            JSON 规则导入
          </button>
        </div>

        <!-- Tab 1: Webhook 表单 -->
        <div v-if="pluginModalTab === 'webhook'" class="space-y-3.5">
          <div class="space-y-1">
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60">插件名称 *</label>
            <input v-model="pluginForm.name" type="text" placeholder="例如：Discord 频道推送 / n8n 自动化" class="input input-bordered w-full rounded-xl text-xs" />
          </div>

          <div class="space-y-1">
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60">目标 Webhook URL *</label>
            <input v-model="pluginForm.url" type="url" placeholder="https://discord.com/api/webhooks/... 或 http://localhost:5678/webhook/..." class="input input-bordered w-full rounded-xl font-mono text-xs" />
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div class="space-y-1">
              <label class="text-[11px] font-black uppercase tracking-wider opacity-60">签名密钥 Secret (可选)</label>
              <input v-model="pluginForm.secret" type="password" placeholder="请求头 X-AniGo-Secret" class="input input-bordered w-full rounded-xl text-xs font-mono" />
            </div>
            <div class="space-y-1">
              <label class="text-[11px] font-black uppercase tracking-wider opacity-60">功能简述 (可选)</label>
              <input v-model="pluginForm.description" type="text" placeholder="简要说明此插件功能" class="input input-bordered w-full rounded-xl text-xs" />
            </div>
          </div>

          <div class="space-y-1.5">
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60">监听触发事件 * (多选)</label>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 bg-base-200/50 p-3.5 rounded-2xl border border-base-300/40">
              <label v-for="ev in availablePluginEvents" :key="ev.id" class="flex items-center gap-2 cursor-pointer hover:opacity-100 opacity-80 py-0.5">
                <input type="checkbox" :value="ev.id" v-model="pluginForm.events" class="checkbox checkbox-primary checkbox-xs rounded" />
                <span class="text-xs select-none">{{ ev.label }}</span>
              </label>
            </div>
          </div>
        </div>

        <!-- Tab 2: JSON 导入 -->
        <div v-else class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60">粘贴 JSON 规则或上传文件</label>
            <label class="btn btn-xs btn-outline rounded-xl gap-1 cursor-pointer">
              <Upload :size="12" />
              <span>选择 .json 文件</span>
              <input type="file" accept=".json,application/json" class="hidden" @change="handlePluginFileImport" />
            </label>
          </div>
          <textarea v-model="pluginJsonText" rows="9" class="textarea textarea-bordered w-full rounded-xl font-mono text-xs leading-relaxed" placeholder="在此粘贴 JSON 格式的插件定义"></textarea>
        </div>

        <p v-if="pluginJsonError" class="text-xs text-error font-bold bg-error/10 p-3 rounded-xl border border-error/20">{{ pluginJsonError }}</p>

        <div class="modal-action flex justify-end gap-2.5 pt-2">
          <button @click="showPluginModal = false" class="btn btn-ghost btn-sm rounded-xl px-5">取消</button>
          <button @click="submitPluginForm" :disabled="pluginSaving" class="btn btn-primary btn-sm rounded-xl px-7 shadow-md">
            <span v-if="pluginSaving" class="loading loading-spinner loading-xs"></span>
            <span v-else>确认导入</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 底部专属版本与作者信息卡片 -->
    <div class="flex flex-col items-center justify-center gap-2 pt-6 text-center opacity-70 hover:opacity-100 transition-opacity">
      <div class="text-xs font-semibold text-base-content/80 flex items-center justify-center gap-1.5 flex-wrap">
        <span>Ani-Go &copy; 2026 • 倾心打造</span>
        <span class="opacity-40">•</span>
        <span>by <a href="https://github.com/xiaoyueRX" target="_blank" rel="noopener noreferrer" class="text-primary font-bold hover:underline">xiaoyue</a></span>
      </div>
      <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" 
         class="px-4 py-1.5 rounded-full bg-base-200/60 border border-base-300/60 text-[11px] font-mono font-bold text-base-content/70 hover:text-primary hover:border-primary/40 transition-all flex items-center gap-2 shadow-sm">
        <svg class="w-3.5 h-3.5 fill-current" viewBox="0 0 24 24"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>
        <span>GitHub: xiaoyueRX/Ani-Go</span>
        <span class="opacity-40">•</span>
        <span>{{ currentVersion }}</span>
      </a>
    </div>

  </div>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 0.3s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
