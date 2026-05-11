<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '../utils/request'
import { 
  Search, Plus, AlertTriangle, 
  X, LayoutGrid, RefreshCw 
} from 'lucide-vue-next'
import { Trash2 } from 'lucide-vue-next'
import SubscriptionCard from '../components/SubscriptionCard.vue'

import { useI18n } from 'vue-i18n'
const { t } = useI18n()

interface Subscription {
  id: number
  title_cn: string
  title_en: string
  title_jp: string
  year: number; season: number
  bangumi_id: string; subgroup_name: string
  cover_url: string; anime_type: string
  total_episodes: number; current_episodes: number
  stalled_episodes: number
  enabled: boolean; completed: boolean
  created_at: string; updated_at: string
}

const router = useRouter()
const subs = ref<Subscription[]>([])
const loading = ref(true)
const error = ref('')
const deletingId = ref<number | null>(null)
const filterText = ref('')
const filterType = ref<'all' | 'active' | 'completed'>('all')

const batchDeleteMode = ref(false)
const batchDeleteSelected = ref<Set<number>>(new Set())
const undoBarVisible = ref(false)
const undoDeletedCount = ref(0)
const undoDeletedIds = ref<number[]>([])
const remainingSeconds = ref(15)
const UNDO_TIMEOUT_SECONDS = 15
let undoInterval: ReturnType<typeof setInterval> | null = null

// 删除确认弹窗
const deleteModalOpen = ref(false)
const deletingSub = ref<Subscription | null>(null)
const deleteFilesChecked = ref(true)

const batchDeleteModalOpen = ref(false)
const batchDeleteFilesChecked = ref(true)

// 延迟删除队列
interface PendingDelete {
  ids: number[]
  deleteFiles: boolean
  timer: ReturnType<typeof setTimeout>
}
const pendingDeletes = ref<PendingDelete[]>([])
const undoDeleteFiles = ref(false)
const undoCount = ref(0)

function enterBatchDeleteMode() {
  batchDeleteMode.value = true
  batchDeleteSelected.value = new Set()
}

function exitBatchDeleteMode() {
  batchDeleteMode.value = false
  batchDeleteSelected.value = new Set()
}

function toggleBatchSelect(id: number) {
  const newSet = new Set(batchDeleteSelected.value)
  if (newSet.has(id)) newSet.delete(id)
  else newSet.add(id)
  batchDeleteSelected.value = newSet
}

// 替换 confirmBatchDelete 为打开 modal
function openBatchDeleteModal() {
  if (batchDeleteSelected.value.size === 0) return
  batchDeleteFilesChecked.value = true
  batchDeleteModalOpen.value = true
}

// 确认批量删除
function confirmBatchDeleteWithFiles() {
  batchDeleteModalOpen.value = false
  const ids = Array.from(batchDeleteSelected.value)
  scheduleDelete(ids, batchDeleteFilesChecked.value)
  exitBatchDeleteMode()
}

// 调度延迟删除
function scheduleDelete(ids: number[], deleteFiles: boolean) {
  // 合并到现有倒计时
  const existing = pendingDeletes.value.length > 0 ? pendingDeletes.value[0] : null
  if (existing) {
    for (const id of ids) {
      if (!existing.ids.includes(id)) existing.ids.push(id)
    }
    existing.deleteFiles = existing.deleteFiles || deleteFiles
    clearTimeout(existing.timer)
    existing.timer = setTimeout(() => executePendingDeletes(), UNDO_TIMEOUT_SECONDS * 1000)
    remainingSeconds.value = UNDO_TIMEOUT_SECONDS
    undoCount.value = existing.ids.length
    undoDeleteFiles.value = existing.deleteFiles
    undoBarVisible.value = true
    return
  }

  const timer = setTimeout(() => executePendingDeletes(), UNDO_TIMEOUT_SECONDS * 1000)
  pendingDeletes.value = [{ ids, deleteFiles, timer }]
  undoCount.value = ids.length
  undoDeleteFiles.value = deleteFiles
  undoBarVisible.value = true
  remainingSeconds.value = UNDO_TIMEOUT_SECONDS
  startUndoCountdown()
}

// 执行真正删除
async function executePendingDeletes() {
  const batch = pendingDeletes.value.shift()
  if (!batch) return
  undoBarVisible.value = false
  clearUndoTimer()
  
  try {
    await request.post('/subscriptions/batch-delete', {
      ids: batch.ids,
      delete_files: batch.deleteFiles
    })
    subs.value = subs.value.filter(s => !batch.ids.includes(s.id))
  } catch (e: any) {
    error.value = '批量删除失败，请重试'
  }
}

// 撤回
function undoDelete() {
  const batch = pendingDeletes.value.shift()
  if (!batch) return
  clearTimeout(batch.timer)
  undoBarVisible.value = false
  clearUndoTimer()
}

// 判断是否待删除
function isPending(id: number): boolean {
  return pendingDeletes.value.some(b => b.ids.includes(id))
}

function startUndoCountdown() {
  remainingSeconds.value = UNDO_TIMEOUT_SECONDS
  undoInterval = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      clearUndoTimer()
      hideUndoBar()
    }
  }, 1000)
}

function clearUndoTimer() {
  if (undoInterval) {
    clearInterval(undoInterval)
    undoInterval = null
  }
}

function hideUndoBar() {
  undoBarVisible.value = false
  undoDeletedIds.value = []
  undoDeletedCount.value = 0
  clearUndoTimer()
}

const filteredSubs = computed(() => {
  let list = subs.value
  // 状态筛选
  if (filterType.value === 'active') list = list.filter(s => s.enabled && !s.completed)
  else if (filterType.value === 'completed') list = list.filter(s => s.completed)
  // 文字搜索
  const q = filterText.value.trim().toLowerCase()
  if (q) {
    list = list.filter(s =>
      s.title_cn.toLowerCase().includes(q) ||
      (s.title_en && s.title_en.toLowerCase().includes(q)) ||
      (s.subgroup_name && s.subgroup_name.toLowerCase().includes(q))
    )
  }
  return list
})

async function fetchSubscriptions() {
  if (loading.value === false) {
     // Background refresh - don't show loading spinner
  }
  error.value = ''
  try {
    const { data } = await request.get('/subscriptions')
    subs.value = data || []
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.load')
  } finally {
    loading.value = false
  }
}

async function toggleEnabled(sub: Subscription) {
  try {
    await request.put(`/subscriptions/${sub.id}`, { enabled: !sub.enabled })
    sub.enabled = !sub.enabled
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.operation')
  }
}

// 替换 handleDelete：打开 modal 替代 confirm
function handleDelete(sub: Subscription) {
  deletingSub.value = sub
  deleteFilesChecked.value = true
  deleteModalOpen.value = true
}

// 确认单条删除
function confirmSingleDelete() {
  const sub = deletingSub.value
  if (!sub) return
  deleteModalOpen.value = false
  scheduleDelete([sub.id], deleteFilesChecked.value)
}

async function triggerSupplement(sub: Subscription) {
  try {
    await request.post(`/subscriptions/${sub.id}/trigger-supplement`)
    alert(t('subscriptions.supplementTriggered'))
  } catch (e: any) {
    error.value = e.response?.data?.error || t('subscriptions.error.supplement')
  }
}

let refreshTimer: ReturnType<typeof setInterval>
onMounted(() => {
  fetchSubscriptions()
  refreshTimer = setInterval(fetchSubscriptions, 30000)
})
onUnmounted(() => {
  clearInterval(refreshTimer)
  clearUndoTimer()
  while (pendingDeletes.value.length > 0) {
    const batch = pendingDeletes.value.shift()!
    clearTimeout(batch.timer)
    request.post('/subscriptions/batch-delete', {
      ids: batch.ids,
      delete_files: batch.deleteFiles
    }).catch(() => {})
  }
})
</script>

<template>
  <div class="space-y-10">
    <!-- Header Section -->
    <div class="flex flex-col md:flex-row md:items-end justify-between gap-6">
      <div class="space-y-1">
        <h1 class="text-4xl font-black tracking-tighter italic">{{ $t('subs.title') }}</h1>
        <p class="text-xs font-bold tracking-[0.3em] uppercase opacity-30">{{ $t('subs.subtitle') }}</p>
      </div>
      
      <div class="flex items-center gap-3">
        <template v-if="batchDeleteMode">
          <div class="flex items-center gap-3">
            <span class="text-xs font-bold opacity-60">已选 {{ batchDeleteSelected.size }} 项</span>
            <button class="btn btn-ghost btn-sm rounded-xl" @click="exitBatchDeleteMode">
              取消
            </button>
            <button class="btn btn-error btn-sm rounded-xl gap-2" @click="openBatchDeleteModal">
              <Trash2 :size="16" />
              删除选中
            </button>
          </div>
        </template>
        <button v-if="!batchDeleteMode"
          class="btn btn-ghost border-base-300 rounded-2xl gap-3 px-6 hover:bg-base-200 transition-all active:scale-95"
          @click="enterBatchDeleteMode">
          <Trash2 :size="20" />
          <span class="text-xs font-black uppercase tracking-widest">{{ $t('subs.batchDelete') || '批量删除' }}</span>
        </button>
        <button 
          class="btn btn-ghost border-base-300 rounded-2xl gap-3 px-6 hover:bg-base-200 transition-all active:scale-95" 
          @click="router.push('/search')"
        >
          <Search :size="20" />
          <span class="text-xs font-black uppercase tracking-widest">{{ $t('subs.find') }}</span>
        </button>
        <button 
          class="btn btn-primary rounded-2xl gap-3 px-6 shadow-xl shadow-lg hover:scale-105 active:scale-95 transition-all" 
          @click="router.push('/subscriptions/new')"
        >
          <Plus :size="20" />
          <span class="text-xs font-black uppercase tracking-widest">{{ $t('subs.new') }}</span>
        </button>
      </div>
    </div>

    <!-- Toolbar Section -->
    <div class="flex flex-col lg:flex-row items-center gap-4 bg-base-100 p-3 rounded-[2rem] border border-base-200/50 shadow-sm">
      <div class="relative w-full lg:w-96 group">
        <div class="absolute inset-y-0 left-4 flex items-center pointer-events-none text-base-content/20 group-focus-within:text-primary transition-colors">
          <Search :size="20" />
        </div>
        <input 
          v-model="filterText" 
          type="text" 
          :placeholder="$t('subs.searchPlaceholder')" 
          class="input w-full bg-base-200/50 border-transparent focus:border-primary/30 focus:bg-base-100 focus:ring-0 rounded-2xl pl-12 transition-all font-bold text-sm h-12"
        />
      </div>

      <div class="flex p-1.5 bg-base-200/50 rounded-2xl gap-1 w-full lg:w-auto overflow-x-auto no-scrollbar">
        <button 
          v-for="t in ['all', 'active', 'completed']" 
          :key="t"
          class="flex-1 lg:flex-none px-6 py-2 rounded-xl text-[10px] font-black uppercase tracking-widest transition-all whitespace-nowrap"
          :class="filterType === t ? 'bg-base-100 text-primary shadow-sm ring-1 ring-base-300' : 'text-base-content/40 hover:text-base-content'"
          @click="filterType = t as any"
        >
          {{ t === 'all' ? $t('subs.filter.all') : t === 'active' ? $t('subs.filter.active') : $t('subs.filter.completed') }}
        </button>
      </div>

      <div class="hidden lg:flex ml-auto px-4 items-center gap-2">
         <div class="flex -space-x-3">
            <div v-for="i in 3" :key="i" class="w-8 h-8 rounded-full border-2 border-base-100 bg-base-200 flex items-center justify-center overflow-hidden">
               <img :src="`https://api.dicebear.com/7.x/bottts/svg?seed=${i+10}`" class="w-full h-full object-cover" />
            </div>
         </div>
         <span class="text-[10px] font-black text-base-content/20 uppercase tracking-widest ml-2">{{ $t('subs.itemsTracked', { count: subs.length }) }}</span>
      </div>
    </div>

    <!-- Status Alerts -->
    <div v-if="error" class="alert bg-error/10 border-error/20 text-error rounded-3xl p-6 flex items-start gap-4">
      <div class="p-3 bg-error/20 rounded-2xl">
        <AlertTriangle :size="24" />
      </div>
      <div class="flex-1">
        <h3 class="font-black text-sm uppercase tracking-widest">{{ $t('subs.error.op') }}</h3>
        <p class="text-sm font-bold opacity-80 mt-1">{{ error }}</p>
      </div>
      <button class="btn btn-ghost btn-circle btn-sm" @click="error = ''">
        <X :size="16" />
      </button>
    </div>

    <!-- Main Content Section -->
    <div v-if="loading" class="grid gap-6 grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 animate-pulse">
      <div v-for="i in 8" :key="i" class="aspect-[3/5] bg-base-200 rounded-[2.5rem]"></div>
    </div>

    <div v-else-if="filteredSubs.length === 0" class="flex flex-col items-center justify-center py-32 text-center bg-base-100/30 rounded-[3rem] border-2 border-dashed border-base-200">
      <div class="w-32 h-32 bg-base-200/50 rounded-full flex items-center justify-center mb-8 rotate-12">
        <LayoutGrid :size="64" class="opacity-10" />
      </div>
      <h3 class="text-2xl font-black tracking-tight mb-2">
        {{ subs.length > 0 ? $t('subs.empty.noResults') : $t('subs.empty.noSubs') }}
      </h3>
      <p class="text-sm font-bold text-base-content/40 max-w-xs mx-auto mb-10 leading-relaxed">
        {{ subs.length > 0 ? $t('subs.empty.noResultsDesc') : $t('subs.empty.noSubsDesc') }}
      </p>
      <button 
        v-if="subs.length === 0" 
        class="btn btn-primary btn-lg rounded-3xl px-12 shadow-2xl shadow-lg gap-4" 
        @click="router.push('/search')"
      >
        <Search :size="24" />
        <span class="font-black uppercase tracking-widest">{{ $t('subs.empty.discover') }}</span>
      </button>
      <button 
        v-else 
        class="btn btn-ghost btn-md rounded-2xl px-10 gap-4 border-base-300" 
        @click="filterText = ''; filterType = 'all'"
      >
        <RefreshCw :size="20" />
        <span class="font-black uppercase tracking-widest text-xs">{{ $t('subs.empty.clear') }}</span>
      </button>
    </div>

    <!-- Subscription Grid -->
    <div v-else>
      <TransitionGroup
        name="list"
        tag="div"
        class="grid gap-6 sm:gap-8 grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5"
      >
        <SubscriptionCard
          v-for="sub in filteredSubs"
          :key="sub.id"
          :sub="sub"
          :deleting="deletingId === sub.id"
          :pending="isPending(sub.id)"
          :batch-delete-mode="batchDeleteMode && !isPending(sub.id)"
          :batch-selected="batchDeleteSelected.has(sub.id)"
          @click="batchDeleteMode ? (!isPending(sub.id) && toggleBatchSelect(sub.id)) : router.push(`/subscriptions/${sub.id}`)"
          @toggle="toggleEnabled(sub)"
          @delete="isPending(sub.id) ? null : handleDelete(sub)"
          @supplement="triggerSupplement(sub)"
        />
      </TransitionGroup>
    </div>

    <!-- 撤回浮条 -->
    <Transition name="slide-up">
      <div v-if="undoBarVisible" class="fixed bottom-0 left-0 right-0 z-50 p-4 pointer-events-none">
        <div class="max-w-lg mx-auto bg-base-300 rounded-2xl shadow-2xl px-6 py-4 flex items-center gap-4 pointer-events-auto">
          <span class="text-sm font-bold flex-1">
            将在 {{ remainingSeconds }} 秒后删除 {{ undoCount }} 个订阅{{ undoDeleteFiles ? '（含文件）' : '' }}
          </span>
          <button class="btn btn-primary btn-sm rounded-xl" @click="undoDelete">撤回</button>
        </div>
      </div>
    </Transition>

    <dialog v-if="deleteModalOpen" class="modal modal-open" @click.self="deleteModalOpen = false">
      <div class="modal-box rounded-3xl">
        <h3 class="text-lg font-black tracking-tight mb-4">删除「{{ deletingSub?.title_cn }}」</h3>
        <p class="text-sm text-base-content/60 mb-6">将在 15 秒后执行删除，可撤回。</p>
        <label class="flex items-center gap-3 p-4 bg-base-200 rounded-2xl cursor-pointer mb-6">
          <input type="checkbox" v-model="deleteFilesChecked" class="checkbox checkbox-primary" />
          <span class="text-sm font-bold">同时删除已下载的文件和种子</span>
        </label>
        <div class="flex gap-3 justify-end">
          <button class="btn btn-ghost rounded-xl" @click="deleteModalOpen = false">取消</button>
          <button class="btn btn-error rounded-xl" @click="confirmSingleDelete">确认删除</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>

    <dialog v-if="batchDeleteModalOpen" class="modal modal-open" @click.self="batchDeleteModalOpen = false">
      <div class="modal-box rounded-3xl">
        <h3 class="text-lg font-black tracking-tight mb-4">确定删除 {{ batchDeleteSelected.size }} 个订阅？</h3>
        <p class="text-sm text-base-content/60 mb-6">将在 15 秒后执行删除，可撤回。</p>
        <label class="flex items-center gap-3 p-4 bg-base-200 rounded-2xl cursor-pointer mb-6">
          <input type="checkbox" v-model="batchDeleteFilesChecked" class="checkbox checkbox-primary" />
          <span class="text-sm font-bold">同时删除已下载的文件和种子</span>
        </label>
        <div class="flex gap-3 justify-end">
          <button class="btn btn-ghost rounded-xl" @click="batchDeleteModalOpen = false">取消</button>
          <button class="btn btn-error rounded-xl" @click="confirmBatchDeleteWithFiles">确认删除</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop"><button>close</button></form>
    </dialog>
  </div>
</template>

<style scoped>
.list-enter-active,
.list-leave-active {
  transition: all 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.list-enter-from,
.list-leave-to {
  opacity: 0;
  transform: translateY(40px) scale(0.9);
}
.list-move {
  transition: transform 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.no-scrollbar::-webkit-scrollbar {
  display: none;
}
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.slide-up-enter-from {
  transform: translateY(100%);
  opacity: 0;
}
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}
</style>
