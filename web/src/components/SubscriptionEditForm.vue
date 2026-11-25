<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Check, StepForward } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import SubscriptionRuleEditor from './v2/SubscriptionRuleEditor.vue'

const { t } = useI18n()

const props = defineProps<{
  sub: Record<string, any>
}>()

const emit = defineEmits<{ save: [updated: Record<string, any>]; cancel: [] }>()

const titleCN = ref(props.sub.title_cn)
const titleEN = ref(props.sub.title_en || '')
const titleJP = ref(props.sub.title_jp || '')
const year = ref(props.sub.year)
const season = ref(props.sub.season)
const bangumiId = ref(props.sub.bangumi_id || '')
const subgroupName = ref(props.sub.subgroup_name || '')
const animeType = ref(props.sub.anime_type || '')
const totalEpisodes = ref(props.sub.total_episodes)
const completed = ref(props.sub.completed)
const customPath = ref(props.sub.custom_path || '')
const rssUrl = ref(props.sub.rss_url || '')
const filterJson = ref(props.sub.filter_json || '')
const excludedKeywords = ref(props.sub.excluded_keywords || '[]')
const skipBulkUpdate = ref(props.sub.skip_bulk_update || false)
const allowedSubgroups = ref(props.sub.allowed_subgroups || '[]')
const stallTimeoutHours = ref(props.sub.stall_timeout_hours || 0)
const subgroupSuggestions = ref<string[]>([])

onMounted(async () => {
  try {
    const { data } = await request.get<string[]>('/subgroups')
    if (Array.isArray(data)) subgroupSuggestions.value = data
  } catch {
    // 测试环境或离线时使用内置常见字幕组列表
  }
})

function handleSubmit() {
  const u: Record<string, any> = {}
  if (titleCN.value !== props.sub.title_cn) u.title_cn = titleCN.value
  if (titleEN.value !== (props.sub.title_en || '')) u.title_en = titleEN.value || null
  if (titleJP.value !== (props.sub.title_jp || '')) u.title_jp = titleJP.value || null
  if (year.value !== props.sub.year) u.year = year.value || null
  if (season.value !== props.sub.season) u.season = season.value
  if (bangumiId.value !== (props.sub.bangumi_id || '')) u.bangumi_id = bangumiId.value || null
  if (subgroupName.value !== (props.sub.subgroup_name || '')) u.subgroup_name = subgroupName.value || null
  if (animeType.value !== (props.sub.anime_type || '')) u.anime_type = animeType.value || null
  if (totalEpisodes.value !== props.sub.total_episodes) u.total_episodes = totalEpisodes.value
  if (completed.value !== props.sub.completed) u.completed = completed.value
  if (customPath.value !== (props.sub.custom_path || '')) u.custom_path = customPath.value
  if (rssUrl.value.trim() !== (props.sub.rss_url || '')) u.rss_url = rssUrl.value.trim()
  if (filterJson.value !== (props.sub.filter_json || '')) u.filter_json = filterJson.value
  if (excludedKeywords.value !== (props.sub.excluded_keywords || '[]')) u.excluded_keywords = excludedKeywords.value || '[]'
  if (skipBulkUpdate.value !== (props.sub.skip_bulk_update || false)) u.skip_bulk_update = skipBulkUpdate.value
  if (allowedSubgroups.value !== (props.sub.allowed_subgroups || '[]')) u.allowed_subgroups = allowedSubgroups.value || '[]'
  const stallVal = stallTimeoutHours.value === '' || stallTimeoutHours.value === null ? 0 : Number(stallTimeoutHours.value)
  if (stallVal !== (props.sub.stall_timeout_hours || 0)) u.stall_timeout_hours = stallVal

  if (Object.keys(u).length === 0) {
    emit('cancel')
    return
  }
  emit('save', u)
}
</script>

<template>
  <form @submit.prevent="handleSubmit" class="space-y-4">
    <div class="space-y-1.5">
      <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.primary') }}</label>
      <input v-model="titleCN" type="text" class="v2-input" required />
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.aliasEn') }}</label>
        <input v-model="titleEN" type="text" class="v2-input" />
      </div>
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.aliasJp') }}</label>
        <input v-model="titleJP" type="text" class="v2-input" />
      </div>
    </div>

    <div class="grid grid-cols-3 gap-3.5">
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.year') }}</label>
        <input v-model.number="year" type="number" class="v2-input font-mono" />
      </div>
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.season') }}</label>
        <input v-model.number="season" type="number" class="v2-input font-mono" />
      </div>
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.classification') }}</label>
        <select v-model="animeType" class="v2-input cursor-pointer">
          <option value="">{{ t('edit.select') }}</option>
          <option value="TV">{{ t('edit.types.tv') }}</option>
          <option value="剧场版">{{ t('edit.types.movie') }}</option>
          <option value="OVA">{{ t('edit.types.ova') }}</option>
          <option value="特别篇">{{ t('edit.types.special') }}</option>
        </select>
      </div>
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.releaseGroup') }}</label>
        <input v-model="subgroupName" type="text" list="edit-subgroup-suggestions" class="v2-input" />
        <datalist id="edit-subgroup-suggestions">
          <option v-for="sg in subgroupSuggestions" :key="sg" :value="sg" />
        </datalist>
      </div>
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.bangumiRef') }}</label>
        <input v-model="bangumiId" type="text" class="v2-input font-mono" placeholder="ID: 123456" />
      </div>
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3.5">
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.totalEpisodes') }}</label>
        <input v-model.number="totalEpisodes" type="number" class="v2-input font-mono" />
      </div>
      <div class="space-y-1.5">
        <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.stallTimeoutHours') }}</label>
        <input v-model.number="stallTimeoutHours" type="number" class="v2-input font-mono" placeholder="0 = 默认 (24h)" />
      </div>
    </div>

    <div class="space-y-1.5">
      <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.rssUrl') }}</label>
      <input
        v-model="rssUrl"
        type="url"
        class="v2-input font-mono text-xs"
        :placeholder="t('edit.label.rssPlaceholder')"
      />
    </div>

    <div class="space-y-1.5">
      <label for="subscription-download-directory" class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.pathOverride') }}</label>
      <input id="subscription-download-directory" v-model="customPath" type="text" class="v2-input font-mono text-xs" :placeholder="t('edit.label.defaultDownloadDirectory')" />
    </div>

    <div class="space-y-1.5">
      <label class="block text-xs font-semibold text-[var(--v2-ink-muted)]">{{ t('edit.label.rulesSection') }}</label>
      <SubscriptionRuleEditor
        v-model:filter-json="filterJson"
        v-model:excluded-keywords="excludedKeywords"
        v-model:allowed-subgroups="allowedSubgroups"
        :subgroup-suggestions="subgroupSuggestions"
      />
    </div>

    <div class="v2-panel-muted p-3.5 flex items-center justify-between gap-4">
      <div class="flex items-center gap-3 min-w-0">
        <div class="w-9 h-9 rounded-full bg-emerald-500/15 flex items-center justify-center text-emerald-400 shrink-0">
          <Check :size="17" />
        </div>
        <div class="min-w-0">
          <p class="text-xs font-bold text-[var(--v2-ink)]">{{ t('edit.label.finalized') }}</p>
          <p class="text-[11px] text-[var(--v2-ink-muted)] truncate">{{ t('edit.label.finalizedDesc') }}</p>
        </div>
      </div>
      <input v-model="completed" type="checkbox" class="toggle" />
    </div>

    <div class="v2-panel-muted p-3.5 flex items-center justify-between gap-4">
      <div class="flex items-center gap-3 min-w-0">
        <div class="w-9 h-9 rounded-full bg-amber-500/15 flex items-center justify-center text-amber-400 shrink-0">
          <StepForward :size="17" />
        </div>
        <div class="min-w-0">
          <p class="text-xs font-bold text-[var(--v2-ink)]">{{ t('edit.label.skipBulkUpdate') }}</p>
          <p class="text-[11px] text-[var(--v2-ink-muted)] truncate">{{ t('edit.label.skipBulkUpdateDesc') }}</p>
        </div>
      </div>
      <input v-model="skipBulkUpdate" type="checkbox" class="toggle" />
    </div>

    <div class="flex justify-end gap-2.5 pt-2">
      <button type="button" class="v2-btn v2-btn-ghost px-5" @click="emit('cancel')">{{ t('edit.abandon') }}</button>
      <button type="submit" class="v2-btn v2-btn-primary px-6">{{ t('edit.commit') }}</button>
    </div>
  </form>
</template>
