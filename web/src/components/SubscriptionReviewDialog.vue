<script lang="ts">
export interface ReviewRelease {
  title: string
  source: string
  bangumi_id?: string
  cover_url?: string
}
</script>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { isAxiosError } from 'axios'
import request from '../utils/request'
import { bindDialogSession } from '../utils/dialogSession'

interface ParsedRelease {
  title: string
  season: number
  episode: number
  subgroup: string
  resolution: string
  is_batch: boolean
  is_special: boolean
}
interface ReviewRow {
  id: number
  items: ReviewRelease[]
  title: string
  season: number
  subgroup: string
  resolution: string
  rss: string
  error: string
}

const props = defineProps<{ items: ReviewRelease[] }>()
const emit = defineEmits<{ close: []; saved: [items: ReviewRelease[]] }>()
const { t } = useI18n()
const dialog = ref<HTMLDialogElement | null>(null)
const rows = ref<ReviewRow[]>([])
const loading = ref(true)
const sending = ref(false)
const saved = ref(0)
const session = bindDialogSession(localStorage, () => dialog.value?.close())
const ready = computed(() => !loading.value && !sending.value && rows.value.length > 0 && rows.value.every(row =>
  row.title.trim() && Number.isSafeInteger(row.season) && row.season >= 1))

function key(row: ReviewRow) {
  const item = row.items[0]
  if (item?.bangumi_id) return JSON.stringify(['source', item.source, item.bangumi_id])
  return JSON.stringify(['title', row.title.trim().toLowerCase(), row.season])
}

async function prepare() {
  const parsedTitles = new Map<string, ParsedRelease | null>()
  const grouped = new Map<string, ReviewRow>()
  for (const [index, item] of props.items.entries()) {
    if (!session.active()) return
    let parsed = parsedTitles.get(item.title)
    if (!item.bangumi_id && parsed === undefined) {
      try {
        const { data } = await request.post<ParsedRelease>('/settings/custom-regex/test', { title: item.title }, session.options)
        parsed = data
      } catch {
        parsed = null
      }
      if (!session.active()) return
      parsedTitles.set(item.title, parsed)
    }
    const candidate = typeof parsed?.title === 'string' ? parsed.title.trim() : ''
    const recognized = parsed && (parsed.episode > 0 || parsed.is_batch || parsed.is_special) && candidate !== item.title.trim()
    const row: ReviewRow = {
      id: index, items: [item], title: item.bangumi_id ? item.title : recognized ? candidate : '',
      season: parsed && Number.isSafeInteger(parsed.season) && parsed.season > 0 ? parsed.season : 1,
      subgroup: typeof parsed?.subgroup === 'string' ? parsed.subgroup : '',
      resolution: typeof parsed?.resolution === 'string' ? parsed.resolution : '', rss: '', error: '',
    }
    const groupKey = row.title ? key(row) : 'release-' + index
    const prior = grouped.get(groupKey)
    if (prior) {
      prior.items.push(item)
      if (prior.subgroup !== row.subgroup) prior.subgroup = ''
      if (prior.resolution !== row.resolution) prior.resolution = ''
    } else grouped.set(groupKey, row)
  }
  rows.value = [...grouped.values()]
  loading.value = false
}

async function submit() {
  if (!session.active() || !ready.value) return
  const seen = new Set<string>()
  let repeated = false
  for (const row of rows.value) {
    row.error = ''
    if (seen.has(key(row))) {
      row.error = t('search.review.duplicate')
      repeated = true
    }
    seen.add(key(row))
  }
  if (repeated) return

  sending.value = true
  const pending: ReviewRow[] = []
  for (const row of rows.value) {
    if (!session.active()) return
    const item = row.items[0]!
    try {
      await request.post('/subscriptions', {
        title_cn: row.title.trim(), season: row.season,
        bangumi_id: item.source === 'Mikan' ? undefined : item.bangumi_id || undefined,
        mikan_id: item.source === 'Mikan' ? item.bangumi_id || undefined : undefined,
        cover_url: item.cover_url || undefined, rss_url: row.rss.trim() || undefined,
        subgroup_name: row.subgroup.trim() || undefined,
        filter_json: JSON.stringify({ resolution: row.resolution.trim() || undefined }),
      }, session.options)
      if (!session.active()) return
      saved.value++
      emit('saved', row.items)
    } catch (cause) {
      if (!session.active()) return
      row.error = isAxiosError(cause) && typeof cause.response?.data?.error === 'string'
        ? cause.response.data.error : t('search.review.failed')
      pending.push(row)
    }
  }
  rows.value = pending
  sending.value = false
  if (!pending.length) {
    window.showToast?.(t('search.action.batchSubscribeSuccess', { count: saved.value }), 'success')
    dialog.value?.close()
  }
}

onMounted(() => {
  dialog.value?.showModal()
  void prepare()
})
onUnmounted(session.abort)
</script>

<template>
  <dialog ref="dialog" class="modal" aria-labelledby="review-subscriptions-title" @close="emit('close')" @cancel="sending && $event.preventDefault()">
    <form class="modal-box flex max-h-[88dvh] flex-col gap-4 p-4 sm:p-6" @submit.prevent="submit">
      <h2 id="review-subscriptions-title" class="text-lg font-bold">{{ t('search.review.heading') }}</h2>
      <p v-if="loading" role="status" class="text-sm">{{ t('search.review.preparing') }}</p>
      <p v-if="saved" role="status" class="text-sm">{{ t('search.action.batchSubscribeSuccess', { count: saved }) }}</p>
      <div class="min-h-0 space-y-4 overflow-y-auto">
        <fieldset v-for="row in rows" :key="row.id" class="space-y-3 rounded-lg border border-base-300 p-3" :disabled="sending">
          <details v-if="row.items.length > 1" class="text-sm">
            <summary class="cursor-pointer">{{ t('search.review.releases', { count: row.items.length }) }}</summary>
            <ul class="list-inside list-disc py-2"><li v-for="(item, i) in row.items" :key="i" class="break-words">{{ item.title }}</li></ul>
          </details>
          <p v-else class="break-words text-sm">{{ row.items[0]?.title }}</p>
          <div class="grid grid-cols-[minmax(0,1fr)_5rem] gap-2">
            <label class="space-y-1 text-sm"><span>{{ t('search.review.title') }}</span>
              <input v-model="row.title" required class="input input-bordered w-full" />
            </label>
            <label class="space-y-1 text-sm"><span>{{ t('search.review.season') }}</span>
              <input v-model.number="row.season" required type="number" min="1" step="1" class="input input-bordered w-full" />
            </label>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <label class="space-y-1 text-sm"><span>{{ t('search.review.group') }}</span>
              <input v-model="row.subgroup" class="input input-bordered w-full" :placeholder="t('search.review.any')" />
            </label>
            <label class="space-y-1 text-sm"><span>{{ t('search.review.resolution') }}</span>
              <input v-model="row.resolution" class="input input-bordered w-full" :placeholder="t('search.review.any')" />
            </label>
          </div>
          <label class="block space-y-1 text-sm"><span>{{ t('search.review.rss') }}</span>
            <input v-model="row.rss" type="url" pattern="https?://.+" class="input input-bordered w-full" placeholder="https://…" />
          </label>
          <p v-if="row.error" role="alert" class="text-sm text-error">{{ row.error }}</p>
          <button v-if="rows.length > 1" type="button" class="btn btn-ghost btn-xs" @click="rows = rows.filter(r => r.id !== row.id)">{{ t('search.review.skip') }}</button>
        </fieldset>
      </div>
      <div class="modal-action mt-0 flex-wrap">
        <button type="button" :disabled="sending" class="btn btn-ghost" @click="dialog?.close()">{{ t('common.cancel') }}</button>
        <button type="submit" :disabled="!ready" class="btn btn-primary">
          <span v-if="sending" class="loading loading-spinner loading-xs"></span>{{ t('search.review.confirm') }}
        </button>
      </div>
    </form>
  </dialog>
</template>
