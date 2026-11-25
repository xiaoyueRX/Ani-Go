<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Plus, X, ChevronDown, AlertCircle } from 'lucide-vue-next'
import {
  RESOLUTION_OPTIONS,
  PRESET_INCLUDE_TAGS,
  PRESET_EXCLUDE_TAGS,
  COMMON_SUBGROUPS,
  parseRuleState,
  serializeRuleState,
  dedupeTags,
} from '../../utils/subscriptionRules'

const props = defineProps<{
  filterJson?: string
  excludedKeywords?: string
  allowedSubgroups?: string
  subgroupSuggestions?: string[]
}>()

const emit = defineEmits<{
  'update:filterJson': [value: string]
  'update:excludedKeywords': [value: string]
  'update:allowedSubgroups': [value: string]
}>()

const ruleState = ref(parseRuleState(props.filterJson, props.excludedKeywords, props.allowedSubgroups))
const rawFilterJson = ref(props.filterJson || '')
const rawExcludedJson = ref(props.excludedKeywords || '[]')
const rawSubgroupsJson = ref(props.allowedSubgroups || '[]')
const jsonError = ref('')

const includeInput = ref('')
const excludeInput = ref('')
const subgroupInput = ref('')

let internalSync = false

function emitAllFromState() {
  const out = serializeRuleState(ruleState.value)
  internalSync = true
  rawFilterJson.value = out.filter_json
  rawExcludedJson.value = out.excluded_keywords
  rawSubgroupsJson.value = out.allowed_subgroups
  jsonError.value = ''
  emit('update:filterJson', out.filter_json)
  emit('update:excludedKeywords', out.excluded_keywords)
  emit('update:allowedSubgroups', out.allowed_subgroups)
  internalSync = false
}

watch(
  () => [props.filterJson, props.excludedKeywords, props.allowedSubgroups] as const,
  ([fj, ek, as]) => {
    if (internalSync) return
    ruleState.value = parseRuleState(fj, ek, as)
    rawFilterJson.value = fj || ''
    rawExcludedJson.value = ek || '[]'
    rawSubgroupsJson.value = as || '[]'
  },
)

const subgroupPool = computed(() =>
  dedupeTags([...(props.subgroupSuggestions || []), ...COMMON_SUBGROUPS, ...ruleState.value.allowedSubgroups])
)

function setResolution(val: string) {
  ruleState.value.resolution = val
  emitAllFromState()
}

function addTag(target: 'include' | 'exclude' | 'subgroup', raw: string) {
  const clean = raw.trim()
  if (!clean) return
  if (target === 'include') {
    ruleState.value.includeKeywords = dedupeTags([...ruleState.value.includeKeywords, clean])
    includeInput.value = ''
  } else if (target === 'exclude') {
    ruleState.value.excludeKeywords = dedupeTags([...ruleState.value.excludeKeywords, clean])
    excludeInput.value = ''
  } else {
    ruleState.value.allowedSubgroups = dedupeTags([...ruleState.value.allowedSubgroups, clean])
    subgroupInput.value = ''
  }
  emitAllFromState()
}

function removeTag(target: 'include' | 'exclude' | 'subgroup', val: string) {
  const norm = val.toLowerCase()
  if (target === 'include') {
    ruleState.value.includeKeywords = ruleState.value.includeKeywords.filter(k => k.toLowerCase() !== norm)
  } else if (target === 'exclude') {
    ruleState.value.excludeKeywords = ruleState.value.excludeKeywords.filter(k => k.toLowerCase() !== norm)
  } else {
    ruleState.value.allowedSubgroups = ruleState.value.allowedSubgroups.filter(k => k.toLowerCase() !== norm)
  }
  emitAllFromState()
}

function togglePreset(target: 'include' | 'exclude' | 'subgroup', tag: string) {
  const list = target === 'include'
    ? ruleState.value.includeKeywords
    : target === 'exclude'
      ? ruleState.value.excludeKeywords
      : ruleState.value.allowedSubgroups
  const exists = list.some(item => item.toLowerCase() === tag.toLowerCase())
  if (exists) removeTag(target, tag)
  else addTag(target, tag)
}

function isSelected(list: string[], tag: string) {
  return list.some(item => item.toLowerCase() === tag.toLowerCase())
}

function applyRawJsonEdit() {
  try {
    if (rawFilterJson.value.trim()) {
      const f = JSON.parse(rawFilterJson.value)
      if (!f || typeof f !== 'object' || Array.isArray(f)) throw new Error('过滤规则 JSON 必须为对象')
    }
    if (rawExcludedJson.value.trim()) {
      const e = JSON.parse(rawExcludedJson.value)
      if (!Array.isArray(e)) throw new Error('排除词 JSON 必须为字符串数组')
    }
    if (rawSubgroupsJson.value.trim()) {
      const s = JSON.parse(rawSubgroupsJson.value)
      if (!Array.isArray(s)) throw new Error('字幕组 JSON 必须为字符串数组')
    }
    jsonError.value = ''
    internalSync = true
    ruleState.value = parseRuleState(rawFilterJson.value, rawExcludedJson.value, rawSubgroupsJson.value)
    emit('update:filterJson', rawFilterJson.value)
    emit('update:excludedKeywords', rawExcludedJson.value || '[]')
    emit('update:allowedSubgroups', rawSubgroupsJson.value || '[]')
    internalSync = false
  } catch (e: any) {
    jsonError.value = e?.message || 'JSON 格式不正确'
  }
}
</script>

<template>
  <div class="v2-panel-muted p-4 space-y-4">
    <!-- 画质优选 -->
    <div class="space-y-2">
      <div class="flex items-center justify-between text-xs">
        <span class="font-semibold text-[var(--v2-ink)]">画质偏好 (Resolution)</span>
        <span class="text-[11px] text-[var(--v2-ink-subtle)]">留空则不限制清晰度</span>
      </div>
      <div class="flex flex-wrap gap-1.5">
        <button
          v-for="res in RESOLUTION_OPTIONS"
          :key="res || 'any'"
          type="button"
          @click="setResolution(res)"
          class="h-7 px-3 rounded-full text-xs font-semibold border transition-colors cursor-pointer"
          :class="ruleState.resolution === res
            ? 'bg-[var(--v2-accent)] text-[var(--v2-accent-ink)] border-[var(--v2-accent)]'
            : 'bg-[var(--v2-surface)] text-[var(--v2-ink-muted)] border-[var(--v2-border)] hover:text-[var(--v2-ink)]'"
        >
          {{ res === '' ? '不限画质' : (res === '2160p' ? '4K (2160p)' : res) }}
        </button>
      </div>
    </div>

    <!-- 包含关键词 -->
    <div class="space-y-2 pt-2 border-t border-[var(--v2-border)]">
      <div class="flex items-center justify-between text-xs">
        <span class="font-semibold text-[var(--v2-ink)]">必须包含关键词</span>
        <span class="text-[11px] text-[var(--v2-ink-subtle)]">至少命中其中一个（回车添加）</span>
      </div>
      <div v-if="ruleState.includeKeywords.length" class="flex flex-wrap gap-1.5">
        <span
          v-for="tag in ruleState.includeKeywords"
          :key="tag"
          class="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-emerald-500/15 border border-emerald-500/30 text-emerald-400 text-xs font-medium"
        >
          <span>{{ tag }}</span>
          <button type="button" class="hover:text-white cursor-pointer" @click="removeTag('include', tag)">
            <X :size="12" />
          </button>
        </span>
      </div>
      <div class="flex gap-2">
        <input
          v-model="includeInput"
          type="text"
          class="v2-input h-8 text-xs flex-1"
          placeholder="如：简日、CHS、Baha，按回车添加"
          @keydown.enter.prevent="addTag('include', includeInput)"
        />
        <button type="button" class="v2-btn v2-btn-sm shrink-0" :disabled="!includeInput.trim()" @click="addTag('include', includeInput)">
          <Plus :size="13" />
          <span>添加</span>
        </button>
      </div>
      <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
        <span class="text-[var(--v2-ink-subtle)]">常用:</span>
        <button
          v-for="preset in PRESET_INCLUDE_TAGS"
          :key="preset"
          type="button"
          @click="togglePreset('include', preset)"
          class="px-2 py-0.5 rounded border transition-colors cursor-pointer"
          :class="isSelected(ruleState.includeKeywords, preset)
            ? 'bg-emerald-500/15 border-emerald-500/40 text-emerald-400 font-semibold'
            : 'bg-[var(--v2-surface)] border-[var(--v2-border)] text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
        >
          + {{ preset }}
        </button>
      </div>
    </div>

    <!-- 排除关键词 -->
    <div class="space-y-2 pt-2 border-t border-[var(--v2-border)]">
      <div class="flex items-center justify-between text-xs">
        <span class="font-semibold text-[var(--v2-ink)]">排除关键词 (Blacklist)</span>
        <span class="text-[11px] text-[var(--v2-ink-subtle)]">命中任意词将被跳过</span>
      </div>
      <div v-if="ruleState.excludeKeywords.length" class="flex flex-wrap gap-1.5">
        <span
          v-for="tag in ruleState.excludeKeywords"
          :key="tag"
          class="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-red-500/15 border border-red-500/30 text-red-400 text-xs font-medium"
        >
          <span>{{ tag }}</span>
          <button type="button" class="hover:text-white cursor-pointer" @click="removeTag('exclude', tag)">
            <X :size="12" />
          </button>
        </span>
      </div>
      <div class="flex gap-2">
        <input
          v-model="excludeInput"
          type="text"
          class="v2-input h-8 text-xs flex-1"
          placeholder="如：720p、RAW、HEVC，按回车添加"
          @keydown.enter.prevent="addTag('exclude', excludeInput)"
        />
        <button type="button" class="v2-btn v2-btn-sm shrink-0" :disabled="!excludeInput.trim()" @click="addTag('exclude', excludeInput)">
          <Plus :size="13" />
          <span>添加</span>
        </button>
      </div>
      <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
        <span class="text-[var(--v2-ink-subtle)]">常用:</span>
        <button
          v-for="preset in PRESET_EXCLUDE_TAGS"
          :key="preset"
          type="button"
          @click="togglePreset('exclude', preset)"
          class="px-2 py-0.5 rounded border transition-colors cursor-pointer"
          :class="isSelected(ruleState.excludeKeywords, preset)
            ? 'bg-red-500/15 border-red-500/40 text-red-400 font-semibold'
            : 'bg-[var(--v2-surface)] border-[var(--v2-border)] text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
        >
          - {{ preset }}
        </button>
      </div>
    </div>

    <!-- 允许字幕组多选 -->
    <div class="space-y-2 pt-2 border-t border-[var(--v2-border)]">
      <div class="flex items-center justify-between text-xs">
        <span class="font-semibold text-[var(--v2-ink)]">允许订阅的字幕组白名单</span>
        <span class="text-[11px] text-[var(--v2-ink-subtle)]">空列表代表不限字幕组</span>
      </div>
      <div class="flex flex-wrap gap-1.5">
        <button
          v-for="grp in subgroupPool"
          :key="grp"
          type="button"
          @click="togglePreset('subgroup', grp)"
          class="h-7 px-2.5 rounded-lg text-xs font-medium border transition-colors cursor-pointer"
          :class="isSelected(ruleState.allowedSubgroups, grp)
            ? 'bg-[var(--v2-accent)]/15 border-[var(--v2-accent)] text-[var(--v2-accent)] font-semibold'
            : 'bg-[var(--v2-surface)] border-[var(--v2-border)] text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)]'"
        >
          {{ grp }}
        </button>
      </div>
      <div class="flex gap-2">
        <input
          v-model="subgroupInput"
          type="text"
          class="v2-input h-8 text-xs flex-1"
          placeholder="输入其他自定义字幕组名，回车添加"
          @keydown.enter.prevent="addTag('subgroup', subgroupInput)"
        />
        <button type="button" class="v2-btn v2-btn-sm shrink-0" :disabled="!subgroupInput.trim()" @click="addTag('subgroup', subgroupInput)">
          <Plus :size="13" />
          <span>加入白名单</span>
        </button>
      </div>
    </div>

    <!-- 折叠的高级 JSON 原文模式（无损兼容） -->
    <details class="pt-2 border-t border-[var(--v2-border)] text-xs">
      <summary class="cursor-pointer font-semibold text-[var(--v2-ink-muted)] hover:text-[var(--v2-ink)] flex items-center gap-1.5 select-none">
        <ChevronDown :size="14" />
        <span>高级模式：查看或直接编辑原始 JSON</span>
      </summary>
      <div class="mt-3 space-y-3">
        <div class="space-y-1">
          <label class="text-[11px] font-mono text-[var(--v2-ink-subtle)]">filter_json</label>
          <textarea v-model="rawFilterJson" @input="applyRawJsonEdit" class="textarea font-mono text-xs h-20" placeholder='{"resolution":"1080p","include_keywords":["简日"]}'></textarea>
        </div>
        <div class="space-y-1">
          <label class="text-[11px] font-mono text-[var(--v2-ink-subtle)]">excluded_keywords</label>
          <textarea v-model="rawExcludedJson" @input="applyRawJsonEdit" class="textarea font-mono text-xs h-16" placeholder='["720p","RAW"]'></textarea>
        </div>
        <div class="space-y-1">
          <label class="text-[11px] font-mono text-[var(--v2-ink-subtle)]">allowed_subgroups</label>
          <textarea v-model="rawSubgroupsJson" @input="applyRawJsonEdit" class="textarea font-mono text-xs h-16" placeholder='["LoliHouse","ANi"]'></textarea>
        </div>
        <p v-if="jsonError" class="text-xs text-red-400 flex items-center gap-1.5">
          <AlertCircle :size="13" />
          <span>{{ jsonError }}</span>
        </p>
      </div>
    </details>
  </div>
</template>
