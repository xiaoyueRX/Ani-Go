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
  Copy, Search, Camera, X,
  Zap, ExternalLink, Play,
  Activity, CheckCircle, AlertCircle, Filter, Clock
} from 'lucide-vue-next'
import { CURRENT_VERSION, currentVersion, useVersion } from '../composables/useVersion'

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

// 版本与更新检测
const { latestVersion, hasNewVersion, checkGitHubUpdate } = useVersion()
const checkingUpdate = ref(false)
const manualCheckMsg = ref('')
const manualCheckMsgType = ref<'success' | 'error'>('success')
const manualCheckResult = ref<string>('')

async function triggerManualCheckUpdate() {
  checkingUpdate.value = true
  manualCheckMsg.value = ''
  manualCheckResult.value = ''
  try {
    const res = await checkGitHubUpdate(true)
    if (res?.hasUpdate) {
      manualCheckResult.value = 'update'
      manualCheckMsgType.value = 'success'
      manualCheckMsg.value = `🎉 发现新版本：${res.latest}！请前往 GitHub Releases 下载并更新。`
    } else if (res?.error) {
      manualCheckResult.value = 'error'
      manualCheckMsgType.value = 'error'
      manualCheckMsg.value = `检查更新失败: ${res.error}`
    } else {
      manualCheckResult.value = 'latest'
      manualCheckMsgType.value = 'success'
      manualCheckMsg.value = `✅ 当前已是最新版本 (${currentVersion.value})，暂无更新。`
    }
  } catch (e: any) {
    manualCheckMsgType.value = 'error'
    manualCheckMsg.value = `检查更新失败: ${e?.message || '网络连接异常'}`
  } finally {
    checkingUpdate.value = false
  }
}

function toggleAutoCheckUpdate(e: Event) {
  const checked = (e.target as HTMLInputElement).checked
  const val = checked ? 'true' : 'false'
  setVal('AUTO_CHECK_UPDATE', val)
  localStorage.setItem('ani-go-auto-update', val)
  if (window.showToast) {
    window.showToast(checked ? '已开启自动检测更新' : '已关闭自动检测更新', 'info')
  }
}

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

// 导航分组结构（根据插件启用状态动态过滤）
const tabGroups = computed(() => {
  const rawGroups = [
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
        { key: 'sources', label: t('settings.tabs.sources'), icon: Search },
        { key: 'mikan', label: t('settings.tabs.mikan'), icon: Antenna },
        { key: 'bangumi', label: t('settings.tabs.bangumi'), icon: Antenna },
      ]
    },
    {
      name: t('settings.groups.extensions'),
      tabs: [
        { key: 'ai', label: t('settings.tabs.ai'), icon: Cpu },
        { key: 'customRegex', label: t('settings.sections.customRegex'), icon: FileText },
        { key: 'notify', label: t('settings.tabs.notify'), icon: Bell, pluginRequired: 'extended-notifiers' },
        { key: 'plugins', label: t('settings.tabs.plugins'), icon: Sparkles },
      ]
    },
    {
      name: t('settings.groups.system'),
      tabs: [
        { key: 'scheduler', label: t('settings.tabs.scheduler'), icon: Timer },
        { key: 'backup', label: t('settings.tabs.backup'), icon: Database },
        { key: 'logs', label: t('settings.tabs.logs'), icon: FileText },
        { key: 'account', label: t('settings.tabs.account'), icon: Lock },
      ]
    }
  ]

  return rawGroups.map(g => ({
    ...g,
    tabs: g.tabs.filter((tab: any) => !tab.pluginRequired || isPluginActive(tab.pluginRequired))
  }))
})

const allFlatTabs = computed(() => tabGroups.value.flatMap(g => g.tabs))

// 扁平化标签页定义与字段映射
interface FieldDef {
  label: string
  key: string
  placeholder: string
  type?: string
  hint?: string
  testChannel?: string
  testDownloader?: string
  selectOptions?: { label: string; value: string }[]
}

interface SectionDef {
  title: string
  desc: string
  fields: FieldDef[]
  pluginRequired?: string
}

const tabs = computed(() => [
  { key: 'paths', label: t('settings.tabs.paths'), icon: Folder, sections: [
    { title: t('settings.sections.pathsStorage'), desc: t('settings.sections.pathsStorageDesc'), fields: [
      { label: t('settings.fields.db'), key: 'DB_PATH', placeholder: './data/ani-go.db' },
      { label: t('settings.fields.media'), key: 'TV_BASE_PATH', placeholder: './TV/Media/番剧' },
      { label: t('settings.fields.movieMedia'), key: 'MOVIE_BASE_PATH', placeholder: './TV/Media/剧场版' },
      { label: t('settings.fields.ovaMedia'), key: 'OVA_BASE_PATH', placeholder: './TV/Media/OVA' },
      { label: t('settings.fields.hardlink'), key: 'USE_HARDLINK', placeholder: 'false', type: 'select', selectOptions: [
        { label: t('settings.options.hardlinkDirect'), value: 'true' },
        { label: t('settings.options.hardlinkCopy'), value: 'false' },
      ]},
      { label: t('settings.fields.tvTemplate'), key: 'TV_TEMPLATE', placeholder: '{title_cn}{year}/Season {season}/{title_en} [tmdbid={tmdb_id}] S{season:02}E{ep:02}{ext}' },
      { label: t('settings.fields.movieTemplate'), key: 'MOVIE_TEMPLATE', placeholder: '{title_cn} ({year})/{title_en}{ext}' },
      { label: t('settings.fields.otherTemplate'), key: 'OTHER_TEMPLATE', placeholder: '{title_cn}{year}/Specials/{title_en} S00E{ep:02}{ext}' },
    ]}
  ]},
  { key: 'downloader', label: t('settings.tabs.downloader'), icon: Download, sections: [
    { title: t('settings.sections.engine'), desc: t('settings.sections.engineDesc'), fields: [
      { label: t('settings.fields.activeDownloader'), key: 'DEFAULT_DOWNLOADER', placeholder: 'qbittorrent', type: 'select', testDownloader: 'current', selectOptions: [
        { label: t('settings.options.qbRecommend'), value: 'qbittorrent' },
        { label: t('settings.options.transmission'), value: 'transmission' },
        { label: t('settings.options.aria2'), value: 'aria2' },
      ]},
    ]},
    { title: t('settings.sections.qb'), desc: t('settings.sections.qbDesc'), fields: [
      { label: t('settings.fields.qbHost'), key: 'QB_HOST', placeholder: 'http://localhost:8081', testDownloader: 'qbittorrent' },
      { label: t('settings.fields.qbCategory'), key: 'QB_CATEGORY', placeholder: 'ani-go' },
      { label: t('settings.fields.qbUser'), key: 'QB_USER', placeholder: 'admin' },
      { label: t('settings.fields.qbPass'), key: 'QB_PASS', placeholder: '••••••••', type: 'password' },
    ]},
    { title: t('settings.sections.tr'), desc: t('settings.sections.trDesc'), fields: [
      { label: t('settings.fields.trHost'), key: 'TR_HOST', placeholder: 'http://localhost:9091', testDownloader: 'transmission' },
      { label: t('settings.fields.trUser'), key: 'TR_USER', placeholder: 'Username' },
      { label: t('settings.fields.trPass'), key: 'TR_PASS', placeholder: '••••••••', type: 'password' },
    ]},
    { title: t('settings.sections.aria2'), desc: t('settings.sections.aria2Desc'), fields: [
      { label: t('settings.fields.aria2Host'), key: 'ARIA2_HOST', placeholder: 'http://localhost:6800/jsonrpc', testDownloader: 'aria2' },
      { label: t('settings.fields.aria2Secret'), key: 'ARIA2_SECRET', placeholder: 'Secret Token', type: 'password' },
    ]},
    { title: t('settings.sections.seedCleanup'), desc: t('settings.sections.seedCleanupDesc'), fields: [
      { label: t('settings.fields.seedCleanupEnabled'), key: 'SEED_CLEANUP_ENABLED', placeholder: '', type: 'select', selectOptions: [
        { label: t('settings.options.seedDeleteTaskOnly'), value: 'true' },
        { label: t('settings.options.disabled'), value: 'false' },
      ]},
      { label: t('settings.fields.seedCleanupInterval'), key: 'SEED_CLEANUP_INTERVAL', placeholder: '1h' },
      { label: t('settings.fields.seedCleanupMinRatio'), key: 'SEED_CLEANUP_MIN_RATIO', placeholder: '1.0' },
      { label: t('settings.fields.seedCleanupMinSeedTime'), key: 'SEED_CLEANUP_MIN_SEED_TIME', placeholder: '48h' },
    ]},
    { title: t('settings.sections.diskGuard'), desc: t('settings.sections.diskGuardDesc'), fields: [
      { label: t('settings.fields.diskMinFreeGb'), key: 'DISK_MIN_FREE_GB', placeholder: '5.0' },
    ]}
  ]},
  { key: 'sources', label: t('settings.tabs.sources'), icon: Search, sections: [
    { title: t('settings.sections.sourcesNyaa'), desc: t('settings.sections.sourcesNyaaDesc'), fields: [
      { label: t('settings.fields.nyaaEnabled'), key: 'NYAA_ENABLED', placeholder: 'false', type: 'select', selectOptions: [
        { label: t('settings.options.enabled'), value: 'true' },
        { label: t('settings.options.disabled'), value: 'false' },
      ]},
      { label: t('settings.fields.nyaaDomain'), key: 'NYAA_DOMAIN', placeholder: 'nyaa.si' },
    ]},
    { title: t('settings.sections.sourcesAcgrip'), desc: t('settings.sections.sourcesAcgripDesc'), fields: [
      { label: t('settings.fields.acgripEnabled'), key: 'ACGRIP_ENABLED', placeholder: 'false', type: 'select', selectOptions: [
        { label: t('settings.options.enabled'), value: 'true' },
        { label: t('settings.options.disabled'), value: 'false' },
      ]},
      { label: t('settings.fields.acgripDomain'), key: 'ACGRIP_DOMAIN', placeholder: 'acg.rip' },
    ]},
    { title: t('settings.sections.sourcesTosho'), desc: t('settings.sections.sourcesToshoDesc'), fields: [
      { label: t('settings.fields.toshoEnabled'), key: 'ANIMETOSHO_ENABLED', placeholder: 'false', type: 'select', selectOptions: [
        { label: t('settings.options.enabled'), value: 'true' },
        { label: t('settings.options.disabled'), value: 'false' },
      ]},
      { label: t('settings.fields.toshoDomain'), key: 'ANIMETOSHO_DOMAIN', placeholder: 'animetosho.org' },
    ]},
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
    { title: t('settings.sections.metadataPrimary'), desc: t('settings.sections.metadataPrimaryDesc'), fields: [
      { label: t('settings.fields.metadataPrimary'), key: 'METADATA_PRIMARY', placeholder: 'bangumi', type: 'select', selectOptions: [
        { label: 'Bangumi (bgm.tv)', value: 'bangumi' },
        { label: 'TMDB (The Movie Database)', value: 'tmdb' },
      ]},
    ]},
    { title: t('settings.sections.tmdbMeta'), desc: t('settings.sections.tmdbMetaDesc'), fields: [
      { label: t('settings.fields.tmdbApiKey'), key: 'TMDB_API_KEY', placeholder: '32-char API Key / v3 Read Token', type: 'password' },
      { label: t('settings.fields.tmdbMirrors'), key: 'TMDB_MIRROR_DOMAINS', placeholder: 'api.themoviedb.org,api.tmdb.org' },
      { label: t('settings.fields.tmdbLanguage'), key: 'TMDB_LANGUAGE', placeholder: 'zh-CN' },
    ]},
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
      { label: t('settings.fields.aiAutoDisambiguate'), key: 'AI_AUTO_DISAMBIGUATE', placeholder: '', type: 'select', selectOptions: [
        { label: t('settings.options.enabled'), value: 'true' },
        { label: t('settings.options.disabled'), value: 'false' },
      ]},
      { label: t('settings.fields.aiDailyQuota'), key: 'AI_DAILY_QUOTA_LIMIT', placeholder: '30' },
    ]}
  ]},
  { key: 'notify', label: t('settings.tabs.notify'), icon: Bell, pluginRequired: 'extended-notifiers', sections: [
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
    ]},
    { title: t('settings.sections.notifyBark'), desc: t('settings.sections.notifyBarkDesc'), fields: [
      { label: t('settings.fields.barkKey'), key: 'BARK_DEVICE_KEY', placeholder: 'Bark Device Key', testChannel: 'Bark' },
      { label: t('settings.fields.barkServer'), key: 'BARK_SERVER_URL', placeholder: 'https://api.day.app' },
    ]},
    { title: t('settings.sections.notifyServerChan'), desc: t('settings.sections.notifyServerChanDesc'), fields: [
      { label: t('settings.fields.serverChanKey'), key: 'SERVERCHAN_KEY', placeholder: 'SCT...', type: 'password', testChannel: 'ServerChan' },
    ]},
    { title: t('settings.sections.notifyDiscord'), desc: t('settings.sections.notifyDiscordDesc'), fields: [
      { label: t('settings.fields.discordWebhook'), key: 'DISCORD_WEBHOOK', placeholder: 'https://discord.com/api/webhooks/...', type: 'password', testChannel: 'Discord' },
    ]},
    { title: t('settings.sections.notifySlack'), desc: t('settings.sections.notifySlackDesc'), fields: [
      { label: t('settings.fields.slackWebhook'), key: 'SLACK_WEBHOOK', placeholder: 'https://hooks.slack.com/services/...', type: 'password', testChannel: 'Slack' },
    ]},
    { title: t('settings.sections.notifyGotify'), desc: t('settings.sections.notifyGotifyDesc'), fields: [
      { label: t('settings.fields.gotifyUrl'), key: 'GOTIFY_URL', placeholder: 'https://gotify.example.com', testChannel: 'Gotify' },
      { label: t('settings.fields.gotifyToken'), key: 'GOTIFY_TOKEN', placeholder: 'Application Token', type: 'password' },
    ]},
    { title: t('settings.sections.notifyNtfy'), desc: t('settings.sections.notifyNtfyDesc'), fields: [
      { label: t('settings.fields.ntfyUrl'), key: 'NTFY_URL', placeholder: 'https://ntfy.sh', testChannel: 'Ntfy' },
      { label: t('settings.fields.ntfyTopic'), key: 'NTFY_TOPIC', placeholder: 'my-anime-topic' },
    ]},
    { title: t('settings.sections.notifyPushover'), desc: t('settings.sections.notifyPushoverDesc'), fields: [
      { label: t('settings.fields.pushoverToken'), key: 'PUSHOVER_TOKEN', placeholder: 'Application Token', type: 'password', testChannel: 'Pushover' },
      { label: t('settings.fields.pushoverUser'), key: 'PUSHOVER_USER', placeholder: 'User Key' },
    ]},
    { title: t('settings.sections.notifyEmail'), desc: t('settings.sections.notifyEmailDesc'), fields: [
      { label: t('settings.fields.smtpHost'), key: 'EMAIL_SMTP_HOST', placeholder: 'smtp.example.com', testChannel: 'Email' },
      { label: t('settings.fields.smtpPort'), key: 'EMAIL_SMTP_PORT', placeholder: '465 / 587' },
      { label: t('settings.fields.smtpUser'), key: 'EMAIL_SMTP_USER', placeholder: 'user@example.com' },
      { label: t('settings.fields.smtpPass'), key: 'EMAIL_SMTP_PASS', placeholder: '••••••••', type: 'password' },
      { label: t('settings.fields.smtpFrom'), key: 'EMAIL_SMTP_FROM', placeholder: '可选发件人，留空默认同用户名' },
      { label: t('settings.fields.smtpTo'), key: 'EMAIL_SMTP_TO', placeholder: '接收者邮箱，多个用逗号隔开' },
    ]},
    { title: t('settings.sections.notifyMatrix'), desc: t('settings.sections.notifyMatrixDesc'), fields: [
      { label: t('settings.fields.matrixHomeserver'), key: 'MATRIX_HOMESERVER', placeholder: 'https://matrix.org', testChannel: 'Matrix' },
      { label: t('settings.fields.matrixToken'), key: 'MATRIX_TOKEN', placeholder: 'syt_...', type: 'password' },
      { label: t('settings.fields.matrixRoomId'), key: 'MATRIX_ROOM_ID', placeholder: '!abc123:matrix.org' },
    ]},
    { title: t('settings.sections.notifyLine'), desc: t('settings.sections.notifyLineDesc'), fields: [
      { label: t('settings.fields.lineChannelToken'), key: 'LINE_CHANNEL_TOKEN', placeholder: 'Channel Access Token', type: 'password', testChannel: 'LINE' },
      { label: t('settings.fields.lineUserId'), key: 'LINE_USER_ID', placeholder: 'U1234567890abcdef...' },
    ]},
    { title: t('settings.sections.notifyWhatsapp'), desc: t('settings.sections.notifyWhatsappDesc'), fields: [
      { label: t('settings.fields.whatsappPhoneId'), key: 'WHATSAPP_PHONE_ID', placeholder: '123456789012345', testChannel: 'WhatsApp' },
      { label: t('settings.fields.whatsappToken'), key: 'WHATSAPP_TOKEN', placeholder: 'EAAB...', type: 'password' },
      { label: t('settings.fields.whatsappTo'), key: 'WHATSAPP_TO', placeholder: '8613800138000' },
    ]},
    { title: t('settings.sections.notifySignal'), desc: t('settings.sections.notifySignalDesc'), fields: [
      { label: t('settings.fields.signalApiUrl'), key: 'SIGNAL_API_URL', placeholder: 'http://localhost:8080', testChannel: 'Signal' },
      { label: t('settings.fields.signalSender'), key: 'SIGNAL_SENDER', placeholder: '+8613800138000' },
      { label: t('settings.fields.signalRecipients'), key: 'SIGNAL_RECIPIENTS', placeholder: '+8613800138000' },
    ]},
  ]},
  { key: 'customRegex', label: t('settings.sections.customRegex'), icon: FileText, sections: [] },
  { key: 'plugins', label: t('settings.tabs.plugins'), icon: Sparkles, sections: [] },
  { key: 'scheduler', label: t('settings.tabs.scheduler'), icon: Timer, sections: [
    { title: t('settings.sections.schedulerCycle'), desc: t('settings.sections.schedulerCycleDesc'), fields: [
      { label: t('settings.fields.rssInterval'), key: 'RSS_INTERVAL', placeholder: '15m' },
      { label: t('settings.fields.organizerInterval'), key: 'ORGANIZER_INTERVAL', placeholder: '2m' },
      { label: t('settings.fields.supplementInterval'), key: 'SUPPLEMENT_INTERVAL', placeholder: '12h' },
      { label: t('settings.fields.seedCleanupInterval'), key: 'SEED_CLEANUP_INTERVAL', placeholder: '1h' },
      { label: t('settings.fields.seedCleanupMinSeedTime'), key: 'SEED_CLEANUP_MIN_SEED_TIME', placeholder: '48h' },
      { label: t('settings.fields.seedCleanupMinRatio'), key: 'SEED_CLEANUP_MIN_RATIO', placeholder: '1.0' },
    ]}
  ]},
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
  m['AUTO_CHECK_UPDATE'] = { label: '自动检测更新', key: 'AUTO_CHECK_UPDATE', placeholder: '' }
  m['MCP_TOKEN'] = { label: 'MCP Token', key: 'MCP_TOKEN', placeholder: '' }
  return m
})

function isPluginActive(id: string): boolean {
  const p = pluginList.value.find(item => item.id === id)
  return p ? !!p.enabled : false
}

const currentTabSections = computed(() => {
  const currentTab = tabs.value.find(t => t.key === activeTab.value)
  if (!currentTab) return []
  if ((currentTab as any).pluginRequired && !isPluginActive((currentTab as any).pluginRequired)) {
    return []
  }
  return currentTab.sections.filter((s: any) => {
    if (s.pluginRequired) {
      return isPluginActive(s.pluginRequired)
    }
    return true
  })
})

watch(() => isPluginActive('extended-notifiers'), (active) => {
  if (!active && activeTab.value === 'notify') {
    activeTab.value = 'plugins'
  }
})

async function enableNotifyPlugin() {
  const p = pluginList.value.find(item => item.id === 'extended-notifiers')
  if (p) {
    p.enabled = false
    await togglePlugin(p)
  }
}

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

// MCP Server 状态与操作
const showMcpToken = ref(false)
const mcpGeneratingToken = ref(false)

const mcpEndpointUrl = computed(() => {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:20001'
  return `${origin}/mcp/sse`
})

const mcpClaudeConfigJson = computed(() => {
  const token = settings.value['MCP_TOKEN'] || 'YOUR_MCP_TOKEN'
  const config = {
    mcpServers: {
      'ani-go': {
        url: mcpEndpointUrl.value,
        headers: {
          Authorization: `Bearer ${token}`
        }
      }
    }
  }
  return JSON.stringify(config, null, 2)
})

async function generateMCPToken() {
  mcpGeneratingToken.value = true
  try {
    const { data } = await request.post('/mcp/token/generate')
    if (data?.token) {
      settings.value['MCP_TOKEN'] = data.token
      showMcpToken.value = true
      saved.value = true
      setTimeout(() => { saved.value = false }, 2500)
      if ((window as any).showToast) {
        (window as any).showToast(t('settings.mcp.tokenGenerated'), 'success')
      }
    }
  } catch (e: any) {
    const errMsg = e.response?.data?.error || t('settings.mcp.generateFailed')
    if ((window as any).showToast) {
      (window as any).showToast(errMsg, 'error')
    } else {
      error.value = errMsg
    }
  } finally {
    mcpGeneratingToken.value = false
  }
}

function copyText(text: string) {
  if (!text) return
  navigator.clipboard.writeText(text)
  saved.value = true
  setTimeout(() => { saved.value = false }, 2000)
  if ((window as any).showToast) {
    (window as any).showToast(t('settings.copied') || '已复制到剪贴板', 'success')
  }
}

function copyClaudeConfig() {
  copyText(mcpClaudeConfigJson.value)
}

function copyCursorConfig() {
  copyText(mcpClaudeConfigJson.value)
}

// 基础网络请求
async function fetchSettings() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.get('/settings')
    settings.value = (data as Record<string, string>) || {}
    if (settings.value['AUTO_CHECK_UPDATE'] === undefined) {
      settings.value['AUTO_CHECK_UPDATE'] = localStorage.getItem('ani-go-auto-update') !== 'false' ? 'true' : 'false'
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || '加载系统设置失败'
  } finally {
    loading.value = false
  }
}

// 自定义正则配置与测试
const customRegexList = ref<string[]>(Array(10).fill(''))
const customRegexCompiled = ref<string[]>([])
const customRegexLoading = ref(false)
const customRegexSaved = ref(false)
const regexTestTitle = ref('')
const regexTestResult = ref<any>(null)
const regexTesting = ref(false)

async function fetchCustomRegex() {
  try {
    const { data } = await request.get('/settings/custom-regex')
    if (data.patterns && Array.isArray(data.patterns)) {
      const list = Array(10).fill('')
      for (let i = 0; i < 10; i++) {
        list[i] = data.patterns[i] || ''
      }
      customRegexList.value = list
    }
    if (data.compiled && Array.isArray(data.compiled)) {
      customRegexCompiled.value = data.compiled
    }
  } catch (e: any) {
    console.error('获取自定义正则失败', e)
  }
}

async function saveCustomRegex() {
  customRegexLoading.value = true
  customRegexSaved.value = false
  error.value = ''
  try {
    const updates: Record<string, string> = {}
    for (let i = 0; i < 10; i++) {
      updates[`custom_regex_${i}`] = customRegexList.value[i] || ''
    }
    await request.put('/settings', { settings: updates })
    const { data } = await request.post('/settings/custom-regex/reload')
    if (data.compiled && Array.isArray(data.compiled)) {
      customRegexCompiled.value = data.compiled
    }
    customRegexSaved.value = true
    setTimeout(() => { customRegexSaved.value = false }, 3000)
  } catch (e: any) {
    error.value = e.response?.data?.error || '保存自定义正则失败'
  } finally {
    customRegexLoading.value = false
  }
}

async function runRegexTest() {
  if (!regexTestTitle.value.trim()) return
  regexTesting.value = true
  regexTestResult.value = null
  try {
    const { data } = await request.post('/settings/custom-regex/test', {
      title: regexTestTitle.value.trim()
    })
    regexTestResult.value = data
  } catch (e: any) {
    error.value = e.response?.data?.error || '测试解析失败'
  } finally {
    regexTesting.value = false
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
  for (let i = 0; i < 10; i++) {
    changed[`custom_regex_${i}`] = customRegexList.value[i] || ''
  }
  try {
    await request.put('/settings', { settings: changed })
    saved.value = true
    setTimeout(() => { saved.value = false }, 3000)
    fetchCustomRegex()
  } catch (e: any) {
    error.value = e.response?.data?.error || '保存设置失败'
  }
}

// 通知投递历史与健康度分析
const notifyLogs = ref<any[]>([])
const notifyLogsLoading = ref(false)
const notifyLogsTotal = ref(0)
const notifyLogsPage = ref(1)
const notifyLogsPageSize = ref(10)
const notifyFilterChannel = ref('all')
const notifyFilterStatus = ref('all')
const notifyStats = ref<{ total: number; success: number; failed: number; dlq: number; channel_stats: Record<string, { success: number; failed: number }> }>({
  total: 0, success: 0, failed: 0, dlq: 0, channel_stats: {}
})
const clearingLogs = ref(false)

async function fetchNotifyStats() {
  try {
    const { data } = await request.get('/notifications/stats')
    if (data) notifyStats.value = data
  } catch (e) {
    console.error('获取通知统计失败', e)
  }
}

async function fetchNotifyLogs() {
  notifyLogsLoading.value = true
  try {
    const { data } = await request.get('/notifications/logs', {
      params: {
        page: notifyLogsPage.value,
        pageSize: notifyLogsPageSize.value,
        channel: notifyFilterChannel.value === 'all' ? undefined : notifyFilterChannel.value,
        status: notifyFilterStatus.value === 'all' ? undefined : notifyFilterStatus.value,
      }
    })
    notifyLogs.value = data.logs || []
    notifyLogsTotal.value = data.total || 0
  } catch (e) {
    console.error('获取通知日志失败', e)
  } finally {
    notifyLogsLoading.value = false
  }
}

async function clearNotifyLogs() {
  if (!confirm('确定要清空全部通知投递历史记录吗？此操作不可撤销。')) return
  clearingLogs.value = true
  try {
    await request.delete('/notifications/logs')
    window.showToast?.('通知历史记录已清空', 'success')
    notifyLogsPage.value = 1
    await Promise.all([fetchNotifyLogs(), fetchNotifyStats()])
  } catch (e: any) {
    window.showToast?.(e.response?.data?.error || '清空通知记录失败', 'error')
  } finally {
    clearingLogs.value = false
  }
}

// RSS 实时源解析探针
const rssInspectUrl = ref('')
const rssInspectLoading = ref(false)
const rssInspectResult = ref<any>(null)
const rssInspectError = ref('')

async function runRSSInspect() {
  const url = rssInspectUrl.value.trim()
  if (!url) {
    rssInspectError.value = '请输入有效的 RSS URL'
    return
  }
  rssInspectLoading.value = true
  rssInspectError.value = ''
  rssInspectResult.value = null
  try {
    const { data } = await request.post('/rss/preview', { url }, { timeout: 18000 })
    if (data.success) {
      rssInspectResult.value = data
    } else {
      rssInspectError.value = data.error || '解析失败'
    }
  } catch (e: any) {
    rssInspectError.value = e.response?.data?.error || '拉取或解析 RSS 失败，请检查链接连通性'
  } finally {
    rssInspectLoading.value = false
  }
}

function fillMikanRSS() {
  const mikanUrl = getVal('MIKAN_RSS_URL')
  if (mikanUrl) {
    rssInspectUrl.value = mikanUrl
  } else {
    window.showToast?.('未配置个人 Mikan RSS 链接，可手动输入或粘贴字幕组 RSS', 'info')
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

// 下载器连接与健康探测
const testingDownloaderKey = ref<string | null>(null)
async function testDownloader(type: string) {
  testingDownloaderKey.value = type
  try {
    let payload: any = { type }
    if (type === 'qbittorrent') {
      payload.host = getVal('QB_HOST') || 'http://localhost:8081'
      payload.username = getVal('QB_USER') || ''
      payload.password = getVal('QB_PASS') || ''
      payload.category = getVal('QB_CATEGORY') || 'ani-go'
    } else if (type === 'transmission') {
      payload.host = getVal('TR_HOST') || 'http://localhost:9091'
      payload.username = getVal('TR_USER') || ''
      payload.password = getVal('TR_PASS') || ''
    } else if (type === 'aria2') {
      payload.host = getVal('ARIA2_HOST') || 'http://localhost:6800/jsonrpc'
      payload.secret = getVal('ARIA2_SECRET') || ''
    } else if (type === 'current') {
      payload = { type: '' }
    }

    const { data } = await request.post('/downloader/test', payload)
    if (data.success) {
      window.showToast?.(data.message || '下载器连接正常', 'success')
      saved.value = true
      setTimeout(() => { saved.value = false }, 3000)
    } else {
      window.showToast?.(data.error || '下载器连接失败', 'error')
      error.value = data.error || '下载器连接失败'
    }
  } catch (e: any) {
    const msg = e.response?.data?.error || '下载器测试请求失败'
    window.showToast?.(msg, 'error')
    error.value = msg
  } finally {
    testingDownloaderKey.value = null
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

// 数据迁移状态
const showMigrateModal = ref(false)
const migrateMode = ref<'upload' | 'path'>('upload')
const migrateFile = ref<File | null>(null)
const migratePath = ref('data/autobangumi.db')
const migrating = ref(false)
const migrateResult = ref<any>(null)
const migrateError = ref('')

function openMigrateModal() {
  migrateFile.value = null
  migratePath.value = 'data/autobangumi.db'
  migrateResult.value = null
  migrateError.value = ''
  showMigrateModal.value = true
}

function onMigrateFileSelect(e: any) {
  const file = e.target.files?.[0]
  if (file) {
    migrateFile.value = file
    migrateError.value = ''
  }
}

async function executeMigration() {
  migrating.value = true
  migrateError.value = ''
  migrateResult.value = null
  try {
    if (migrateMode.value === 'upload') {
      if (!migrateFile.value) {
        throw new Error('请先选择要上传的数据库文件 (.db / .sqlite)')
      }
      const formData = new FormData()
      formData.append('file', migrateFile.value)
      const { data } = await request.post('/migrate', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      migrateResult.value = data.stats || data
      window.showToast?.(t('settings.migrate.resultTitle'), 'success')
      await fetchPlugins()
    } else {
      if (!migratePath.value.trim()) {
        throw new Error('请输入源数据库文件路径')
      }
      const { data } = await request.post('/migrate', { source_path: migratePath.value.trim() })
      migrateResult.value = data.stats || data
      window.showToast?.(t('settings.migrate.resultTitle'), 'success')
      await fetchPlugins()
    }
  } catch (e: any) {
    migrateError.value = e.response?.data?.error || e.message || '迁移失败，请检查文件格式与权限'
    window.showToast?.(migrateError.value, 'error')
  } finally {
    migrating.value = false
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
  } else if (newTab === 'customRegex') {
    fetchCustomRegex()
  } else if (newTab === 'notify') {
    fetchNotifyStats()
    fetchNotifyLogs()
  }
})

onMounted(() => {
  fetchSettings()
  fetchPlugins()
  fetchBackupList()
  fetchLogs()
  fetchCustomRegex()
  if (activeTab.value === 'notify') {
    fetchNotifyStats()
    fetchNotifyLogs()
  }
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

        <!-- 2.5 AI Token 成本防爆与调用安全警示横幅 -->
        <div 
          v-if="activeTab === 'ai'" 
          class="w-full bg-warning/10 border border-warning/30 rounded-3xl p-5 text-left space-y-2.5 shadow-sm block"
        >
          <div class="flex items-center gap-2 font-black text-sm text-warning">
            <AlertCircle :size="18" />
            <span>{{ $t('settings.ai.warningTitle') }}</span>
          </div>
          <p class="text-xs opacity-80 leading-relaxed">
            {{ $t('settings.ai.warningDesc') }}
          </p>
          <div class="flex flex-wrap items-center gap-2 pt-1">
            <span class="badge badge-sm badge-warning badge-outline font-mono text-[11px]">{{ $t('settings.ai.badgeRecommend') }}: DeepSeek-V3 / Gemini 2.0 Flash / Ollama</span>
            <span class="badge badge-sm badge-warning badge-outline font-mono text-[11px]">{{ $t('settings.ai.badgeProtection') }}</span>
          </div>
        </div>

        <!-- 3. 标准化表单渲染区块 (适用常规 Sections，垂直排列) -->
        <div 
          v-for="section in currentTabSections" 
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
                <button 
                  v-if="field.testDownloader" 
                  type="button"
                  :disabled="testingDownloaderKey === field.testDownloader"
                  class="btn btn-ghost btn-xs text-primary font-bold gap-1 px-2 py-0 h-6 min-h-0 rounded-lg hover:bg-primary/10" 
                  @click.prevent="testDownloader(field.testDownloader)"
                >
                  <RefreshCw v-if="testingDownloaderKey === field.testDownloader" :size="10" class="animate-spin" />
                  <span>{{ testingDownloaderKey === field.testDownloader ? $t('settings.testing') : $t('settings.testNow') }}</span>
                </button>
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

        <!-- MCP Server 外部控制与模型上下文协议卡片 (activeTab === 'ai') -->
        <div 
          v-if="activeTab === 'ai'" 
          class="w-full bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-6 block text-left"
        >
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-2xl bg-secondary/10 text-secondary flex items-center justify-center shadow-inner">
                <Cpu :size="20" />
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="text-base font-black tracking-tight">{{ $t('settings.mcp.title') }}</h3>
                  <span class="badge badge-sm badge-secondary font-mono text-[10px] font-bold">SSE Protocol</span>
                </div>
                <p class="text-xs opacity-50 leading-normal">{{ $t('settings.mcp.desc') }}</p>
              </div>
            </div>

            <button 
              @click="generateMCPToken" 
              :disabled="mcpGeneratingToken"
              class="btn btn-outline btn-secondary btn-sm rounded-xl gap-2 px-5 hover:scale-[1.02] active:scale-95 transition-all"
            >
              <RefreshCw v-if="mcpGeneratingToken" :size="14" class="animate-spin" />
              <Sparkles v-else :size="14" />
              <span class="text-xs font-bold">{{ settings['MCP_TOKEN'] ? $t('settings.mcp.regenToken') : $t('settings.mcp.generateToken') }}</span>
            </button>
          </div>

          <!-- SSE Endpoint 与 Token 展示 -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="space-y-1.5">
              <label class="text-xs font-bold text-base-content/75 flex items-center justify-between">
                <span>{{ $t('settings.mcp.endpointLabel') }}</span>
                <span class="text-[10px] text-success font-semibold">GET (SSE)</span>
              </label>
              <div class="join w-full">
                <input 
                  type="text" 
                  readonly 
                  :value="mcpEndpointUrl" 
                  class="input input-bordered join-item w-full text-xs font-mono bg-base-200/50" 
                />
                <button 
                  @click="copyText(mcpEndpointUrl)" 
                  class="btn join-item btn-square btn-outline border-base-300"
                  :title="$t('settings.mcp.copyTooltip')"
                >
                  <Copy :size="14" />
                </button>
              </div>
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-bold text-base-content/75 flex items-center justify-between">
                <span>{{ $t('settings.mcp.tokenLabel') }}</span>
                <span v-if="settings['MCP_TOKEN']" class="text-[10px] text-success font-semibold">Bearer Auth</span>
                <span v-else class="text-[10px] text-warning font-semibold">{{ $t('settings.mcp.notConfigured') }}</span>
              </label>
              <div class="join w-full">
                <input 
                  :type="showMcpToken ? 'text' : 'password'" 
                  readonly 
                  :value="settings['MCP_TOKEN'] || ''" 
                  :placeholder="$t('settings.mcp.tokenPlaceholder')"
                  class="input input-bordered join-item w-full text-xs font-mono bg-base-200/50" 
                />
                <button 
                  @click="showMcpToken = !showMcpToken" 
                  class="btn join-item btn-square btn-outline border-base-300"
                >
                  <EyeOff v-if="showMcpToken" :size="14" />
                  <Eye v-else :size="14" />
                </button>
                <button 
                  @click="copyText(settings['MCP_TOKEN'] || '')" 
                  :disabled="!settings['MCP_TOKEN']"
                  class="btn join-item btn-square btn-outline border-base-300"
                  :title="$t('settings.mcp.copyTooltip')"
                >
                  <Copy :size="14" />
                </button>
              </div>
            </div>
          </div>

          <!-- 一键复制客户端配置文件 -->
          <div class="space-y-2 bg-base-200/40 p-4 rounded-2xl border border-base-200">
            <div class="flex items-center justify-between">
              <span class="text-xs font-bold text-base-content/80 flex items-center gap-2">
                <FileText :size="14" />
                <span>{{ $t('settings.mcp.clientConfigTitle') }}</span>
              </span>
              <div class="flex items-center gap-2">
                <button 
                  @click="copyClaudeConfig" 
                  :disabled="!settings['MCP_TOKEN']"
                  class="btn btn-xs btn-primary rounded-lg gap-1.5 font-bold"
                >
                  <Copy :size="12" />
                  <span>{{ $t('settings.mcp.copyClaudeConfig') }}</span>
                </button>
                <button 
                  @click="copyCursorConfig" 
                  :disabled="!settings['MCP_TOKEN']"
                  class="btn btn-xs btn-ghost border border-base-300 rounded-lg gap-1.5 font-bold"
                >
                  <Copy :size="12" />
                  <span>{{ $t('settings.mcp.copyCursorConfig') }}</span>
                </button>
              </div>
            </div>
            <pre class="text-[11px] font-mono bg-base-300/40 p-3 rounded-xl overflow-x-auto text-base-content/80 select-all leading-relaxed">{{ mcpClaudeConfigJson }}</pre>
            <p class="text-[10px] text-base-content/50">{{ $t('settings.mcp.clientConfigHint') }}</p>
          </div>
        </div>

        <!-- 通知投递日志与送达监控看板 (Notify Logs & Delivery Matrix) -->
        <div v-if="activeTab === 'notify' && isPluginActive('extended-notifiers')" class="w-full bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shadow-inner">
                <Activity :size="20" />
              </div>
              <div>
                <h3 class="text-base font-black tracking-tight flex items-center gap-2">
                  <span>通知投递履历与送达监控</span>
                  <span class="badge badge-primary badge-sm font-mono font-bold">{{ notifyLogsTotal }} 条记录</span>
                  <span class="badge badge-outline badge-xs opacity-70">消息通知推送插件驱动</span>
                </h3>
                <p class="text-xs opacity-50 mt-0.5">内建插件「消息通知推送 (2.0.0)」统一纳管 · 实时跟踪全系统 16 种推送渠道的发送流水、重试与死信隔离</p>
              </div>
            </div>

            <div class="flex items-center gap-2 flex-wrap">
              <button 
                class="btn btn-ghost btn-sm rounded-xl gap-1.5 border border-base-300/80 text-xs font-bold"
                @click="fetchNotifyLogs(); fetchNotifyStats()"
                :disabled="notifyLogsLoading"
              >
                <RefreshCw :size="14" :class="{ 'animate-spin': notifyLogsLoading }" />
                <span>刷新流水</span>
              </button>
              <button 
                class="btn btn-error btn-outline btn-sm rounded-xl gap-1.5 text-xs font-bold"
                @click="clearNotifyLogs"
                :disabled="clearingLogs || notifyLogsTotal === 0"
              >
                <Trash2 :size="14" />
                <span>清空记录</span>
              </button>
            </div>
          </div>

          <!-- 统计概览指标卡 -->
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-3.5">
            <div class="p-4 rounded-2xl bg-base-200/40 border border-base-300/60 space-y-1">
              <span class="text-[11px] font-bold opacity-50 uppercase tracking-wider block">累计总投递</span>
              <div class="text-xl font-black font-mono text-base-content">{{ notifyStats.total }}</div>
            </div>

            <div class="p-4 rounded-2xl bg-success/10 border border-success/20 space-y-1">
              <span class="text-[11px] font-bold text-success uppercase tracking-wider block">送达成功率</span>
              <div class="text-xl font-black font-mono text-success">
                {{ notifyStats.total > 0 ? ((notifyStats.success / notifyStats.total) * 100).toFixed(1) : '100' }}%
              </div>
            </div>

            <div class="p-4 rounded-2xl bg-error/10 border border-error/20 space-y-1">
              <span class="text-[11px] font-bold text-error uppercase tracking-wider block">失败次数</span>
              <div class="text-xl font-black font-mono text-error">{{ notifyStats.failed }}</div>
            </div>

            <div class="p-4 rounded-2xl bg-secondary/10 border border-secondary/20 space-y-1">
              <span class="text-[11px] font-bold text-secondary uppercase tracking-wider block">死信隔离 (DLQ)</span>
              <div class="text-xl font-black font-mono text-secondary">{{ notifyStats.dlq }}</div>
            </div>
          </div>

          <!-- 筛选过滤器 -->
          <div class="flex flex-wrap items-center justify-between gap-3 pt-2">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-xs font-bold text-base-content/60 flex items-center gap-1">
                <Filter :size="13" /> 筛选:
              </span>
              <select 
                v-model="notifyFilterStatus" 
                @change="notifyLogsPage = 1; fetchNotifyLogs()"
                class="select select-bordered select-xs rounded-lg font-bold"
              >
                <option value="all">全部状态</option>
                <option value="success">仅成功 (Success)</option>
                <option value="failed">仅失败 (Failed)</option>
                <option value="dlq">仅死信 (DLQ)</option>
              </select>

              <select 
                v-model="notifyFilterChannel" 
                @change="notifyLogsPage = 1; fetchNotifyLogs()"
                class="select select-bordered select-xs rounded-lg font-bold"
              >
                <option value="all">全部渠道</option>
                <option value="Telegram">Telegram</option>
                <option value="WeChat">企业微信</option>
                <option value="DingTalk">钉钉</option>
                <option value="Feishu">飞书</option>
                <option value="Bark">Bark</option>
                <option value="ServerChan">Server酱</option>
                <option value="Discord">Discord</option>
                <option value="Slack">Slack</option>
                <option value="Gotify">Gotify</option>
                <option value="Ntfy">Ntfy</option>
                <option value="Email">邮件 (Email)</option>
                <option value="PushPlus">PushPlus</option>
                <option value="PushDeer">PushDeer</option>
                <option value="Qmsg">Qmsg酱</option>
                <option value="Pushover">Pushover</option>
                <option value="Matrix">Matrix</option>
              </select>
            </div>

            <!-- 分页控件 -->
            <div class="flex items-center gap-2 text-xs font-bold">
              <button 
                class="btn btn-ghost btn-xs border border-base-300"
                @click="if (notifyLogsPage > 1) { notifyLogsPage--; fetchNotifyLogs() }"
                :disabled="notifyLogsPage <= 1 || notifyLogsLoading"
              >
                上一页
              </button>
              <span class="text-base-content/60 font-mono">
                {{ notifyLogsPage }} / {{ Math.ceil(notifyLogsTotal / notifyLogsPageSize) || 1 }}
              </span>
              <button 
                class="btn btn-ghost btn-xs border border-base-300"
                @click="if (notifyLogsPage * notifyLogsPageSize < notifyLogsTotal) { notifyLogsPage++; fetchNotifyLogs() }"
                :disabled="notifyLogsPage * notifyLogsPageSize >= notifyLogsTotal || notifyLogsLoading"
              >
                下一页
              </button>
            </div>
          </div>

          <!-- 流水日志列表 -->
          <div v-if="notifyLogsLoading" class="py-12 flex justify-center">
            <span class="loading loading-spinner loading-md text-primary"></span>
          </div>

          <div v-else-if="notifyLogs.length === 0" class="text-center py-10 border border-dashed border-base-300 rounded-2xl text-xs text-base-content/50 space-y-1">
            <p class="font-bold">暂无投递流水记录</p>
            <p class="text-[11px] opacity-70">当后台触发真实下载、入库事件或点击测试时，此处将实时生成详细日志</p>
          </div>

          <div v-else class="space-y-2.5">
            <div 
              v-for="log in notifyLogs" 
              :key="log.ID" 
              class="p-3.5 rounded-2xl border bg-base-200/30 border-base-300/70 space-y-2 text-xs transition-all hover:bg-base-200/60"
            >
              <div class="flex items-center justify-between gap-2 flex-wrap">
                <div class="flex items-center gap-2 flex-wrap">
                  <span 
                    class="badge badge-sm font-bold capitalize"
                    :class="{
                      'badge-success text-success-content': log.status === 'success',
                      'badge-error text-error-content': log.status === 'failed',
                      'badge-secondary text-secondary-content': log.status === 'dlq'
                    }"
                  >
                    {{ log.status === 'success' ? '送达成功' : (log.status === 'failed' ? '投递失败' : '死信丢弃') }}
                  </span>

                  <span class="badge badge-outline badge-sm font-mono font-bold">{{ log.channel }}</span>

                  <span class="badge badge-ghost badge-sm text-[10px]">
                    {{ log.event_type === 'manual.test' ? '自检测试' : (log.event_type === 'download.started' ? '开始下载' : (log.event_type === 'download.completed' ? '下载完成' : (log.event_type === 'file.organized' ? '媒体整理' : log.event_type))) }}
                  </span>

                  <span class="font-bold text-base-content/90">{{ log.title }}</span>
                </div>

                <div class="flex items-center gap-1.5 text-[11px] font-mono opacity-50">
                  <Clock :size="12" />
                  <span>{{ new Date(log.CreatedAt).toLocaleString() }}</span>
                </div>
              </div>

              <div class="text-base-content/75 font-mono text-[11px] break-all bg-base-100 p-2.5 rounded-xl border border-base-200/80 leading-relaxed">
                {{ log.content }}
              </div>

              <div v-if="log.error_msg" class="text-error font-mono text-[11px] bg-error/10 p-2 rounded-xl border border-error/20 flex items-start gap-1.5">
                <AlertCircle :size="14" class="shrink-0 mt-0.5" />
                <span>{{ log.error_msg }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 消息通知推送插件未启用占位卡片 -->
        <div 
          v-if="activeTab === 'notify' && !isPluginActive('extended-notifiers')" 
          class="w-full bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-10 flex flex-col items-center justify-center text-center space-y-4"
        >
          <div class="w-16 h-16 rounded-3xl bg-base-200/80 text-base-content/40 flex items-center justify-center">
            <Bell :size="32" />
          </div>
          <div class="space-y-1.5 max-w-md">
            <h4 class="text-base font-black text-base-content">消息通知推送插件已停用</h4>
            <p class="text-xs text-base-content/60 leading-relaxed">
              消息通知推送功能已整体模块化为插件管理。当前插件处于停用状态，全部推送渠道设置已收起，后台通知监听已暂停。
            </p>
          </div>
          <div class="pt-2 flex items-center gap-3">
            <button 
              @click="enableNotifyPlugin" 
              class="btn btn-sm btn-primary rounded-xl px-5 text-xs font-bold gap-2 shadow-md shadow-primary/20"
            >
              <Sparkles :size="14" />
              立即启用插件
            </button>
            <button 
              @click="activeTab = 'plugins'" 
              class="btn btn-sm btn-ghost border border-base-300 rounded-xl px-4 text-xs font-bold"
            >
              前往插件管理
            </button>
          </div>
        </div>

        <!-- 3.5. 自定义标题正则管理与沙箱测试 (Custom Regex Tab) -->
        <div v-if="activeTab === 'customRegex'" class="space-y-6">
          <div class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-6">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="space-y-1">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-2xl bg-primary/10 text-primary flex items-center justify-center">
                    <FileText :size="20" />
                  </div>
                  <div>
                    <h3 class="text-base font-black tracking-tight flex items-center gap-2">
                      <span>{{ $t('settings.customRegexTab.title') }}</span>
                      <span class="badge badge-primary badge-sm font-mono font-bold">{{ $t('settings.customRegexTab.compiledCount', { count: customRegexCompiled.length }) }}</span>
                    </h3>
                    <p class="text-xs opacity-50 mt-0.5">{{ $t('settings.customRegexTab.desc') }}</p>
                  </div>
                </div>
              </div>

              <div class="flex items-center gap-2">
                <button 
                  class="btn btn-primary btn-sm rounded-xl px-5 gap-2 shadow-md shadow-primary/20" 
                  @click="saveCustomRegex" 
                  :disabled="customRegexLoading"
                >
                  <RefreshCw v-if="customRegexLoading" :size="14" class="animate-spin" />
                  <Check v-else :size="14" />
                  <span class="text-xs font-bold">{{ customRegexSaved ? $t('settings.customRegexTab.savedSuccess') : $t('settings.customRegexTab.saveAndReload') }}</span>
                </button>
              </div>
            </div>

            <!-- 10 个正则规则槽位网格 -->
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div 
                v-for="index in 10" 
                :key="index - 1" 
                class="p-4 rounded-2xl border bg-base-200/20 border-base-300/80 space-y-2"
              >
                <div class="flex items-center justify-between">
                  <label class="text-xs font-bold text-base-content/80 flex items-center gap-2">
                    <span class="badge badge-ghost badge-xs font-mono font-bold">#{{ index - 1 }}</span>
                    <span>{{ $t('settings.customRegexTab.slotLabel', { index: index - 1 }) }}</span>
                  </label>
                  <span v-if="customRegexList[index - 1]?.trim()" class="badge badge-success badge-xs gap-1 font-bold">
                    <span class="w-1.5 h-1.5 rounded-full bg-success-content"></span>已配置
                  </span>
                </div>
                <input 
                  type="text" 
                  v-model="customRegexList[index - 1]" 
                  :placeholder="$t('settings.customRegexTab.slotPlaceholder')"
                  class="input input-bordered w-full rounded-xl text-xs font-mono font-medium focus:border-primary"
                />
              </div>
            </div>
          </div>

          <!-- 实时正则匹配沙箱测试卡片 -->
          <div class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-5">
            <div class="space-y-1">
              <h4 class="text-base font-black tracking-tight flex items-center gap-2">
                <Play :size="18" class="text-primary fill-primary" />
                <span>{{ $t('settings.customRegexTab.testTitle') }}</span>
              </h4>
              <p class="text-xs opacity-50">{{ $t('settings.customRegexTab.testDesc') }}</p>
            </div>

            <div class="flex flex-col sm:flex-row gap-3">
              <input 
                type="text" 
                v-model="regexTestTitle" 
                :placeholder="$t('settings.customRegexTab.testInputPlaceholder')" 
                class="input input-bordered flex-1 rounded-xl text-xs font-mono font-medium focus:border-primary"
                @keydown.enter="runRegexTest"
              />
              <button 
                class="btn btn-primary btn-sm sm:btn-md rounded-xl px-6 gap-2 shrink-0 font-bold" 
                @click="runRegexTest" 
                :disabled="regexTesting || !regexTestTitle.trim()"
              >
                <RefreshCw v-if="regexTesting" :size="15" class="animate-spin" />
                <Play v-else :size="15" class="fill-current" />
                <span>{{ regexTesting ? $t('settings.customRegexTab.testing') : $t('settings.customRegexTab.testBtn') }}</span>
              </button>
            </div>

            <!-- 测试解析结果面板 -->
            <div v-if="regexTestResult" class="p-5 rounded-2xl bg-base-200/40 border border-base-300 space-y-4 animate-in fade-in">
              <div class="flex items-center justify-between border-b border-base-300/60 pb-3">
                <span class="text-xs font-bold text-base-content/70">{{ $t('settings.customRegexTab.parseResult') }}</span>
                <span class="badge badge-primary badge-sm font-mono font-bold">解析完成</span>
              </div>
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                <div class="p-3 rounded-xl bg-base-100 border border-base-200/80 space-y-1 col-span-2">
                  <span class="text-[11px] opacity-50 font-medium">{{ $t('settings.customRegexTab.animeTitle') }}</span>
                  <div class="font-bold font-mono truncate" :title="regexTestResult.title">{{ regexTestResult.title || '-' }}</div>
                </div>
                <div class="p-3 rounded-xl bg-base-100 border border-base-200/80 space-y-1">
                  <span class="text-[11px] opacity-50 font-medium">{{ $t('settings.customRegexTab.subgroup') }}</span>
                  <div class="font-bold font-mono truncate">{{ regexTestResult.subgroup || '未识别' }}</div>
                </div>
                <div class="p-3 rounded-xl bg-base-100 border border-base-200/80 space-y-1">
                  <span class="text-[11px] opacity-50 font-medium">{{ $t('settings.customRegexTab.resolution') }}</span>
                  <div class="font-bold font-mono text-primary">{{ regexTestResult.resolution || '未知' }}</div>
                </div>
                <div class="p-3 rounded-xl bg-base-100 border border-base-200/80 space-y-1">
                  <span class="text-[11px] opacity-50 font-medium">{{ $t('settings.customRegexTab.season') }}</span>
                  <div class="font-black font-mono text-base text-success">S{{ regexTestResult.season }}</div>
                </div>
                <div class="p-3 rounded-xl bg-base-100 border border-base-200/80 space-y-1">
                  <span class="text-[11px] opacity-50 font-medium">{{ $t('settings.customRegexTab.episode') }}</span>
                  <div class="font-black font-mono text-base text-primary">E{{ regexTestResult.episode }}</div>
                </div>
                <div class="p-3 rounded-xl bg-base-100 border border-base-200/80 space-y-1">
                  <span class="text-[11px] opacity-50 font-medium">{{ $t('settings.customRegexTab.isBatch') }}</span>
                  <div class="font-bold" :class="regexTestResult.is_batch ? 'text-warning' : 'opacity-60'">
                    {{ regexTestResult.is_batch ? '合集 / Batch' : '单集' }}
                  </div>
                </div>
                <div class="p-3 rounded-xl bg-base-100 border border-base-200/80 space-y-1">
                  <span class="text-[11px] opacity-50 font-medium">{{ $t('settings.customRegexTab.isSpecial') }}</span>
                  <div class="font-bold" :class="regexTestResult.is_special ? 'text-secondary' : 'opacity-60'">
                    {{ regexTestResult.is_special ? 'SP / OVA' : '常规剧集' }}
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- RSS 实时源深度探针与解析校验 (RSS Feed Inspector) -->
          <div class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-5">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-2xl bg-secondary/10 text-secondary flex items-center justify-center shadow-inner">
                  <Antenna :size="20" />
                </div>
                <div>
                  <h3 class="text-base font-black tracking-tight">RSS 实时源解析与正则特征探针</h3>
                  <p class="text-xs opacity-50 mt-0.5">实时在线拉取任意 Mikan、Nyaa 或字幕组订阅源，预览条目与正则提取效果</p>
                </div>
              </div>

              <button 
                class="btn btn-ghost btn-sm rounded-xl border border-base-300 text-xs font-bold gap-1.5"
                @click="fillMikanRSS"
              >
                <span>自动填入我的 Mikan RSS</span>
              </button>
            </div>

            <div class="flex flex-col sm:flex-row gap-3">
              <input 
                type="text" 
                v-model="rssInspectUrl" 
                placeholder="输入或粘贴 RSS URL (如 https://mikanani.me/RSS/Bangumi?bangumiId=...)"
                class="input input-bordered flex-1 rounded-xl text-xs font-mono font-medium focus:border-secondary"
                @keydown.enter="runRSSInspect"
              />
              <button 
                class="btn btn-secondary btn-sm sm:btn-md rounded-xl px-6 gap-2 shrink-0 font-bold" 
                @click="runRSSInspect" 
                :disabled="rssInspectLoading || !rssInspectUrl.trim()"
              >
                <RefreshCw v-if="rssInspectLoading" :size="15" class="animate-spin" />
                <Play v-else :size="15" class="fill-current" />
                <span>{{ rssInspectLoading ? '正在探查中...' : '开始探查' }}</span>
              </button>
            </div>

            <!-- 探查错误展示 -->
            <div v-if="rssInspectError" class="alert bg-error/15 border border-error/30 text-error rounded-2xl p-3 text-xs flex items-center gap-2">
              <AlertCircle :size="16" />
              <span>{{ rssInspectError }}</span>
            </div>

            <!-- 探查解析结果 -->
            <div v-if="rssInspectResult" class="space-y-4 pt-2 animate-in fade-in">
              <div class="flex items-center justify-between p-3.5 rounded-2xl bg-secondary/10 border border-secondary/20">
                <div class="space-y-0.5">
                  <div class="text-xs font-black text-secondary">{{ rssInspectResult.feed?.title || 'RSS 订阅源' }}</div>
                  <div class="text-[10px] font-mono opacity-60 truncate max-w-md">{{ rssInspectResult.feed?.url }}</div>
                </div>
                <span class="badge badge-secondary badge-sm font-mono font-bold">共 {{ rssInspectResult.total }} 条</span>
              </div>

              <div class="space-y-3 max-h-[460px] overflow-y-auto pr-1">
                <div 
                  v-for="(item, idx) in rssInspectResult.items" 
                  :key="idx" 
                  class="p-4 rounded-2xl border bg-base-200/30 border-base-300/70 space-y-2.5 text-xs"
                >
                  <div class="font-bold text-base-content leading-snug font-mono text-xs">
                    {{ item.title }}
                  </div>

                  <div class="flex flex-wrap items-center gap-1.5">
                    <span v-if="item.parsed_title" class="badge badge-primary badge-sm font-bold">
                      {{ item.parsed_title }}
                    </span>
                    <span v-if="item.episode > 0" class="badge badge-info badge-sm font-mono font-bold">
                      E{{ item.episode }}
                    </span>
                    <span v-if="item.season > 0" class="badge badge-ghost badge-sm font-mono font-bold">
                      S{{ item.season }}
                    </span>
                    <span v-if="item.subgroup" class="badge badge-neutral badge-sm font-mono">
                      {{ item.subgroup }}
                    </span>
                    <span v-if="item.resolution" class="badge badge-outline badge-sm font-mono">
                      {{ item.resolution }}
                    </span>
                    <span v-if="item.is_batch" class="badge badge-warning badge-sm">合集</span>
                    <span v-if="item.is_special" class="badge badge-secondary badge-sm">特别篇/OVA</span>
                  </div>

                  <div class="flex items-center justify-between text-[11px] opacity-60 font-mono pt-1 border-t border-base-300/40">
                    <span>发布时间: {{ item.pub_date }}</span>
                    <span>大小: {{ (item.size / 1e6).toFixed(1) }} MB</span>
                  </div>
                </div>
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
                        <span class="badge badge-xs text-[9px] font-bold" :class="plugin.enabled ? 'badge-success text-success-content' : 'badge-ghost opacity-60'">
                          {{ plugin.enabled ? '运行中' : '已停用' }}
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
                <!-- 插件专属操作入口 (仅在启用时呈现对应操作，停用时明确标识休眠) -->
                <div v-if="plugin.enabled">
                  <div v-if="plugin.id === 'data-migrate'" class="pt-2 flex items-center justify-between gap-2 border-t border-base-content/5">
                    <span class="text-[10px] opacity-60">从 AutoBangumi 或 ani-rss 迁移现有数据</span>
                    <button class="btn btn-primary btn-xs rounded-xl gap-1 font-bold shadow-sm" @click="openMigrateModal">
                      <Upload :size="12" />
                      <span>{{ $t('settings.plugins.migrateBtn') }}</span>
                    </button>
                  </div>
                  <div v-else-if="plugin.id === 'extended-notifiers'" class="pt-2.5 space-y-2 border-t border-base-content/5">
                    <div class="flex items-center justify-between gap-2 flex-wrap text-[11px]">
                      <span class="opacity-70 font-medium">驱动全系统 16 种推送渠道及队列监控</span>
                      <div class="flex items-center gap-1.5 text-[10px] font-mono">
                        <span class="badge badge-ghost badge-sm">累计: {{ notifyStats.total }}</span>
                        <span class="badge badge-sm" :class="notifyStats.failed > 0 ? 'badge-warning' : 'badge-success'">
                          送达率: {{ notifyStats.total > 0 ? ((notifyStats.success / notifyStats.total) * 100).toFixed(1) : '100' }}%
                        </span>
                      </div>
                    </div>
                    <div class="flex items-center justify-end gap-2">
                      <button class="btn btn-outline btn-xs rounded-xl gap-1 font-bold" @click="activeTab = 'notify'">
                        <Activity :size="12" />
                        <span>投递监控与履历</span>
                      </button>
                      <button class="btn btn-primary btn-xs rounded-xl gap-1 font-bold shadow-sm" @click="activeTab = 'notify'">
                        <Bell :size="12" />
                        <span>通道详细设置</span>
                      </button>
                    </div>
                  </div>
                  <div v-else-if="plugin.id === 'yuc_schedule'" class="pt-2 flex items-center justify-between gap-2 border-t border-base-content/5">
                    <span class="text-[10px] opacity-60">長門番堂新番时间表已激活</span>
                    <router-link to="/schedule" class="btn btn-ghost btn-xs rounded-xl gap-1 border border-base-300 font-bold">
                      <Calendar :size="12" />
                      <span>查看时间表</span>
                    </router-link>
                  </div>
                  <div v-else class="pt-2 flex items-center justify-between gap-2 border-t border-base-content/5">
                    <span class="text-[10px] opacity-60 flex items-center gap-1.5 font-medium">
                      <span class="w-1.5 h-1.5 rounded-full bg-success"></span>
                      <span>后台服务运行中</span>
                    </span>
                    <span class="badge badge-success badge-outline badge-xs text-[9px] font-bold">已启用</span>
                  </div>
                </div>
                <div v-else class="pt-2 flex items-center justify-between gap-2 border-t border-base-content/5 opacity-50">
                  <span class="text-[10px] opacity-60 italic">插件已停用，设置项已收起，后台推送已休眠</span>
                  <span class="badge badge-ghost badge-xs text-[9px]">已停用</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 5. 数据归档与恢复 (Backup Tab) -->
        <div v-if="activeTab === 'backup'" class="space-y-6">
          <!-- 数据导入与迁移入口 -->
          <div v-if="isPluginActive('data-migrate')" class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-4">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div>
                <h3 class="text-base font-black tracking-tight flex items-center gap-2">
                  <Sparkles class="text-primary" :size="18" />
                  {{ $t('settings.migrate.title') }}
                </h3>
                <p class="text-xs opacity-50 mt-0.5">{{ $t('settings.migrate.desc') }}</p>
              </div>
              <button 
                @click="openMigrateModal" 
                class="btn btn-primary btn-sm rounded-xl px-5 gap-1.5 shadow-md"
              >
                <Upload :size="14" />
                <span class="text-xs font-bold">{{ $t('settings.plugins.migrateBtn') }}</span>
              </button>
            </div>
          </div>

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

          <!-- 系统版本与在线更新设置卡片 -->
          <div class="bg-base-100 rounded-3xl border border-base-200/80 shadow-sm p-6 sm:p-7 space-y-6">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-base font-black tracking-tight flex items-center gap-2">
                  <Zap class="text-primary" :size="18" />
                  系统版本与在线更新
                </h3>
                <p class="text-xs opacity-50 mt-0.5">检测并获取 Ani-Go 官方 GitHub 最新发布版本与更新日志</p>
              </div>
              <span class="badge badge-neutral text-xs font-mono font-bold">{{ currentVersion }}</span>
            </div>

            <!-- 当前版本信息与快速检查 -->
            <div class="p-5 rounded-2xl bg-base-200/40 border border-base-200/70 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="space-y-1">
                <div class="flex items-center gap-2">
                  <span class="text-xs font-black">当前安装版本：</span>
                  <span class="font-mono text-xs font-bold text-primary">{{ currentVersion }}</span>
                  <span v-if="hasNewVersion" class="badge badge-error badge-xs font-bold animate-pulse">发现新版本 {{ latestVersion }}</span>
                  <span v-else-if="manualCheckResult === 'latest'" class="badge badge-success badge-xs font-bold">已是最新</span>
                </div>
                <p class="text-[11px] opacity-60">
                  官方主仓：<a href="https://github.com/xiaoyueRX/Ani-Go/releases" target="_blank" rel="noopener noreferrer" class="hover:underline text-primary">github.com/xiaoyueRX/Ani-Go</a>
                </p>
              </div>

              <div class="flex items-center gap-2">
                <a v-if="hasNewVersion" :href="`https://github.com/xiaoyueRX/Ani-Go/releases/tag/${latestVersion}`" target="_blank" rel="noopener noreferrer" class="btn btn-error btn-sm rounded-xl gap-1.5 font-bold shadow-md shadow-error/20">
                  <ExternalLink :size="14" />
                  <span>前往下载 {{ latestVersion }}</span>
                </a>
                <button 
                  type="button" 
                  @click="triggerManualCheckUpdate" 
                  :disabled="checkingUpdate"
                  class="btn btn-outline btn-sm rounded-xl gap-2 font-bold hover:bg-primary hover:text-primary-content hover:border-primary transition-all"
                >
                  <RefreshCw :size="14" :class="{ 'animate-spin': checkingUpdate }" />
                  <span>{{ checkingUpdate ? '正在检测...' : '立即检查新版本' }}</span>
                </button>
              </div>
            </div>

            <p v-if="manualCheckMsg" class="text-xs font-bold p-3 rounded-xl border" :class="manualCheckMsgType === 'success' ? 'bg-success/10 text-success border-success/20' : 'bg-error/10 text-error border-error/20'">
              {{ manualCheckMsg }}
            </p>

            <!-- 自动检测更新开关 -->
            <div class="flex items-center justify-between p-4 rounded-2xl bg-base-200/30 border border-base-200/50">
              <div class="space-y-0.5">
                <label class="text-xs font-bold text-base-content/80 cursor-pointer" for="auto-check-update-toggle">自动检测最新版本 (AUTO_CHECK_UPDATE)</label>
                <p class="text-[11px] opacity-50">开启后，系统在页面打开与登录时将自动查询 GitHub Releases，并在侧边栏提示更新</p>
              </div>
              <input 
                id="auto-check-update-toggle" 
                type="checkbox" 
                class="toggle toggle-primary toggle-sm" 
                :checked="getVal('AUTO_CHECK_UPDATE') !== 'false'" 
                @change="toggleAutoCheckUpdate" 
              />
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

    <!-- 数据导入与迁移弹窗 Modal -->
    <div v-if="showMigrateModal" class="modal modal-open z-50 animate-in fade-in duration-200">
      <div class="modal-box max-w-xl bg-base-100 rounded-3xl p-6 sm:p-7 border border-base-200/80 shadow-2xl space-y-5">
        <div class="flex items-center justify-between">
          <div class="space-y-0.5">
            <h3 class="text-lg font-black tracking-tight flex items-center gap-2">
              <Database class="text-primary" :size="20" />
              {{ $t('settings.migrate.title') }}
            </h3>
            <p class="text-xs opacity-50">{{ $t('settings.migrate.desc') }}</p>
          </div>
          <button @click="showMigrateModal = false" class="btn btn-ghost btn-sm btn-circle">✕</button>
        </div>

        <!-- 模式切换标签 -->
        <div class="flex rounded-2xl bg-base-200 p-1">
          <button class="flex-1 py-1.5 rounded-xl text-xs font-black transition-all"
            :class="migrateMode === 'upload' ? 'bg-primary text-primary-content shadow-sm' : 'opacity-60 hover:opacity-100'"
            @click="migrateMode = 'upload'">
            {{ $t('settings.migrate.tabUpload') }}
          </button>
          <button class="flex-1 py-1.5 rounded-xl text-xs font-black transition-all"
            :class="migrateMode === 'path' ? 'bg-primary text-primary-content shadow-sm' : 'opacity-60 hover:opacity-100'"
            @click="migrateMode = 'path'">
            {{ $t('settings.migrate.tabPath') }}
          </button>
        </div>

        <!-- 上传文件模式 -->
        <div v-if="migrateMode === 'upload'" class="space-y-3">
          <div class="border-2 border-dashed border-base-300 rounded-2xl p-6 text-center hover:border-primary/50 transition-colors bg-base-200/30 cursor-pointer relative">
            <input 
              type="file" 
              accept=".db,.sqlite,.sqlite3" 
              class="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
              @change="onMigrateFileSelect" 
            />
            <div class="flex flex-col items-center gap-2 pointer-events-none">
              <div class="w-12 h-12 rounded-2xl bg-primary/10 text-primary flex items-center justify-center">
                <Upload :size="24" />
              </div>
              <div>
                <p class="text-xs font-bold">{{ migrateFile ? migrateFile.name : $t('settings.migrate.selectFile') }}</p>
                <p class="text-[11px] opacity-50 mt-0.5">{{ $t('settings.migrate.fileHint') }}</p>
              </div>
            </div>
          </div>
        </div>

        <!-- 服务器路径模式 -->
        <div v-else class="space-y-2">
          <label class="text-[11px] font-black uppercase tracking-wider opacity-60">{{ $t('settings.migrate.pathLabel') }}</label>
          <input 
            v-model="migratePath" 
            type="text" 
            :placeholder="$t('settings.migrate.pathPlaceholder')" 
            class="input input-bordered w-full rounded-xl font-mono text-xs" 
          />
        </div>

        <!-- 提示说明 -->
        <div class="p-3 bg-base-200/60 rounded-xl text-xs text-base-content/70 leading-relaxed font-medium">
          {{ $t('settings.migrate.notice') }}
        </div>

        <!-- 成功结果展示 -->
        <div v-if="migrateResult" class="p-4 bg-success/10 border border-success/30 rounded-2xl space-y-2">
          <div class="flex items-center gap-2 text-success font-black text-xs">
            <Check :size="16" />
            <span>{{ $t('settings.migrate.resultTitle') }}</span>
          </div>
          <div class="grid grid-cols-3 gap-2 pt-1 text-center font-mono">
            <div class="bg-base-100 p-2 rounded-xl">
              <div class="text-lg font-black text-primary">+{{ migrateResult.Subscriptions || 0 }}</div>
              <div class="text-[10px] opacity-60">{{ $t('settings.migrate.subsCount') }}</div>
            </div>
            <div class="bg-base-100 p-2 rounded-xl">
              <div class="text-lg font-black text-secondary">+{{ migrateResult.Episodes || 0 }}</div>
              <div class="text-[10px] opacity-60">{{ $t('settings.migrate.epsCount') }}</div>
            </div>
            <div class="bg-base-100 p-2 rounded-xl">
              <div class="text-lg font-black text-accent">+{{ migrateResult.Downloads || 0 }}</div>
              <div class="text-[10px] opacity-60">{{ $t('settings.migrate.dlsCount') }}</div>
            </div>
          </div>
          <div v-if="migrateResult.Errors && migrateResult.Errors.length" class="pt-2 text-[11px] text-error/80">
            <div class="font-bold mb-1">{{ $t('settings.migrate.errorsTitle') }}:</div>
            <ul class="list-disc list-inside space-y-0.5 font-mono">
              <li v-for="err in migrateResult.Errors" :key="err">{{ err }}</li>
            </ul>
          </div>
        </div>

        <p v-if="migrateError" class="text-xs text-error font-bold bg-error/10 p-3 rounded-xl border border-error/20">{{ migrateError }}</p>

        <div class="modal-action flex justify-end gap-2.5 pt-2">
          <button @click="showMigrateModal = false" class="btn btn-ghost btn-sm rounded-xl px-5">
            {{ migrateResult ? $t('settings.migrate.closeBtn') : '取消' }}
          </button>
          <button 
            v-if="!migrateResult" 
            @click="executeMigration" 
            :disabled="migrating || (migrateMode === 'upload' && !migrateFile)" 
            class="btn btn-primary btn-sm rounded-xl px-7 shadow-md gap-1.5"
          >
            <span v-if="migrating" class="loading loading-spinner loading-xs"></span>
            <Upload v-else :size="14" />
            <span>{{ migrating ? $t('settings.migrate.importing') : $t('settings.migrate.startBtn') }}</span>
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
