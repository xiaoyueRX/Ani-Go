import { ref } from 'vue'
import request from '../utils/request'
import { dedupeTags } from '../utils/subscriptionRules'

const STORAGE_KEY = 'anigo_default_subgroups'

function readLocalDefaults(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? dedupeTags(parsed) : []
  } catch {
    return []
  }
}

function writeLocalDefaults(names: string[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(names))
  } catch {
    // 忽略隐私模式等存储受限场景
  }
}

const defaultSubgroups = ref<string[]>(readLocalDefaults())
let loadedFromServer = false

export function useDefaultSubgroups() {
  async function loadDefaultSubgroups(force = false): Promise<string[]> {
    if (loadedFromServer && !force) return defaultSubgroups.value
    try {
      const { data } = await request.get<Record<string, string>>('/settings')
      const raw = data?.DEFAULT_SUBGROUPS
      if (typeof raw === 'string' && raw.trim()) {
        const parsed = JSON.parse(raw)
        if (Array.isArray(parsed)) {
          const clean = dedupeTags(parsed)
          defaultSubgroups.value = clean
          writeLocalDefaults(clean)
        }
      } else if (raw === '' || raw === '[]') {
        defaultSubgroups.value = []
        writeLocalDefaults([])
      }
      loadedFromServer = true
    } catch {
      // 未登录或离线场景回退至 localStorage 缓存
    }
    return defaultSubgroups.value
  }

  async function saveDefaultSubgroups(names: string[]): Promise<string[]> {
    const clean = dedupeTags(names)
    defaultSubgroups.value = clean
    writeLocalDefaults(clean)
    try {
      await request.put('/settings', {
        settings: {
          DEFAULT_SUBGROUPS: JSON.stringify(clean),
        },
      })
      loadedFromServer = true
    } catch {
      // 允许未认证/测试环境下保留本地设置
    }
    return clean
  }

  function isDefaultSubgroup(name: string): boolean {
    const norm = name.trim().toLowerCase()
    if (!norm) return false
    return defaultSubgroups.value.some(item => item.toLowerCase() === norm)
  }

  return {
    defaultSubgroups,
    loadDefaultSubgroups,
    saveDefaultSubgroups,
    isDefaultSubgroup,
  }
}
