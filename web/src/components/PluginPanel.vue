<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { isAxiosError } from 'axios'
import request from '../utils/request'

interface PanelAction {
  id: string
  label: string
  input?: { key: string; label: string; placeholder?: string; format?: string }
  result: 'message' | 'tasks'
}
interface PanelPlugin {
  id: string
  is_builtin?: boolean
  origin?: string
  url?: string
  panel: { credential?: { label: string; required?: boolean }; actions: PanelAction[] }
}
interface Credential {
  configured: boolean
  source: 'environment' | 'stored' | 'none'
}
interface TaskPage {
  page: number
  page_count: number
  tasks: { info_hash: string; name: string; status: string; progress: number }[]
}

const props = defineProps<{ plugin: PanelPlugin }>()
const { t, te } = useI18n()
const endpoint = computed(() => '/plugins/' + encodeURIComponent(props.plugin.id) + '/panel')
const official = computed(() => props.plugin.is_builtin === true && props.plugin.origin === 'official')
const config = ref<Credential | null>(null)
const available = computed(() => !props.plugin.panel.credential?.required || config.value?.configured)
const tokenInput = ref('')
const inputs = ref<Record<string, string>>(Object.create(null))
const pages = ref<Record<string, TaskPage>>(Object.create(null))
const actions = computed(() => props.plugin.panel.actions.map(action => ({ action, result: pages.value[action.id] })))
const busy = ref(false)
const notice = ref('')
const noticeFromAction = ref(false)
const error = ref('')

function label(value: string) {
  return official.value && te(value) ? t(value) : value
}

function copy(key: string) {
  return t('settings.pluginPanel.' + key)
}

function resetFeedback() {
  notice.value = ''
  error.value = ''
  noticeFromAction.value = false
}

async function readConfig() {
  const { data } = await request.get<Credential>(endpoint.value + '/credential')
  config.value = { configured: data.configured, source: data.source }
}

async function loadConfig() {
  if (!props.plugin.panel.credential || busy.value) return
  busy.value = true
  resetFeedback()
  try {
    await readConfig()
  } catch {
    error.value = 'configReadFailed'
  } finally {
    busy.value = false
  }
}

async function updateToken(action: 'save' | 'clear') {
  if (busy.value || !config.value || config.value.source === 'environment') return
  const token = tokenInput.value.trim()
  if ((action === 'save' && !token) || (action === 'clear' && config.value.source !== 'stored')) return
  busy.value = true
  resetFeedback()
  try {
    const url = endpoint.value + '/credential'
    const { data } = action === 'save'
      ? await request.put<Credential>(url, { access_token: token })
      : await request.delete<Credential>(url)
    tokenInput.value = ''
    inputs.value = Object.create(null)
    pages.value = Object.create(null)
    config.value = { configured: data.configured, source: data.source }
    notice.value = action === 'save' ? 'tokenSaved' : 'tokenCleared'
  } catch {
    error.value = 'configWriteFailed'
  } finally {
    busy.value = false
  }
}

async function execute(action: PanelAction, page = 1) {
  if (busy.value || !available.value) return
  resetFeedback()
  const payload: { input?: string; page?: number } = {}
  if (action.input) {
    payload.input = (inputs.value[action.id] ?? '').trim()
    if (!payload.input) return
    if (action.input.format === 'magnet' && !/^magnet:\?/i.test(payload.input)) {
      error.value = 'invalidMagnet'
      return
    }
  }
  if (action.result === 'tasks') payload.page = page
  busy.value = true
  try {
    const { data } = await request.post<Partial<TaskPage> & { message?: string }>(
      endpoint.value + '/actions/' + encodeURIComponent(action.id), payload,
    )
    if (action.result === 'tasks') {
      if (!Array.isArray(data.tasks) || data.page === undefined || data.page_count === undefined) {
        error.value = 'loadFailed'
        return
      }
      pages.value[action.id] = { tasks: data.tasks, page: data.page, page_count: data.page_count }
    } else {
      if (typeof data?.message !== 'string' || !data.message.trim()) {
        error.value = 'submitFailed'
        return
      }
      if (action.input) inputs.value[action.id] = ''
      noticeFromAction.value = true
      notice.value = data.message
    }
  } catch (cause) {
    const status = isAxiosError(cause) ? cause.response?.status : undefined
    if (status === 400 && action.input?.format === 'magnet') error.value = 'invalidMagnet'
    else if (status === 422) error.value = 'authorizationFailed'
    else if (status === 429) error.value = 'rateLimited'
    else error.value = action.result === 'tasks' ? 'loadFailed' : 'submitFailed'
  } finally {
    busy.value = false
  }
}

onMounted(loadConfig)
</script>

<template>
  <section class="space-y-3 border-t border-base-content/10 pt-3 text-xs" :aria-busy="busy">
    <template v-if="plugin.panel.credential">
      <p v-if="plugin.url" class="break-all text-base-content/80">
        {{ t('settings.pluginPanel.credentialTarget', { url: plugin.url }) }}
      </p>
      <p v-if="config" class="text-base-content/75">{{ copy('source.' + config.source) }}</p>
      <button v-else type="button" class="btn btn-outline btn-xs" :disabled="busy" @click="loadConfig">
        {{ busy ? t('common.loading') : copy('retryConfig') }}
      </button>
      <form v-if="config && config.source !== 'environment'" class="space-y-2" @submit.prevent="updateToken('save')">
        <label class="block">
          <span class="mb-1 block font-semibold">
            {{ label(plugin.panel.credential.label) }}
            <span v-if="!plugin.panel.credential.required" class="font-normal">{{ t('settings.pluginPanel.optional') }}</span>
          </span>
          <input v-model="tokenInput" type="password" autocomplete="new-password" :spellcheck="false" :disabled="busy"
            class="input input-bordered input-sm w-full" />
        </label>
        <div class="flex flex-wrap gap-2">
          <button type="submit" class="btn btn-primary btn-xs" :disabled="busy || !tokenInput.trim()">{{ copy('saveToken') }}</button>
          <button v-if="config.source === 'stored'" type="button" class="btn btn-outline btn-xs" :disabled="busy" @click="updateToken('clear')">
            {{ copy('clearToken') }}
          </button>
        </div>
      </form>
    </template>

    <template v-if="available">
      <div v-for="{ action, result } in actions" :key="action.id" class="space-y-2 border-t border-base-content/10 pt-2">
        <form class="space-y-2" @submit.prevent="execute(action, action.input ? 1 : result?.page ?? 1)">
          <label v-if="action.input" class="block">
            <span class="mb-1 block font-semibold">{{ label(action.input.label) }}</span>
            <input v-model="inputs[action.id]" type="text" :placeholder="action.input.placeholder && label(action.input.placeholder)"
              @input="delete pages[action.id]"
              :name="action.input.key" :spellcheck="false" autocomplete="off" :disabled="busy" class="input input-bordered input-sm w-full" />
          </label>
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h4 v-if="action.result === 'tasks'" class="font-semibold">{{ label(action.label) }}</h4>
            <button type="submit" class="btn btn-xs h-auto min-h-6 max-w-full whitespace-normal [overflow-wrap:anywhere]" :class="action.result === 'tasks' ? 'btn-outline' : 'btn-primary'"
              :disabled="busy || (!!action.input && !inputs[action.id]?.trim())">
              {{ action.result === 'tasks' ? t('common.refresh') : label(action.label) }}
            </button>
          </div>
        </form>
        <template v-if="result">
          <ul v-if="result.tasks.length" class="divide-y divide-base-content/10">
            <li v-for="task in result.tasks" :key="task.info_hash" class="flex flex-wrap justify-between gap-x-2 py-1">
              <span class="min-w-0 break-all">{{ task.name || task.info_hash }}</span>
              <span class="whitespace-nowrap">{{ copy('status.' + task.status) }} · {{ task.progress }}%</span>
            </li>
          </ul>
          <p v-else>{{ copy('empty') }}</p>
          <div v-if="result.page_count > 1" class="flex flex-wrap items-center justify-between gap-2">
            <button class="btn btn-ghost btn-xs" :disabled="busy || result.page <= 1" @click="execute(action, result.page - 1)">{{ copy('previous') }}</button>
            <span>{{ t('settings.pluginPanel.page', { page: result.page, count: result.page_count }) }}</span>
            <button class="btn btn-ghost btn-xs" :disabled="busy || result.page >= result.page_count" @click="execute(action, result.page + 1)">{{ copy('next') }}</button>
          </div>
        </template>
      </div>
    </template>
    <p v-if="notice" role="status" class="text-success">{{ noticeFromAction ? label(notice) : copy(notice) }}</p>
    <p v-if="error" role="alert" class="text-error">{{ copy(error) }}</p>
  </section>
</template>
