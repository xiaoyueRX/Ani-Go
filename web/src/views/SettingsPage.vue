<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

const settings = ref<Record<string, string>>({})
const loading = ref(true)
const error = ref('')
const saved = ref(false)
const activeTab = ref('mikan')
const showPasswords = ref<Set<string>>(new Set())

interface FieldDef {
  label: string; key: string; placeholder: string; type?: string; hint?: string
}

interface TabDef {
  key: string; label: string; icon: string
  sections: { title: string; desc: string; fields: FieldDef[] }[]
}

const tabs: TabDef[] = [
  { key: 'mikan', label: 'Mikan', icon: 'antenna', sections: [{ title: '基础配置', desc: 'Mikan 资源站连接信息', fields: [
    { label: 'Mikan 个人 RSS 地址', key: 'MIKAN_RSS_URL', placeholder: 'https://mikanani.me/RSS/MyBangumi?token=...', hint: '在 Mikan 网站登录后 → 头像 → RSS订阅 → 复制链接' },
    { label: 'Mikan 主域名', key: 'MIKAN_DOMAIN', placeholder: 'mikanani.me' },
    { label: 'Mikan 代理域名', key: 'MIKAN_PROXY_DOMAIN', placeholder: 'GFW 环境代理地址', hint: '国内网络无法直连时使用' },
    { label: 'Mikan 镜像域名', key: 'MIKAN_MIRROR_DOMAINS', placeholder: 'mikanani.me,mikanime.tv', hint: '逗号分隔，自动回退' },
  ]}]},
  { key: 'downloader', label: '下载器', icon: 'download', sections: [
    { title: '全局设置', desc: '默认下载引擎', fields: [
      { label: '默认下载器', key: 'DEFAULT_DOWNLOADER', placeholder: 'qbittorrent', hint: 'qbittorrent / transmission / aria2' },
    ]},
    { title: 'qBittorrent', desc: '最常用的下载客户端', fields: [
      { label: 'qBittorrent 地址', key: 'QB_HOST', placeholder: 'http://localhost:8081' },
      { label: 'qBittorrent 用户名', key: 'QB_USER', placeholder: 'admin' },
      { label: 'qBittorrent 密码', key: 'QB_PASS', placeholder: '密码', type: 'password' },
      { label: 'qBittorrent 分类', key: 'QB_CATEGORY', placeholder: 'ani-go' },
    ]},
    { title: 'Transmission', desc: '备选下载客户端', fields: [
      { label: 'Transmission 地址', key: 'TR_HOST', placeholder: 'http://localhost:9091' },
      { label: 'Transmission 用户名', key: 'TR_USER', placeholder: '用户名' },
      { label: 'Transmission 密码', key: 'TR_PASS', placeholder: '密码', type: 'password' },
    ]},
    { title: 'Aria2', desc: '轻量级下载客户端', fields: [
      { label: 'Aria2 地址', key: 'ARIA2_HOST', placeholder: 'http://localhost:6800' },
      { label: 'Aria2 RPC Secret', key: 'ARIA2_SECRET', placeholder: 'rpc-secret', type: 'password' },
    ]},
  ]},
  { key: 'paths', label: '目录', icon: 'folder', sections: [{ title: '存储路径', desc: '番剧文件和数据库的存放位置', fields: [
    { label: '数据库路径', key: 'DB_PATH', placeholder: '/data/ani-go.db', hint: 'Windows: D:/data/ani-go.db' },
    { label: '番剧根目录', key: 'TV_BASE_PATH', placeholder: '/TV/Media/番剧' },
    { label: '剧场版目录', key: 'MOVIE_BASE_PATH', placeholder: '/TV/Media/剧场版' },
    { label: 'OVA 目录', key: 'OVA_BASE_PATH', placeholder: '/TV/Media/OVA' },
  ]}]},
  { key: 'notify', label: '通知', icon: 'notification', sections: [
    { title: '即时通讯', desc: '即时通讯平台推送', fields: [
      { label: 'Telegram Bot Token', key: 'TELEGRAM_BOT_TOKEN', placeholder: '123456:ABC...' },
      { label: 'Telegram Chat ID', key: 'TELEGRAM_CHAT_ID', placeholder: '123456789' },
      { label: 'Discord Webhook', key: 'DISCORD_WEBHOOK', placeholder: 'https://discord.com/api/webhooks/...' },
      { label: 'Slack Webhook', key: 'SLACK_WEBHOOK', placeholder: 'https://hooks.slack.com/services/...' },
      { label: 'QQ OneBot 地址', key: 'ONEBOT_HOST', placeholder: 'http://localhost:3000', hint: 'NapCat / go-cqhttp' },
      { label: 'QQ OneBot Token', key: 'ONEBOT_TOKEN', placeholder: 'token' },
      { label: 'QQ 用户 ID', key: 'ONEBOT_USER_ID', placeholder: '123456789', hint: '私聊目标' },
      { label: 'QQ 群号', key: 'ONEBOT_GROUP_ID', placeholder: '987654321', hint: '群聊目标' },
    ]},
    { title: '办公协作', desc: '团队协作平台推送', fields: [
      { label: '企业微信 Webhook', key: 'WECOM_WEBHOOK', placeholder: 'https://qyapi.weixin.qq.com/...' },
      { label: '飞书 Webhook', key: 'FEISHU_WEBHOOK', placeholder: 'https://open.feishu.cn/...' },
      { label: '钉钉 Webhook', key: 'DINGTALK_WEBHOOK', placeholder: 'https://oapi.dingtalk.com/...' },
      { label: 'Matrix Homeserver', key: 'MATRIX_HOMESERVER', placeholder: 'https://matrix.org' },
      { label: 'Matrix Token', key: 'MATRIX_TOKEN', placeholder: 'syt_...' },
      { label: 'Matrix Room ID', key: 'MATRIX_ROOM_ID', placeholder: '!abc123:matrix.org' },
    ]},
    { title: '社交平台', desc: '社交媒体消息推送', fields: [
      { label: 'LINE Channel Token', key: 'LINE_CHANNEL_TOKEN', placeholder: 'LINE API token' },
      { label: 'LINE User ID', key: 'LINE_USER_ID', placeholder: 'Uxxx' },
      { label: 'WhatsApp Phone ID', key: 'WHATSAPP_PHONE_ID', placeholder: '123456789' },
      { label: 'WhatsApp Token', key: 'WHATSAPP_TOKEN', placeholder: 'Meta Cloud API token' },
      { label: 'WhatsApp 收件人', key: 'WHATSAPP_TO', placeholder: '8613800138000' },
    ]},
    { title: '推送通道', desc: '通用推送服务', fields: [
      { label: 'Server酱 Key', key: 'SERVERCHAN_KEY', placeholder: 'SCT...' },
      { label: 'Bark Device Key', key: 'BARK_DEVICE_KEY', placeholder: 'iOS key' },
      { label: 'Pushover Token', key: 'PUSHOVER_TOKEN', placeholder: 'token' },
      { label: 'Pushover User', key: 'PUSHOVER_USER', placeholder: 'user key' },
      { label: 'Gotify URL', key: 'GOTIFY_URL', placeholder: 'https://gotify.example.com' },
      { label: 'Gotify Token', key: 'GOTIFY_TOKEN', placeholder: 'token' },
      { label: 'ntfy URL', key: 'NTFY_URL', placeholder: 'https://ntfy.sh/mytopic' },
    ]},
    { title: '邮件', desc: 'SMTP 邮件推送', fields: [
      { label: 'SMTP 服务器', key: 'EMAIL_SMTP_HOST', placeholder: 'smtp.gmail.com' },
      { label: 'SMTP 端口', key: 'EMAIL_SMTP_PORT', placeholder: '587' },
      { label: 'Email 用户名', key: 'EMAIL_USERNAME', placeholder: 'xxx@gmail.com' },
      { label: 'Email 密码', key: 'EMAIL_PASSWORD', placeholder: '密码', type: 'password' },
      { label: 'Email 发件人', key: 'EMAIL_FROM', placeholder: 'xxx@gmail.com' },
      { label: 'Email 收件人', key: 'EMAIL_TO', placeholder: 'admin@example.com', hint: '多个用逗号分隔' },
    ]},
  ]},
  { key: 'ai', label: 'AI', icon: 'cpu', sections: [
    { title: 'AI 服务配置', desc: '智能辅助模块连接信息', fields: [
      { label: 'AI 协议', key: 'AI_PROTOCOL', placeholder: 'auto', hint: 'openai/google/anthropic/ollama/auto' },
      { label: 'AI Endpoint', key: 'AI_ENDPOINT', placeholder: 'https://api.openai.com/v1/chat/completions' },
      { label: 'AI API Key', key: 'AI_API_KEY', placeholder: 'sk-...', type: 'password' },
      { label: 'AI 模型', key: 'AI_MODEL', placeholder: 'gpt-4o-mini' },
    ]},
    { title: '特定平台密钥', desc: '各 AI 提供商独立配置', fields: [
      { label: 'Gemini API Key', key: 'GEMINI_API_KEY', placeholder: 'Google Gemini key', type: 'password' },
      { label: 'Claude API Key', key: 'CLAUDE_API_KEY', placeholder: 'sk-ant-...', type: 'password' },
      { label: 'Ollama Host', key: 'OLLAMA_HOST', placeholder: 'http://localhost:11434' },
      { label: 'Ollama Model', key: 'OLLAMA_MODEL', placeholder: 'llama3' },
    ]},
  ]},
  { key: 'metadata', label: '元数据', icon: 'document', sections: [{ title: '元数据提供者', desc: '番剧元数据来源', fields: [
    { label: 'TMDB API Key', key: 'TMDB_API_KEY', placeholder: 'TMDB API key', type: 'password' },
    { label: 'TMDB 镜像域名', key: 'TMDB_MIRROR_DOMAINS', placeholder: '逗号分隔' },
    { label: 'BGM.tv User Token', key: 'BGMTV_USER_TOKEN', placeholder: 'bangumi token', type: 'password' },
    { label: 'BGM.tv 镜像域名', key: 'BGMTV_MIRROR_DOMAINS', placeholder: 'api.bgm.tv,api.bangumi.tv' },
  ]}]},
  { key: 'advanced', label: '高级', icon: 'setting', sections: [
    { title: '服务设置', desc: '影响系统运行的核心参数', fields: [
      { label: '监听地址', key: 'HOST', placeholder: '0.0.0.0', hint: '修改后需重启生效' },
      { label: '服务器端口', key: 'PORT', placeholder: '20001', hint: '修改后需重启生效' },
      { label: 'Nyaa 域名', key: 'NYAA_DOMAIN', placeholder: 'nyaa.si', hint: '留空禁用' },
      { label: 'ACG.RIP 域名', key: 'ACGRIP_DOMAIN', placeholder: 'acg.rip' },
      { label: 'AnimeTosho 域名', key: 'ANIMETOSHO_DOMAIN', placeholder: 'feed.animetosho.org' },
    ]},
    { title: '定时任务', desc: '后台任务执行间隔', fields: [
      { label: 'RSS 轮询间隔 (分钟)', key: 'RSS_INTERVAL_MIN', placeholder: '30' },
      { label: '补全间隔 (小时)', key: 'SUPPLEMENT_INTERVAL_HOURS', placeholder: '24' },
      { label: '整理间隔 (分钟)', key: 'ORGANIZER_INTERVAL_MIN', placeholder: '2' },
    ]},
  ]},
]

const allFields = computed(() => {
  const m: Record<string, FieldDef> = {}
  for (const tab of tabs)
    for (const section of tab.sections)
      for (const f of section.fields) m[f.key] = f
  return m
})

function getVal(key: string): string { return settings.value[key] || '' }
function setVal(key: string, val: string) { settings.value[key] = val }
function isConfigured(key: string): boolean { return getVal(key).length > 0 }

function togglePassword(key: string) {
  if (showPasswords.value.has(key)) showPasswords.value.delete(key)
  else showPasswords.value.add(key)
}

function inputType(field: FieldDef): string {
  if (field.type !== 'password') return 'text'
  return showPasswords.value.has(field.key) ? 'text' : 'password'
}

async function fetchSettings() {
  loading.value = true; error.value = ''
  try {
    const { data } = await request.get('/settings')
    settings.value = (data as Record<string, string>) || {}
  } catch (e: any) {
    error.value = e.response?.data?.error || '加载设置失败'
  } finally { loading.value = false }
}

async function saveAll() {
  error.value = ''; saved.value = false
  const changed: Record<string, string> = {}
  for (const key of Object.keys(allFields.value)) {
    const val = settings.value[key] ?? ''
    if (val !== '') changed[key] = val
  }
  if (Object.keys(changed).length === 0) {
    saved.value = true; setTimeout(() => { saved.value = false }, 3000); return
  }
  try {
    await request.put('/settings', { settings: changed })
    saved.value = true; setTimeout(() => { saved.value = false }, 3000)
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
      <button class="btn btn-primary btn-sm gap-1" @click="saveAll">
        <IconSax name="check" :size="16" /> 保存所有设置
      </button>
    </div>

    <div v-if="saved" class="alert alert-success mb-4 shadow-sm">
      <IconSax name="check" class="shrink-0" />
      <span>设置已保存，部分配置需重启后生效</span>
    </div>
    <div v-if="error" class="alert alert-error mb-4 shadow-sm">
      <IconSax name="warning" class="shrink-0" />
      <span>{{ error }}</span>
      <button class="btn btn-ghost btn-sm" @click="error = ''">
        <IconSax name="close" :size="16" />
      </button>
    </div>

    <div v-if="loading" class="flex justify-center py-16">
      <span class="loading loading-spinner loading-lg"></span>
    </div>

    <div v-else class="flex flex-col lg:flex-row gap-6">
      <div class="flex flex-row lg:flex-col gap-1 overflow-x-auto lg:w-36 shrink-0 -mx-3 px-3 lg:mx-0 lg:px-0">
        <button v-for="tab in tabs" :key="tab.key"
          class="btn btn-sm gap-2 justify-start"
          :class="activeTab === tab.key ? 'btn-primary' : 'btn-ghost'"
          @click="activeTab = tab.key">
          <IconSax :name="tab.icon" :size="16" />
          {{ tab.label }}
        </button>
      </div>

      <div class="flex-1 min-w-0 space-y-6">
        <div v-for="section in tabs.find(t => t.key === activeTab)?.sections" :key="section.title">
          <div class="mb-3">
            <h2 class="text-base font-semibold flex items-center gap-2">
              {{ section.title }}
              <span class="badge badge-xs"
                :class="section.fields.some(f => isConfigured(f.key)) ? 'badge-success' : 'badge-ghost'">
                {{ section.fields.filter(f => isConfigured(f.key)).length }}/{{ section.fields.length }}
              </span>
            </h2>
            <p class="text-xs text-base-content/40">{{ section.desc }}</p>
          </div>

          <div class="bg-base-100 rounded-box border border-base-200 divide-y divide-base-200">
            <div v-for="field in section.fields" :key="field.key"
              class="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3 px-4 py-2.5">
              <label class="sm:w-36 shrink-0 text-sm flex items-center gap-1.5">
                <span class="truncate">{{ field.label }}</span>
                <IconSax v-if="isConfigured(field.key)" name="check" :size="12" class="text-success shrink-0" />
              </label>
              <div class="flex-1 flex items-center gap-1">
                <div class="relative flex-1 max-w-sm">
                  <input :type="inputType(field)" :value="getVal(field.key)"
                    @input="(e: Event) => setVal(field.key, (e.target as HTMLInputElement).value)"
                    :placeholder="field.placeholder"
                    class="input input-bordered input-sm w-full pr-8"
                    :class="{ 'border-success/50': isConfigured(field.key) }" />
                  <button v-if="field.type === 'password' && getVal(field.key)"
                    class="absolute right-1 top-1/2 -translate-y-1/2 btn btn-ghost btn-xs btn-square"
                    @click="togglePassword(field.key)">
                    <IconSax :name="showPasswords.has(field.key) ? 'lock' : 'user'" :size="14" />
                  </button>
                </div>
              </div>
              <p v-if="field.hint" class="text-xs text-base-content/40 sm:w-44 shrink-0 hidden sm:block">{{ field.hint }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
