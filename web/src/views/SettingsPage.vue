<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  Check, Antenna, Download, 
  Folder, Bell, Cpu, 
  Settings, Timer, Lock, 
  FileText, Eye, EyeOff 
} from 'lucide-vue-next'

const { t } = useI18n()
const router = useRouter()
const settings = ref<Record<string, string>>({})
const loading = ref(true)
const error = ref('')
const saved = ref(false)
const activeTab = ref('mikan')
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
    const { data } = await request.post('/mikan/test-mirrors', {}, { timeout: 15000 })
    mirrorResults.value = data || []
  } catch (e: any) {
    error.value = t('settings.error.testFailed')
  } finally {
    mirrorTesting.value = false
  }
}

async function selectMirror(domain: string) {
  try {
    await request.post('/mikan/select-mirror', { domain })
    setVal('MIKAN_DOMAIN', domain)
    selectedMirror.value = domain
  } catch (e: any) {
    error.value = t('settings.error.switchFailed')
  }
}

interface FieldDef {
  label: string; key: string; placeholder: string; type?: string; hint?: string; selectOptions?: {label: string, value: string}[]
}

interface TabDef {
  key: string; label: string; icon: any
  sections: { title: string; desc: string; fields: FieldDef[] }[]
}

const tabs = computed<TabDef[]>(() => [
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
    { label: t('settings.fields.ova'), key: 'OVA_BASE_PATH', placeholder: '/TV/Media/OVA' },
  ]}]},
  { key: 'notify', label: t('settings.tabs.notify'), icon: Bell, sections: [
    { title: t('settings.sections.im'), desc: t('settings.sections.imDesc'), fields: [
      { label: t('settings.fields.tgToken'), key: 'TELEGRAM_BOT_TOKEN', placeholder: '123456:ABC...' },
      { label: t('settings.fields.tgId'), key: 'TELEGRAM_CHAT_ID', placeholder: '123456789' },
      { label: t('settings.fields.discord'), key: 'DISCORD_WEBHOOK', placeholder: 'Endpoint URL' },
      { label: t('settings.fields.slack'), key: 'SLACK_WEBHOOK', placeholder: 'Endpoint URL' },
      { label: t('settings.fields.onebotHost'), key: 'ONEBOT_HOST', placeholder: 'http://localhost:3000' },
      { label: t('settings.fields.onebotToken'), key: 'ONEBOT_TOKEN', placeholder: 'Access token' },
    ]},
    { title: t('settings.sections.enterprise'), desc: t('settings.sections.enterpriseDesc'), fields: [
      { label: t('settings.fields.wecom'), key: 'WECOM_WEBHOOK', placeholder: 'Endpoint URL' },
      { label: t('settings.fields.feishu'), key: 'FEISHU_WEBHOOK', placeholder: 'Endpoint URL' },
      { label: t('settings.fields.dingtalk'), key: 'DINGTALK_WEBHOOK', placeholder: 'Endpoint URL' },
    ]},
  ]},
  { key: 'ai', label: t('settings.tabs.ai'), icon: Cpu, sections: [
    { title: t('settings.sections.ai'), desc: t('settings.sections.aiDesc'), fields: [
      { label: t('settings.fields.protocol'), key: 'AI_PROTOCOL', placeholder: 'auto' },
      { label: t('settings.fields.endpoint'), key: 'AI_ENDPOINT', placeholder: 'https://api.openai.com/v1/chat/completions' },
      { label: t('settings.fields.key'), key: 'AI_API_KEY', placeholder: 'API key', type: 'password' },
      { label: t('settings.fields.model'), key: 'AI_MODEL', placeholder: 'gpt-4o-mini' },
    ]},
    { title: t('settings.sections.vendor'), desc: t('settings.sections.vendorDesc'), fields: [
      { label: t('settings.fields.gemini'), key: 'GEMINI_API_KEY', placeholder: 'Google key', type: 'password' },
      { label: t('settings.fields.claude'), key: 'CLAUDE_API_KEY', placeholder: 'Anthropic key', type: 'password' },
      { label: t('settings.fields.ollama'), key: 'OLLAMA_HOST', placeholder: 'http://localhost:11434' },
    ]},
  ]},
  { key: 'advanced', label: t('settings.tabs.advanced'), icon: Settings, sections: [
    { title: t('settings.sections.kernel'), desc: t('settings.sections.kernelDesc'), fields: [
      { label: t('settings.fields.bind'), key: 'HOST', placeholder: '0.0.0.0' },
      { label: t('settings.fields.port'), key: 'PORT', placeholder: '20001' },
      { label: t('settings.fields.nyaa'), key: 'NYAA_DOMAIN', placeholder: 'nyaa.si' },
    ]},
    { title: t('settings.sections.schedule'), desc: t('settings.sections.scheduleDesc'), fields: [
      { label: t('settings.fields.rssInterval'), key: 'RSS_INTERVAL_MIN', placeholder: '30' },
      { label: t('settings.fields.syncInterval'), key: 'SUPPLEMENT_INTERVAL_HOURS', placeholder: '24' },
      { label: t('settings.fields.ioInterval'), key: 'ORGANIZER_INTERVAL_MIN', placeholder: '2' },
    ]},
    { title: t('settings.sections.update'), desc: t('settings.sections.updateDesc'), fields: [
      { label: t('settings.fields.autoUpdate'), key: 'AUTO_CHECK_UPDATE', placeholder: '', type: 'switch', hint: t('settings.fields.autoUpdateHint') },
    ]},
  ]},
  { key: 'account', label: t('settings.tabs.account'), icon: Lock, sections: [] },
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
  fetchSettings()
  fetchLogs()
})
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
        
        <!-- Special: Mikan Latency Card -->
        <div v-if="activeTab === 'mikan'" class="bg-base-100 rounded-3xl lg:rounded-[2.5rem] border border-base-200/60 shadow-xl overflow-hidden group">
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
                  <span v-if="getVal('MIKAN_DOMAIN') === r.domain" class="text-[9px] font-black uppercase tracking-widest text-primary ml-4">{{ $t('settings.mikan.currentRoute') }}</span>
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
                      @change="(e: any) => setVal(field.key, e.target.checked ? 'true' : 'false')" />
                  </template>
                  <template v-else-if="field.type === 'select' && field.selectOptions">
                    <select 
                      :value="getVal(field.key) || field.selectOptions[0].value"
                      @change="(e: any) => setVal(field.key, e.target.value)"
                      class="w-full bg-base-200/50 border border-transparent focus:border-primary/20 focus:bg-base-100 focus:ring-4 focus:ring-primary/5 rounded-xl lg:rounded-2xl pl-12 lg:pl-14 pr-10 lg:pr-12 py-3.5 lg:py-4 transition-all outline-none font-bold text-sm lg:text-base appearance-none cursor-pointer">
                      <option v-for="opt in field.selectOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
                    </select>
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
                  </template>
              </div>
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
            <h2 class="text-xl font-black tracking-tight italic">系统日志</h2>
            <p class="text-[10px] font-black uppercase tracking-widest opacity-30">{{ logs.length }} 条记录</p>
          </div>
          <button class="btn btn-ghost btn-sm rounded-xl text-[10px] font-black uppercase tracking-widest" @click="fetchLogs" :disabled="logLoading">
            <span v-if="logLoading" class="loading loading-spinner loading-xs"></span>
            <template v-else>刷新</template>
          </button>
        </div>
        <div class="bg-base-300/30 rounded-2xl p-4 max-h-80 overflow-y-auto font-mono text-[11px] leading-relaxed space-y-1">
          <div v-for="(line, i) in logs" :key="i" class="opacity-70 hover:opacity-100 transition-opacity">
            {{ line }}
          </div>
          <div v-if="logs.length === 0 && !logLoading" class="text-center py-8 text-[10px] font-bold opacity-30">暂无日志数据</div>
        </div>
      </div>
    </div>

    <!-- Version Footer -->
    <div class="flex justify-center pb-10">
      <div class="px-6 py-2 rounded-full bg-base-200/50 border border-base-300/50 text-[10px] font-black uppercase tracking-[0.2em] opacity-40">
        Ani-Go Engine v0.2.0 • Build 20260510
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
