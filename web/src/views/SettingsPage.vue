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
  Play, AlertCircle, Globe, X,
  Plus, RotateCcw, Activity, Filter, Clock, Calendar,
  Link2, FolderCheck, Film, CloudDownload, Puzzle
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

// AI 模型列表
const modelLoading = ref(false)
const modelOptions = ref<{ label: string; value: string }[]>([])

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
const pluginListError = ref('')
const customPlugins = computed(() => pluginList.value.filter(p => !p.is_builtin))
let pluginListRequest = 0
const pendingPlugins = ref(new Set<string>())
const pluginSourceFilter = ref<'all' | 'official' | 'community'>('all')
const visiblePlugins = computed(() => pluginList.value.filter(plugin =>
  pluginSourceFilter.value === 'all' ||
  (plugin.origin === 'official' ? 'official' : 'community') === pluginSourceFilter.value
))
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
const pluginFileReading = ref(false)
let pluginFileRequest = 0

const availablePluginEvents = [
  { id: 'subscription.added', label: 'subscriptionAdded' },
  { id: 'download.started', label: 'downloadStarted' },
  { id: 'download.completed', label: 'downloadCompleted' },
  { id: 'file.organized', label: 'fileOrganized' },
  { id: 'supplement.completed', label: 'supplementCompleted' },
  { id: 'download.failed', label: 'downloadFailed' },
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

function pluginIconComponent(icon?: string) {
  switch (icon) {
    case 'Link':
      return Link2
    case 'FolderCheck':
      return FolderCheck
    case 'Calendar':
      return Calendar
    case 'Database':
      return Database
    case 'Bell':
      return Bell
    case 'Film':
      return Film
    case 'CloudDownload':
      return CloudDownload
    default:
      return Puzzle
  }
}

const tabGroups = computed(() => {
  const rawGroups = [
    {
      name: t('settings.groups.core'),
      tabs: [
        { key: 'appearance', label: locale.value === 'zh' ? '外观与主题' : 'Themes & Appearance', icon: Palette },
        { key: 'paths', label: t('settings.tabs.paths'), icon: Folder },
        { key: 'downloader', label: t('settings.tabs.downloader'), icon: Download },
      ]
    },
    {
      name: t('settings.groups.sources'),
      tabs: [
        { key: 'sources', label: t('settings.tabs.sources'), icon: Globe },
        { key: 'mikan', label: t('settings.tabs.mikan'), icon: Antenna },
        { key: 'bangumi', label: t('settings.tabs.bangumi'), icon: Antenna },
      ]
    },
    {
      name: t('settings.groups.extensions'),
      tabs: [
        { key: 'plugins', label: t('settings.tabs.plugins'), icon: Sparkles },
        { key: 'ai', label: t('settings.tabs.ai'), icon: Cpu },
        { key: 'customRegex', label: t('settings.sections.customRegex'), icon: FileText },
        { key: 'notify', label: t('settings.tabs.notify'), icon: Bell, pluginRequired: 'extended-notifiers' },
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

interface FieldDef {
  label: string
  key: string
  placeholder: string
  type?: 'text' | 'password' | 'select' | 'switch'
  hint?: string
  testChannel?: string
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
  ai: [
    {
      title: t('settings.sections.aiModel'),
      desc: t('settings.sections.aiModelDesc'),
      fields: [
        {
          label: t('settings.fields.aiProtocol'), key: 'AI_PROTOCOL', placeholder: 'openai', type: 'select',
          selectOptions: [
            { label: t('settings.options.aiOpenai'), value: 'openai' },
            { label: t('settings.options.aiGoogle'), value: 'google' },
            { label: t('settings.options.aiAnthropic'), value: 'anthropic' },
            { label: t('settings.options.aiOllama'), value: 'ollama' },
          ]
        },
        { label: t('settings.fields.aiEndpoint'), key: 'AI_ENDPOINT', placeholder: 'https://api.openai.com/v1' },
        { label: t('settings.fields.aiKey'), key: 'AI_API_KEY', placeholder: 'sk-...', type: 'password' },
        { label: t('settings.fields.aiModel'), key: 'AI_MODEL', placeholder: 'gpt-4o-mini / deepseek-chat' },
        { label: t('settings.fields.aiSmartSearch'), key: 'AI_SMART_SEARCH', placeholder: 'false', type: 'switch', hint: '搜索无果或中文别名多义时自动由 AI 改写多组英文/罗马音查询词。' },
        { label: t('settings.fields.aiAutoDisambiguate'), key: 'AI_AUTO_DISAMBIGUATE', placeholder: 'false', type: 'switch', hint: '正则难以解析生肉或特番命名时回退至 AI 消歧提取季集信息。' },
        { label: t('settings.fields.aiDailyQuota'), key: 'AI_DAILY_QUOTA_LIMIT', placeholder: '30' },
      ]
    }
  ],
  notify: [
    {
      title: t('settings.sections.notifyTg'),
      desc: t('settings.sections.notifyTgDesc'),
      fields: [
        { label: t('settings.fields.tgToken'), key: 'TELEGRAM_BOT_TOKEN', placeholder: '123456:ABC...', type: 'password', testChannel: 'Telegram' },
        { label: t('settings.fields.tgChatId'), key: 'TELEGRAM_CHAT_ID', placeholder: '123456789' },
      ]
    },
    {
      title: t('settings.sections.notifyWebhook'),
      desc: t('settings.sections.notifyWebhookDesc'),
      fields: [
        { label: t('settings.fields.dingtalkWebhook'), key: 'DINGTALK_WEBHOOK', placeholder: 'https://oapi.dingtalk.com/...', testChannel: 'DingTalk' },
        { label: t('settings.fields.dingtalkSecret'), key: 'DINGTALK_SECRET', placeholder: 'SEC...', type: 'password' },
        { label: t('settings.fields.wecomWebhook'), key: 'WECOM_WEBHOOK', placeholder: 'https://qyapi.weixin.qq.com/...', testChannel: 'WeCom' },
        { label: t('settings.fields.feishuWebhook'), key: 'FEISHU_WEBHOOK', placeholder: 'https://open.feishu.cn/...', testChannel: 'Feishu' },
      ]
    },
    {
      title: t('settings.sections.notifyQQ'),
      desc: t('settings.sections.notifyQQDesc'),
      fields: [
        { label: t('settings.fields.qqHost'), key: 'ONEBOT_HOST', placeholder: 'http://localhost:5700', testChannel: 'OneBot' },
        { label: t('settings.fields.qqToken'), key: 'ONEBOT_TOKEN', placeholder: 'Access Token', type: 'password' },
        { label: t('settings.fields.qqUserId'), key: 'ONEBOT_USER_ID', placeholder: '10001' },
        { label: t('settings.fields.qqGroupId'), key: 'ONEBOT_GROUP_ID', placeholder: '20002' },
      ]
    },
    {
      title: t('settings.sections.notifyBark'),
      desc: t('settings.sections.notifyBarkDesc'),
      fields: [
        { label: t('settings.fields.barkKey'), key: 'BARK_DEVICE_KEY', placeholder: 'Bark Device Key', testChannel: 'Bark' },
        { label: t('settings.fields.barkServer'), key: 'BARK_SERVER_URL', placeholder: 'https://api.day.app' },
      ]
    },
    {
      title: t('settings.sections.notifyServerChan'),
      desc: t('settings.sections.notifyServerChanDesc'),
      fields: [
        { label: t('settings.fields.serverChanKey'), key: 'SERVERCHAN_KEY', placeholder: 'SCT...', type: 'password', testChannel: 'ServerChan' },
      ]
    },
    {
      title: t('settings.sections.notifyDiscord'),
      desc: t('settings.sections.notifyDiscordDesc'),
      fields: [
        { label: t('settings.fields.discordWebhook'), key: 'DISCORD_WEBHOOK', placeholder: 'https://discord.com/api/webhooks/...', type: 'password', testChannel: 'Discord' },
      ]
    },
    {
      title: t('settings.sections.notifySlack'),
      desc: t('settings.sections.notifySlackDesc'),
      fields: [
        { label: t('settings.fields.slackWebhook'), key: 'SLACK_WEBHOOK', placeholder: 'https://hooks.slack.com/services/...', type: 'password', testChannel: 'Slack' },
      ]
    },
    {
      title: t('settings.sections.notifyGotify'),
      desc: t('settings.sections.notifyGotifyDesc'),
      fields: [
        { label: t('settings.fields.gotifyUrl'), key: 'GOTIFY_URL', placeholder: 'https://gotify.example.com', testChannel: 'Gotify' },
        { label: t('settings.fields.gotifyToken'), key: 'GOTIFY_TOKEN', placeholder: 'Application Token', type: 'password' },
      ]
    },
    {
      title: t('settings.sections.notifyNtfy'),
      desc: t('settings.sections.notifyNtfyDesc'),
      fields: [
        { label: t('settings.fields.ntfyUrl'), key: 'NTFY_URL', placeholder: 'https://ntfy.sh', testChannel: 'Ntfy' },
        { label: t('settings.fields.ntfyTopic'), key: 'NTFY_TOPIC', placeholder: 'my-anime-topic' },
      ]
    },
    {
      title: t('settings.sections.notifyPushover'),
      desc: t('settings.sections.notifyPushoverDesc'),
      fields: [
        { label: t('settings.fields.pushoverToken'), key: 'PUSHOVER_TOKEN', placeholder: 'Application Token', type: 'password', testChannel: 'Pushover' },
        { label: t('settings.fields.pushoverUser'), key: 'PUSHOVER_USER', placeholder: 'User Key' },
      ]
    },
    {
      title: t('settings.sections.notifyEmail'),
      desc: t('settings.sections.notifyEmailDesc'),
      fields: [
        { label: t('settings.fields.smtpHost'), key: 'EMAIL_SMTP_HOST', placeholder: 'smtp.example.com', testChannel: 'Email' },
        { label: t('settings.fields.smtpPort'), key: 'EMAIL_SMTP_PORT', placeholder: '465 / 587' },
        { label: t('settings.fields.smtpUser'), key: 'EMAIL_SMTP_USER', placeholder: 'user@example.com' },
        { label: t('settings.fields.smtpPass'), key: 'EMAIL_SMTP_PASS', placeholder: '••••••••', type: 'password' },
        { label: t('settings.fields.smtpFrom'), key: 'EMAIL_SMTP_FROM', placeholder: '可选发件人，留空默认同用户名' },
        { label: t('settings.fields.smtpTo'), key: 'EMAIL_SMTP_TO', placeholder: '接收者邮箱，多个用逗号隔开' },
      ]
    },
    {
      title: t('settings.sections.notifyMatrix'),
      desc: t('settings.sections.notifyMatrixDesc'),
      fields: [
        { label: t('settings.fields.matrixHomeserver'), key: 'MATRIX_HOMESERVER', placeholder: 'https://matrix.org', testChannel: 'Matrix' },
        { label: t('settings.fields.matrixToken'), key: 'MATRIX_TOKEN', placeholder: 'syt_...', type: 'password' },
        { label: t('settings.fields.matrixRoomId'), key: 'MATRIX_ROOM_ID', placeholder: '!abc123:matrix.org' },
      ]
    },
    {
      title: t('settings.sections.notifyLine'),
      desc: t('settings.sections.notifyLineDesc'),
      fields: [
        { label: t('settings.fields.lineChannelToken'), key: 'LINE_CHANNEL_TOKEN', placeholder: 'Channel Access Token', type: 'password', testChannel: 'LINE' },
        { label: t('settings.fields.lineUserId'), key: 'LINE_USER_ID', placeholder: 'U1234567890abcdef...' },
      ]
    },
    {
      title: t('settings.sections.notifyWhatsapp'),
      desc: t('settings.sections.notifyWhatsappDesc'),
      fields: [
        { label: t('settings.fields.whatsappPhoneId'), key: 'WHATSAPP_PHONE_ID', placeholder: '123456789012345', testChannel: 'WhatsApp' },
        { label: t('settings.fields.whatsappToken'), key: 'WHATSAPP_TOKEN', placeholder: 'EAAB...', type: 'password' },
        { label: t('settings.fields.whatsappTo'), key: 'WHATSAPP_TO', placeholder: '8613800138000' },
      ]
    },
    {
      title: t('settings.sections.notifySignal'),
      desc: t('settings.sections.notifySignalDesc'),
      fields: [
        { label: t('settings.fields.signalApiUrl'), key: 'SIGNAL_API_URL', placeholder: 'http://localhost:8080', testChannel: 'Signal' },
        { label: t('settings.fields.signalSender'), key: 'SIGNAL_SENDER', placeholder: '+8613800138000' },
        { label: t('settings.fields.signalRecipients'), key: 'SIGNAL_RECIPIENTS', placeholder: '+8613800138000' },
      ]
    },
    {
      title: t('settings.sections.notifyWechat'),
      desc: t('settings.sections.notifyWechatDesc'),
      fields: [
        { label: t('settings.fields.wechatAppId'), key: 'WECHAT_APP_ID', placeholder: 'wx1234567890abcdef', testChannel: 'WeChat' },
        { label: t('settings.fields.wechatAppSecret'), key: 'WECHAT_APP_SECRET', placeholder: 'abcdef1234567890...', type: 'password' },
        { label: t('settings.fields.wechatUserId'), key: 'WECHAT_USER_ID', placeholder: 'oAbc123_xyz...' },
        { label: t('settings.fields.wechatTemplateId'), key: 'WECHAT_TEMPLATE_ID', placeholder: 'ngqIpbwh8bUfcSsXYZ...' },
      ]
    },
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
  backup: [
    {
      title: t('settings.sections.backupCron'),
      desc: t('settings.sections.backupCronDesc'),
      fields: [
        { label: t('settings.fields.backupPath'), key: 'BACKUP_PATH', placeholder: './data/backups' },
        { label: t('settings.fields.backupCron'), key: 'BACKUP_CRON', placeholder: '0 0 * * *' },
        { label: t('settings.fields.backupKeepCount'), key: 'BACKUP_KEEP_COUNT', placeholder: '7' },
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

const currentTabSections = computed<SectionDef[]>(() => {
  if (activeTab.value === 'notify' && !isPluginActive('extended-notifiers')) {
    return []
  }
  return sectionsByTab.value[activeTab.value] || []
})

watch(() => isPluginActive('extended-notifiers'), (active) => {
  if (!active && activeTab.value === 'notify') {
    activeTab.value = 'plugins'
  }
})

async function enableNotifyPlugin() {
  const p = pluginList.value.find(item => item.id === 'extended-notifiers')
  if (p && !p.enabled) await togglePlugin(p)
}

function getVal(key: string): string {
  return settings.value[key] || ''
}

function setVal(key: string, val: string) {
  settings.value[key] = val
}

function isConfigured(key: string): boolean {
  const val = settings.value[key]
  const field = allFields.value[key]
  if (field?.type === 'password') return val !== undefined
  return val !== undefined && val.length > 0
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

// MCP Server 状态与操作
const showMcpToken = ref(false)
const generatedMcpToken = ref('')
const mcpConfigured = computed(() => !!generatedMcpToken.value || settings.value.MCP_TOKEN_CONFIGURED === 'true')
const mcpGeneratingToken = ref(false)

const mcpEndpointUrl = computed(() => {
  const origin = typeof window !== 'undefined' && window.location?.origin ? window.location.origin : 'http://localhost:20001'
  return `${origin}/mcp/sse`
})

const mcpClaudeConfigJson = computed(() => {
  const token = generatedMcpToken.value || 'YOUR_MCP_TOKEN'
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
      generatedMcpToken.value = data.token
      showMcpToken.value = true
      saved.value = true
      setTimeout(() => { saved.value = false }, 2500)
      window.showToast?.(t('settings.mcp.tokenGenerated'), 'success')
    }
  } catch (e: any) {
    const errMsg = e.response?.data?.error || t('settings.mcp.generateFailed')
    if (window.showToast) window.showToast(errMsg, 'error')
    else error.value = errMsg
  } finally {
    mcpGeneratingToken.value = false
  }
}

function copyText(text: string) {
  if (!text) return
  navigator.clipboard.writeText(text)
  saved.value = true
  setTimeout(() => { saved.value = false }, 2000)
  window.showToast?.(t('settings.copied') || '已复制到剪贴板', 'success')
}

function copyClaudeConfig() {
  copyText(mcpClaudeConfigJson.value)
}

function copyCursorConfig() {
  copyText(mcpClaudeConfigJson.value)
}

// 自定义正则配置与测试
const customRegexList = ref<string[]>(Array(10).fill(''))
let savedRegex: string[] = Array(10).fill('')
let regexRead = 0
const customRegexCompiled = ref<string[]>([])
const customRegexLoading = ref(false)
const customRegexSaved = ref(false)
const regexTestTitle = ref('')
const regexTestResult = ref<any>(null)
const regexTesting = ref(false)

async function fetchCustomRegex() {
  if (savingSettings.value || customRegexLoading.value) return
  const sequence = ++regexRead
  try {
    const { data } = await request.get('/settings/custom-regex')
    if (sequence !== regexRead) return
    if (data.patterns && Array.isArray(data.patterns)) {
      const list = Array(10).fill('')
      for (let i = 0; i < 10; i++) {
        list[i] = data.patterns[i] || ''
      }
      customRegexList.value = list.map((value, i) => customRegexList.value[i] === savedRegex[i] ? value : customRegexList.value[i])
      savedRegex = list
    }
    if (data.compiled && Array.isArray(data.compiled)) {
      customRegexCompiled.value = data.compiled
    }
  } catch (e: any) {
    console.error('获取自定义正则失败', e)
  }
}

function changedRegexSettings(): Record<string, string> {
  const updates: Record<string, string> = {}
  for (let i = 0; i < savedRegex.length; i++) {
    const value = customRegexList.value[i] || ''
    if (value !== savedRegex[i]) updates[`custom_regex_${i}`] = value
  }
  return updates
}

function rememberSavedRegex(changes: Record<string, string>) {
  for (let i = 0; i < savedRegex.length; i++) {
    const key = `custom_regex_${i}`
    if (key in changes) savedRegex[i] = changes[key]
  }
}

async function saveCustomRegex() {
  if (savingSettings.value || customRegexLoading.value) return
  ++regexRead
  customRegexLoading.value = true
  customRegexSaved.value = false
  error.value = ''
  try {
    const updates = changedRegexSettings()
    if (Object.keys(updates).length) {
      await request.put('/settings', { settings: updates })
      rememberSavedRegex(updates)
    }
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
    if (data.success) rssInspectResult.value = data
    else rssInspectError.value = data.error || '解析失败'
  } catch (e: any) {
    rssInspectError.value = e.response?.data?.error || '拉取或解析 RSS 失败，请检查链接连通性'
  } finally {
    rssInspectLoading.value = false
  }
}

function fillMikanRSS() {
  const mikanUrl = getVal('MIKAN_RSS_URL')
  if (mikanUrl) rssInspectUrl.value = mikanUrl
  else window.showToast?.('未配置个人 Mikan RSS 链接，可手动输入或粘贴字幕组 RSS', 'info')
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
  if (!settingsReady.value || savingSettings.value || customRegexLoading.value) return
  savingSettings.value = true
  ++settingsRead
  ++regexRead
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
  const regexUpdates = changedRegexSettings()
  Object.assign(changed, regexUpdates)
  try {
    if (Object.keys(changed).length) {
      await request.put('/settings', { settings: changed })
      Object.assign(savedSettings, changed)
      rememberSavedRegex(regexUpdates)
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
  if (saved.value && Object.keys(regexUpdates).length) void fetchCustomRegex()
}

// 通知投递历史与健康度分析
const notifyLogs = ref<any[]>([])
const notifyLogsLoading = ref(false)
let notifyRead = 0
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
  const sequence = ++notifyRead
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
    if (sequence !== notifyRead) return
    notifyLogs.value = data.logs || []
    notifyLogsTotal.value = data.total || 0
  } catch (e) {
    if (sequence === notifyRead) console.error('获取通知日志失败', e)
  } finally {
    if (sequence === notifyRead) notifyLogsLoading.value = false
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

async function sendTestNotify(channel: string) {
  try {
    const { data } = await request.post('/notify/test', {
      channel,
      title: 'Ani-Go 联通测试',
      message: `来自 ${channel} 的自检通知，收到说明配置成功。`
    })
    if (data.success) {
      saved.value = true
      window.showToast?.(`已发送至 ${channel}`, 'success')
      setTimeout(() => { saved.value = false }, 3000)
    } else {
      error.value = data.error || '测试发送失败'
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || '发送测试请求失败'
  }
}

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

// 插件管理逻辑
async function fetchPlugins(afterChange = false) {
  const currentRequest = ++pluginListRequest
  pluginLoading.value = true
  try {
    const { data } = await request.get('/plugins')
    if (currentRequest !== pluginListRequest) return false
    if (!Array.isArray(data)) throw new Error()
    pluginList.value = data
    pluginListError.value = ''
    if (activeTab.value === 'notify' && pluginList.value.some(p => p.id === 'extended-notifiers' && p.enabled && !pluginRunning(p))) {
      activeTab.value = 'plugins'
    }
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
    if (!await fetchPlugins(true)) return
    saved.value = true
    window.showToast?.('插件已重载', 'success')
    setTimeout(() => { saved.value = false }, 2500)
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
    if (!await fetchPlugins(true)) return
    saved.value = true
    setTimeout(() => { saved.value = false }, 2000)
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
    if (!await fetchPlugins(true)) return
    saved.value = true
    setTimeout(() => { saved.value = false }, 2000)
  } catch (e: any) {
    error.value = e.response?.data?.error || t('settings.plugins.deleteFailed')
  } finally {
    pendingPlugins.value.delete(id)
  }
}

function openAddPluginModal() {
  if (pluginSaving.value) return
  ++pluginFileRequest
  pluginFileReading.value = false
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
    name: t('settings.plugins.exampleName'),
    description: t('settings.plugins.exampleDescription'),
    version: '1.0.0',
    type: 'webhook',
    url: 'https://your-server.com/webhook',
    events: ['download.completed', 'file.organized']
  }, null, 2)
  pluginJsonError.value = ''
  showPluginModal.value = true
}

function newCustomPluginID() {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return 'custom-' + Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
}

async function submitPluginForm() {
  if (pluginSaving.value || pluginFileReading.value) return
  pluginSaving.value = true
  pluginJsonError.value = ''
  try {
    if (pluginModalTab.value === 'webhook') {
      if (!pluginForm.value.name.trim()) throw new Error(t('settings.plugins.nameRequired'))
      if (!pluginForm.value.url.trim()) throw new Error(t('settings.plugins.urlRequired'))
      if (pluginForm.value.events.length === 0) throw new Error(t('settings.plugins.eventsRequired'))
      pluginForm.value.id ||= newCustomPluginID()
      await request.post('/plugins/save', pluginForm.value)
    } else {
      const parsed = JSON.parse(pluginJsonText.value)
      const isBundle = Array.isArray(parsed)
      const items = isBundle ? parsed : [parsed]
      if (!items.length || items.some(item => !item || typeof item !== 'object' || Array.isArray(item))) {
        throw new Error(t('settings.plugins.invalidManifest'))
      }
      for (const item of items) {
        if (item.id == null || (typeof item.id === 'string' && !item.id.trim())) item.id = newCustomPluginID()
      }
      for (const [index, item] of items.entries()) {
        try {
          await request.post('/plugins/save', item)
        } catch (e: any) {
          pluginJsonText.value = JSON.stringify(isBundle ? items.slice(index) : item, null, 2)
          if (!index) throw e
          const reason = e.response?.data?.error || e.message || t('settings.plugins.saveFailed')
          pluginJsonError.value = t('settings.plugins.partialImport', { count: index, total: items.length, reason })
          await fetchPlugins(true)
          return
        }
      }
    }
    showPluginModal.value = false
    if (!await fetchPlugins(true)) return
    saved.value = true
    setTimeout(() => { saved.value = false }, 2500)
  } catch (e: any) {
    pluginJsonError.value = e.response?.data?.error || e.message || t('settings.plugins.saveFailed')
  } finally {
    pluginSaving.value = false
  }
}

function exportPluginsJSON() {
  if (!customPlugins.value.length || pluginListError.value) return
  const dataStr = 'data:application/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(customPlugins.value, null, 2))
  const dlAnchor = document.createElement('a')
  dlAnchor.setAttribute('href', dataStr)
  dlAnchor.setAttribute('download', `anigo-plugins-${new Date().toISOString().slice(0, 10)}.json`)
  dlAnchor.click()
}

async function handlePluginFileImport(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || pluginSaving.value) return
  input.value = ''
  const current = ++pluginFileRequest
  pluginFileReading.value = true
  pluginJsonError.value = ''
  try {
    const contents = await file.text()
    if (current === pluginFileRequest) pluginJsonText.value = contents
  } catch {
    if (current === pluginFileRequest) pluginJsonError.value = t('settings.plugins.fileReadFailed')
  } finally {
    if (current === pluginFileRequest) pluginFileReading.value = false
  }
}

// 数据迁移状态与操作
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
        throw new Error('请选择 AutoBangumi SQLite、ani.v2.json 或 ani-rss ZIP 备份')
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

// 备份管理操作
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
    window.showToast?.('备份已创建', 'success')
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

function pollLogsIfVisible() {
  if (activeTab.value === 'logs' && autoRefreshLogs.value && (typeof document === 'undefined' || !document.hidden)) {
    void fetchLogs()
  }
}

watch(() => activeTab.value === 'logs' && autoRefreshLogs.value, (val) => {
  if (val) {
    if (!logTimer) logTimer = setInterval(pollLogsIfVisible, 3000)
  } else if (logTimer) {
    clearInterval(logTimer)
    logTimer = null
  }
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

function loadTabResources(tab: string) {
  if (tab === 'logs') fetchLogs()
  else if (tab === 'customRegex') fetchCustomRegex()
  else if (tab === 'backup') fetchBackupList()
  else if (tab === 'notify') {
    fetchNotifyLogs()
    fetchNotifyStats()
  }
}

watch(activeTab, (newTab) => {
  loadTabResources(newTab)
})

onMounted(() => {
  fetchSettings()
  fetchPlugins()
  loadTabResources(activeTab.value)
  window.addEventListener('keydown', handleGlobalKeydown)
  if (typeof document !== 'undefined') document.addEventListener('visibilitychange', pollLogsIfVisible)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', pollLogsIfVisible)
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
        :disabled="loading || !settingsReady || savingSettings || customRegexLoading"
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
            <span v-if="activeTab === tab.key" class="w-1.5 h-1.5 rounded-full bg-[var(--v2-accent)] shrink-0"></span>
          </button>
        </div>
      </aside>

      <!-- 右侧内容区 -->
      <div class="flex-1 min-w-0 w-full space-y-5">
        <!-- 外观与多套主题工坊 -->
        <ThemeAppearancePanel v-if="activeTab === 'appearance'" />

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

        <!-- Bangumi OAuth 授权横幅 -->
        <div v-if="activeTab === 'bangumi'" class="v2-panel p-5 sm:p-6 space-y-4">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div class="flex items-start gap-3 min-w-0">
              <div class="w-10 h-10 rounded-xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center shrink-0">
                <Antenna :size="20" />
              </div>
              <div class="min-w-0 space-y-1">
                <div class="flex items-center gap-2 flex-wrap">
                  <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.bangumi.connectTitle') }}</h3>
                  <span v-if="getVal('BGMTV_USERNAME')" class="badge badge-success badge-xs">
                    {{ $t('settings.bangumi.bound') }}: {{ getVal('BGMTV_USERNAME') }}
                  </span>
                  <span v-else class="v2-pill text-[10px]">{{ $t('settings.bangumi.unconnected') }}</span>
                </div>
                <p class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">{{ $t('settings.bangumi.connectDesc') }}</p>
              </div>
            </div>
            <button type="button" @click="connectBangumi" class="v2-btn v2-btn-primary v2-btn-sm shrink-0">
              <Antenna :size="14" />
              <span>{{ $t('settings.bangumi.connectNow') }}</span>
            </button>
          </div>
          <div class="v2-panel-muted p-3.5 text-xs text-[var(--v2-ink-muted)] space-y-1">
            <p class="font-semibold text-[var(--v2-accent)]">{{ $t('settings.bangumi.tipsTitle') }}</p>
            <p>{{ $t('settings.bangumi.tip1Token') }} <a href="https://next.bgm.tv/demo/access-token" target="_blank" class="text-[var(--v2-accent)] hover:underline font-semibold">{{ $t('settings.bangumi.tip1Link') }}</a> {{ $t('settings.bangumi.tip1Suffix') }}</p>
            <p>{{ $t('settings.bangumi.tip2OAuth') }} <a href="https://bgm.tv/dev/app" target="_blank" class="text-[var(--v2-accent)] hover:underline font-semibold">{{ $t('settings.bangumi.tip2Link') }}</a> {{ $t('settings.bangumi.tip2Suffix') }}</p>
          </div>
        </div>

        <!-- AI 预算警示横幅 -->
        <div v-if="activeTab === 'ai'" class="v2-panel border-amber-500/35 bg-amber-500/8 p-5 space-y-2.5">
          <div class="flex items-center gap-2 font-bold text-sm text-amber-400">
            <AlertCircle :size="17" />
            <span>{{ $t('settings.ai.warningTitle') }}</span>
          </div>
          <p class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">{{ $t('settings.ai.warningDesc') }}</p>
          <div class="flex flex-wrap items-center gap-2 pt-1">
            <span class="v2-pill !h-auto !whitespace-normal max-w-full py-1 leading-snug font-mono text-[11px] border-amber-500/30 text-amber-400">{{ $t('settings.ai.badgeRecommend') }}: DeepSeek-V3 / Gemini 2.0 Flash / Ollama</span>
            <span class="v2-pill !h-auto !whitespace-normal max-w-full py-1 leading-snug font-mono text-[11px] border-amber-500/30 text-amber-400">{{ $t('settings.ai.badgeProtection') }}</span>
          </div>
        </div>

        <!-- 通用配置分组渲染（paths, downloader, sources, mikan, bangumi, ai, notify, scheduler, backup） -->
        <template v-if="currentTabSections.length > 0">
          <div v-for="section in currentTabSections" :key="section.title" class="v2-panel p-5 sm:p-6 space-y-5">
            <div class="border-b border-[var(--v2-border)] pb-3">
              <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ section.title }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ section.desc }}</p>
            </div>

            <div class="grid gap-4">
              <div v-for="field in section.fields" :key="field.key" class="space-y-1.5">
                <!-- 布尔 Switch 行 -->
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

                <!-- 常规控件 -->
                <template v-else>
                  <div class="flex items-center justify-between gap-2">
                    <label class="text-xs font-semibold text-[var(--v2-ink-muted)] flex items-center gap-1.5">
                      <span>{{ field.label }}</span>
                      <span v-if="isConfigured(field.key)" class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                    </label>
                    <button
                      v-if="field.testChannel"
                      type="button"
                      class="text-xs font-semibold text-[var(--v2-accent)] hover:underline cursor-pointer bg-transparent border-0 p-0"
                      @click="sendTestNotify(field.testChannel)"
                    >
                      {{ $t('settings.notify.sendTest') }}
                    </button>
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

                  <!-- AI 模型专用输入 + 拉取组合 -->
                  <div v-else-if="field.key === 'AI_MODEL'" class="space-y-2">
                    <div class="flex gap-2">
                      <input
                        type="text"
                        :value="getVal(field.key)"
                        @input="(e: Event) => setVal(field.key, (e.target as HTMLInputElement).value)"
                        :placeholder="field.placeholder"
                        class="v2-input font-mono text-xs flex-1"
                      />
                      <button
                        type="button"
                        @click="fetchAIModels"
                        :disabled="modelLoading || !getVal('AI_ENDPOINT')"
                        class="v2-btn shrink-0"
                        :title="$t('settings.ai.fetchTooltip')"
                      >
                        <RefreshCw :size="13" :class="{ 'animate-spin': modelLoading }" />
                        <span>{{ modelLoading ? $t('settings.ai.loadingModels') : $t('settings.ai.fetchModels') }}</span>
                      </button>
                    </div>
                    <select
                      v-if="modelOptions.length"
                      :value="getVal('AI_MODEL')"
                      @change="(e: Event) => setVal('AI_MODEL', (e.target as HTMLSelectElement).value)"
                      class="v2-input font-mono text-xs cursor-pointer"
                    >
                      <option v-for="opt in modelOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                    </select>
                  </div>

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

        <!-- MCP Server 外部控制卡片 (AI 标签下) -->
        <div v-if="activeTab === 'ai'" class="v2-panel p-5 sm:p-6 space-y-5">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center shrink-0">
                <Cpu :size="19" />
              </div>
              <div>
                <div class="flex items-center gap-2 flex-wrap">
                  <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.mcp.title') }}</h3>
                  <span class="v2-pill font-mono text-[10px]">SSE Protocol</span>
                </div>
                <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.mcp.desc') }}</p>
              </div>
            </div>

            <button
              type="button"
              @click="generateMCPToken"
              :disabled="mcpGeneratingToken"
              class="v2-btn v2-btn-sm shrink-0"
            >
              <RefreshCw v-if="mcpGeneratingToken" :size="13" class="animate-spin" />
              <Sparkles v-else :size="13" />
              <span>{{ mcpConfigured ? $t('settings.mcp.regenToken') : $t('settings.mcp.generateToken') }}</span>
            </button>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="space-y-1.5">
              <div class="flex items-center justify-between text-xs">
                <label class="font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.mcp.endpointLabel') }}</label>
                <span class="text-[10px] text-emerald-400 font-semibold">GET (SSE)</span>
              </div>
              <div class="flex gap-2">
                <input type="text" readonly :value="mcpEndpointUrl" class="v2-input font-mono text-xs flex-1" />
                <button type="button" @click="copyText(mcpEndpointUrl)" class="v2-btn px-3 shrink-0" :title="$t('settings.mcp.copyTooltip')">
                  <Copy :size="13" />
                </button>
              </div>
            </div>

            <div class="space-y-1.5">
              <div class="flex items-center justify-between text-xs">
                <label class="font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.mcp.tokenLabel') }}</label>
                <span v-if="mcpConfigured" class="text-[10px] text-emerald-400 font-semibold">Bearer Auth</span>
                <span v-else class="text-[10px] text-amber-400 font-semibold">{{ $t('settings.mcp.notConfigured') }}</span>
              </div>
              <div class="flex gap-2">
                <input
                  :type="showMcpToken ? 'text' : 'password'"
                  readonly
                  :value="generatedMcpToken"
                  :placeholder="$t(mcpConfigured ? 'settings.mcp.tokenHidden' : 'settings.mcp.tokenPlaceholder')"
                  class="v2-input font-mono text-xs flex-1"
                />
                <button type="button" @click="showMcpToken = !showMcpToken" class="v2-btn px-3 shrink-0">
                  <EyeOff v-if="showMcpToken" :size="13" />
                  <Eye v-else :size="13" />
                </button>
                <button type="button" @click="copyText(generatedMcpToken)" :disabled="!generatedMcpToken" class="v2-btn px-3 shrink-0" :title="$t('settings.mcp.copyTooltip')">
                  <Copy :size="13" />
                </button>
              </div>
            </div>
          </div>

          <div class="v2-panel-muted p-4 space-y-2.5">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="text-xs font-semibold text-[var(--v2-ink)] flex items-center gap-1.5">
                <FileText :size="14" />
                <span>{{ $t('settings.mcp.clientConfigTitle') }}</span>
              </span>
              <div class="flex items-center gap-2">
                <button type="button" @click="copyClaudeConfig" :disabled="!generatedMcpToken" class="v2-btn v2-btn-primary v2-btn-sm">
                  <Copy :size="12" />
                  <span>{{ $t('settings.mcp.copyClaudeConfig') }}</span>
                </button>
                <button type="button" @click="copyCursorConfig" :disabled="!generatedMcpToken" class="v2-btn v2-btn-sm">
                  <Copy :size="12" />
                  <span>{{ $t('settings.mcp.copyCursorConfig') }}</span>
                </button>
              </div>
            </div>
            <pre class="text-[11px] font-mono bg-[#09090b] text-zinc-200 p-3 rounded-lg overflow-x-auto select-all border border-[var(--v2-border)]">{{ mcpClaudeConfigJson }}</pre>
            <p class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.mcp.clientConfigHint') }}</p>
          </div>
        </div>

        <!-- 自定义标题正则与 RSS 探针 -->
        <div v-if="activeTab === 'customRegex'" class="space-y-5">
          <div class="v2-panel p-5 sm:p-6 space-y-5">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div>
                <div class="flex items-center gap-2 flex-wrap">
                  <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.customRegexTab.title') }}</h3>
                  <span class="v2-pill font-mono text-[10px]">{{ $t('settings.customRegexTab.compiledCount', { count: customRegexCompiled.length }) }}</span>
                </div>
                <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.customRegexTab.desc') }}</p>
              </div>
              <button
                type="button"
                class="v2-btn v2-btn-primary v2-btn-sm shrink-0"
                @click="saveCustomRegex"
                :disabled="customRegexLoading || savingSettings"
              >
                <RefreshCw v-if="customRegexLoading" :size="13" class="animate-spin" />
                <Check v-else :size="13" />
                <span>{{ customRegexSaved ? $t('settings.customRegexTab.savedSuccess') : $t('settings.customRegexTab.saveAndReload') }}</span>
              </button>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-3.5">
              <div v-for="index in 10" :key="index - 1" class="v2-panel-muted p-3.5 space-y-2">
                <div class="flex items-center justify-between gap-2">
                  <label class="text-xs font-semibold text-[var(--v2-ink)] flex items-center gap-1.5">
                    <span class="font-mono text-[10px] px-1.5 py-0.5 rounded bg-[var(--v2-surface)] border border-[var(--v2-border)]">#{{ index - 1 }}</span>
                    <span>{{ $t('settings.customRegexTab.slotLabel', { index: index - 1 }) }}</span>
                  </label>
                  <span v-if="customRegexList[index - 1]?.trim()" class="text-[10px] font-semibold text-emerald-400">已配置</span>
                </div>
                <input
                  type="text"
                  v-model="customRegexList[index - 1]"
                  :placeholder="$t('settings.customRegexTab.slotPlaceholder')"
                  class="v2-input font-mono text-xs"
                />
              </div>
            </div>
          </div>

          <!-- 正则实时沙盒测试 -->
          <div class="v2-panel p-5 sm:p-6 space-y-4">
            <div>
              <h4 class="text-sm font-bold text-[var(--v2-ink)] flex items-center gap-2">
                <Play :size="15" class="text-[var(--v2-accent)]" />
                <span>{{ $t('settings.customRegexTab.testTitle') }}</span>
              </h4>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.customRegexTab.testDesc') }}</p>
            </div>

            <div class="flex flex-col sm:flex-row gap-2.5">
              <input
                type="text"
                v-model="regexTestTitle"
                :placeholder="$t('settings.customRegexTab.testInputPlaceholder')"
                class="v2-input font-mono text-xs flex-1"
                @keydown.enter="runRegexTest"
              />
              <button
                type="button"
                class="v2-btn v2-btn-primary shrink-0"
                @click="runRegexTest"
                :disabled="regexTesting || !regexTestTitle.trim()"
              >
                <RefreshCw v-if="regexTesting" :size="14" class="animate-spin" />
                <Play v-else :size="14" />
                <span>{{ regexTesting ? $t('settings.customRegexTab.testing') : $t('settings.customRegexTab.testBtn') }}</span>
              </button>
            </div>

            <div v-if="regexTestResult" class="v2-panel-muted p-4 space-y-3">
              <div class="flex items-center justify-between border-b border-[var(--v2-border)] pb-2 text-xs">
                <span class="font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.customRegexTab.parseResult') }}</span>
                <span class="v2-pill text-[10px]">解析完成</span>
              </div>
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 text-xs">
                <div class="p-3 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)] col-span-2">
                  <div class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.customRegexTab.animeTitle') }}</div>
                  <div class="font-bold font-mono truncate mt-0.5" :title="regexTestResult.title">{{ regexTestResult.title || '-' }}</div>
                </div>
                <div class="p-3 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)]">
                  <div class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.customRegexTab.subgroup') }}</div>
                  <div class="font-bold font-mono truncate mt-0.5">{{ regexTestResult.subgroup || '未识别' }}</div>
                </div>
                <div class="p-3 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)]">
                  <div class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.customRegexTab.resolution') }}</div>
                  <div class="font-bold font-mono text-[var(--v2-accent)] mt-0.5">{{ regexTestResult.resolution || '未知' }}</div>
                </div>
                <div class="p-3 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)]">
                  <div class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.customRegexTab.season') }}</div>
                  <div class="font-bold font-mono text-emerald-400 mt-0.5">S{{ regexTestResult.season }}</div>
                </div>
                <div class="p-3 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)]">
                  <div class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.customRegexTab.episode') }}</div>
                  <div class="font-bold font-mono text-[var(--v2-accent)] mt-0.5">E{{ regexTestResult.episode }}</div>
                </div>
                <div class="p-3 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)]">
                  <div class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.customRegexTab.isBatch') }}</div>
                  <div class="font-semibold mt-0.5" :class="regexTestResult.is_batch ? 'text-amber-400' : 'text-[var(--v2-ink-muted)]'">
                    {{ regexTestResult.is_batch ? '合集 / Batch' : '单集' }}
                  </div>
                </div>
                <div class="p-3 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)]">
                  <div class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.customRegexTab.isSpecial') }}</div>
                  <div class="font-semibold mt-0.5" :class="regexTestResult.is_special ? 'text-[var(--v2-accent)]' : 'text-[var(--v2-ink-muted)]'">
                    {{ regexTestResult.is_special ? 'SP / OVA' : '常规剧集' }}
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- RSS 实时源深度探针 -->
          <div class="v2-panel p-5 sm:p-6 space-y-4">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-bold text-[var(--v2-ink)]">RSS 实时源解析与正则特征探针</h3>
                <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">实时拉取 Mikan、Nyaa 或字幕组订阅源，预览条目与正则提取结果</p>
              </div>
              <button type="button" class="v2-btn v2-btn-sm shrink-0" @click="fillMikanRSS">
                自动填入我的 Mikan RSS
              </button>
            </div>

            <div class="flex flex-col sm:flex-row gap-2.5">
              <input
                type="text"
                v-model="rssInspectUrl"
                placeholder="输入或粘贴 RSS URL (如 https://mikanani.me/RSS/Bangumi?bangumiId=...)"
                class="v2-input font-mono text-xs flex-1"
                @keydown.enter="runRSSInspect"
              />
              <button
                type="button"
                class="v2-btn v2-btn-primary shrink-0"
                @click="runRSSInspect"
                :disabled="rssInspectLoading || !rssInspectUrl.trim()"
              >
                <RefreshCw v-if="rssInspectLoading" :size="14" class="animate-spin" />
                <Play v-else :size="14" />
                <span>{{ rssInspectLoading ? '正在探查中...' : '开始探查' }}</span>
              </button>
            </div>

            <div v-if="rssInspectError" class="p-3 rounded-lg bg-red-500/10 border border-red-500/25 text-red-400 text-xs flex items-center gap-2">
              <AlertCircle :size="15" />
              <span>{{ rssInspectError }}</span>
            </div>

            <div v-if="rssInspectResult" class="space-y-3 pt-1">
              <div class="v2-panel-muted p-3.5 flex items-center justify-between gap-3">
                <div class="min-w-0">
                  <div class="text-xs font-bold text-[var(--v2-ink)] truncate">{{ rssInspectResult.feed?.title || 'RSS 订阅源' }}</div>
                  <div class="text-[10px] font-mono text-[var(--v2-ink-subtle)] truncate">{{ rssInspectResult.feed?.url }}</div>
                </div>
                <span class="v2-pill font-mono text-[10px] shrink-0">共 {{ rssInspectResult.total }} 条</span>
              </div>

              <div class="space-y-2.5 max-h-[440px] overflow-y-auto pr-1">
                <div v-for="(item, idx) in rssInspectResult.items" :key="idx" class="v2-panel-muted p-3.5 space-y-2 text-xs">
                  <div class="font-semibold font-mono text-[var(--v2-ink)] break-all">{{ item.title }}</div>
                  <div class="flex flex-wrap items-center gap-1.5">
                    <span v-if="item.parsed_title" class="badge badge-primary badge-xs">{{ item.parsed_title }}</span>
                    <span v-if="item.episode > 0" class="v2-pill font-mono text-[10px]">E{{ item.episode }}</span>
                    <span v-if="item.season > 0" class="v2-pill font-mono text-[10px]">S{{ item.season }}</span>
                    <span v-if="item.subgroup" class="v2-pill font-mono text-[10px]">{{ item.subgroup }}</span>
                    <span v-if="item.resolution" class="v2-pill font-mono text-[10px]">{{ item.resolution }}</span>
                    <span v-if="item.is_batch" class="badge badge-warning badge-xs">合集</span>
                    <span v-if="item.is_special" class="badge badge-neutral badge-xs">SP/OVA</span>
                  </div>
                  <div class="flex items-center justify-between text-[11px] font-mono text-[var(--v2-ink-subtle)] pt-1 border-t border-[var(--v2-border)]">
                    <span>发布时间: {{ item.pub_date }}</span>
                    <span>大小: {{ (item.size / 1e6).toFixed(1) }} MB</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 通知投递监控面板与未启用提示 -->
        <div v-if="activeTab === 'notify' && isPluginActive('extended-notifiers')" class="v2-panel p-5 sm:p-6 space-y-5">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center shrink-0">
                <Activity :size="19" />
              </div>
              <div>
                <h3 class="text-sm font-bold text-[var(--v2-ink)] flex items-center gap-2 flex-wrap">
                  <span>通知投递履历与送达监控</span>
                  <span class="v2-pill font-mono text-[10px]">{{ notifyLogsTotal }} 条记录</span>
                </h3>
                <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">内建扩展通知器统一纳管 · 跟踪推送流水、失败重试与死信队列</p>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <button type="button" class="v2-btn v2-btn-sm" @click="fetchNotifyLogs(); fetchNotifyStats()" :disabled="notifyLogsLoading">
                <RefreshCw :size="13" :class="{ 'animate-spin': notifyLogsLoading }" />
                <span>刷新流水</span>
              </button>
              <button type="button" class="v2-btn v2-btn-sm text-red-400" @click="clearNotifyLogs" :disabled="clearingLogs || notifyLogsTotal === 0">
                <Trash2 :size="13" />
                <span>清空记录</span>
              </button>
            </div>
          </div>

          <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <div class="v2-panel-muted p-3.5">
              <span class="text-[11px] font-semibold text-[var(--v2-ink-subtle)]">累计总投递</span>
              <div class="text-xl font-bold font-mono mt-1 tabular-nums">{{ notifyStats.total }}</div>
            </div>
            <div class="v2-panel-muted p-3.5 border-emerald-500/25">
              <span class="text-[11px] font-semibold text-emerald-400">送达成功率</span>
              <div class="text-xl font-bold font-mono text-emerald-400 mt-1 tabular-nums">
                {{ notifyStats.total > 0 ? ((notifyStats.success / notifyStats.total) * 100).toFixed(1) : '100' }}%
              </div>
            </div>
            <div class="v2-panel-muted p-3.5 border-red-500/25">
              <span class="text-[11px] font-semibold text-red-400">失败次数</span>
              <div class="text-xl font-bold font-mono text-red-400 mt-1 tabular-nums">{{ notifyStats.failed }}</div>
            </div>
            <div class="v2-panel-muted p-3.5 border-amber-500/25">
              <span class="text-[11px] font-semibold text-amber-400">死信隔离 (DLQ)</span>
              <div class="text-xl font-bold font-mono text-amber-400 mt-1 tabular-nums">{{ notifyStats.dlq }}</div>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="text-xs font-semibold text-[var(--v2-ink-muted)] flex items-center gap-1">
                <Filter :size="13" /> 筛选:
              </span>
              <select v-model="notifyFilterStatus" @change="notifyLogsPage = 1; fetchNotifyLogs()" class="v2-input h-8 w-36 text-xs cursor-pointer">
                <option value="all">全部状态</option>
                <option value="success">仅成功 (Success)</option>
                <option value="failed">仅失败 (Failed)</option>
                <option value="dlq">仅死信 (DLQ)</option>
              </select>
              <select v-model="notifyFilterChannel" @change="notifyLogsPage = 1; fetchNotifyLogs()" class="v2-input h-8 w-36 text-xs cursor-pointer">
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
                <option value="Pushover">Pushover</option>
                <option value="Matrix">Matrix</option>
              </select>
            </div>
            <div class="flex items-center gap-2 text-xs">
              <button type="button" class="v2-btn v2-btn-sm" @click="if (notifyLogsPage > 1) { notifyLogsPage--; fetchNotifyLogs() }" :disabled="notifyLogsPage <= 1 || notifyLogsLoading">上一页</button>
              <span class="font-mono text-[var(--v2-ink-muted)]">{{ notifyLogsPage }} / {{ Math.ceil(notifyLogsTotal / notifyLogsPageSize) || 1 }}</span>
              <button type="button" class="v2-btn v2-btn-sm" @click="if (notifyLogsPage * notifyLogsPageSize < notifyLogsTotal) { notifyLogsPage++; fetchNotifyLogs() }" :disabled="notifyLogsPage * notifyLogsPageSize >= notifyLogsTotal || notifyLogsLoading">下一页</button>
            </div>
          </div>

          <div v-if="notifyLogsLoading" class="py-10 flex justify-center">
            <span class="loading loading-spinner loading-sm text-[var(--v2-accent)]"></span>
          </div>
          <div v-else-if="notifyLogs.length === 0" class="text-center py-8 border border-dashed border-[var(--v2-border-strong)] rounded-xl text-xs text-[var(--v2-ink-muted)] space-y-1">
            <p class="font-semibold">暂无投递流水记录</p>
            <p class="text-[11px] text-[var(--v2-ink-subtle)]">当触发下载、入库或点击测试通知时将在此记录流水</p>
          </div>
          <div v-else class="space-y-2.5">
            <div v-for="log in notifyLogs" :key="log.ID" class="v2-panel-muted p-3.5 space-y-2 text-xs">
              <div class="flex items-center justify-between gap-2 flex-wrap">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="badge badge-xs" :class="log.status === 'success' ? 'badge-success' : log.status === 'failed' ? 'badge-error' : 'badge-warning'">
                    {{ log.status === 'success' ? '送达成功' : log.status === 'failed' ? '投递失败' : '死信丢弃' }}
                  </span>
                  <span class="v2-pill font-mono text-[10px]">{{ log.channel }}</span>
                  <span class="font-semibold text-[var(--v2-ink)]">{{ log.title }}</span>
                </div>
                <span class="flex items-center gap-1 text-[11px] font-mono text-[var(--v2-ink-subtle)]">
                  <Clock :size="12" />{{ new Date(log.CreatedAt).toLocaleString() }}
                </span>
              </div>
              <div class="font-mono text-[11px] text-[var(--v2-ink-muted)] break-all bg-[var(--v2-surface)] p-2 rounded-lg border border-[var(--v2-border)]">{{ log.content }}</div>
              <div v-if="log.error_msg" class="text-red-400 font-mono text-[11px] bg-red-500/10 p-2 rounded-lg border border-red-500/20 flex items-start gap-1.5">
                <AlertCircle :size="13" class="shrink-0 mt-0.5" /><span>{{ log.error_msg }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-if="activeTab === 'notify' && !isPluginActive('extended-notifiers')" class="v2-panel p-8 text-center space-y-4">
          <div class="w-12 h-12 rounded-2xl bg-[var(--v2-surface-2)] text-[var(--v2-ink-subtle)] flex items-center justify-center mx-auto">
            <Bell :size="24" />
          </div>
          <div class="space-y-1 max-w-md mx-auto">
            <h4 class="text-sm font-bold text-[var(--v2-ink)]">消息通知推送插件已停用</h4>
            <p class="text-xs text-[var(--v2-ink-muted)] leading-relaxed">消息通知推送功能由内建扩展通知插件提供。启用后可配置多渠道消息推送并查看投递监控流水。</p>
          </div>
          <div class="flex items-center justify-center gap-2">
            <button type="button" @click="enableNotifyPlugin" class="v2-btn v2-btn-primary v2-btn-sm">
              <Sparkles :size="13" />立即启用插件
            </button>
            <button type="button" @click="activeTab = 'plugins'" class="v2-btn v2-btn-sm">前往插件管理</button>
          </div>
        </div>

        <!-- 插件生态面板 -->
        <div v-if="activeTab === 'plugins'" class="v2-panel p-5 sm:p-6 space-y-5">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.plugins.title') }}</h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.plugins.desc') }}</p>
            </div>
            <div class="flex items-center gap-2 flex-wrap">
              <select v-model="pluginSourceFilter" class="v2-input h-8 w-32 text-xs cursor-pointer">
                <option value="all">{{ $t('settings.plugins.allSources') }}</option>
                <option value="official">{{ $t('settings.plugins.official') }}</option>
                <option value="community">{{ $t('settings.plugins.community') }}</option>
              </select>
              <button type="button" class="v2-btn v2-btn-sm" @click="exportPluginsJSON" :disabled="!customPlugins.length || pluginLoading || !!pluginListError">
                <Download :size="13" />
                <span>{{ $t('settings.plugins.exportConfig') }}</span>
              </button>
              <button type="button" class="v2-btn v2-btn-sm" @click="reloadPlugins" :disabled="pluginLoading || pendingPlugins.size > 0">
                <RefreshCw :size="13" :class="{ 'animate-spin': pluginLoading }" />
                <span>{{ $t('settings.plugins.reload') }}</span>
              </button>
              <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="openAddPluginModal">
                <Plus :size="13" />
                <span>{{ $t('settings.plugins.add') }}</span>
              </button>
            </div>
          </div>

          <div v-if="pluginListError" role="alert" class="text-xs text-red-400 flex items-center gap-2">
            <span>{{ $t(pluginListError) }}</span>
            <button type="button" class="v2-btn v2-btn-sm" :disabled="pluginLoading" @click="fetchPlugins()">{{ $t('common.refresh') }}</button>
          </div>
          <div v-else-if="!visiblePlugins.length" class="py-8 text-center text-xs text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.noPluginsFromSource') }}</div>
          <div v-else class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div
              v-for="plugin in visiblePlugins"
              :key="plugin.id"
              class="v2-panel-muted p-4 flex flex-col justify-between gap-3 min-w-0 [overflow-wrap:anywhere]"
              :class="{ 'border-[var(--v2-accent)]/35': plugin.enabled }"
            >
              <div class="space-y-2">
                <div class="flex items-start justify-between gap-2">
                  <div class="flex items-start gap-2.5 min-w-0">
                    <div class="w-8 h-8 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)] text-[var(--v2-accent)] flex items-center justify-center shrink-0 mt-0.5">
                      <component :is="pluginIconComponent(plugin.icon)" :size="16" />
                    </div>
                    <div class="min-w-0">
                      <div class="flex items-center gap-2 flex-wrap">
                        <h4 class="font-bold text-sm text-[var(--v2-ink)]">{{ plugin.name }}</h4>
                        <span class="v2-pill font-mono text-[10px]">v{{ plugin.version || '1.0' }}</span>
                      </div>
                      <div class="flex items-center gap-1.5 mt-1 text-[10px] flex-wrap">
                        <span class="v2-pill h-5">{{ plugin.origin === 'official' ? $t('settings.plugins.official') : $t('settings.plugins.community') }}</span>
                        <span v-if="plugin.status === 'suspended'" class="badge badge-warning badge-xs">{{ $t('settings.plugins.statusSuspended') }}</span>
                        <span v-else-if="plugin.status === 'loading'" class="badge badge-warning badge-xs">{{ $t('settings.plugins.statusLoading') }}</span>
                        <span v-else-if="plugin.status === 'disposing'" class="badge badge-warning badge-xs">{{ $t('settings.plugins.statusDisposing') }}</span>
                        <span v-else-if="plugin.status === 'error'" class="badge badge-error badge-xs">{{ $t('settings.plugins.statusError') }}</span>
                        <span v-else-if="pluginRunning(plugin)" class="text-emerald-400 font-semibold">{{ $t('settings.plugins.statusRunning') }}</span>
                        <span v-else class="text-[var(--v2-ink-subtle)]">{{ $t('settings.plugins.statusStopped') }}</span>
                      </div>
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

                <p v-if="plugin.status === 'suspended' && plugin.missing_dependencies?.length" class="rounded-lg bg-amber-500/10 border border-amber-500/25 p-2.5 text-xs text-amber-400">
                  {{ $t('settings.plugins.missingDependencies', { names: plugin.missing_dependencies.join(', ') }) }}
                </p>
                <div v-else-if="plugin.status === 'error' || plugin.error" class="p-2 rounded-lg bg-red-500/10 border border-red-500/25 text-red-400 text-[11px] font-mono">
                  <AlertCircle :size="12" class="inline mr-1" />{{ plugin.error || $t('settings.plugins.statusError') }}
                </div>
              </div>

              <fieldset v-if="pluginRunning(plugin)" class="min-w-0 pt-2 border-t border-[var(--v2-border)]" :disabled="pluginLoading || pluginSaving || pendingPlugins.has(plugin.id)">
                <PluginPanel v-if="plugin.panel" :key="JSON.stringify([plugin.url, plugin.panel])" :plugin="plugin" />
                <div v-else-if="plugin.id === 'data-migrate'" class="flex items-center justify-between gap-2">
                  <span class="text-[11px] text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.dataMigrateDesc') }}</span>
                  <button type="button" class="v2-btn v2-btn-primary v2-btn-sm" @click="openMigrateModal">
                    <Upload :size="12" />
                    <span>{{ $t('settings.plugins.migrateBtn') }}</span>
                  </button>
                </div>
                <div v-else-if="plugin.id === 'extended-notifiers'" class="space-y-2">
                  <div class="flex items-center justify-between gap-2 flex-wrap text-[11px]">
                    <span class="text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.extendedNotifiersDesc') }}</span>
                    <span class="font-mono text-[10px] text-[var(--v2-ink-subtle)]">{{ $t('settings.plugins.totalNotified') }}: {{ notifyStats.total }}</span>
                  </div>
                  <div class="flex items-center justify-end gap-2">
                    <button type="button" class="v2-btn v2-btn-sm" @click="activeTab = 'notify'">
                      <Bell :size="12" />
                      <span>{{ $t('settings.plugins.notifyChannelSettings') }}</span>
                    </button>
                  </div>
                </div>
                <div v-else-if="plugin.id === 'yuc_schedule'" class="flex items-center justify-between gap-2">
                  <span class="text-[11px] text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.yucScheduleDesc') }}</span>
                  <router-link to="/schedule" class="v2-btn v2-btn-sm shrink-0">
                    <Calendar :size="12" />
                    <span>{{ $t('settings.plugins.viewSchedule') }}</span>
                  </router-link>
                </div>
                <div v-else-if="plugin.id === 'metadata-mapping'" class="flex items-center justify-between gap-2">
                  <span class="text-[11px] text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.metaMappingDesc') }}</span>
                  <button type="button" class="v2-btn v2-btn-sm shrink-0" @click="activeTab = 'metadata'">
                    <Link2 :size="12" />
                    <span>{{ $t('settings.plugins.goMetadataSettings') }}</span>
                  </button>
                </div>
                <div v-else-if="plugin.id === 'standard-naming'" class="flex items-center justify-between gap-2">
                  <span class="text-[11px] text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.geekNamingDesc') }}</span>
                  <button type="button" class="v2-btn v2-btn-sm shrink-0" @click="activeTab = 'paths'">
                    <FolderCheck :size="12" />
                    <span>{{ $t('settings.plugins.goPathSettings') }}</span>
                  </button>
                </div>
                <div v-else-if="plugin.id === 'nfo_scraper'" class="flex items-center justify-between gap-2">
                  <span class="text-[11px] text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.nfoScraperDesc') }}</span>
                  <button type="button" class="v2-btn v2-btn-sm shrink-0" @click="activeTab = 'paths'">
                    <Film :size="12" />
                    <span>{{ $t('settings.plugins.goPathSettings') }}</span>
                  </button>
                </div>
                <div v-else class="flex items-center justify-between gap-2 text-[11px] text-[var(--v2-ink-subtle)]">
                  <span>{{ $t('settings.plugins.serviceRunning') }}</span>
                  <span class="text-emerald-400 font-semibold">{{ $t('settings.plugins.statusRunning') }}</span>
                </div>
              </fieldset>
            </div>
          </div>
        </div>

        <!-- 数据归档、备份列表与迁移 -->
        <div v-if="activeTab === 'backup'" class="space-y-5">
          <div v-if="isPluginActive('data-migrate')" class="v2-panel p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h3 class="text-sm font-bold text-[var(--v2-ink)] flex items-center gap-2">
                <Sparkles class="text-[var(--v2-accent)]" :size="16" />
                <span>{{ $t('settings.migrate.title') }}</span>
              </h3>
              <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.migrate.desc') }}</p>
            </div>
            <button type="button" @click="openMigrateModal" class="v2-btn v2-btn-primary v2-btn-sm shrink-0">
              <Upload :size="13" />
              <span>{{ $t('settings.plugins.migrateBtn') }}</span>
            </button>
          </div>

          <div class="v2-panel p-5 sm:p-6 space-y-4">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-bold text-[var(--v2-ink)]">{{ $t('settings.backup.title') }}</h3>
                <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.backup.desc') }}</p>
              </div>
              <div class="flex items-center gap-3 flex-wrap">
                <label class="flex items-center gap-2 cursor-pointer text-xs">
                  <input type="checkbox" v-model="showBackupEpisodes" class="checkbox" />
                  <span>{{ $t('settings.backup.includeEpisodes') }}</span>
                </label>
                <button type="button" @click="createBackup" :disabled="creatingBackup" class="v2-btn v2-btn-primary v2-btn-sm">
                  <Database :size="13" />
                  <span>{{ creatingBackup ? $t('settings.backup.creating') : $t('settings.backup.createNow') }}</span>
                </button>
              </div>
            </div>

            <div class="overflow-x-auto border border-[var(--v2-border)] rounded-xl">
              <table class="w-full text-left text-xs">
                <thead>
                  <tr class="bg-[var(--v2-surface-2)] text-[var(--v2-ink-muted)] border-b border-[var(--v2-border)]">
                    <th class="px-4 py-2.5 font-semibold">{{ $t('settings.backup.tableFile') }}</th>
                    <th class="px-4 py-2.5 font-semibold">{{ $t('settings.backup.tableSize') }}</th>
                    <th class="px-4 py-2.5 font-semibold">{{ $t('settings.backup.tableTime') }}</th>
                    <th class="px-4 py-2.5 font-semibold text-right">{{ $t('settings.backup.tableAction') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[var(--v2-border)]">
                  <tr v-if="backupList.length === 0">
                    <td colspan="4" class="text-center py-8 text-[var(--v2-ink-subtle)]">{{ $t('settings.backup.empty') }}</td>
                  </tr>
                  <tr v-for="b in backupList" :key="b.name" class="hover:bg-[var(--v2-surface-2)]/60">
                    <td class="px-4 py-2.5 font-mono font-semibold">{{ b.name }}</td>
                    <td class="px-4 py-2.5 font-mono tabular-nums">{{ formatBackupSize(b.size) }}</td>
                    <td class="px-4 py-2.5 text-[var(--v2-ink-muted)]">{{ formatBackupTime(b.mod_time) }}</td>
                    <td class="px-4 py-2.5 text-right">
                      <div class="inline-flex items-center gap-1">
                        <button type="button" @click="downloadBackup(b.name)" class="v2-btn v2-btn-ghost v2-btn-sm px-2" :title="$t('settings.backup.download')">
                          <Download :size="13" />
                        </button>
                        <button type="button" @click="restoreBackup(b.name)" :disabled="restoringBackup" class="v2-btn v2-btn-ghost v2-btn-sm px-2 text-amber-400" :title="$t('settings.backup.restore')">
                          <RotateCcw :size="13" />
                        </button>
                        <button type="button" @click="deleteBackup(b.name)" :disabled="deletingBackup === b.name" class="v2-btn v2-btn-ghost v2-btn-sm px-2 text-red-400" :title="$t('settings.backup.delete')">
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

        <!-- 实时运行日志控制台 -->
        <div v-if="activeTab === 'logs'" class="v2-panel p-5 sm:p-6 space-y-4">
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
                <input type="checkbox" v-model="autoRefreshLogs" class="checkbox" />
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

        <!-- 账户与安全 -->
        <div v-if="activeTab === 'account'" class="space-y-5">
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
      </div>
    </div>

    <!-- 新增 / 导入插件弹窗 -->
    <div v-if="showPluginModal" class="modal modal-open">
      <fieldset :disabled="pluginSaving" class="modal-box max-w-2xl space-y-5">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-bold flex items-center gap-2 text-[var(--v2-ink)]">
              <Sparkles class="text-[var(--v2-accent)]" :size="18" />
              <span>{{ $t('settings.plugins.modalTitle') }}</span>
            </h3>
            <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.plugins.modalDesc') }}</p>
          </div>
          <button type="button" @click="showPluginModal = false" :aria-label="$t('settings.plugins.cancel')" class="v2-btn v2-btn-ghost v2-btn-sm px-2">
            <X :size="16" />
          </button>
        </div>

        <div class="app-segmented w-full grid grid-cols-2">
          <button
            type="button"
            class="app-segmented-btn justify-center"
            :data-active="pluginModalTab === 'webhook'"
            @click="pluginModalTab = 'webhook'"
          >
            {{ $t('settings.plugins.tabWebhook') }}
          </button>
          <button
            type="button"
            class="app-segmented-btn justify-center"
            :data-active="pluginModalTab === 'json'"
            @click="pluginModalTab = 'json'"
          >
            {{ $t('settings.plugins.tabJson') }}
          </button>
        </div>

        <div v-if="pluginModalTab === 'webhook'" class="space-y-3.5">
          <div class="space-y-1">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.nameLabel') }}</label>
            <input v-model="pluginForm.name" type="text" :placeholder="$t('settings.plugins.namePlaceholder')" class="v2-input text-xs" />
          </div>
          <div class="space-y-1">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.urlLabel') }}</label>
            <input v-model="pluginForm.url" type="url" :placeholder="$t('settings.plugins.urlPlaceholder')" class="v2-input font-mono text-xs" />
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div class="space-y-1">
              <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.secretLabel') }}</label>
              <input v-model="pluginForm.secret" type="password" :placeholder="$t('settings.plugins.secretPlaceholder')" class="v2-input font-mono text-xs" />
            </div>
            <div class="space-y-1">
              <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.descLabel') }}</label>
              <input v-model="pluginForm.description" type="text" :placeholder="$t('settings.plugins.descPlaceholder')" class="v2-input text-xs" />
            </div>
          </div>
          <div class="space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.eventsLabel') }}</label>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 v2-panel-muted p-3">
              <label v-for="ev in availablePluginEvents" :key="ev.id" class="flex items-center gap-2 cursor-pointer text-xs">
                <input type="checkbox" :value="ev.id" v-model="pluginForm.events" class="checkbox" />
                <span>{{ $t('settings.plugins.events.' + ev.label) }} ({{ ev.id }})</span>
              </label>
            </div>
          </div>
        </div>

        <div v-else class="space-y-3">
          <div class="flex items-center justify-between gap-2">
            <label class="text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.plugins.jsonLabel') }}</label>
            <label class="v2-btn v2-btn-sm cursor-pointer">
              <Upload :size="12" />
              <span>{{ $t('settings.plugins.selectFile') }}</span>
              <input type="file" accept=".json,application/json" class="hidden" @change="handlePluginFileImport" />
            </label>
          </div>
          <textarea
            :disabled="pluginFileReading"
            v-model="pluginJsonText"
            rows="9"
            class="textarea font-mono text-xs leading-relaxed"
            :placeholder="$t('settings.plugins.jsonPlaceholder')"
          ></textarea>
        </div>

        <p v-if="pluginJsonError" class="text-xs text-red-400 bg-red-500/10 border border-red-500/25 p-3 rounded-lg">{{ pluginJsonError }}</p>

        <div class="modal-action">
          <button type="button" @click="showPluginModal = false" class="v2-btn v2-btn-ghost">{{ $t('settings.plugins.cancel') }}</button>
          <button type="button" @click="submitPluginForm" :disabled="pluginSaving || pluginFileReading" class="v2-btn v2-btn-primary">
            <span v-if="pluginSaving" class="loading loading-spinner loading-xs"></span>
            <span v-else>{{ $t('settings.plugins.confirmImport') }}</span>
          </button>
        </div>
      </fieldset>
    </div>

    <!-- 数据导入与迁移弹窗 -->
    <div v-if="showMigrateModal" class="modal modal-open">
      <div class="modal-box max-w-xl space-y-5">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-bold flex items-center gap-2 text-[var(--v2-ink)]">
              <Database class="text-[var(--v2-accent)]" :size="18" />
              <span>{{ $t('settings.migrate.title') }}</span>
            </h3>
            <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('settings.migrate.desc') }}</p>
          </div>
          <button type="button" @click="showMigrateModal = false" class="v2-btn v2-btn-ghost v2-btn-sm px-2">
            <X :size="16" />
          </button>
        </div>

        <div class="app-segmented w-full grid grid-cols-2">
          <button type="button" class="app-segmented-btn justify-center" :data-active="migrateMode === 'upload'" @click="migrateMode = 'upload'">
            {{ $t('settings.migrate.tabUpload') }}
          </button>
          <button type="button" class="app-segmented-btn justify-center" :data-active="migrateMode === 'path'" @click="migrateMode = 'path'">
            {{ $t('settings.migrate.tabPath') }}
          </button>
        </div>

        <div v-if="migrateMode === 'upload'" class="border border-dashed border-[var(--v2-border-strong)] rounded-xl p-6 text-center bg-[var(--v2-surface-2)] cursor-pointer relative hover:border-[var(--v2-accent)]/60 transition-colors">
          <input type="file" accept=".db,.sqlite,.sqlite3,.json,.zip" class="absolute inset-0 w-full h-full opacity-0 cursor-pointer" @change="onMigrateFileSelect" />
          <div class="flex flex-col items-center gap-2 pointer-events-none">
            <div class="w-11 h-11 rounded-xl bg-[var(--v2-accent)]/12 text-[var(--v2-accent)] flex items-center justify-center">
              <Upload :size="20" />
            </div>
            <p class="text-xs font-semibold">{{ migrateFile ? migrateFile.name : $t('settings.migrate.selectFile') }}</p>
            <p class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('settings.migrate.fileHint') }}</p>
          </div>
        </div>

        <div v-else class="space-y-1.5">
          <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('settings.migrate.pathLabel') }}</label>
          <input v-model="migratePath" type="text" :placeholder="$t('settings.migrate.pathPlaceholder')" class="v2-input font-mono text-xs" />
        </div>

        <div class="v2-panel-muted p-3 text-xs text-[var(--v2-ink-muted)] leading-relaxed">{{ $t('settings.migrate.notice') }}</div>

        <div v-if="migrateResult" class="v2-panel border-emerald-500/30 bg-emerald-500/10 p-4 space-y-2.5">
          <div class="flex items-center gap-2 text-emerald-400 font-bold text-xs">
            <Check :size="15" />
            <span>{{ $t('settings.migrate.resultTitle') }}</span>
          </div>
          <div class="grid grid-cols-3 gap-2 text-center font-mono">
            <div class="p-2 rounded-lg bg-[var(--v2-surface)]">
              <div class="text-base font-bold text-[var(--v2-accent)]">+{{ migrateResult.Subscriptions || 0 }}</div>
              <div class="text-[10px] text-[var(--v2-ink-subtle)]">{{ $t('settings.migrate.subsCount') }}</div>
            </div>
            <div class="p-2 rounded-lg bg-[var(--v2-surface)]">
              <div class="text-base font-bold text-emerald-400">+{{ migrateResult.Episodes || 0 }}</div>
              <div class="text-[10px] text-[var(--v2-ink-subtle)]">{{ $t('settings.migrate.epsCount') }}</div>
            </div>
            <div class="p-2 rounded-lg bg-[var(--v2-surface)]">
              <div class="text-base font-bold">+{{ migrateResult.Downloads || 0 }}</div>
              <div class="text-[10px] text-[var(--v2-ink-subtle)]">{{ $t('settings.migrate.dlsCount') }}</div>
            </div>
          </div>
          <p v-if="migrateResult.Skipped" class="text-xs text-[var(--v2-ink-muted)]">{{ migrateResult.Skipped }} {{ $t('settings.migrate.skippedCount') }}</p>
          <div v-if="migrateResult.Errors && migrateResult.Errors.length" class="text-[11px] text-red-400">
            <div class="font-semibold mb-1">{{ $t('settings.migrate.errorsTitle') }}:</div>
            <ul class="list-disc list-inside space-y-0.5 font-mono">
              <li v-for="err in migrateResult.Errors" :key="err">{{ err }}</li>
            </ul>
          </div>
        </div>

        <p v-if="migrateError" class="text-xs text-red-400 bg-red-500/10 border border-red-500/25 p-3 rounded-lg">{{ migrateError }}</p>

        <div class="modal-action">
          <button type="button" @click="showMigrateModal = false" class="v2-btn v2-btn-ghost">{{ migrateResult ? $t('settings.migrate.closeBtn') : '取消' }}</button>
          <button
            v-if="!migrateResult"
            type="button"
            @click="executeMigration"
            :disabled="migrating || (migrateMode === 'upload' && !migrateFile)"
            class="v2-btn v2-btn-primary"
          >
            <span v-if="migrating" class="loading loading-spinner loading-xs"></span>
            <Upload v-else :size="13" />
            <span>{{ migrating ? $t('settings.migrate.importing') : $t('settings.migrate.startBtn') }}</span>
          </button>
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
