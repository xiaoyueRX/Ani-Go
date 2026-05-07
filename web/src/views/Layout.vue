<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import request from '../utils/request'
import IconSax from '../components/IconSax.vue'

const router = useRouter()
const route = useRoute()
const username = ref('')

onMounted(async () => {
  try {
    const { data } = await request.get('/me')
    username.value = data.username
  } catch { /* 401 handled by interceptor */ }
})

function logout() {
  localStorage.removeItem('token')
  router.push('/login')
}

const nav = [
  { path: '/schedule', label: '新番时间表', icon: 'calendar' },
  { path: '/subscriptions', label: '订阅管理', icon: 'category' },
  { path: '/search', label: '搜索番剧', icon: 'search' },
  { path: '/downloads', label: '下载队列', icon: 'download' },
  { path: '/settings', label: '设置', icon: 'setting' },
]
</script>

<template>
  <div class="drawer lg:drawer-open">
    <input id="drawer-toggle" type="checkbox" class="drawer-toggle" />

    <div class="drawer-content">
      <!-- top bar (mobile) -->
      <div class="navbar bg-base-100 shadow lg:hidden">
        <div class="flex-1">
          <label for="drawer-toggle" class="btn btn-ghost drawer-button">
            <IconSax name="menu" />
          </label>
          <span class="font-bold text-lg">Ani-Go</span>
        </div>
        <div class="flex-none flex items-center gap-1">
          <span class="text-sm opacity-70 mr-1">{{ username }}</span>
          <button class="btn btn-ghost btn-sm" @click="logout">
            <IconSax name="logout" :size="18" />
          </button>
        </div>
      </div>

      <!-- page content -->
      <div class="p-3 sm:p-4 md:p-8">
        <router-view />
      </div>
    </div>

    <!-- sidebar -->
    <div class="drawer-side">
      <label for="drawer-toggle" class="drawer-overlay"></label>
      <aside class="bg-base-200 w-64 min-h-screen flex flex-col">
        <div class="p-5">
          <h2 class="text-xl font-bold tracking-tight">
            <IconSax name="antenna" :size="22" class="inline-block mr-2 text-primary" />
            Ani-Go
          </h2>
        </div>
        <ul class="menu flex-1 gap-1 px-3">
          <li v-for="item in nav" :key="item.path">
            <router-link
              :to="item.path"
              :class="{ active: route.path.startsWith(item.path) }"
            >
              <IconSax :name="item.icon" />
              {{ item.label }}
            </router-link>
          </li>
        </ul>
        <div class="p-4 border-t border-base-300">
          <div class="flex items-center gap-2 text-sm text-base-content/60 mb-2 px-2">
            <IconSax name="user" :size="16" />
            {{ username }}
          </div>
          <button class="btn btn-ghost btn-sm w-full justify-start gap-2" @click="logout">
            <IconSax name="logout" :size="16" />
            退出登录
          </button>
        </div>
      </aside>
    </div>
  </div>
</template>
