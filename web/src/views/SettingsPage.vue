<script setup lang="ts">
import { ref, onMounted } from 'vue'
import request from '../utils/request'

const settings = ref<Record<string, string>>({})
const loading = ref(true)
const error = ref('')
const saved = ref(false)
const activeTab = ref('mikan')

const tabs = [
  { key: 'mikan', label: 'Mikan', icon: '📡' },
  { key: 'downloader', label: '下载器', icon: '⬇️' },
  { key: 'paths', label: '目录', icon: '📁' },
  { key: 'notify', label: '通知', icon: '🔔' },
  { key: 'ai', label: 'AI', icon: '🤖' },
  { key: 'metadata', label: '元数据', icon: '📋' },
  { key: 'advanced', label: '高级', icon: '⚙️' },
]

const fieldDefs: Record<string, { label: string; placeholder: string; type?: string; hint?: string }[]> = {
  mikan: [
    { label: 'Mikan 个人 RSS 地址', placeholder: 'https://mikanani.me/RSS/MyBangumi?token=...', hint: '必填。在 Mikan 网站登录后 → 头像 → RSS订阅 → 复制链接' },
    { label: 'Mikan 主域名', placeholder: 'mikanani.me' },
    { label: 'Mikan 代理域名', placeholder: 'GFW 环境下的代理地址', hint: '国内网络无法直连时使用' },
    { label: 'Mikan 镜像域名', placeholder: 'mikanani.me,mikanime.tv', hint: '逗号分隔，主域名不可用时自动回退' },
  ],
  downloader: [
    { label: '默认下载器', placeholder: 'qbittorrent', hint: 'qbittorrent / transmission / aria2' },
    { label: 'qBittorrent 地址', placeholder: 'http://localhost:8081' },
    { label: 'qBittorrent 用户名', placeholder: 'admin' },
    { label: 'qBittorrent 密码', placeholder: '密码', type: 'password' },
    { label: 'qBittorrent 分类', placeholder: 'ani-go' },
    { label: 'Transmission 地址', placeholder: 'http://localhost:9091' },
    { label: 'Transmission 用户名', placeholder: '用户名' },
    { label: 'Transmission 密码', placeholder: '密码', type: 'password' },
    { label: 'Aria2 地址', placeholder: 'http://localhost:6800' },
    { label: 'Aria2 RPC Secret', placeholder: 'rpc-secret', type: 'password' },
  ],
  paths: [
    { label: '数据库路径', placeholder: '/data/ani-go.db', hint: 'Windows 示例: D:/data/ani-go.db' },
    { label: '番剧根目录', placeholder: '/TV/Media/番剧', hint: 'Windows 示例: D:/TV/Media/番剧' },
    { label: '剧场版目录', placeholder: '/TV/Media/剧场版' },
    { label: 'OVA 目录', placeholder: '/TV/Media/OVA' },
  ],
  notify: [
    { label: 'Telegram Bot Token', placeholder: '123456:ABC...' },
    { label: 'Telegram Chat ID', placeholder: '123456789' },
    { label: 'Discord Webhook', placeholder: 'https://discord.com/api/webhooks/...' },
    { label: '企业微信 Webhook', placeholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/...' },
    { label: '飞书 Webhook', placeholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/...' },
    { label: '钉钉 Webhook', placeholder: 'https://oapi.dingtalk.com/robot/...' },
    { label: 'QQ OneBot 地址', placeholder: 'http://localhost:3000', hint: 'NapCat / go-cqhttp / Lagrange' },
    { label: 'QQ OneBot Token', placeholder: 'token' },
    { label: 'QQ 用户 ID', placeholder: '123456789', hint: '私聊目标' },
    { label: 'QQ 群号', placeholder: '987654321', hint: '群聊目标' },
    { label: 'Slack Webhook', placeholder: 'https://hooks.slack.com/services/...' },
    { label: 'Matrix Homeserver', placeholder: 'https://matrix.org' },
    { label: 'Matrix Token', placeholder: 'syt_...' },
    { label: 'Matrix Room ID', placeholder: '!abc123:matrix.org' },
    { label: 'LINE Channel Token', placeholder: 'LINE Messaging API token' },
    { label: 'LINE User ID', placeholder: 'Uxxx' },
    { label: 'WhatsApp Phone ID', placeholder: '123456789' },
    { label: 'WhatsApp Token', placeholder: 'Meta Cloud API token' },
    { label: 'WhatsApp 收件人', placeholder: '8613800138000' },
    { label: 'Server酱 Key', placeholder: 'SCT...' },
    { label: 'Bark Device Key', placeholder: 'iOS 推送 key' },
    { label: 'Pushover Token', placeholder: 'token' },
    { label: 'Pushover User', placeholder: 'user key' },
    { label: 'Gotify URL', placeholder: 'https://gotify.example.com' },
    { label: 'Gotify Token', placeholder: 'token' },
    { label: 'ntfy URL', placeholder: 'https://ntfy.sh/mytopic' },
    { label: 'Email SMTP 服务器', placeholder: 'smtp.gmail.com' },
    { label: 'Email SMTP 端口', placeholder: '587' },
    { label: 'Email 用户名', placeholder: 'xxx@gmail.com' },
    { label: 'Email 密码', placeholder: '密码', type: 'password' },
    { label: 'Email 发件人', placeholder: 'xxx@gmail.com' },
    { label: 'Email 收件人', placeholder: 'admin@example.com', hint: '多个用逗号分隔' },
  ],
  ai: [
    { label: 'AI 协议', placeholder: 'auto', hint: 'openai / google / anthropic / ollama / auto' },
    { label: 'AI Endpoint', placeholder: 'https://api.openai.com/v1/chat/completions' },
    { label: 'AI API Key', placeholder: 'sk-...', type: 'password' },
    { label: 'AI 模型', placeholder: 'gpt-4o-mini' },
    { label: 'Gemini API Key', placeholder: 'Google Gemini key', type: 'password' },
    { label: 'Claude API Key', placeholder: 'sk-ant-...', type: 'password' },
    { label: 'Ollama Host', placeholder: 'http://localhost:11434' },
    { label: 'Ollama Model', placeholder: 'llama3' },
  ],
  metadata: [
    { label: 'TMDB API Key', placeholder: 'TMDB API key', type: 'password' },
    { label: 'TMDB 镜像域名', placeholder: '逗号分隔' },
    { label: 'BGM.tv User Token', placeholder: 'bangumi token', type: 'password' },
    { label: 'BGM.tv 镜像域名', placeholder: 'api.bgm.tv,api.bangumi.tv' },
  ],
  advanced: [
    { label: '服务器端口', placeholder: '20001', hint: '修改后需重启生效' },
    { label: '额外资源站 (Nyaa)', placeholder: 'nyaa.si', hint: '留空禁用' },
    { label: '额外资源站 (ACGRIP)', placeholder: 'acg.rip' },
    { label: '额外资源站 (AnimeTosho)', placeholder: 'feed.animetosho.org' },
    { label: 'RSS 轮询间隔 (分钟)', placeholder: '30' },
    { label: '补全间隔 (小时)', placeholder: '24' },
    { label: '文件整理间隔 (分钟)', placeholder: '2' },
  ],
}

const keyToLabel: Record<string, string> = {}
for (const [_tab, fields] of Object.entries(fieldDefs)) {
  for (const f of fields) {
    keyToLabel[f.label] = f.label
  }
}

function fieldKey(label: string): string {
  const map: Record<string, string> = {
    'Mikan 个人 RSS 地址': 'MIKAN_RSS_URL',
    'Mikan 主域名': 'MIKAN_DOMAIN',
    'Mikan 代理域名': 'MIKAN_PROXY_DOMAIN',
    'Mikan 镜像域名': 'MIKAN_MIRROR_DOMAINS',
    '默认下载器': 'DEFAULT_DOWNLOADER',
    'qBittorrent 地址': 'QB_HOST',
    'qBittorrent 用户名': 'QB_USER',
    'qBittorrent 密码': 'QB_PASS',
    'qBittorrent 分类': 'QB_CATEGORY',
    'Transmission 地址': 'TR_HOST',
    'Transmission 用户名': 'TR_USER',
    'Transmission 密码': 'TR_PASS',
    'Aria2 地址': 'ARIA2_HOST',
    'Aria2 RPC Secret': 'ARIA2_SECRET',
    '数据库路径': 'DB_PATH',
    '番剧根目录': 'TV_BASE_PATH',
    '剧场版目录': 'MOVIE_BASE_PATH',
    'OVA 目录': 'OVA_BASE_PATH',
    'Telegram Bot Token': 'TELEGRAM_BOT_TOKEN',
    'Telegram Chat ID': 'TELEGRAM_CHAT_ID',
    'Discord Webhook': 'DISCORD_WEBHOOK',
    '企业微信 Webhook': 'WECOM_WEBHOOK',
    '飞书 Webhook': 'FEISHU_WEBHOOK',
    '钉钉 Webhook': 'DINGTALK_WEBHOOK',
    'QQ OneBot 地址': 'ONEBOT_HOST',
    'QQ OneBot Token': 'ONEBOT_TOKEN',
    'QQ 用户 ID': 'ONEBOT_USER_ID',
    'QQ 群号': 'ONEBOT_GROUP_ID',
    'Slack Webhook': 'SLACK_WEBHOOK',
    'Matrix Homeserver': 'MATRIX_HOMESERVER',
    'Matrix Token': 'MATRIX_TOKEN',
    'Matrix Room ID': 'MATRIX_ROOM_ID',
    'LINE Channel Token': 'LINE_CHANNEL_TOKEN',
    'LINE User ID': 'LINE_USER_ID',
    'WhatsApp Phone ID': 'WHATSAPP_PHONE_ID',
    'WhatsApp Token': 'WHATSAPP_TOKEN',
    'WhatsApp 收件人': 'WHATSAPP_TO',
    'Server酱 Key': 'SERVERCHAN_KEY',
    'Bark Device Key': 'BARK_DEVICE_KEY',
    'Pushover Token': 'PUSHOVER_TOKEN',
    'Pushover User': 'PUSHOVER_USER',
    'Gotify URL': 'GOTIFY_URL',
    'Gotify Token': 'GOTIFY_TOKEN',
    'ntfy URL': 'NTFY_URL',
    'Email SMTP 服务器': 'EMAIL_SMTP_HOST',
    'Email SMTP 端口': 'EMAIL_SMTP_PORT',
    'Email 用户名': 'EMAIL_USERNAME',
    'Email 密码': 'EMAIL_PASSWORD',
    'Email 发件人': 'EMAIL_FROM',
    'Email 收件人': 'EMAIL_TO',
    'AI 协议': 'AI_PROTOCOL',
    'AI Endpoint': 'AI_ENDPOINT',
    'AI API Key': 'AI_API_KEY',
    'AI 模型': 'AI_MODEL',
    'Gemini API Key': 'GEMINI_API_KEY',
    'Claude API Key': 'CLAUDE_API_KEY',
    'Ollama Host': 'OLLAMA_HOST',
    'Ollama Model': 'OLLAMA_MODEL',
    'TMDB API Key': 'TMDB_API_KEY',
    'TMDB 镜像域名': 'TMDB_MIRROR_DOMAINS',
    'BGM.tv User Token': 'BGMTV_USER_TOKEN',
    'BGM.tv 镜像域名': 'BGMTV_MIRROR_DOMAINS',
    '服务器端口': 'PORT',
    '额外资源站 (Nyaa)': 'NYAA_DOMAIN',
    '额外资源站 (ACGRIP)': 'ACGRIP_DOMAIN',
    '额外资源站 (AnimeTosho)': 'ANIMETOSHO_DOMAIN',
    'RSS 轮询间隔 (分钟)': 'RSS_INTERVAL_MIN',
    '补全间隔 (小时)': 'SUPPLEMENT_INTERVAL_HOURS',
    '文件整理间隔 (分钟)': 'ORGANIZER_INTERVAL_MIN',
  }
  return map[label] || label.toUpperCase()
}

function getVal(label: string): string {
  return settings.value[fieldKey(label)] || ''
}

function setVal(label: string, val: string) {
  settings.value[fieldKey(label)] = val
}

async function fetchSettings() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.get('/settings')
    settings.value = (data as Record<string, string>) || {}
  } catch (e: any) {
    error.value = e.response?.data?.error || '加载设置失败'
  } finally {
    loading.value = false
  }
}

async function saveAll() {
  error.value = ''
  saved.value = false
  const changed: Record<string, string> = {}
  for (const tab of tabs) {
    for (const field of fieldDefs[tab.key]) {
      const key = fieldKey(field.label)
      const val = settings.value[key] ?? ''
      if (val !== '') {
        changed[key] = val
      }
    }
  }
  if (Object.keys(changed).length === 0) {
    saved.value = true
    setTimeout(() => { saved.value = false }, 3000)
    return
  }
  try {
    await request.put('/settings', { settings: changed })
    saved.value = true
    setTimeout(() => { saved.value = false }, 3000)
  } catch (e: any) {
    error.value = e.response?.data?.error || '保存设置失败'
  }
}

onMounted(fetchSettings)
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">设置</h1>
      <button class="btn btn-primary" @click="saveAll">保存所有设置</button>
    </div>

    <div v-if="saved" class="alert alert-success mb-4">
      <span>设置已保存，部分配置需重启后生效</span>
    </div>

    <div v-if="error" class="alert alert-error mb-4">
      <span>{{ error }}</span>
      <button class="btn btn-ghost btn-sm" @click="error = ''">✕</button>
    </div>

    <div v-if="loading" class="flex justify-center py-16">
      <span class="loading loading-spinner loading-lg"></span>
    </div>

    <div v-else>
      <!-- Tabs -->
      <div class="tabs tabs-box mb-4 bg-base-200">
        <a
          v-for="tab in tabs" :key="tab.key"
          class="tab" :class="{ 'tab-active': activeTab === tab.key }"
          @click="activeTab = tab.key"
        >{{ tab.icon }} {{ tab.label }}</a>
      </div>

      <!-- Tab content -->
      <div class="card bg-base-100 shadow">
        <div class="card-body">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div v-for="field in fieldDefs[activeTab]" :key="field.label">
              <label class="label">
                <span class="label-text font-medium">{{ field.label }}</span>
              </label>
              <input
                :type="field.type || 'text'"
                :value="getVal(field.label)"
                @input="(e: Event) => setVal(field.label, (e.target as HTMLInputElement).value)"
                :placeholder="field.placeholder"
                class="input input-bordered w-full"
              />
              <label v-if="field.hint" class="label">
                <span class="label-text-alt text-base-content/50">{{ field.hint }}</span>
              </label>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
