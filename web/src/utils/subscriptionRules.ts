// 订阅过滤规则工具：对应后端 internal/scheduler/scheduler.go 中的 FilterJSON / ExcludedKeywords / AllowedSubgroups 契约。

export const RESOLUTION_OPTIONS = ['', '2160p', '1080p', '720p'] as const

export const PRESET_INCLUDE_TAGS = ['简日', 'CHS', 'CHT', '简繁', 'WebRip', 'Baha']
export const PRESET_EXCLUDE_TAGS = ['720p', 'HEVC', 'RAW', '繁日', '合集', '内封']
export const COMMON_SUBGROUPS = [
  'LoliHouse',
  'ANi',
  'Lilith-Raws',
  'NC-Raws',
  '桜都字幕组',
  '喵萌奶茶屋',
  '北宇治字幕组',
  '霜庭云花Sub',
  'SweetSub',
  '千夏字幕组',
]

export interface SubscriptionRuleState {
  resolution: string
  includeKeywords: string[]
  excludeKeywords: string[]
  allowedSubgroups: string[]
  extraFilterKeys: Record<string, unknown>
}

export interface SerializedRuleState {
  filter_json: string
  excluded_keywords: string
  allowed_subgroups: string
}

export interface RuleSummaryItem {
  kind: 'rss' | 'resolution' | 'include' | 'exclude' | 'subgroups' | 'path'
  value: string
}

export function dedupeTags(values: unknown[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const item of values) {
    if (typeof item !== 'string') continue
    const clean = item.trim()
    if (!clean) continue
    const norm = clean.toLowerCase()
    if (seen.has(norm)) continue
    seen.add(norm)
    out.push(clean)
  }
  return out
}

function safeParseStringArray(raw?: string | null): string[] {
  if (!raw || !raw.trim()) return []
  try {
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) return dedupeTags(parsed)
  } catch {
    return []
  }
  return []
}

const KNOWN_FILTER_KEYS = new Set([
  'resolution',
  'include_keywords',
  'exclude_keywords',
  'keywords',
  'exclude',
])

export function parseRuleState(
  filterJson?: string | null,
  excludedKeywords?: string | null,
  allowedSubgroups?: string | null,
): SubscriptionRuleState {
  let resolution = ''
  let includeKeywords: string[] = []
  let filterExcludeKeywords: string[] = []
  const extraFilterKeys: Record<string, unknown> = {}

  if (filterJson && filterJson.trim()) {
    try {
      const parsed = JSON.parse(filterJson)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const obj = parsed as Record<string, unknown>
        if (typeof obj.resolution === 'string') {
          resolution = obj.resolution.trim()
        }
        const rawInc = Array.isArray(obj.include_keywords)
          ? obj.include_keywords
          : (Array.isArray(obj.keywords) ? obj.keywords : [])
        includeKeywords = dedupeTags(rawInc)

        const rawExc = Array.isArray(obj.exclude_keywords)
          ? obj.exclude_keywords
          : (Array.isArray(obj.exclude) ? obj.exclude : [])
        filterExcludeKeywords = dedupeTags(rawExc)

        for (const [k, v] of Object.entries(obj)) {
          if (!KNOWN_FILTER_KEYS.has(k) && v !== undefined) {
            extraFilterKeys[k] = v
          }
        }
      }
    } catch {
      // 保留空可视化态，用户可在高级 JSON 栏查看/修正
    }
  }

  const excludedArr = safeParseStringArray(excludedKeywords)
  const excludeKeywords = dedupeTags([...filterExcludeKeywords, ...excludedArr])
  const subgroups = safeParseStringArray(allowedSubgroups)

  return {
    resolution,
    includeKeywords,
    excludeKeywords,
    allowedSubgroups: subgroups,
    extraFilterKeys,
  }
}

export function serializeRuleState(state: SubscriptionRuleState): SerializedRuleState {
  const filterObj: Record<string, unknown> = { ...state.extraFilterKeys }
  const res = state.resolution.trim()
  const include = dedupeTags(state.includeKeywords)
  const exclude = dedupeTags(state.excludeKeywords)
  const subgroups = dedupeTags(state.allowedSubgroups)

  if (res) filterObj.resolution = res
  else delete filterObj.resolution

  if (include.length > 0) filterObj.include_keywords = include
  else delete filterObj.include_keywords

  delete filterObj.exclude_keywords
  delete filterObj.keywords
  delete filterObj.exclude

  return {
    filter_json: Object.keys(filterObj).length > 0 ? JSON.stringify(filterObj) : '',
    excluded_keywords: JSON.stringify(exclude),
    allowed_subgroups: JSON.stringify(subgroups),
  }
}

export function summarizeRules(sub: {
  rss_url?: string
  filter_json?: string
  excluded_keywords?: string
  allowed_subgroups?: string
  custom_path?: string
}): RuleSummaryItem[] {
  const items: RuleSummaryItem[] = []
  if (sub.rss_url?.trim()) {
    items.push({ kind: 'rss', value: sub.rss_url.trim() })
  }
  const parsed = parseRuleState(sub.filter_json, sub.excluded_keywords, sub.allowed_subgroups)
  if (parsed.resolution) {
    items.push({ kind: 'resolution', value: parsed.resolution })
  }
  if (parsed.includeKeywords.length > 0) {
    items.push({ kind: 'include', value: parsed.includeKeywords.join(', ') })
  }
  if (parsed.excludeKeywords.length > 0) {
    items.push({ kind: 'exclude', value: parsed.excludeKeywords.join(', ') })
  }
  if (parsed.allowedSubgroups.length > 0) {
    items.push({ kind: 'subgroups', value: parsed.allowedSubgroups.join(', ') })
  }
  if (sub.custom_path?.trim()) {
    items.push({ kind: 'path', value: sub.custom_path.trim() })
  }
  return items
}

export interface MikanGroupOption {
  name: string
  rss_url: string
}

export interface MikanSelectionPayload {
  subgroup_name?: string
  subgroup_names?: string[]
  allowed_subgroups?: string[]
  rss_url?: string
}

export function stripMikanSubgroupParam(rawUrl?: string): string {
  const clean = (rawUrl || '').trim()
  if (!clean) return ''
  try {
    const u = new URL(clean)
    for (const key of Array.from(u.searchParams.keys())) {
      if (key.toLowerCase() === 'subgroupid') {
        u.searchParams.delete(key)
      }
    }
    return u.toString()
  } catch {
    return ''
  }
}

export function resolveMikanSelectionPayload(
  groups: MikanGroupOption[],
  selectedNames: string[],
): MikanSelectionPayload {
  const picked = dedupeTags(selectedNames)
  if (picked.length === 1) {
    const target = picked[0]!
    const matched = groups.find(g => g.name.trim().toLowerCase() === target.toLowerCase())
    return {
      subgroup_name: target,
      subgroup_names: [target],
      allowed_subgroups: [target],
      rss_url: matched?.rss_url?.trim() || '',
    }
  }
  if (picked.length > 1) {
    const firstLower = picked[0]!.toLowerCase()
    const matched = groups.find(g => g.name.trim().toLowerCase() === firstLower) || groups[0]
    const seriesRss = stripMikanSubgroupParam(matched?.rss_url)
    return {
      subgroup_name: '',
      subgroup_names: picked,
      allowed_subgroups: picked,
      rss_url: seriesRss || undefined,
    }
  }
  const seriesRss = stripMikanSubgroupParam(groups[0]?.rss_url)
  return {
    subgroup_name: undefined,
    subgroup_names: undefined,
    allowed_subgroups: undefined,
    rss_url: seriesRss || undefined,
  }
}

export type SubgroupSubmissionDecision =
  | { action: 'submit'; groups: string[]; usedDefaults: boolean }
  | { action: 'prompt_missing_default'; defaultSubgroups: string[] }
  | { action: 'prompt_no_selection' }

export function evaluateSubgroupSubmission(params: {
  selectedNames: string[]
  defaultSubgroups: string[]
  availableGroups?: MikanGroupOption[] | null
  hasLiveGroupList: boolean
  allowUnrestricted?: boolean
}): SubgroupSubmissionDecision {
  const picked = dedupeTags(params.selectedNames)
  if (picked.length > 0) {
    return { action: 'submit', groups: picked, usedDefaults: false }
  }
  if (params.allowUnrestricted) {
    return { action: 'submit', groups: [], usedDefaults: false }
  }

  const defaults = dedupeTags(params.defaultSubgroups)
  if (defaults.length === 0) {
    return { action: 'prompt_no_selection' }
  }

  if (params.hasLiveGroupList) {
    const available = Array.isArray(params.availableGroups) ? params.availableGroups : []
    const defaultSet = new Set(defaults.map(d => d.toLowerCase()))
    const matched = dedupeTags(
      available
        .filter(g => typeof g?.name === 'string' && defaultSet.has(g.name.trim().toLowerCase()))
        .map(g => g.name),
    )
    if (matched.length === 0) {
      return { action: 'prompt_missing_default', defaultSubgroups: defaults }
    }
    return { action: 'submit', groups: matched, usedDefaults: true }
  }

  return { action: 'submit', groups: defaults, usedDefaults: true }
}
