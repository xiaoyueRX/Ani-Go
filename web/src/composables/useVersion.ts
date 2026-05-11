import { ref, onMounted } from 'vue'
import request from '../utils/request'

export const CURRENT_VERSION = 'v0.2.0'
const VERSION_KEY = 'ani-go-last-version'
const AUTO_UPDATE_KEY = 'ani-go-auto-update'

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

  const checkGitHubUpdate = async () => {
    const autoUpdate = localStorage.getItem(AUTO_UPDATE_KEY) === 'true'
    if (!autoUpdate) return

    try {
      // Use GitHub API to check latest release
      const res = await fetch('https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest')
      if (res.ok) {
        const data = await res.json()
        const latest = data.tag_name
        if (latest !== CURRENT_VERSION) {
          latestVersion.value = latest
          hasNewVersion.value = true
        }
      }
    } catch (e) {
      console.error('Failed to check GitHub update:', e)
    }
  }

  return {
    latestVersion,
    changelog,
    showChangelog,
    hasNewVersion,
    checkVersion,
    checkGitHubUpdate
  }
}
