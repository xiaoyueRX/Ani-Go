import { ref, onMounted } from 'vue'
import request from '../utils/request'

export const CURRENT_VERSION = 'v0.6.0'
const VERSION_KEY = 'ani-go-last-version'
const AUTO_UPDATE_KEY = 'ani-go-auto-update'

export const currentVersion = ref(CURRENT_VERSION)

export const formatVersion = (v: string) => (v ? (v.startsWith('v') ? v : `v${v}`) : 'v0.6.0')

export interface VersionInfo {
  version: string
  changelog: string[]
}

export function useVersion() {
  const latestVersion = ref('')
  const changelog = ref<string[]>([])
  const showChangelog = ref(false)
  const hasNewVersion = ref(false)

  const checkVersion = async () => {
    try {
      const { data } = await request.get<VersionInfo>('/version')
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
      console.error('Failed to fetch version info:', e)
    }
  }

  const checkGitHubUpdate = async (force: boolean = false) => {
    if (!force) {
      const autoUpdate = localStorage.getItem(AUTO_UPDATE_KEY) === 'true'
      if (!autoUpdate) return { checked: false, hasUpdate: false }
    }

    try {
      // Use GitHub API to check latest release
      const res = await fetch('https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest')
      if (res.ok) {
        const data = await res.json()
        const latest = data.tag_name
        if (latest && latest !== currentVersion.value) {
          latestVersion.value = latest
          hasNewVersion.value = true
          return { checked: true, hasUpdate: true, latest }
        }
        hasNewVersion.value = false
        return { checked: true, hasUpdate: false, latest: currentVersion.value }
      }
      return { checked: true, hasUpdate: false, error: `GitHub API HTTP ${res.status}` }
    } catch (e: any) {
      console.error('Failed to check GitHub update:', e)
      return { checked: true, hasUpdate: false, error: e?.message || '网络连接超时' }
    }
  }

  return {
    currentVersion,
    latestVersion,
    changelog,
    showChangelog,
    hasNewVersion,
    checkVersion,
    checkGitHubUpdate
  }
}
