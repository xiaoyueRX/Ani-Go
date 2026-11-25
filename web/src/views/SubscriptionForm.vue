<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { parseRuleState, serializeRuleState, dedupeTags } from '../utils/subscriptionRules'
import SubscriptionRuleEditor from '../components/v2/SubscriptionRuleEditor.vue'
import {
  ChevronLeft, Plus, AlertTriangle, X, LayoutGrid,
  Rss, Loader2, Sparkles, FolderOutput, CheckCircle2, FileEdit
} from 'lucide-vue-next'

interface RSSPreviewItem {
  title: string
  parsed_title?: string
  season?: number
  episode?: number
  subgroup?: string
  resolution?: string
  is_batch?: boolean
  is_special?: boolean
  pub_date?: string
  size?: string
}

interface RSSPreviewResponse {
  success: boolean
  feed?: { title?: string; url?: string }
  total?: number
  items?: RSSPreviewItem[]
}

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const mode = ref<'manual' | 'rss'>('manual')

// 核心表单状态
const titleCN = ref('')
const season = ref<number>(1)
const animeType = ref('TV')
const subgroupName = ref('')
const bangumiId = ref('')
const rssUrl = ref('')
const customPath = ref('')
const filterJson = ref('')
const excludedKeywords = ref('[]')
const allowedSubgroups = ref('[]')

// 字幕组候选项 & RSS 解析状态
const knownSubgroups = ref<string[]>([])
const rssSubgroups = ref<string[]>([])
const rssPreviewing = ref(false)
const rssPreviewError = ref('')
const rssPreview = ref<RSSPreviewResponse | null>(null)

// 整理路径实时预览
const organizePreview = ref<{ final_path?: string; template?: string } | null>(null)
const organizePreviewLoading = ref(false)
let organizeTimer: ReturnType<typeof setTimeout> | null = null
let organizeController: AbortController | null = null
let rssController: AbortController | null = null

const loading = ref(false)
const error = ref('')

const subgroupSuggestions = computed(() =>
  dedupeTags([...rssSubgroups.value, ...knownSubgroups.value, subgroupName.value])
)

const isValidHttpUrl = (raw: string) => {
  const s = raw.trim()
  if (!s) return true
  try {
    const u = new URL(s)
    return (u.protocol === 'http:' || u.protocol === 'https:') && !!u.host
  } catch {
    return false
  }
}

async function loadSubgroups() {
  try {
    const { data } = await request.get<string[]>('/subgroups')
    if (Array.isArray(data)) {
      knownSubgroups.value = dedupeTags(data)
    }
  } catch {
    // 非阻塞辅助数据，静默忽略
  }
}

function selectSuggestedSubgroup(group: string) {
  subgroupName.value = group
  const state = parseRuleState(filterJson.value, excludedKeywords.value, allowedSubgroups.value)
  state.allowedSubgroups = dedupeTags([...state.allowedSubgroups, group])
  const out = serializeRuleState(state)
  filterJson.value = out.filter_json
  excludedKeywords.value = out.excluded_keywords
  allowedSubgroups.value = out.allowed_subgroups
}

async function handlePreviewRSS() {
  const target = rssUrl.value.trim()
  if (!target) {
    rssPreviewError.value = t('form.rss.emptyUrl')
    return
  }
  if (!isValidHttpUrl(target)) {
    rssPreviewError.value = t('form.rss.invalidUrl')
    return
  }

  rssController?.abort()
  rssController = new AbortController()
  rssPreviewing.value = true
  rssPreviewError.value = ''
  rssPreview.value = null

  try {
    const { data } = await request.post<RSSPreviewResponse>(
      '/rss/preview',
      { url: target },
      { timeout: 18000, signal: rssController.signal },
    )
    rssPreview.value = data
    const items = Array.isArray(data.items) ? data.items : []
    const parsedGroups = dedupeTags(items.map(i => i.subgroup || '').filter(Boolean))
    rssSubgroups.value = parsedGroups

    // 根据解析出的剧集条目智能回填缺失表单字段
    const firstTitle = items.find(i => i.parsed_title?.trim())?.parsed_title?.trim() || data.feed?.title?.trim() || ''
    if (!titleCN.value.trim() && firstTitle) {
      titleCN.value = firstTitle
    }
    const validSeasons = items.map(i => i.season).filter((s): s is number => typeof s === 'number' && s >= 1)
    if (validSeasons.length > 0 && season.value === 1) {
      season.value = validSeasons[0]
    }
    if (!subgroupName.value.trim() && parsedGroups.length === 1) {
      subgroupName.value = parsedGroups[0]
    }
    const resolutions = dedupeTags(items.map(i => i.resolution || '').filter(Boolean))
    const currentState = parseRuleState(filterJson.value, excludedKeywords.value, allowedSubgroups.value)
    if (!currentState.resolution && resolutions.length === 1) {
      currentState.resolution = resolutions[0]
      const out = serializeRuleState(currentState)
      filterJson.value = out.filter_json
    }
  } catch (e: any) {
    if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') return
    rssPreviewError.value = e?.response?.data?.error || t('form.rss.previewFailed')
  } finally {
    rssPreviewing.value = false
  }
}

async function fetchOrganizePreview() {
  const title = titleCN.value.trim()
  if (!title) {
    organizePreview.value = null
    return
  }
  organizeController?.abort()
  organizeController = new AbortController()
  organizePreviewLoading.value = true
  const rule = parseRuleState(filterJson.value, excludedKeywords.value, allowedSubgroups.value)
  const group = subgroupName.value.trim() || rule.allowedSubgroups[0] || 'SubGroup'
  const resTag = rule.resolution || '1080p'
  const epSeason = Number.isFinite(season.value) && season.value >= 0 ? Math.floor(season.value) : 1
  const sampleFile = `[${group}] ${title} - 01 [${resTag}].mkv`

  try {
    const { data } = await request.post<{ code?: number; data?: { final_path?: string; template?: string } }>(
      '/organize/preview',
      {
        file_path: sampleFile,
        anime: {
          title_cn: title,
          season: epSeason,
          type: animeType.value || 'TV',
        },
        episode: {
          season: epSeason,
          number: 1,
          group_name: group,
        },
      },
      { signal: organizeController.signal },
    )
    if (data?.data?.final_path) {
      organizePreview.value = {
        final_path: data.data.final_path,
        template: data.data.template,
      }
    } else {
      organizePreview.value = null
    }
  } catch {
    // 若未配置媒体库或接口返回 503，静默隐藏预览卡片而不打断创建流程
    organizePreview.value = null
  } finally {
    organizePreviewLoading.value = false
  }
}

function scheduleOrganizePreview() {
  if (organizeTimer) clearTimeout(organizeTimer)
  if (!titleCN.value.trim()) {
    organizePreview.value = null
    return
  }
  organizeTimer = setTimeout(() => {
    void fetchOrganizePreview()
  }, 450)
}

watch([titleCN, season, animeType, subgroupName, filterJson], scheduleOrganizePreview)

function applyRoutePrefill() {
  const q = route.query || {}
  const str = (v: unknown) => (typeof v === 'string' ? v.trim() : '')
  if (str(q.title)) titleCN.value = str(q.title)
  if (str(q.subgroup)) subgroupName.value = str(q.subgroup)
  if (str(q.bangumi_id)) bangumiId.value = str(q.bangumi_id)
  if (str(q.type)) animeType.value = str(q.type)
  if (str(q.season)) {
    const parsedSeason = Number.parseInt(str(q.season), 10)
    if (Number.isFinite(parsedSeason) && parsedSeason >= 0) {
      season.value = parsedSeason
    }
  }
  if (str(q.rss)) {
    rssUrl.value = str(q.rss)
    if (!titleCN.value) {
      mode.value = 'rss'
      if (isValidHttpUrl(rssUrl.value)) {
        void handlePreviewRSS()
      }
    }
  }
}

async function handleSubmit() {
  if (!titleCN.value.trim()) {
    error.value = t('form.error.titleEmpty')
    return
  }
  if (rssUrl.value.trim() && !isValidHttpUrl(rssUrl.value)) {
    error.value = t('form.rss.invalidUrl')
    return
  }

  loading.value = true
  error.value = ''
  try {
    const rule = parseRuleState(filterJson.value, excludedKeywords.value, allowedSubgroups.value)
    const { data } = await request.post('/subscriptions', {
      title_cn: titleCN.value.trim(),
      season: Number.isFinite(season.value) && season.value >= 0 ? Math.floor(season.value) : 1,
      anime_type: animeType.value,
      bangumi_id: bangumiId.value.trim(),
      subgroup_name: subgroupName.value.trim(),
      rss_url: rssUrl.value.trim(),
      filter_json: filterJson.value.trim(),
      excluded_keywords: excludedKeywords.value,
      allowed_subgroups: rule.allowedSubgroups,
      custom_path: customPath.value.trim(),
    })
    window.showToast?.(t('form.createSuccess'), 'success')
    router.push(data?.id ? `/subscriptions/${data.id}` : '/subscriptions')
  } catch (e: any) {
    error.value = e?.response?.data?.error || t('form.error.createFailed')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  applyRoutePrefill()
  void loadSubgroups()
  if (titleCN.value.trim()) {
    scheduleOrganizePreview()
  }
})

onUnmounted(() => {
  if (organizeTimer) clearTimeout(organizeTimer)
  organizeController?.abort()
  rssController?.abort()
})
</script>

<template>
  <div class="max-w-3xl mx-auto space-y-5 pb-16">
    <!-- 顶部标题与双模式切换 -->
    <section class="v2-panel p-4 sm:p-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div class="min-w-0">
        <h1 class="text-lg sm:text-xl font-bold tracking-tight text-[var(--v2-ink)]">{{ $t('form.title') }}</h1>
        <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('form.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2 shrink-0 flex-wrap">
        <div class="inline-flex rounded-xl p-1 bg-[var(--v2-surface-2)] border border-[var(--v2-border)]">
          <button
            type="button"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-1.5 transition-colors"
            :class="mode === 'manual' ? 'bg-[var(--v2-surface)] text-[var(--v2-ink)] shadow-sm' : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
            @click="mode = 'manual'"
          >
            <FileEdit :size="13" />
            <span>{{ $t('form.mode.manual') }}</span>
          </button>
          <button
            type="button"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-1.5 transition-colors"
            :class="mode === 'rss' ? 'bg-[var(--v2-surface)] text-[var(--v2-ink)] shadow-sm' : 'text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
            @click="mode = 'rss'"
          >
            <Rss :size="13" />
            <span>{{ $t('form.mode.rss') }}</span>
          </button>
        </div>
        <button type="button" class="v2-btn v2-btn-sm shrink-0" @click="router.push('/subscriptions')">
          <ChevronLeft :size="15" />
          <span>{{ $t('common.back') }}</span>
        </button>
      </div>
    </section>

    <!-- RSS 粘贴解析工作区（RSS 模式默认展开；手动模式保留在表单内） -->
    <section v-if="mode === 'rss'" class="v2-panel p-5 space-y-4">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="text-sm font-bold text-[var(--v2-ink)] flex items-center gap-1.5">
            <Rss :size="15" class="text-[var(--v2-accent)]" />
            <span>{{ $t('form.rss.panelTitle') }}</span>
          </h2>
          <p class="text-xs text-[var(--v2-ink-muted)] mt-0.5">{{ $t('form.rss.panelDesc') }}</p>
        </div>
      </div>

      <div class="flex flex-col sm:flex-row gap-2.5">
        <input
          v-model="rssUrl"
          type="url"
          class="v2-input flex-1 font-mono text-xs"
          :placeholder="$t('form.rss.placeholder')"
          @keydown.enter.prevent="handlePreviewRSS"
        />
        <button
          type="button"
          class="v2-btn v2-btn-primary shrink-0"
          :disabled="rssPreviewing"
          @click="handlePreviewRSS"
        >
          <Loader2 v-if="rssPreviewing" :size="14" class="animate-spin" />
          <Sparkles v-else :size="14" />
          <span>{{ rssPreviewing ? $t('form.rss.parsing') : $t('form.rss.parseBtn') }}</span>
        </button>
      </div>

      <p v-if="rssPreviewError" role="alert" class="text-xs text-red-400 bg-red-500/10 border border-red-500/25 rounded-lg px-3 py-2">
        {{ rssPreviewError }}
      </p>

      <div v-if="rssPreview" class="rounded-xl bg-[var(--v2-surface-2)] border border-[var(--v2-border)] p-3.5 space-y-2.5">
        <div class="flex flex-wrap items-center justify-between gap-2 text-xs">
          <span class="font-semibold text-[var(--v2-ink)] flex items-center gap-1.5">
            <CheckCircle2 :size="14" class="text-emerald-400" />
            <span>{{ rssPreview.feed?.title || $t('form.rss.feedReady') }}</span>
          </span>
          <span class="v2-pill font-mono text-[11px]">{{ $t('form.rss.matchedCount', { count: rssPreview.total ?? rssPreview.items?.length ?? 0 }) }}</span>
        </div>

        <div v-if="rssSubgroups.length" class="flex flex-wrap items-center gap-1.5 text-[11px]">
          <span class="text-[var(--v2-ink-muted)]">{{ $t('form.rss.detectedSubgroups') }}:</span>
          <button
            v-for="grp in rssSubgroups"
            :key="grp"
            type="button"
            class="v2-pill text-[11px] hover:border-[var(--v2-accent)] cursor-pointer transition-colors"
            :class="subgroupName === grp ? 'border-[var(--v2-accent)] text-[var(--v2-accent)] bg-[var(--v2-accent)]/10' : ''"
            @click="selectSuggestedSubgroup(grp)"
          >
            {{ grp }}
          </button>
        </div>

        <div v-if="rssPreview.items?.length" class="space-y-1.5 max-h-44 overflow-y-auto pr-1">
          <div
            v-for="(item, idx) in rssPreview.items.slice(0, 6)"
            :key="idx"
            class="text-[11px] px-2.5 py-1.5 rounded-lg bg-[var(--v2-surface)] border border-[var(--v2-border)] flex flex-wrap items-center justify-between gap-2"
          >
            <span class="truncate font-medium text-[var(--v2-ink)] flex-1 min-w-44">{{ item.title }}</span>
            <div class="flex items-center gap-1 shrink-0 font-mono text-[10px]">
              <span v-if="item.subgroup" class="v2-pill">{{ item.subgroup }}</span>
              <span v-if="item.resolution" class="v2-pill">{{ item.resolution }}</span>
              <span v-if="item.episode" class="v2-pill">E{{ item.episode }}</span>
              <span v-if="item.size" class="text-[var(--v2-ink-subtle)]">{{ item.size }}</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 录入主表单 -->
    <section class="v2-panel p-5 sm:p-6">
      <div
        v-if="error"
        role="alert"
        class="mb-5 rounded-xl bg-red-500/10 border border-red-500/30 text-red-400 px-4 py-3 flex items-center gap-3 text-xs"
      >
        <AlertTriangle :size="17" class="shrink-0" />
        <div class="flex-1 min-w-0">
          <p class="font-bold">{{ $t('form.error.title') }}</p>
          <p class="opacity-85 mt-0.5">{{ error }}</p>
        </div>
        <button type="button" class="p-1 hover:bg-red-500/15 rounded" @click="error = ''">
          <X :size="14" />
        </button>
      </div>

      <form @submit.prevent="handleSubmit" class="space-y-5">
        <div class="grid grid-cols-1 sm:grid-cols-12 gap-4">
          <div class="sm:col-span-6 space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.title') }}</label>
            <div class="relative">
              <LayoutGrid :size="15" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--v2-ink-subtle)]" />
              <input
                v-model="titleCN"
                type="text"
                class="v2-input pl-9"
                :placeholder="$t('form.placeholder.title')"
                required
              />
            </div>
          </div>

          <div class="sm:col-span-3 space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.season') }}</label>
            <input
              v-model.number="season"
              type="number"
              min="0"
              max="99"
              class="v2-input font-mono"
            />
          </div>

          <div class="sm:col-span-3 space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.type') }}</label>
            <select v-model="animeType" class="select select-bordered w-full text-xs">
              <option value="TV">{{ $t('edit.types.tv') }}</option>
              <option value="剧场版">{{ $t('edit.types.movie') }}</option>
              <option value="OVA">{{ $t('edit.types.ova') }}</option>
              <option value="特别篇">{{ $t('edit.types.special') }}</option>
            </select>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.subgroup') }}</label>
            <input
              v-model="subgroupName"
              type="text"
              list="form-known-subgroups"
              class="v2-input"
              :placeholder="$t('form.placeholder.subgroup')"
            />
            <datalist id="form-known-subgroups">
              <option v-for="sg in subgroupSuggestions" :key="sg" :value="sg" />
            </datalist>
          </div>
          <div class="space-y-1.5">
            <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.bangumiId') }}</label>
            <input
              v-model="bangumiId"
              type="text"
              class="v2-input font-mono"
              :placeholder="$t('form.placeholder.bangumiId')"
            />
          </div>
        </div>

        <div v-if="mode === 'manual'" class="space-y-1.5">
          <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.rssUrl') }}</label>
          <div class="flex gap-2">
            <input
              v-model="rssUrl"
              type="url"
              class="v2-input flex-1 font-mono text-xs"
              :placeholder="$t('form.rss.placeholder')"
            />
            <button
              v-if="rssUrl.trim()"
              type="button"
              class="v2-btn v2-btn-sm shrink-0"
              :disabled="rssPreviewing"
              @click="handlePreviewRSS"
            >
              {{ rssPreviewing ? $t('form.rss.parsing') : $t('form.rss.parseBtn') }}
            </button>
          </div>
        </div>

        <div class="space-y-1.5">
          <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.path') }}</label>
          <input
            v-model="customPath"
            type="text"
            class="v2-input font-mono text-xs"
            :placeholder="$t('form.placeholder.path')"
          />
        </div>

        <!-- 可视化规则编辑器（替代裸 JSON 文本框） -->
        <div class="space-y-2 pt-1 border-t border-[var(--v2-border)]">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-[var(--v2-ink-muted)]">{{ $t('form.label.rules') }}</span>
            <span class="text-[11px] text-[var(--v2-ink-subtle)]">{{ $t('form.label.rulesHint') }}</span>
          </div>
          <SubscriptionRuleEditor
            :filter-json="filterJson"
            :excluded-keywords="excludedKeywords"
            :allowed-subgroups="allowedSubgroups"
            :subgroup-suggestions="subgroupSuggestions"
            @update:filter-json="filterJson = $event"
            @update:excluded-keywords="excludedKeywords = $event"
            @update:allowed-subgroups="allowedSubgroups = $event"
          />
        </div>

        <!-- 整理归档目标路径实时预览 -->
        <div
          v-if="organizePreview || organizePreviewLoading"
          class="rounded-xl bg-[var(--v2-surface-2)] border border-[var(--v2-border)] px-3.5 py-2.5 space-y-1"
        >
          <div class="flex items-center justify-between gap-2 text-[11px] font-semibold text-[var(--v2-ink-muted)]">
            <span class="flex items-center gap-1.5">
              <FolderOutput :size="13" class="text-[var(--v2-accent)]" />
              <span>{{ $t('form.organizePreview.title') }}</span>
            </span>
            <span v-if="organizePreviewLoading" class="text-[var(--v2-ink-subtle)]">{{ $t('form.organizePreview.loading') }}</span>
          </div>
          <p v-if="organizePreview?.final_path" class="text-xs font-mono text-[var(--v2-ink)] break-all">
            {{ organizePreview.final_path }}
          </p>
        </div>

        <div class="pt-2 flex justify-end gap-2">
          <button type="button" class="v2-btn v2-btn-ghost" @click="router.push('/subscriptions')">
            {{ $t('common.cancel') }}
          </button>
          <button type="submit" class="v2-btn v2-btn-primary px-6" :disabled="loading">
            <span v-if="loading" class="loading loading-spinner loading-xs"></span>
            <template v-else>
              <Plus :size="15" />
              <span>{{ $t('form.submit') }}</span>
            </template>
          </button>
        </div>
      </form>
    </section>
  </div>
</template>
