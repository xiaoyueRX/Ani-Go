<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  ChevronLeft, Plus, AlertTriangle, 
  X, LayoutGrid 
} from 'lucide-vue-next'

const router = useRouter()
const { t } = useI18n()
const titleCN = ref('')
const bangumiId = ref('')
const subgroupName = ref('')
const filterJson = ref('')
const customPath = ref('')
const loading = ref(false)
const error = ref('')

async function handleSubmit() {
  if (!titleCN.value.trim()) {
    error.value = t('form.error.titleEmpty')
    return
  }
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.post('/subscriptions', {
      title_cn: titleCN.value,
      bangumi_id: bangumiId.value,
      subgroup_name: subgroupName.value,
      filter_json: filterJson.value,
      custom_path: customPath.value,
    })
    router.push(`/subscriptions/${data.id}`)
  } catch (e: any) {
    error.value = e.response?.data?.error || t('form.error.createFailed')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="space-y-10 pb-20">
    <div class="flex items-center">
       <button class="btn btn-ghost border-base-300 rounded-2xl px-6 hover:bg-base-200 transition-all active:scale-95 group" @click="router.push('/')">
        <ChevronLeft :size="20" class="group-hover:-translate-x-1 transition-transform" />
        <span class="text-xs font-black uppercase tracking-widest">{{ $t('common.back') }}</span>
      </button>
    </div>

    <div class="max-w-2xl mx-auto">
      <div class="bg-base-100 rounded-[3rem] border border-base-200/60 shadow-2xl overflow-hidden relative">
        <!-- Decoration -->
        <div class="absolute -right-20 -top-20 w-64 h-64 bg-primary/5 rounded-full blur-3xl"></div>
        
        <div class="p-10 sm:p-14 relative z-10 space-y-10">
          <div class="flex items-center gap-6">
            <div class="w-16 h-16 rounded-2xl bg-primary flex items-center justify-center shadow-xl shadow-lg rotate-6 group-hover:rotate-0 transition-transform duration-500">
               <Plus :size="32" class="text-primary-content" />
            </div>
            <div class="space-y-1">
              <h1 class="text-3xl font-black tracking-tighter italic">{{ $t('form.title') }}</h1>
              <p class="text-[10px] font-black tracking-widest uppercase opacity-30">{{ $t('form.subtitle') }}</p>
            </div>
          </div>

          <div v-if="error" class="bg-error/10 border border-error/20 text-error rounded-[1.5rem] p-6 flex items-center gap-4">
            <AlertTriangle :size="24" class="shrink-0" />
            <div class="flex-1">
               <p class="text-xs font-black uppercase tracking-widest">{{ $t('form.error.title') }}</p>
               <p class="text-sm font-bold opacity-80">{{ error }}</p>
            </div>
            <button class="btn btn-ghost btn-circle btn-sm" @click="error = ''">
               <X :size="16" />
            </button>
          </div>

          <form @submit.prevent="handleSubmit" class="space-y-8">
            <div class="space-y-2">
               <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ $t('form.label.title') }}</label>
               <div class="relative group">
                  <div class="absolute inset-y-0 left-6 flex items-center text-base-content/20 group-focus-within:text-primary transition-colors">
                     <LayoutGrid :size="20" />
                  </div>
                  <input v-model="titleCN" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 focus:ring-4 focus:ring-primary/5 rounded-2xl pl-16 pr-6 py-5 transition-all outline-none font-bold" :placeholder="$t('form.placeholder.title')" required />
               </div>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
              <div class="space-y-2">
                <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ $t('form.label.subgroup') }}</label>
                <input v-model="subgroupName" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" :placeholder="$t('form.placeholder.subgroup')" />
              </div>
              <div class="space-y-2">
                <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ $t('form.label.bangumiId') }}</label>
                <input v-model="bangumiId" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" :placeholder="$t('form.placeholder.bangumiId')" />
              </div>
            </div>

            <div class="space-y-2">
              <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ $t('form.label.path') }}</label>
              <input v-model="customPath" type="text" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-2xl px-6 py-4 transition-all outline-none font-bold" :placeholder="$t('form.placeholder.path')" />
            </div>

            <div class="space-y-2">
              <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ $t('form.label.filter') }}</label>
              <textarea v-model="filterJson" class="w-full bg-base-200/50 border border-base-300 focus:border-primary/30 focus:bg-base-100 rounded-[2rem] px-8 py-6 transition-all outline-none font-bold h-32" placeholder='{"keywords": ["1080p"], "exclude": ["RAW"]}'></textarea>
            </div>

            <div class="pt-6">
               <button 
                 type="submit" 
                 class="w-full btn btn-primary h-16 rounded-[1.8rem] shadow-2xl shadow-lg text-xs font-black uppercase tracking-[0.3em] gap-4 transition-all hover:scale-[1.02] active:scale-95 group" 
                 :disabled="loading"
               >
                 <span v-if="loading" class="loading loading-spinner"></span>
                 <template v-else>
                    {{ $t('form.submit') }}
                    <Plus :size="20" class="group-hover:rotate-90 transition-transform duration-500" />
                 </template>
               </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>
