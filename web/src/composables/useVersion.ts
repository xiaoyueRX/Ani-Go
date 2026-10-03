import { ref } from "vue"
import request from "../utils/request"

export const CURRENT_VERSION = "v0.6.0"
const VERSION_KEY = "ani-go-last-version"

export const currentVersion = ref(CURRENT_VERSION)
export const formatVersion = (v: string) => (v ? (v.startsWith("v") ? v : `v${v}`) : "v0.6.0")

export interface VersionInfo {
  version: string
  changelog: string[]
}

export interface BackendUpdateInfo {
  current_version: string
  latest_version: string
  has_update: boolean
  release_notes?: string
  published_at?: string
  environment: "binary" | "docker"
  can_self_update: boolean
  docker_sock: boolean
  error?: string
}

export interface UpdateProgress {
  status: "idle" | "checking" | "downloading" | "applying" | "restarting" | "success" | "failed"
  progress: number
  message: string
  error?: string
}

export function useVersion() {
  const latestVersion = ref("")
  const changelog = ref<string[]>([])
  const showChangelog = ref(false)
  const hasNewVersion = ref(false)
  const releaseNotes = ref("")
  const canSelfUpdate = ref(true)
  const environment = ref<"binary" | "docker">("binary")
  const updating = ref(false)
  const updateProgress = ref<UpdateProgress>({
    status: "idle",
    progress: 0,
    message: "准备就绪",
  })

  const checkVersion = async () => {
    try {
      const { data } = await request.get<VersionInfo>("/version")
      if (data?.version) {
        currentVersion.value = data.version
      }
      const lastVersion = localStorage.getItem(VERSION_KEY)
      if (lastVersion && lastVersion !== data.version) {
        changelog.value = data.changelog
        showChangelog.value = true
      }
      localStorage.setItem(VERSION_KEY, data.version)
    } catch (e) {
      console.error("Failed to fetch version info:", e)
    }
  }

  const checkGitHubUpdate = async (_force: boolean = false) => {
    try {
      const { data } = await request.get<BackendUpdateInfo>("/system/update/check")
      if (data) {
        if (data.current_version) currentVersion.value = data.current_version
        if (data.latest_version) latestVersion.value = data.latest_version
        hasNewVersion.value = !!data.has_update
        releaseNotes.value = data.release_notes || ""
        canSelfUpdate.value = data.can_self_update
        environment.value = data.environment
        return { checked: true, hasUpdate: !!data.has_update, latest: data.latest_version }
      }
      return { checked: false, hasUpdate: false }
    } catch (e: any) {
      console.error("Failed to check update via backend:", e)
      return { checked: false, hasUpdate: false, error: e?.message || "检查更新失败" }
    }
  }

  let progressTimer: any = null

  const pollProgress = async () => {
    try {
      const { data } = await request.get<UpdateProgress>("/system/update/progress")
      if (data) {
        updateProgress.value = data
        if (data.status === "success" || data.status === "restarting") {
          // 服务正在重启，轮询等待服务恢复后刷新页面
          clearInterval(progressTimer)
          pollHealthAndReload()
        } else if (data.status === "failed") {
          updating.value = false
          clearInterval(progressTimer)
        }
      }
    } catch {}
  }

  const pollHealthAndReload = () => {
    let attempts = 0
    const healthTimer = setInterval(async () => {
      attempts++
      try {
        const res = await fetch("/api/health", { cache: "no-store" })
        if (res.ok && attempts > 2) {
          clearInterval(healthTimer)
          window.location.reload()
        }
      } catch {}
      if (attempts > 60) {
        clearInterval(healthTimer)
        window.location.reload()
      }
    }, 1500)
  }

  const triggerUpdate = async () => {
    updating.value = true
    try {
      await request.post("/system/update/execute")
      updateProgress.value = {
        status: "downloading",
        progress: 10,
        message: "已下发更新任务...",
      }
      progressTimer = setInterval(pollProgress, 1000)
    } catch (e: any) {
      updating.value = false
      window.showToast?.(e.response?.data?.error || "触发更新失败", "error")
    }
  }

  return {
    currentVersion,
    latestVersion,
    changelog,
    showChangelog,
    hasNewVersion,
    releaseNotes,
    canSelfUpdate,
    environment,
    updating,
    updateProgress,
    checkVersion,
    checkGitHubUpdate,
    triggerUpdate,
  }
}
