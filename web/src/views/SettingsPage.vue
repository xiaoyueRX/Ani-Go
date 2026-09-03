<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  Check, Antenna, Download, 
  Folder, Bell, Cpu, 
  Settings, Timer, Lock, 
  FileText, Eye, EyeOff,
  Loader2,
  ChevronDown,
  RefreshCw,
  User,
  Database,
  RotateCcw,
  Upload,
  HardDrive,
  Trash2,
  ArrowDown,
  Shield,
  Sparkles,
  Plus
} from 'lucide-vue-next'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const settings = ref<Record<string, string>>({})
const loading = ref(true)
const error = ref('')
const saved = ref(false)
const activeTab = ref(route.query.tab ? String(route.query.tab) : 'mikan')
const showPasswords = ref<Set<string>>(new Set())

// 镜像测速
const mirrorTesting = ref(false)
const mirrorResults = ref<{ domain: string; latency_ms: number; ok: boolean }[]>([])
const selectedMirror = ref('')

const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const changingPassword = ref(false)
const passwordMsg = ref('')
const passwordError = ref('')

// 备份管理状态
const backupLoading = ref(false)
const backupList = ref<{ name: string; size: number; mod_time: string }[]>([])
const creatingBackup = ref(false)
const restoringBackup = ref(false)
const deletingBackup = ref<string | null>(null)
const showBackupEpisodes = ref(false)

// 插件管理状态
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
    setTimeout(() => { saved.value = false }, 3000)
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
    setTimeout(() => { saved.value = false }, 2500)
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
    setTimeout(() => { saved.value = false }, 2500)
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
        for (const item of parsed) {
          await request.post('/plugins/save', item)
        }
      } else {
        await request.post('/plugins/save', parsed)
      }
    }
    showPluginModal.value = false
    await fetchPlugins()
    saved.value = true
    setTimeout(() => { saved.value = false }, 3000)
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
      const content = evt.target?.result as string
      pluginJsonText.value = content
      pluginModalTab.value = 'json'
    } catch (err) {
      pluginJsonError.value = '读取文件失败'
    }
  }
  reader.readAsText(file)
}

async function changePassword() {
  passwordMsg.value = ''; passwordError.value = ''
  if (newPassword.value.length < 6) { passwordError.value = '新密码不能少于6位'; return }
  if (newPassword.value !== confirmPassword.value) { passwordError.value = '两次密码不一致'; return }
  changingPassword.value = true
  try {
    await request.post('/user/change-password', { old_password: oldPassword.value, new_password: newPassword.value })
    passwordMsg.value = '密码修改成功，即将重新登录...'
    oldPassword.value = ''; newPassword.value = ''; confirmPassword.value = ''
    localStorage.removeItem('token')
    setTimeout(() => { router.push('/login') }, 1500)
  } catch (e: any) {
    passwordError.value = e?.response?.data?.error || '修改失败'
  } finally { changingPassword.value = false }
}

async function testMirrors() {
  mirrorTesting.value = true
  mirrorResults.value = []
  try {
    const endpoint = activeTab.value === 'bangumi' ? '/bgm/test-mirrors' : '/mikan/test-mirrors'
    const { data } = await request.post(endpoint, {}, { timeout: 15000 })
    mirrorResults.value = data || []
  } catch (e: any) {
    error.value = t('settings.error.testFailed')
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
  } catch (e: any) {
    error.value = t('settings.error.switchFailed')
  }
}

interface FieldDef {
  label: string; key: string; placeholder: string; type?: string; hint?: string; selectOptions?: {label: string, value: string}[]
}

const tabs = computed(() => [
  { key: "bangumi", label: "Bangumi", icon: Antenna, sections: [
    { title: "Bangumi OAuth2", desc: "连接您的 Bangumi 账号以同步收藏数据", fields: [
      { label: "Client ID", key: "BANGUMI_CLIENT_ID", placeholder: "bgm33669389335a4f78e (Default)", type: "text" },
      { label: "Client Secret", key: "BANGUMI_CLIENT_SECRET", placeholder: "从 Bangumi 开发者平台获取", type: "password" },
    ]},
    { title: "Bangumi 同步", desc: "自动从 Bangumi.tv 同步收藏番剧到 Ani-Go", fields: [


      { label: "Bangumi 用户名", key: "BGMTV_USERNAME", placeholder: "Your Bangumi ID or Username" },
      { label: "Bangumi Token", key: "BGMTV_USER_TOKEN", placeholder: "Bearer Token (OAuth 后自动填入)", type: "password" },
      { label: "同步间隔", key: "BGMTV_SYNC_INTERVAL", placeholder: "6h (Default)" },
    ]}
  ]},
  { key: 'mikan', label: t('settings.tabs.mikan'), icon: Antenna, sections: [{ title: t('settings.sections.mikan'), desc: t('settings.sections.mikanDesc'), fields: [
    { label: t('settings.fields.rss'), key: 'MIKAN_RSS_URL', placeholder: 'https://mikanani.me/RSS/MyBangumi?token=***' },
    { label: t('settings.fields.rssMode'), key: 'MIKAN_RSS_MODE', placeholder: '', type: 'select', selectOptions: [{label: t('settings.rssMode.personal'), value: 'personal'}, {label: t('settings.rssMode.classic'), value: 'classic'}] },
    { label: t('settings.fields.domain'), key: 'MIKAN_DOMAIN', placeholder: 'mikanani.me' },
    { label: t('settings.fields.proxy'), key: 'MIKAN_PROXY_DOMAIN', placeholder: 'Optional proxy address' },
    { label: t('settings.fields.mirrors'), key: 'MIKAN_MIRROR_DOMAINS', placeholder: 'mikanani.me,mikanime.tv' },
  ]}]},
  { key: 'downloader', label: t('settings.tabs.downloader'), icon: Download, sections: [
    { title: t('settings.sections.engine'), desc: t('settings.sections.engineDesc'), fields: [
      { label: t('settings.fields.downloader'), key: 'DEFAULT_DOWNLOADER', placeholder: 'qbittorrent' },
    ]},
    { title: t('settings.sections.qb'), desc: t('settings.sections.qbDesc'), fields: [
      { label: t('settings.fields.host'), key: 'QB_HOST', placeholder: 'http://localhost:8081' },
      { label: t('settings.fields.user'), key: 'QB_USER', placeholder: 'admin' },
      { label: t('settings.fields.pass'), key: 'QB_PASS', placeholder: 'Access key', type: 'password' },
      { label: t('settings.fields.category'), key: 'QB_CATEGORY', placeholder: 'ani-go' },
    ]},
    { title: t('settings.sections.tr'), desc: t('settings.sections.trDesc'), fields: [
      { label: t('settings.fields.host'), key: 'TR_HOST', placeholder: 'http://localhost:9091' },
      { label: t('settings.fields.user'), key: 'TR_USER', placeholder: 'Username' },
      { label: t('settings.fields.pass'), key: 'TR_PASS', placeholder: 'Access key', type: 'password' },
    ]},
    { title: t('settings.sections.aria2'), desc: t('settings.sections.aria2Desc'), fields: [
      { label: t('settings.fields.rpc'), key: 'ARIA2_HOST', placeholder: 'http://localhost:6800' },
      { label: t('settings.fields.secret'), key: 'ARIA2_SECRET', placeholder: 'Secret key', type: 'password' },
    ]},
  ]},
  { key: 'paths', label: t('settings.tabs.paths'), icon: Folder, sections: [{ title: t('settings.sections.storage'), desc: t('settings.sections.storageDesc'), fields: [
    { label: t('settings.fields.db'), key: 'DB_PATH', placeholder: '/data/ani-go.db' },
    { label: t('settings.fields.tv'), key: 'TV_BASE_PATH', placeholder: '/TV/Media/Anime' },
    { label: t('settings.fields.movie'), key: 'MOVIE_BASE_PATH', placeholder: '/TV/Media/Movies' },
  ]}]},
  { key: 'notify', label: t('settings.tabs.notify'), icon: Bell, sections: [
    { title: t('settings.sections.im'), desc: t('settings.sections.imDesc'), fields: [
      { label: t('settings.fields.tgToken'), key: 'TELEGRAM_BOT_TOKEN', placeholder: '123456:ABC...', testChannel: 'Telegram' },
      { label: t('settings.fields.tgId'), key: 'TELEGRAM_CHAT_ID', placeholder: '123456789' },
    ]},
  ]},
  { key: 'plugins', label: t('settings.tabs.plugins'), icon: Sparkles, sections: [
    { title: t('settings.sections.plugins'), desc: t('settings.sections.pluginsDesc'), fields: [] }
  ]},
  { key: 'account', label: t('settings.tabs.account'), icon: Lock, sections: [
    { title: t('settings.sections.profile'), desc: t('settings.sections.profileDesc'), fields: [
      { label: t('settings.fields.avatar'), key: 'USER_AVATAR_URL', placeholder: 'https://example.com/avatar.png', type: 'text' },
    ]},
  ]},
  { key: 'backup', label: t('settings.tabs.backup'), icon: Database, sections: [
    { title: t('settings.sections.backup'), desc: t('settings.sections.backupDesc'), fields: [
      { label: t('settings.fields.backupPath'), key: 'BACKUP_PATH', placeholder: './data/backups', type: 'text' },
      { label: t('settings.fields.backupCron'), key: 'BACKUP_CRON', placeholder: '0 0 * * *', type: 'text' },
    ]},
  ]},
])

const allFields = computed(() => {
  const m: Record<string, FieldDef> = {}
  for (const tab of tabs.value)
    for (const section of tab.sections)
      for (const f of section.fields) m[f.key] = f
  return m
})

function getVal(key: string): string { return settings.value[key] || '' }
function setVal(key: string, val: string) { settings.value[key] = val }
function isConfigured(key: string): boolean {
  const val = settings.value[key]
  const field = allFields.value[key]
  if (field?.type === 'password') {
    return val !== undefined
  }
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

async function connectBangumi() {
  // 先打开空白新窗口防止被浏览器弹窗拦截 (Popup Blocker)
  const newTab = window.open('', '_blank')
  try {
    const { data } = await request.get("/bangumi/auth/link")
    if (data.url) {
      if (newTab) {
        newTab.location.href = data.url
      } else {
        window.open(data.url, '_blank')
      }
    } else {
      if (newTab) newTab.close()
      error.value = "授权链接为空"
    }
  } catch (e: any) {
    if (newTab) newTab.close()
    error.value = e.response?.data?.error || "无法获取 Bangumi 授权链接"
  }
}

async function fetchSettings() {

  loading.value = true; error.value = ''
  try {
    const { data } = await request.get('/settings')
    settings.value = (data as Record<string, string>) || {}
  } catch (e: any) {
    error.value = e.response?.data?.error || t('settings.error.loadFailed')
  } finally { loading.value = false }
}

async function saveAll() {
  error.value = ''; saved.value = false
  const changed: Record<string, string> = {}
  for (const key of Object.keys(allFields.value)) {
    const val = settings.value[key]
    const field = allFields.value[key]
    // 密码字段如果为空（说明未修改），则不包含在请求中，防止覆盖旧密码
    if (field.type === 'password' && (val === '' || val === undefined)) {
      continue
    }
    if (val !== undefined && val !== '') {
      changed[key] = val
    }
  }
  try {
    await request.put('/settings', { settings: changed })
    saved.value = true; setTimeout(() => { saved.value = false }, 3000)
  } catch (e: any) {
    error.value = e.response?.data?.error || t('settings.error.saveFailed')
  }
}

// AI 模型列表相关
const modelLoading = ref(false)
const modelOptions = ref<{label: string, value: string}[]>([])

async function fetchAIModels() {
  const protocol = getVal('AI_PROTOCOL')
  const endpoint = getVal('AI_ENDPOINT')
  const apiKey = getVal('AI_API_KEY')
  
  if (!endpoint) {
    error.value = t('settings.ai.error.endpointRequired')
    return
  }
  
  modelLoading.value = true
  modelOptions.value = []
  
  try {
    const { data } = await request.post('/ai/models', { 
      protocol, 
      endpoint, 
      apiKey 
    }, { timeout: 15000 })
    
    if (data.success && data.models && data.models.length > 0) {
      modelOptions.value = data.models.map((m: string) => ({ label: m, value: m }))
      // 自动选择第一个模型（如果当前没有选中）
      if (!getVal('AI_MODEL')) {
        setVal('AI_MODEL', data.models[0])
      }
      saved.value = true
      setTimeout(() => { saved.value = false }, 2000)
    } else {
      error.value = data.error || t('settings.ai.error.fetchFailed')
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || t('settings.ai.error.fetchFailed')
  } finally {
    modelLoading.value = false
  }
}

// 检查 AI 是否已完整配置（用于控制智能搜索开关）
const isAIConfigured = computed(() => {
  const endpoint = getVal('AI_ENDPOINT')
  const apiKey = getVal('AI_API_KEY')
  const model = getVal('AI_MODEL')
  return endpoint && apiKey && model
})

async function sendTestNotify(channel: string) {
  try {
    const { data } = await request.post('/notify/test', { channel, title: 'Ani-Go 测试通知', message: `这是一条来自 ${channel} 的测试消息，如果您收到说明通知配置正常。` })
    if (data.success) {
      saved.value = true
      setTimeout(() => { saved.value = false }, 3000)
    } else {
      error.value = data.error || '发送失败'
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || '发送请求失败'
  }
}

onMounted(fetchSettings)

// 日志
const logs = ref<string[]>([])
const logLoading = ref(false)
const logLines = ref(100)

async function fetchLogs() {
  logLoading.value = true
  try {
    const { data } = await request.get('/logs', { params: { lines: logLines.value } })
    logs.value = data.lines || []
  } catch (e) {
    // 静默失败
  } finally {
    logLoading.value = false
  }
}

onMounted(() => {
  if (route.query.status === 'success') {
    saved.value = true
    setTimeout(() => { saved.value = false }, 4000)
  }
  fetchSettings()
  fetchLogs()
  fetchBackupList()
  fetchPlugins()

  // 监听窗口重新获得焦点，如果在 bangumi 标签页则刷新一次配置
  window.addEventListener('focus', () => {
    if (activeTab.value === 'bangumi') {
      fetchSettings()
    }
  })
})

// 备份管理函数
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
    setTimeout(() => { saved.value = false }, 3000)
    await fetchBackupList()
  } catch (e: any) {
    error.value = e.response?.data?.error || '创建备份失败'
  } finally {
    creatingBackup.value = false
  }
}

async function restoreBackup(name: string) {
  if (!confirm(`确定要从备份 "${name}" 恢复吗？这将覆盖当前的设置和订阅数据。建议恢复后重启服务。`)) return
  
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
</script>

<template>
  <div class="space-y-10 pb-20">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-6">
      <div class="space-y-1">
        <h1 class="text-4xl font-black tracking-tighter italic">{{ $t('settings.title') }}</h1>
        <p class="text-xs font-bold tracking-[0.3em] uppercase opacity-30">{{ $t('settings.subtitle') }}</p>
      </div>
      
      <button 
        class="btn btn-primary rounded-2xl gap-3 px-8 shadow-xl shadow-lg hover:scale-105 active:scale-95 transition-all group" 
        @click="saveAll"
      >
        <Check :size="20" class="group-hover:scale-125 transition-transform" />
        <span class="text-xs font-black uppercase tracking-widest">{{ $t('settings.commit') }}</span>
      </button>
    </div>

    <!-- Status Alerts -->
    <Transition name="fade">
       <div v-if="saved" class="alert bg-success/10 border-success/20 text-success rounded-[2rem] p-6 shadow-xl shadow-lg">
          <Check :size="24" class="shrink-0" />
          <div class="flex-1">
             <h3 class="font-black text-sm uppercase tracking-widest">{{ $t('settings.updateSuccess') }}</h3>
             <p class="text-sm font-bold opacity-80 mt-1">{{ $t('settings.updateSuccessDesc') }}</p>
          </div>
       </div>
    </Transition>

    <div v-if="loading" class="flex justify-center py-32">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <div v-else class="flex flex-col lg:flex-row gap-6 lg:gap-10">
      <!-- Navigation Sidebar -->
      <div class="flex flex-row lg:flex-col gap-2 overflow-x-auto lg:w-56 shrink-0 no-scrollbar pb-2 lg:pb-0">
        <button v-for="tab in tabs" :key="tab.key"
          class="flex items-center gap-4 px-5 py-3.5 lg:px-6 lg:py-4 rounded-xl lg:rounded-2xl transition-all duration-300 relative group overflow-hidden whitespace-nowrap lg:w-full shrink-0"
          :class="activeTab === tab.key ? 'bg-primary text-primary-content shadow-xl shadow-lg font-black' : 'bg-base-100 hover:bg-base-200 text-base-content/50 hover:text-base-content border border-base-200/50'"
          @click="activeTab = tab.key">
          <component :is="tab.icon" :size="20" />
          <span class="text-xs uppercase tracking-widest">{{ tab.label }}</span>
          <div v-if="activeTab === tab.key" class="absolute right-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-white/40 rounded-l-full hidden lg:block"></div>
        </button>
      </div>

      <!-- Main Config Area -->
      <div class="flex-1 min-w-0 space-y-12 animate-in fade-in slide-in-from-bottom-4 duration-500">
        
        <!-- Special: Latency Test Card (Shared between Mikan/Bangumi) -->
        <div v-if="activeTab === 'mikan' || activeTab === 'bangumi'" class="bg-base-100 rounded-3xl lg:rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden group">
          <div class="p-6 sm:p-8 lg:p-10 space-y-8">
            <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-6">
               <div class="flex items-center gap-4">
               <div class="w-12 h-12 rounded-2xl bg-primary/10 flex items-center justify-center text-primary">
                  <Timer :size="24" />
               </div>
               <div>
                     <h3 class="text-xl font-black tracking-tight italic">{{ $t('settings.mikan.mirrorAudit') }}</h3>
                     <p class="text-[10px] font-black uppercase tracking-widest opacity-30 mt-1">{{ $t('settings.mikan.autoRoute') }}</p>
                  </div>
               </div>
               <button class="btn btn-primary btn-sm rounded-xl px-6 uppercase font-black tracking-widest text-[9px] h-10 min-h-0" @click="testMirrors" :disabled="mirrorTesting">
                 <span v-if="mirrorTesting" class="loading loading-spinner loading-xs"></span>
                 <template v-else>{{ $t('settings.mikan.runDiagnostics') }}</template>
               </button>
            </div>

            <div v-if="mirrorResults.length > 0" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div v-for="r in mirrorResults" :key="r.domain"
                class="group/item flex items-center justify-between p-5 rounded-2xl border transition-all duration-300"
                :class="r.ok ? 'bg-base-200/30 border-base-300 hover:border-primary/50 cursor-pointer active:scale-95' : 'bg-error/5 border-error/20 opacity-60 cursor-not-allowed'"
                @click="r.ok && selectMirror(r.domain)">
                <div class="flex flex-col gap-1">
                  <div class="flex items-center gap-2">
                     <div class="w-2 h-2 rounded-full shadow-[0_0_8px]" :class="r.ok ? 'bg-success shadow-lg' : 'bg-error shadow-lg'"></div>
                     <span class="text-sm font-black font-mono tracking-tight group-hover/item:text-primary transition-colors">{{ r.domain }}</span>
                  </div>
                  <span v-if="(activeTab === 'bangumi' ? getVal('BGMTV_DOMAIN') : getVal('MIKAN_DOMAIN')) === r.domain" class="text-[9px] font-black uppercase tracking-widest text-primary ml-4">{{ $t('settings.mikan.currentRoute') }}</span>
                </div>
                <div class="text-right">
                  <p v-if="r.ok" class="text-lg font-black tracking-tighter" :class="r.latency_ms < 500 ? 'text-success' : r.latency_ms < 1000 ? 'text-warning' : 'text-error'">
                    {{ r.latency_ms }}<span class="text-[10px] ml-0.5 opacity-50 uppercase tracking-widest">ms</span>
                  </p>
                  <p v-else class="text-xs font-black uppercase tracking-widest text-error">{{ $t('settings.mikan.unreachable') }}</p>
                </div>
              </div>
            </div>
            
            <div v-else-if="!mirrorTesting" class="flex flex-col items-center justify-center py-12 text-center bg-base-200/30 rounded-3xl border border-dashed border-base-300">
               <p class="text-[10px] font-black uppercase tracking-widest opacity-20 italic">{{ $t('settings.mikan.noData') }}</p>
            </div>
          </div>
        </div>

        <!-- Account Security: Change Password -->
        <div v-if="activeTab === 'account'" class="animate-in fade-in slide-in-from-bottom-4 duration-500">
          <div class="px-4 mb-6">
            <h2 class="text-2xl font-black tracking-tight italic flex items-center gap-4">
              {{ $t('settings.tabs.account') }}
              <div class="h-1 w-12 bg-primary/20 rounded-full"></div>
            </h2>
            <p class="text-[10px] font-black uppercase tracking-widest opacity-30">更新账户凭据以确保安全</p>
          </div>

          <div class="bg-base-100 rounded-3xl lg:rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden p-6 sm:p-8 lg:p-10">
            <div class="space-y-6 max-w-md">
              <div class="space-y-2">
                <label class="text-xs font-black uppercase tracking-widest opacity-50 ml-1">当前密码</label>
                <div class="relative">
                  <div class="absolute inset-y-0 left-5 flex items-center text-base-content/10">
                    <Lock :size="20" />
                  </div>
                  <input v-model="oldPassword" type="password" class="w-full bg-base-200/50 border border-transparent focus:border-primary/20 focus:bg-base-100 focus:ring-4 focus:ring-primary/5 rounded-2xl pl-14 py-4 transition-all outline-none font-bold text-sm" placeholder="输入当前密码" />
                </div>
              </div>

              <div class="space-y-2">
                <label class="text-xs font-black uppercase tracking-widest opacity-50 ml-1">新密码</label>
                <div class="relative">
                  <div class="absolute inset-y-0 left-5 flex items-center text-base-content/10">
                    <Lock :size="20" />
                  </div>
                  <input v-model="newPassword" type="password" class="w-full bg-base-200/50 border border-transparent focus:border-primary/20 focus:bg-base-100 focus:ring-4 focus:ring-primary/5 rounded-2xl pl-14 py-4 transition-all outline-none font-bold text-sm" placeholder="至少 6 位" />
                </div>
              </div>

              <div class="space-y-2">
                <label class="text-xs font-black uppercase tracking-widest opacity-50 ml-1">确认新密码</label>
                <div class="relative">
                  <div class="absolute inset-y-0 left-5 flex items-center text-base-content/10">
                    <Lock :size="20" />
                  </div>
                  <input v-model="confirmPassword" type="password" class="w-full bg-base-200/50 border border-transparent focus:border-primary/20 focus:bg-base-100 focus:ring-4 focus:ring-primary/5 rounded-2xl pl-14 py-4 transition-all outline-none font-bold text-sm" placeholder="再次输入新密码" />
                </div>
              </div>

              <div class="pt-4 flex flex-col gap-4">
                <button @click="changePassword" :disabled="changingPassword" class="btn btn-primary rounded-2xl gap-3 h-14 min-h-0 px-8 shadow-xl shadow-lg hover:scale-[1.02] active:scale-95 transition-all group">
                  <span v-if="changingPassword" class="loading loading-spinner loading-sm"></span>
                  <span v-else class="text-xs font-black uppercase tracking-widest">确认修改密码</span>
                </button>
                <p v-if="passwordMsg" class="text-success text-xs font-bold bg-success/10 p-4 rounded-xl border border-success/20 animate-in fade-in zoom-in-95">{{ passwordMsg }}</p>
                <p v-if="passwordError" class="text-error text-xs font-bold bg-error/10 p-4 rounded-xl border border-error/20 animate-in fade-in zoom-in-95">{{ passwordError }}</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Section Fields -->
        <div v-for="section in tabs.find(t => t.key === activeTab)?.sections" :key="section.title" class="space-y-4 lg:space-y-6">
          <div class="px-4 flex flex-col sm:flex-row items-start sm:items-end justify-between gap-4">
            <div v-if="section.title === 'Bangumi OAuth2'" class="p-6 bg-primary/5 rounded-[2rem] border border-primary/10 w-full mb-6 flex flex-col items-start gap-5">
              <div class="w-full flex flex-col sm:flex-row items-center justify-between gap-6">
                <div class="flex items-center gap-6">
                  <div class="w-16 h-16 rounded-[1.5rem] bg-primary/10 flex items-center justify-center text-primary shadow-inner">
                    <Antenna :size="32" />
                  </div>
                  <div class="space-y-1">
                    <h3 class="text-xl font-black tracking-tight italic">连接 Bangumi 账号</h3>
                    <p class="text-[10px] font-black uppercase tracking-widest opacity-40 leading-relaxed">连接后将自动同步您的收藏列表（想看、在看）</p>
                  </div>
                </div>
                <button @click="connectBangumi" class="btn btn-primary btn-lg rounded-2xl gap-3 px-10 shadow-xl shadow-primary/20 hover:scale-105 active:scale-95 transition-all group">
                  <Antenna :size="20" class="group-hover:animate-pulse" />
                  <span class="text-xs font-black uppercase tracking-widest">OAuth 授权连接</span>
                </button>
              </div>
              <div class="w-full text-xs opacity-75 bg-base-200/50 p-4 rounded-2xl border border-base-content/5 space-y-1.5">
                <p class="font-bold text-primary">💡 支持两种连接方式：</p>
                <p>• <b>方式一（推荐，超简单）</b>：直接前往 <a href="https://next.bgm.tv/demo/access-token" target="_blank" class="link link-primary font-bold">Bangumi 个人令牌页面 ↗</a> 生成一个 Token，粘贴到下方的 <code>Bangumi Token</code>，并在下方填写您的 <code>Bangumi 用户名</code>，点击页面底部「保存配置」即可！</p>
                <p>• <b>方式二（OAuth2 自动授权）</b>：前往 <a href="https://bgm.tv/dev/app" target="_blank" class="link link-primary font-bold">Bangumi 开发者平台 ↗</a> 创建客户端应用，将获得的 Client ID 和 Secret 填入下方保存后，点击上方「OAuth 授权连接」按钮即可。</p>
              </div>
            </div>

            <div class="space-y-1">
              <h2 class="text-xl lg:text-2xl font-black tracking-tight italic flex items-center gap-4">
                {{ section.title }}
                <div class="h-1 w-12 bg-primary/20 rounded-full"></div>
              </h2>
              <p class="text-[10px] font-black uppercase tracking-widest opacity-30">{{ section.desc }}</p>
            </div>
            <div class="px-4 py-1.5 rounded-full bg-base-200 border border-base-300/50 flex items-center gap-2">
               <span class="text-[10px] font-black uppercase tracking-widest text-base-content/40">{{ $t('settings.status.syncStatus') }}:</span>
               <span class="text-[10px] font-black text-primary">{{ section.fields.filter(f => isConfigured(f.key)).length }}/{{ section.fields.length }}</span>
            </div>
          </div>

          <div class="bg-base-100 rounded-3xl lg:rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden divide-y divide-base-200/50">
            <div v-for="field in section.fields" :key="field.key"
              class="group/field flex flex-col sm:flex-row sm:items-center gap-4 sm:gap-10 p-5 sm:p-6 lg:p-8 hover:bg-base-200/30 transition-colors">
              
              <div class="sm:w-56 shrink-0 space-y-1">
                <p class="text-xs font-black uppercase tracking-widest flex items-center gap-2">
                   {{ field.label }}
                   <Check v-if="isConfigured(field.key)" :size="12" class="text-success" />
                </p>
                <p v-if="field.hint" class="text-[9px] font-bold opacity-30 uppercase leading-relaxed">{{ field.hint }}</p>
              </div>

              <div class="flex-1 relative group flex items-center">
                  <template v-if="field.type === 'switch'">
                    <input type="checkbox" class="toggle toggle-primary toggle-lg" 
                      :checked="getVal(field.key) === 'true'"
                      @change="(e: any) => setVal(field.key, e.target.checked ? 'true' : 'false')"
                      :disabled="field.key === 'AI_SMART_SEARCH_ENABLED' && !isAIConfigured" />
                    <span v-if="field.key === 'AI_SMART_SEARCH_ENABLED' && !isAIConfigured" class="text-[9px] font-bold opacity-30 uppercase ml-2">
                      {{ $t('settings.ai.error.aiNotConfigured') }}
                    </span>
                  </template>
                  <template v-else-if="field.type === 'select' && field.selectOptions">
                    <div class="flex w-full items-center gap-2">
                      <select 
                        :value="getVal(field.key) || (field.key === 'AI_MODEL' ? (modelOptions[0]?.value || '') : field.selectOptions[0]?.value)"
                        @change="(e: any) => setVal(field.key, e.target.value)"
                        :disabled="field.key === 'AI_MODEL' && modelOptions.length === 0 && !modelLoading"
                        class="flex-1 bg-base-200/50 border border-transparent focus:border-primary/20 focus:bg-base-100 focus:ring-4 focus:ring-primary/5 rounded-xl lg:rounded-2xl pl-12 lg:pl-14 pr-10 lg:pr-12 py-3.5 lg:py-4 transition-all outline-none font-bold text-sm lg:text-base appearance-none cursor-pointer">
                        <option v-for="opt in (field.key === 'AI_MODEL' ? modelOptions : field.selectOptions)" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                      </select>
                      <!-- AI 模型获取按钮 -->
                      <button v-if="field.key === 'AI_MODEL'"
                        class="btn btn-ghost btn-sm rounded-xl gap-2 px-4 py-3.5 lg:py-4 hover:bg-primary/20 hover:text-primary transition-all shrink-0"
                        @click="fetchAIModels"
                        :disabled="modelLoading || !getVal('AI_ENDPOINT')"
                        title="从端点获取可用模型列表">
                        <RefreshCw v-if="modelLoading" :size="16" class="animate-spin" />
                        <RefreshCw v-else :size="16" />
                        <span class="hidden sm:inline text-xs font-black uppercase tracking-widest">{{ modelLoading ? $t('settings.ai.loadingModels') : $t('settings.ai.fetchModels') }}</span>
                      </button>
                    </div>
                  </template>
                  <template v-else>
                    <div class="absolute inset-y-0 left-5 flex items-center text-base-content/10 group-focus-within:text-primary transition-colors">
                       <component :is="field.type === 'password' ? Lock : FileText" :size="20" />
                    </div>
                    <input :type="inputType(field)" :value="getVal(field.key)"
                      @input="(e: Event) => setVal(field.key, (e.target as HTMLInputElement).value)"
                      :placeholder="(field.type === 'password' && settings[field.key] !== undefined) ? '已配置，输入新值覆盖' : field.placeholder"
                      class="w-full bg-base-200/50 border border-transparent focus:border-primary/20 focus:bg-base-100 focus:ring-4 focus:ring-primary/5 rounded-xl lg:rounded-2xl pl-12 lg:pl-14 pr-10 lg:pr-12 py-3.5 lg:py-4 transition-all outline-none font-bold placeholder:text-base-content/20 text-sm lg:text-base" />
                    
                    <button v-if="field.type === 'password' && getVal(field.key)"
                      class="absolute right-3 top-1/2 -translate-y-1/2 btn btn-ghost btn-circle btn-xs hover:bg-primary/20 hover:text-primary transition-all"
                      @click="togglePassword(field.key)">
                      <component :is="showPasswords.has(field.key) ? Eye : EyeOff" :size="16" />
                    </button>
                    <button v-if="field.testChannel"
                      class="absolute right-36 top-1/2 -translate-y-1/2 btn btn-ghost btn-circle btn-xs hover:bg-primary/20 hover:text-primary transition-all"
                      @click="sendTestNotify(field.testChannel)">
                      <Bell :size="16" />
                    </button>
                  </template>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 插件管理面板 -->
    <div v-if="activeTab === 'plugins'" class="animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div class="space-y-6">
        <div class="bg-base-100 rounded-3xl lg:rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden">
          <div class="p-6 sm:p-8 lg:p-10 space-y-6">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="space-y-1">
                <h2 class="text-xl lg:text-2xl font-black tracking-tight italic flex items-center gap-4">
                  插件扩展与自动化中心
                  <div class="h-1 w-12 bg-primary/20 rounded-full"></div>
                </h2>
                <p class="text-[10px] font-black uppercase tracking-widest opacity-40">已启用 {{ pluginList.filter(p => p.enabled).length }} / 共 {{ pluginList.length }} 个扩展 · 支持内建引擎与 Webhook 自动化</p>
              </div>
              <div class="flex items-center gap-3 flex-wrap">
                <button class="btn btn-ghost btn-sm rounded-xl gap-2 border border-base-300/50 hover:bg-base-200" @click="exportPluginsJSON" title="导出插件配置">
                  <Download :size="16" />
                  <span class="text-xs font-bold">导出配置</span>
                </button>
                <button class="btn btn-ghost btn-sm rounded-xl gap-2 border border-base-300/50 hover:bg-base-200" @click="reloadPlugins" :disabled="pluginLoading" title="重新载入">
                  <RefreshCw :size="16" :class="{ 'animate-spin': pluginLoading }" />
                  <span class="text-xs font-bold">重新加载</span>
                </button>
                <button class="btn btn-primary btn-sm rounded-xl gap-2 shadow-lg shadow-primary/20" @click="openAddPluginModal">
                  <Plus :size="16" />
                  <span class="text-xs font-black uppercase tracking-wider">导入 / 添加插件</span>
                </button>
              </div>
            </div>

            <!-- 加载状态 -->
            <div v-if="pluginLoading && pluginList.length === 0" class="flex justify-center py-12">
              <span class="loading loading-spinner loading-lg text-primary"></span>
            </div>

            <!-- 空状态 -->
            <div v-else-if="pluginList.length === 0" class="flex flex-col items-center justify-center py-16 text-center bg-base-200/30 rounded-3xl border border-dashed border-base-300 space-y-3">
              <Sparkles :size="48" class="opacity-20 text-primary animate-pulse" />
              <p class="text-sm font-black uppercase tracking-widest opacity-40">暂无任何插件</p>
              <button class="btn btn-primary btn-sm rounded-xl gap-2" @click="openAddPluginModal">
                <Plus :size="14" />
                <span>立即添加第一个插件</span>
              </button>
            </div>

            <!-- 插件卡片网格 -->
            <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-5">
              <div v-for="plugin in pluginList" :key="plugin.id"
                class="p-6 rounded-3xl border bg-base-200/30 hover:bg-base-200/50 transition-all flex flex-col justify-between gap-5 relative group"
                :class="plugin.enabled ? 'border-primary/30 shadow-lg shadow-primary/5' : 'border-base-300/40 opacity-75'">
                
                <!-- 卡片顶部 -->
                <div>
                  <div class="flex items-start justify-between gap-4 mb-3">
                    <div class="flex items-center gap-3.5">
                      <div class="w-12 h-12 rounded-2xl flex items-center justify-center transition-transform group-hover:scale-105 shadow-inner"
                        :class="plugin.is_builtin ? 'bg-primary/10 text-primary' : 'bg-secondary/10 text-secondary'">
                        <Sparkles v-if="plugin.is_builtin" :size="24" />
                        <Antenna v-else :size="24" />
                      </div>
                      <div>
                        <div class="flex items-center gap-2">
                          <h3 class="font-black text-base tracking-tight text-base-content">{{ plugin.name }}</h3>
                          <span class="text-[10px] font-mono px-2 py-0.5 rounded-full bg-base-300 text-base-content/60 font-bold">
                            v{{ plugin.version || '1.0' }}
                          </span>
                        </div>
                        <div class="flex items-center gap-2 mt-1">
                          <span class="badge badge-xs font-black uppercase tracking-wider text-[9px] px-2 py-1"
                            :class="plugin.is_builtin ? 'badge-primary' : 'badge-secondary'">
                            {{ plugin.is_builtin ? '内建插件' : (plugin.type === 'webhook' ? 'Webhook 联动' : '扩展插件') }}
                          </span>
                          <span v-if="plugin.author" class="text-[10px] opacity-40 font-semibold">
                            by {{ plugin.author }}
                          </span>
                        </div>
                      </div>
                    </div>

                    <!-- 开关与操作按钮 -->
                    <div class="flex items-center gap-3">
                      <input type="checkbox" class="toggle toggle-primary toggle-md"
                        :checked="plugin.enabled"
                        @change="togglePlugin(plugin)"
                        :title="plugin.enabled ? '点击停用' : '点击启用'" />
                      <button v-if="!plugin.is_builtin" @click="deletePlugin(plugin.id)"
                        class="btn btn-ghost btn-xs btn-circle text-error/40 hover:text-error hover:bg-error/10 transition-colors"
                        title="删除插件">
                        <Trash2 :size="14" />
                      </button>
                    </div>
                  </div>

                  <!-- 插件描述 -->
                  <p class="text-xs text-base-content/70 leading-relaxed font-medium mt-2">
                    {{ plugin.description || '暂无描述' }}
                  </p>
                </div>

                <!-- Webhook URL & 监听事件 -->
                <div class="space-y-3 pt-3 border-t border-base-content/5">
                  <div v-if="plugin.url" class="space-y-1">
                    <p class="text-[9px] font-black uppercase tracking-widest opacity-40">目标 Webhook URL</p>
                    <div class="flex items-center gap-2 bg-base-300/60 px-3 py-1.5 rounded-xl">
                      <span class="text-xs font-mono break-all opacity-80 select-all flex-1">{{ plugin.url }}</span>
                    </div>
                  </div>

                  <div v-if="plugin.events && plugin.events.length" class="space-y-1.5">
                    <p class="text-[9px] font-black uppercase tracking-widest opacity-40">监听事件触发点</p>
                    <div class="flex flex-wrap gap-1.5">
                      <span v-for="ev in plugin.events" :key="ev"
                        class="px-2.5 py-0.5 rounded-lg bg-base-300/80 text-[10px] font-bold text-base-content/70 border border-base-content/5">
                        {{ ev }}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

          </div>
        </div>
      </div>
    </div>

    <!-- 备份管理面板 -->
    <div v-if="activeTab === 'backup'" class="animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div class="space-y-6">
        <!-- 备份列表 -->
        <div class="bg-base-100 rounded-3xl lg:rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden">
          <div class="p-6 sm:p-8 lg:p-10 space-y-6">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="space-y-1">
                <h2 class="text-xl lg:text-2xl font-black tracking-tight italic flex items-center gap-4">
                  {{ $t('settings.sections.backup') }}
                  <div class="h-1 w-12 bg-primary/20 rounded-full"></div>
                </h2>
                <p class="text-[10px] font-black uppercase tracking-widest opacity-30">{{ $t('settings.sections.backupDesc') }} ({{ $t('settings.backup.fileCount', { count: backupList.length }) }})</p>
              </div>
              <div class="flex gap-3">
                <label class="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" class="checkbox checkbox-primary" v-model="showBackupEpisodes" />
                  <span class="text-xs font-black uppercase tracking-widest">{{ $t('settings.backup.includeEpisodes') }}</span>
                </label>
                <button class="btn btn-primary rounded-2xl gap-3 px-6" @click="createBackup" :disabled="creatingBackup">
                  <HardDrive :size="20" />
                  <span class="text-xs font-black uppercase tracking-widest">{{ $t('settings.backup.createBackup') }}</span>
                  <span v-if="creatingBackup" class="loading loading-spinner loading-xs"></span>
                </button>
              </div>
            </div>

            <div v-if="backupLoading" class="flex justify-center py-12">
              <span class="loading loading-spinner loading-lg text-primary"></span>
            </div>

            <div v-else-if="backupList.length === 0" class="flex flex-col items-center justify-center py-12 text-center bg-base-200/30 rounded-3xl border border-dashed border-base-300">
              <HardDrive :size="48" class="opacity-20 mb-4" />
              <p class="text-[10px] font-black uppercase tracking-widest opacity-20 italic">{{ $t('settings.backup.noBackups') }}</p>
            </div>

            <div v-else class="overflow-x-auto">
              <table class="table table-lg w-full">
                <thead>
                  <tr class="border-b border-base-200 bg-base-200/30">
                    <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6 pl-10">{{ $t('settings.backup.filename') }}</th>
                    <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ $t('settings.backup.size') }}</th>
                    <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6">{{ $t('settings.backup.created') }}</th>
                    <th class="text-[10px] font-black uppercase tracking-widest opacity-40 py-6 pr-10">{{ $t('settings.backup.actions') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-base-200/50">
                  <tr v-for="backup in backupList" :key="backup.name" class="hover:bg-base-200/30 transition-colors">
                    <td class="pl-10 font-mono text-sm">{{ backup.name }}</td>
                    <td class="font-mono text-sm">{{ formatBackupSize(backup.size) }}</td>
                    <td class="text-sm">{{ formatBackupTime(backup.mod_time) }}</td>
                    <td class="pr-10">
                      <div class="flex items-center gap-2">
                        <button class="btn btn-ghost btn-sm btn-circle hover:bg-primary/20 hover:text-primary" @click="downloadBackup(backup.name)" :title="$t('settings.backup.download')">
                          <ArrowDown :size="16" />
                        </button>
                        <button class="btn btn-ghost btn-sm btn-circle hover:bg-success/20 hover:text-success" @click="restoreBackup(backup.name)" :disabled="restoringBackup" :title="$t('settings.backup.restore')">
                          <RotateCcw :size="16" />
                          <span v-if="restoringBackup" class="loading loading-spinner loading-xs"></span>
                        </button>
                        <button class="btn btn-ghost btn-sm btn-circle hover:bg-error/20 hover:text-error" @click="deleteBackup(backup.name)" :disabled="deletingBackup === backup.name" :title="$t('settings.backup.delete')">
                          <Trash2 :size="16" />
                          <span v-if="deletingBackup === backup.name" class="loading loading-spinner loading-xs"></span>
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 系统日志 -->
    <div class="bg-base-100 rounded-3xl lg:rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden">
      <div class="p-6 sm:p-8 lg:p-10 space-y-6">
        <div class="flex items-center justify-between">
          <div class="space-y-1">
            <h2 class="text-xl font-black tracking-tight italic">{{ $t('settings.logs.title') }}</h2>
            <p class="text-[10px] font-black uppercase tracking-widest opacity-30">{{ $t('settings.logs.count', { count: logs.length }) }}</p>
          </div>
          <button class="btn btn-ghost btn-sm rounded-xl text-[10px] font-black uppercase tracking-widest" @click="fetchLogs" :disabled="logLoading">
            <span v-if="logLoading" class="loading loading-spinner loading-xs"></span>
            <template v-else>{{ $t('settings.logs.refresh') }}</template>
          </button>
        </div>
        <div class="bg-base-300/30 rounded-2xl p-4 max-h-80 overflow-y-auto font-mono text-[11px] leading-relaxed space-y-1">
          <div v-for="(line, i) in logs" :key="i" class="opacity-70 hover:opacity-100 transition-opacity">
            {{ line }}
          </div>
          <div v-if="logs.length === 0 && !logLoading" class="text-center py-8 text-[10px] font-bold opacity-30">{{ $t('settings.logs.empty') }}</div>
        </div>
      </div>
    </div>

    <!-- Version Footer -->
    <div class="flex flex-col items-center justify-center gap-2 pb-10 text-center opacity-70 hover:opacity-100 transition-opacity">
      <div class="text-xs font-semibold text-base-content/80 flex items-center justify-center gap-1.5 flex-wrap">
        <span>Ani-Go &copy; 2026 • 倾心打造</span>
        <span class="opacity-40">•</span>
        <span>by <a href="https://github.com/xiaoyueRX" target="_blank" rel="noopener noreferrer" class="text-primary font-bold hover:underline">xiaoyue</a></span>
      </div>
      <a href="https://github.com/xiaoyueRX/Ani-Go" target="_blank" rel="noopener noreferrer" 
         class="px-5 py-2 rounded-full bg-base-200/60 border border-base-300/60 text-[11px] font-mono font-bold text-base-content/70 hover:text-primary hover:border-primary/40 transition-all flex items-center gap-2 shadow-sm">
        <svg class="w-3.5 h-3.5 fill-current" viewBox="0 0 24 24"><path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/></svg>
        <span>GitHub: xiaoyueRX/Ani-Go</span>
        <span class="opacity-40">•</span>
        <span>v{{ CURRENT_VERSION }}</span>
      </a>
    </div>
    <!-- 导入 / 添加插件弹窗 Modal -->
    <div v-if="showPluginModal" class="modal modal-open z-50 animate-in fade-in duration-200">
      <div class="modal-box max-w-2xl bg-base-100 rounded-3xl p-6 sm:p-8 border border-base-300/60 shadow-2xl space-y-6">
        <div class="flex items-center justify-between">
          <div class="space-y-1">
            <h3 class="text-xl font-black italic tracking-tight flex items-center gap-3">
              <Sparkles class="text-primary" :size="24" />
              添加 / 导入插件扩展
            </h3>
            <p class="text-xs opacity-50">配置 Webhook 外部系统联动，或直接导入 JSON 格式插件配置</p>
          </div>
          <button @click="showPluginModal = false" class="btn btn-ghost btn-sm btn-circle">✕</button>
        </div>

        <!-- 模式切换标签 -->
        <div class="flex rounded-2xl bg-base-200 p-1">
          <button class="flex-1 py-2 rounded-xl text-xs font-black transition-all"
            :class="pluginModalTab === 'webhook' ? 'bg-primary text-primary-content shadow-md' : 'opacity-60 hover:opacity-100'"
            @click="pluginModalTab = 'webhook'">
            快捷创建 Webhook
          </button>
          <button class="flex-1 py-2 rounded-xl text-xs font-black transition-all"
            :class="pluginModalTab === 'json' ? 'bg-primary text-primary-content shadow-md' : 'opacity-60 hover:opacity-100'"
            @click="pluginModalTab = 'json'">
            JSON 配置导入
          </button>
        </div>

        <!-- Tab 1: Webhook 表单 -->
        <div v-if="pluginModalTab === 'webhook'" class="space-y-4">
          <div>
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60 block mb-1.5">插件名称 *</label>
            <input v-model="pluginForm.name" type="text" placeholder="例如：Discord 追番频道推送 / n8n 自动化" class="input input-bordered w-full rounded-2xl text-sm" />
          </div>

          <div>
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60 block mb-1.5">目标 Webhook URL *</label>
            <input v-model="pluginForm.url" type="url" placeholder="https://discord.com/api/webhooks/... 或 http://localhost:5678/webhook/..." class="input input-bordered w-full rounded-2xl font-mono text-sm" />
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="text-[11px] font-black uppercase tracking-wider opacity-60 block mb-1.5">签名密钥 Secret (可选)</label>
              <input v-model="pluginForm.secret" type="password" placeholder="请求头 X-AniGo-Secret" class="input input-bordered w-full rounded-2xl text-sm" />
            </div>
            <div>
              <label class="text-[11px] font-black uppercase tracking-wider opacity-60 block mb-1.5">插件描述 (可选)</label>
              <input v-model="pluginForm.description" type="text" placeholder="简要说明此插件功能" class="input input-bordered w-full rounded-2xl text-sm" />
            </div>
          </div>

          <div>
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60 block mb-2">监听触发事件 * (可多选)</label>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 bg-base-200/50 p-4 rounded-2xl border border-base-300/40">
              <label v-for="ev in availablePluginEvents" :key="ev.id" class="flex items-center gap-2.5 cursor-pointer hover:opacity-100 opacity-80 py-1">
                <input type="checkbox" :value="ev.id" v-model="pluginForm.events" class="checkbox checkbox-primary checkbox-sm rounded-md" />
                <span class="text-xs font-semibold select-none">{{ ev.label }}</span>
              </label>
            </div>
          </div>
        </div>

        <!-- Tab 2: JSON 导入 -->
        <div v-else class="space-y-4">
          <div class="flex items-center justify-between">
            <label class="text-[11px] font-black uppercase tracking-wider opacity-60">粘贴 JSON 配置或上传文件</label>
            <label class="btn btn-xs btn-outline rounded-xl gap-1 cursor-pointer">
              <Upload :size="12" />
              <span>选择 .json 文件</span>
              <input type="file" accept=".json,application/json" class="hidden" @change="handlePluginFileImport" />
            </label>
          </div>
          <textarea v-model="pluginJsonText" rows="10" class="textarea textarea-bordered w-full rounded-2xl font-mono text-xs leading-relaxed" placeholder="在此粘贴 JSON 格式的插件定义"></textarea>
        </div>

        <p v-if="pluginJsonError" class="text-xs text-error font-bold bg-error/10 p-3 rounded-xl border border-error/20">{{ pluginJsonError }}</p>

        <!-- 底部按钮 -->
        <div class="modal-action flex justify-end gap-3 pt-2">
          <button @click="showPluginModal = false" class="btn btn-ghost rounded-2xl px-6">取消</button>
          <button @click="submitPluginForm" :disabled="pluginSaving" class="btn btn-primary rounded-2xl px-8 shadow-lg shadow-primary/20">
            <span v-if="pluginSaving" class="loading loading-spinner loading-sm"></span>
            <span v-else>确认添加</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 0.5s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

.no-scrollbar::-webkit-scrollbar { display: none; }
.no-scrollbar { -ms-overflow-style: none; scrollbar-width: none; }
</style>
