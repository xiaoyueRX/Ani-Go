<script setup lang="ts">
import { ref, onMounted } from "vue"
import { useI18n } from "vue-i18n"
import request from "../utils/request"
import {
  ShieldCheck,
  FolderSync,
  Download,
  Rss,
  Sparkles,
  ChevronRight,
  ChevronLeft,
  Check,
  X,
  RefreshCw,
  HardDrive
} from "lucide-vue-next"

const { t } = useI18n()
const show = ref(false)
const currentStep = ref(0)
const saving = ref(false)
const errorMsg = ref("")

const steps = ["security", "storage", "downloader", "source", "ready"]

const isDefaultPassword = ref(false)
const keepDefaultPassword = ref(false)
const newPassword = ref("")
const confirmPassword = ref("")

const tvBasePath = ref("/vol2/1000/TV/Media/番剧")
const movieBasePath = ref("/vol2/1000/TV/Media/剧场版")
const useHardLink = ref("true")

const presets = {
  fnos: { tv: "/vol2/1000/TV/Media/番剧", movie: "/vol2/1000/TV/Media/剧场版" },
  synology: { tv: "/volume1/video/Anime/TV", movie: "/volume1/video/Anime/Movies" },
  linux: { tv: "/data/media/番剧", movie: "/data/media/剧场版" },
  windows: { tv: "D:/Media/Anime/TV", movie: "D:/Media/Anime/Movies" }
}

function applyPreset(key: "fnos" | "synology" | "linux" | "windows") {
  const p = presets[key]
  if (p) {
    tvBasePath.value = p.tv
    movieBasePath.value = p.movie
  }
}

const dlType = ref("qbittorrent")
const dlHost = ref("http://192.168.1.220:8080")
const dlUser = ref("admin")
const dlPass = ref("adminadmin")
const dlCategory = ref("ani-go")
const dlSecret = ref("")
const dlTesting = ref(false)
const dlTestResult = ref<{ success?: boolean; error?: string } | null>(null)

async function testDownloader() {
  dlTesting.value = true
  dlTestResult.value = null
  try {
    const { data } = await request.post("/downloader/test", {
      type: dlType.value,
      host: dlHost.value,
      username: dlUser.value,
      password: dlPass.value,
      category: dlCategory.value,
      secret: dlSecret.value,
    })
    dlTestResult.value = data
  } catch (e: any) {
    dlTestResult.value = { success: false, error: e.response?.data?.error || e.message }
  } finally {
    dlTesting.value = false
  }
}

const mikanRssUrl = ref("")
const bgmUserToken = ref("")
const STORAGE_KEY = "ani-go-setup-wizard-dismissed"

onMounted(async () => {
  const dismissed = localStorage.getItem(STORAGE_KEY)
  if (dismissed === "true") return
  try {
    const { data: me } = await request.get("/me")
    isDefaultPassword.value = !!me.is_default_password
    const { data: s } = await request.get("/settings")
    if (s) {
      if (s.TV_BASE_PATH) tvBasePath.value = s.TV_BASE_PATH
      if (s.MOVIE_BASE_PATH) movieBasePath.value = s.MOVIE_BASE_PATH
      if (s.USE_HARDLINK !== undefined) useHardLink.value = s.USE_HARDLINK
      if (s.DOWNLOADER_TYPE) dlType.value = s.DOWNLOADER_TYPE
      if (s.QB_HOST) dlHost.value = s.QB_HOST
      if (s.QB_USER) dlUser.value = s.QB_USER
      if (s.MIKAN_RSS_URL) mikanRssUrl.value = s.MIKAN_RSS_URL
      if (s.BGMTV_USER_TOKEN) bgmUserToken.value = s.BGMTV_USER_TOKEN
    }
    if (isDefaultPassword.value || !s?.QB_HOST || !s?.MIKAN_RSS_URL) {
      show.value = true
    }
  } catch {}
})

function skipWizard() {
  localStorage.setItem(STORAGE_KEY, "true")
  show.value = false
}

function nextStep() {
  errorMsg.value = ""
  if (currentStep.value === 0 && isDefaultPassword.value && !keepDefaultPassword.value) {
    if (newPassword.value || confirmPassword.value) {
      if (newPassword.value.length < 6) {
        errorMsg.value = "新密码不能少于 6 位"
        return
      }
      if (newPassword.value !== confirmPassword.value) {
        errorMsg.value = "两次输入的密码不一致"
        return
      }
    }
  }
  if (currentStep.value < steps.length - 1) {
    currentStep.value++
  }
}

function prevStep() {
  errorMsg.value = ""
  if (currentStep.value > 0) currentStep.value--
}

async function finishSetup() {
  saving.value = true
  errorMsg.value = ""
  try {
    if (isDefaultPassword.value && !keepDefaultPassword.value && newPassword.value) {
      await request.post("/user/change-password", {
        old_password: "admin",
        new_password: newPassword.value,
      })
    }
    const payload: Record<string, string> = {
      TV_BASE_PATH: tvBasePath.value,
      MOVIE_BASE_PATH: movieBasePath.value,
      USE_HARDLINK: useHardLink.value,
      DOWNLOADER_TYPE: dlType.value,
    }
    if (dlType.value === "qbittorrent") {
      payload.QB_HOST = dlHost.value
      payload.QB_USER = dlUser.value
      payload.QB_PASS = dlPass.value
      payload.QB_CATEGORY = dlCategory.value
    } else if (dlType.value === "transmission") {
      payload.TR_HOST = dlHost.value
      payload.TR_USER = dlUser.value
      payload.TR_PASS = dlPass.value
    } else if (dlType.value === "aria2") {
      payload.ARIA2_HOST = dlHost.value
      payload.ARIA2_SECRET = dlSecret.value
    }
    if (mikanRssUrl.value) payload.MIKAN_RSS_URL = mikanRssUrl.value
    if (bgmUserToken.value) payload.BGMTV_USER_TOKEN = bgmUserToken.value

    await request.put("/settings", { settings: payload })
    window.showToast?.("🎉 系统配置已成功初始化！", "success")
    localStorage.setItem(STORAGE_KEY, "true")
    show.value = false
    if (newPassword.value) {
      setTimeout(() => window.location.reload(), 1000)
    }
  } catch (e: any) {
    errorMsg.value = e.response?.data?.error || e.message || "保存配置失败"
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-[120] flex items-center justify-center p-3 sm:p-6">
    <div class="absolute inset-0 bg-black/75 backdrop-blur-md transition-opacity" @click="skipWizard"></div>

    <div class="relative w-full max-w-2xl bg-base-100 border border-base-200/80 rounded-[2.5rem] shadow-2xl overflow-hidden flex flex-col max-h-[92vh] animate-in fade-in zoom-in duration-300">
      <div class="px-6 sm:px-10 pt-8 pb-4 border-b border-base-200/60 bg-base-200/30">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-2xl bg-gradient-to-tr from-primary to-secondary flex items-center justify-center text-primary-content shadow-md shadow-primary/20">
              <Sparkles :size="20" />
            </div>
            <div>
              <h2 class="text-xl font-black tracking-tight text-base-content">{{ $t("wizard.title") }}</h2>
              <p class="text-xs font-bold text-base-content/50">{{ $t("wizard.subtitle") }}</p>
            </div>
          </div>
          <button @click="skipWizard" class="btn btn-ghost btn-sm rounded-xl text-xs font-bold text-base-content/50 hover:text-base-content">
            {{ $t("wizard.skipBtn") }}
          </button>
        </div>

        <div class="flex items-center justify-between gap-1 sm:gap-2 mt-4">
          <div
            v-for="(st, idx) in steps"
            :key="st"
            class="flex-1 flex flex-col items-center gap-1.5 cursor-pointer"
            @click="idx <= currentStep ? currentStep = idx : null"
          >
            <div
              class="w-full h-1.5 rounded-full transition-all duration-300"
              :class="idx <= currentStep ? 'bg-primary shadow-sm shadow-primary/30' : 'bg-base-300/60'"
            ></div>
            <span
              class="text-[10px] font-black uppercase tracking-wider transition-colors hidden sm:inline"
              :class="idx === currentStep ? 'text-primary' : (idx < currentStep ? 'text-base-content/70' : 'text-base-content/30')"
            >
              {{ $t(`wizard.steps.${st}`) }}
            </span>
          </div>
        </div>
      </div>

      <div class="p-6 sm:p-10 overflow-y-auto flex-1 space-y-6">
        <div v-if="errorMsg" class="alert alert-error text-xs font-bold rounded-2xl py-2.5">
          <span>{{ errorMsg }}</span>
        </div>

        <!-- Step 0: 安全初始化 -->
        <div v-if="currentStep === 0" class="space-y-5 animate-in fade-in duration-200">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-2xl bg-warning/10 text-warning flex items-center justify-center shrink-0">
              <ShieldCheck :size="24" />
            </div>
            <div class="space-y-1">
              <h3 class="text-base font-black text-base-content">{{ $t("wizard.security.title") }}</h3>
              <p class="text-xs text-base-content/60 leading-relaxed font-medium">{{ $t("wizard.security.desc") }}</p>
            </div>
          </div>

          <div v-if="isDefaultPassword" class="space-y-4 pt-2">
            <div class="space-y-1.5">
              <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.security.newPass") }}</label>
              <input
                v-model="newPassword"
                type="password"
                :placeholder="$t('wizard.security.newPassPlaceholder')"
                class="input input-bordered w-full rounded-2xl font-bold bg-base-200/50 focus:bg-base-100"
                :disabled="keepDefaultPassword"
              />
            </div>

            <div class="space-y-1.5">
              <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.security.confirmPass") }}</label>
              <input
                v-model="confirmPassword"
                type="password"
                :placeholder="$t('wizard.security.confirmPassPlaceholder')"
                class="input input-bordered w-full rounded-2xl font-bold bg-base-200/50 focus:bg-base-100"
                :disabled="keepDefaultPassword"
              />
            </div>

            <label class="flex items-center gap-2.5 cursor-pointer pt-1">
              <input type="checkbox" v-model="keepDefaultPassword" class="checkbox checkbox-warning checkbox-sm rounded-lg" />
              <span class="text-xs font-bold text-base-content/60">{{ $t("wizard.security.keepDefault") }}</span>
            </label>
          </div>
          <div v-else class="p-4 rounded-2xl bg-success/10 text-success text-xs font-bold flex items-center gap-2">
            <Check :size="16" />
            <span>管理员密码已设定为个性化密码，可直接进入下一步。</span>
          </div>
        </div>

        <!-- Step 1: 存储与目录 -->
        <div v-if="currentStep === 1" class="space-y-6 animate-in fade-in duration-200">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-2xl bg-primary/10 text-primary flex items-center justify-center shrink-0">
              <FolderSync :size="24" />
            </div>
            <div class="space-y-1">
              <h3 class="text-base font-black text-base-content">{{ $t("wizard.storage.title") }}</h3>
              <p class="text-xs text-base-content/60 leading-relaxed font-medium">{{ $t("wizard.storage.desc") }}</p>
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.storage.presetLabel") }}</label>
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
              <button type="button" @click="applyPreset('fnos')" class="btn btn-outline btn-sm rounded-xl font-bold text-xs hover:btn-primary">fnOS 飞牛</button>
              <button type="button" @click="applyPreset('synology')" class="btn btn-outline btn-sm rounded-xl font-bold text-xs hover:btn-primary">群晖 DSM</button>
              <button type="button" @click="applyPreset('linux')" class="btn btn-outline btn-sm rounded-xl font-bold text-xs hover:btn-primary">通用 Linux</button>
              <button type="button" @click="applyPreset('windows')" class="btn btn-outline btn-sm rounded-xl font-bold text-xs hover:btn-primary">Windows</button>
            </div>
          </div>

          <div class="space-y-4">
            <div class="space-y-1.5">
              <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.storage.tvPath") }}</label>
              <input v-model="tvBasePath" type="text" class="input input-bordered w-full rounded-2xl font-mono text-xs font-bold bg-base-200/50" />
            </div>
            <div class="space-y-1.5">
              <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.storage.moviePath") }}</label>
              <input v-model="movieBasePath" type="text" class="input input-bordered w-full rounded-2xl font-mono text-xs font-bold bg-base-200/50" />
            </div>
          </div>

          <div class="space-y-2 pt-2 border-t border-base-200/60">
            <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.storage.modeLabel") }}</label>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <label class="p-4 rounded-2xl border cursor-pointer transition-all flex flex-col gap-1" :class="useHardLink === 'true' ? 'border-primary bg-primary/5 shadow-sm' : 'border-base-200 bg-base-200/30'">
                <div class="flex items-center justify-between">
                  <span class="font-black text-xs text-base-content">规范硬链接 (Hardlink)</span>
                  <input type="radio" value="true" v-model="useHardLink" class="radio radio-primary radio-sm" />
                </div>
                <span class="text-[10px] text-base-content/50 font-medium">适合做种挂机，不占额外硬盘空间</span>
              </label>
              <label class="p-4 rounded-2xl border cursor-pointer transition-all flex flex-col gap-1" :class="useHardLink === 'false' ? 'border-primary bg-primary/5 shadow-sm' : 'border-base-200 bg-base-200/30'">
                <div class="flex items-center justify-between">
                  <span class="font-black text-xs text-base-content">直接移动 (Move)</span>
                  <input type="radio" value="false" v-model="useHardLink" class="radio radio-primary radio-sm" />
                </div>
                <span class="text-[10px] text-base-content/50 font-medium">直接移动归档至媒体库，不做种</span>
              </label>
            </div>
          </div>
        </div>

        <!-- Step 2: 下载客户端 -->
        <div v-if="currentStep === 2" class="space-y-6 animate-in fade-in duration-200">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-2xl bg-secondary/10 text-secondary flex items-center justify-center shrink-0">
              <Download :size="24" />
            </div>
            <div class="space-y-1">
              <h3 class="text-base font-black text-base-content">{{ $t("wizard.downloader.title") }}</h3>
              <p class="text-xs text-base-content/60 leading-relaxed font-medium">{{ $t("wizard.downloader.desc") }}</p>
            </div>
          </div>

          <div class="grid grid-cols-3 gap-2">
            <button type="button" @click="dlType = 'qbittorrent'" class="btn btn-sm rounded-xl font-bold text-xs" :class="dlType === 'qbittorrent' ? 'btn-primary' : 'btn-outline'">qBittorrent</button>
            <button type="button" @click="dlType = 'transmission'" class="btn btn-sm rounded-xl font-bold text-xs" :class="dlType === 'transmission' ? 'btn-primary' : 'btn-outline'">Transmission</button>
            <button type="button" @click="dlType = 'aria2'" class="btn btn-sm rounded-xl font-bold text-xs" :class="dlType === 'aria2' ? 'btn-primary' : 'btn-outline'">Aria2</button>
          </div>

          <div class="space-y-3.5">
            <div class="space-y-1.5">
              <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.downloader.host") }}</label>
              <input v-model="dlHost" type="text" class="input input-bordered w-full rounded-2xl font-mono text-xs font-bold bg-base-200/50" />
            </div>

            <div v-if="dlType !== 'aria2'" class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.downloader.user") }}</label>
                <input v-model="dlUser" type="text" class="input input-bordered w-full rounded-2xl text-xs font-bold bg-base-200/50" />
              </div>
              <div class="space-y-1.5">
                <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.downloader.pass") }}</label>
                <input v-model="dlPass" type="password" class="input input-bordered w-full rounded-2xl text-xs font-bold bg-base-200/50" />
              </div>
            </div>

            <div v-if="dlType === 'aria2'" class="space-y-1.5">
              <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.downloader.secret") }}</label>
              <input v-model="dlSecret" type="password" class="input input-bordered w-full rounded-2xl text-xs font-bold bg-base-200/50" />
            </div>

            <div class="pt-2 flex items-center justify-between">
              <button type="button" @click="testDownloader" :disabled="dlTesting" class="btn btn-outline btn-sm rounded-xl font-bold gap-2 text-xs">
                <RefreshCw v-if="dlTesting" :size="13" class="animate-spin" />
                <HardDrive v-else :size="13" />
                <span>{{ dlTesting ? $t("wizard.downloader.testing") : $t("wizard.downloader.testBtn") }}</span>
              </button>
              <div v-if="dlTestResult" class="text-xs font-bold">
                <span v-if="dlTestResult.success" class="text-success flex items-center gap-1"><Check :size="14" /> {{ $t("wizard.downloader.testSuccess") }}</span>
                <span v-else class="text-error flex items-center gap-1"><X :size="14" /> {{ $t("wizard.downloader.testFailed") }}{{ dlTestResult.error }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Step 3: 数据源与账号 -->
        <div v-if="currentStep === 3" class="space-y-6 animate-in fade-in duration-200">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-2xl bg-accent/10 text-accent flex items-center justify-center shrink-0">
              <Rss :size="24" />
            </div>
            <div class="space-y-1">
              <h3 class="text-base font-black text-base-content">{{ $t("wizard.source.title") }}</h3>
              <p class="text-xs text-base-content/60 leading-relaxed font-medium">{{ $t("wizard.source.desc") }}</p>
            </div>
          </div>

          <div class="space-y-4">
            <div class="space-y-1.5">
              <div class="flex items-center justify-between">
                <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.source.mikanTokenLabel") }}</label>
                <a href="https://mikanani.me" target="_blank" class="text-[10px] text-primary hover:underline font-bold">前往 Mikan 登录 ↗</a>
              </div>
              <input v-model="mikanRssUrl" type="text" :placeholder="$t('wizard.source.mikanPlaceholder')" class="input input-bordered w-full rounded-2xl font-mono text-xs font-bold bg-base-200/50" />
              <p class="text-[10px] text-base-content/40 leading-relaxed">{{ $t("wizard.source.mikanTip") }}</p>
            </div>

            <div class="space-y-1.5 pt-2 border-t border-base-200/60">
              <div class="flex items-center justify-between">
                <label class="text-xs font-black text-base-content/70 uppercase tracking-wider">{{ $t("wizard.source.bgmTokenLabel") }}</label>
                <a href="https://bgm.tv/dev/app" target="_blank" class="text-[10px] text-primary hover:underline font-bold">申请令牌 ↗</a>
              </div>
              <input v-model="bgmUserToken" type="text" :placeholder="$t('wizard.source.bgmPlaceholder')" class="input input-bordered w-full rounded-2xl font-mono text-xs font-bold bg-base-200/50" />
              <p class="text-[10px] text-base-content/40 leading-relaxed">{{ $t("wizard.source.bgmTip") }}</p>
            </div>
          </div>
        </div>

        <!-- Step 4: 准备就绪 -->
        <div v-if="currentStep === 4" class="space-y-6 animate-in fade-in duration-200">
          <div class="text-center py-4 space-y-2">
            <div class="w-16 h-16 rounded-3xl bg-gradient-to-tr from-primary to-secondary text-primary-content flex items-center justify-center mx-auto shadow-xl shadow-primary/25">
              <Sparkles :size="32" />
            </div>
            <h3 class="text-2xl font-black text-base-content pt-2">{{ $t("wizard.ready.title") }}</h3>
            <p class="text-xs text-base-content/60 max-w-md mx-auto leading-relaxed font-medium">{{ $t("wizard.ready.desc") }}</p>
          </div>

          <div class="bg-base-200/40 border border-base-200 rounded-3xl p-5 space-y-2.5">
            <h4 class="text-xs font-black uppercase tracking-wider text-base-content/40 mb-3">{{ $t("wizard.ready.summaryTitle") }}</h4>
            <div class="flex items-center justify-between text-xs font-bold">
              <span class="text-base-content/70">动漫媒体目录</span>
              <span class="font-mono text-primary">{{ tvBasePath }}</span>
            </div>
            <div class="flex items-center justify-between text-xs font-bold">
              <span class="text-base-content/70">下载引擎</span>
              <span class="badge badge-neutral badge-xs font-bold uppercase">{{ dlType }}</span>
            </div>
            <div class="flex items-center justify-between text-xs font-bold">
              <span class="text-base-content/70">Mikan 订阅源</span>
              <span :class="mikanRssUrl ? 'text-success' : 'text-base-content/40'">{{ mikanRssUrl ? $t("wizard.ready.configured") : $t("wizard.ready.unconfigured") }}</span>
            </div>
            <div class="flex items-center justify-between text-xs font-bold">
              <span class="text-base-content/70">Bangumi 令牌</span>
              <span :class="bgmUserToken ? 'text-success' : 'text-base-content/40'">{{ bgmUserToken ? $t("wizard.ready.configured") : $t("wizard.ready.unconfigured") }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Bottom Nav Bar -->
      <div class="px-6 sm:px-10 py-5 bg-base-200/50 border-t border-base-200/60 flex items-center justify-between">
        <button
          v-if="currentStep > 0"
          type="button"
          @click="prevStep"
          class="btn btn-ghost btn-sm rounded-xl font-bold gap-1 text-xs"
        >
          <ChevronLeft :size="16" />
          <span>{{ $t("wizard.prevBtn") }}</span>
        </button>
        <div v-else></div>

        <div class="flex items-center gap-2">
          <button
            v-if="currentStep < steps.length - 1"
            type="button"
            @click="nextStep"
            class="btn btn-primary btn-sm rounded-xl px-6 font-black tracking-wide text-xs gap-1.5 shadow-md shadow-primary/20"
          >
            <span>{{ $t("wizard.nextBtn") }}</span>
            <ChevronRight :size="16" />
          </button>

          <button
            v-else
            type="button"
            @click="finishSetup"
            :disabled="saving"
            class="btn btn-primary btn-sm rounded-xl px-7 font-black tracking-wide text-xs gap-2 shadow-lg shadow-primary/25 hover:scale-[1.02] active:scale-95 transition-all"
          >
            <RefreshCw v-if="saving" :size="14" class="animate-spin" />
            <Sparkles v-else :size="14" />
            <span>{{ saving ? $t("wizard.saving") : $t("wizard.finishBtn") }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.animate-in {
  animation-fill-mode: both;
}
@keyframes fade-in {
  from { opacity: 0; }
  to { opacity: 1; }
}
@keyframes zoom-in {
  from { transform: scale(0.96); }
  to { transform: scale(1); }
}
.fade-in { animation-name: fade-in; }
.zoom-in { animation-name: zoom-in; }
</style>