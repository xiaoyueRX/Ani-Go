<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import request from '../utils/request'
import { 
  Antenna, AlertTriangle, User, Lock, LogIn, Languages
} from 'lucide-vue-next'

const router = useRouter()
const { t, locale } = useI18n()
const username = ref('')
const password = ref('')
const remember = ref(false)
const error = ref('')
const loading = ref(false)

function toggleLanguage() {
  locale.value = locale.value === 'zh' ? 'en' : 'zh'
  localStorage.setItem('lang', locale.value)
}

onMounted(() => {
  const saved = localStorage.getItem('remembered_user')
  if (saved) {
    try {
      const u = JSON.parse(saved)
      username.value = u.username || ''
      password.value = u.password || ''
      remember.value = true
    } catch { /* ignore */ }
  }
})

async function handleLogin() {
  if (!username.value || !password.value) {
    error.value = t('login.errorEmpty')
    return
  }
  loading.value = true
  error.value = ''
  try {
    const { data } = await request.post('/login', {
      username: username.value,
      password: password.value,
    })
    localStorage.setItem('token', data.token)
    if (remember.value) {
      localStorage.setItem('remembered_user', JSON.stringify({
        username: username.value, password: password.value,
      }))
    } else {
      localStorage.removeItem('remembered_user')
    }
    router.push('/')
  } catch (e: any) {
    error.value = e.response?.data?.message || t('login.errorFailed')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-[#0a0c10] overflow-hidden relative font-sans">
    <!-- Animated Background Blobs -->
    <div class="absolute top-0 left-0 w-full h-full overflow-hidden z-0">
      <div class="absolute -top-[10%] -left-[10%] w-[40%] h-[40%] bg-primary/20 rounded-full blur-[120px] animate-pulse"></div>
      <div class="absolute top-[60%] -right-[5%] w-[35%] h-[35%] bg-secondary/10 rounded-full blur-[100px] animate-pulse" style="animation-delay: 2s"></div>
      <div class="absolute -bottom-[5%] left-[20%] w-[30%] h-[30%] bg-accent/10 rounded-full blur-[80px] animate-pulse" style="animation-delay: 4s"></div>
    </div>

    <!-- Language Switcher -->
    <div class="absolute top-8 right-8 z-20">
      <button @click="toggleLanguage" class="btn btn-ghost btn-circle hover:bg-white/10">
        <Languages :size="20" class="text-white/60" />
      </button>
    </div>

    <!-- Login Card -->
    <div class="relative z-10 w-full max-w-md p-4">
      <div class="card bg-base-100/40 backdrop-blur-2xl border border-white/5 shadow-[0_32px_64px_-16px_rgba(0,0,0,0.5)] rounded-[3rem] overflow-hidden">
        <div class="card-body p-8 sm:p-12">
          <!-- Logo & Title -->
          <div class="flex flex-col items-center text-center mb-10">
            <div class="relative mb-6 group">
               <div class="absolute inset-0 bg-primary blur-2xl opacity-20 group-hover:opacity-40 transition-opacity"></div>
               <div class="w-20 h-20 rounded-3xl bg-primary flex items-center justify-center shadow-2xl rotate-3 group-hover:rotate-0 transition-all duration-500 relative z-10">
                 <Antenna :size="40" class="text-primary-content" />
               </div>
            </div>
            <h1 class="text-4xl font-black tracking-tighter italic bg-gradient-to-r from-white to-white/60 bg-clip-text text-transparent mb-2">{{ $t('login.title') }}</h1>
            <p class="text-[10px] font-black tracking-[0.4em] uppercase opacity-30">{{ $t('login.subtitle') }}</p>
          </div>

          <!-- Error Message -->
          <Transition name="fade">
            <div v-if="error" class="bg-error/10 border border-error/20 text-error rounded-2xl p-4 mb-6 flex items-center gap-3 text-sm font-bold">
              <AlertTriangle :size="18" class="shrink-0" />
              <span>{{ error }}</span>
            </div>
          </Transition>

          <!-- Form -->
          <form @submit.prevent="handleLogin" class="space-y-5">
            <div class="space-y-1.5">
               <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ $t('login.username') }}</label>
               <div class="relative group">
                  <div class="absolute inset-y-0 left-5 flex items-center text-base-content/20 group-focus-within:text-primary transition-colors">
                     <User :size="20" />
                  </div>
                  <input 
                    v-model="username" 
                    type="text" 
                    :placeholder="$t('login.usernamePlaceholder')" 
                    class="w-full bg-white/5 border border-white/5 focus:border-primary/30 focus:bg-white/10 focus:ring-4 focus:ring-primary/10 rounded-[1.5rem] pl-14 pr-6 py-4 transition-all outline-none font-bold placeholder:text-base-content/20"
                    autocomplete="username" 
                  />
               </div>
            </div>

            <div class="space-y-1.5">
               <label class="text-[10px] font-black uppercase tracking-widest opacity-30 ml-4">{{ $t('login.password') }}</label>
               <div class="relative group">
                  <div class="absolute inset-y-0 left-5 flex items-center text-base-content/20 group-focus-within:text-primary transition-colors">
                     <Lock :size="20" />
                  </div>
                  <input 
                    v-model="password" 
                    type="password" 
                    :placeholder="$t('login.passwordPlaceholder')" 
                    class="w-full bg-white/5 border border-white/5 focus:border-primary/30 focus:bg-white/10 focus:ring-4 focus:ring-primary/10 rounded-[1.5rem] pl-14 pr-6 py-4 transition-all outline-none font-bold placeholder:text-base-content/20"
                    autocomplete="current-password" 
                  />
               </div>
            </div>

            <div class="flex items-center justify-between px-2 pt-2">
               <label class="flex items-center gap-3 cursor-pointer group">
                  <input v-model="remember" type="checkbox" class="checkbox checkbox-primary rounded-lg border-white/10" />
                  <span class="text-xs font-bold text-base-content/40 group-hover:text-base-content transition-colors">{{ $t('login.remember') }}</span>
               </label>
               <a href="#" class="text-[10px] font-black uppercase tracking-widest text-primary hover:underline underline-offset-4">{{ $t('login.forgot') }}</a>
            </div>

            <button 
              type="submit" 
              class="w-full btn btn-primary h-16 rounded-[1.5rem] shadow-xl shadow-lg text-xs font-black uppercase tracking-[0.2em] gap-3 mt-4 group" 
              :disabled="loading"
            >
              <span v-if="loading" class="loading loading-spinner"></span>
              <template v-else>
                 {{ $t('login.submit') }}
                 <LogIn :size="20" class="group-hover:translate-x-1 transition-transform" />
              </template>
            </button>
          </form>

          <!-- Footer -->
          <div class="mt-12 text-center">
             <p class="text-[9px] font-bold text-base-content/20 uppercase tracking-[0.3em]">{{ $t('login.securing') }}</p>
          </div>
        </div>
      </div>
      
      <!-- Bottom Links -->
      <div class="mt-8 flex justify-center gap-6 opacity-30">
         <a href="#" class="text-[10px] font-black uppercase tracking-widest hover:opacity-100 transition-opacity">GitHub</a>
         <a href="#" class="text-[10px] font-black uppercase tracking-widest hover:opacity-100 transition-opacity">Documentation</a>
         <a href="#" class="text-[10px] font-black uppercase tracking-widest hover:opacity-100 transition-opacity">Support</a>
      </div>
    </div>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

input:-webkit-autofill,
input:-webkit-autofill:hover,
input:-webkit-autofill:focus {
  -webkit-text-fill-color: white;
  -webkit-box-shadow: 0 0 0px 1000px rgba(255, 255, 255, 0.05) inset;
  transition: background-color 5000s ease-in-out 0s;
}
</style>
