<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import request from '../utils/request'

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
  { path: '/subscriptions', label: '订阅管理', icon: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10' },
  { path: '/downloads', label: '下载队列', icon: 'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4' },
  { path: '/settings', label: '设置', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z' },
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
            <svg xmlns="http://www.w3.org/2000/svg" class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6h16M4 12h16M4 18h16"/></svg>
          </label>
          <span class="font-bold text-lg">Ani-Go</span>
        </div>
        <div class="flex-none">
          <span class="text-sm opacity-70 mr-2">{{ username }}</span>
          <button class="btn btn-ghost btn-sm" @click="logout">退出</button>
        </div>
      </div>

      <!-- page content -->
      <div class="p-4 md:p-8">
        <router-view />
      </div>
    </div>

    <!-- sidebar -->
    <div class="drawer-side">
      <label for="drawer-toggle" class="drawer-overlay"></label>
      <aside class="bg-base-200 w-64 min-h-screen flex flex-col">
        <div class="p-4">
          <h2 class="text-xl font-bold">Ani-Go</h2>
        </div>
        <ul class="menu flex-1 gap-1 px-2">
          <li v-for="item in nav" :key="item.path">
            <router-link
              :to="item.path"
              :class="{ active: route.path.startsWith(item.path) }"
            >
              <svg xmlns="http://www.w3.org/2000/svg" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path :d="item.icon"/></svg>
              {{ item.label }}
            </router-link>
          </li>
        </ul>
        <div class="p-4 border-t border-base-300">
          <div class="text-sm text-base-content/60">{{ username }}</div>
          <button class="btn btn-ghost btn-sm mt-2 w-full" @click="logout">退出登录</button>
        </div>
      </aside>
    </div>
  </div>
</template>
