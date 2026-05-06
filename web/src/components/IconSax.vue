<script setup lang="ts">
withDefaults(defineProps<{
  name: string
  size?: number
  color?: string
}>(), { size: 20, color: 'currentColor' })

// Iconsax Linear 风格图标
const icons: Record<string, string> = {
  // 导航
  'category': 'M7 3H4a1 1 0 0 0-1 1v3a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1V4a1 1 0 0 0-1-1zm0 13H4a1 1 0 0 0-1 1v3a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-3a1 1 0 0 0-1-1zm13-13h-3a1 1 0 0 0-1 1v3a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1V4a1 1 0 0 0-1-1zm0 13h-3a1 1 0 0 0-1 1v3a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-3a1 1 0 0 0-1-1z',
  'search': 'M21 21l-4.35-4.35M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z',
  'download': 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4m7-10v12m0 0l-5.36-5.36M12 17l5.36-5.36',
  'setting': 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zm9-3c0-.6-.1-1.2-.2-1.8l1.8-1.4-1.8-3.2-2.2.8c-.5-.4-1.1-.8-1.8-1L16.5 3h-3.6l-.7 2.2c-.6.1-1.2.3-1.8.6L8.5 4.6l-1.8 3.2 1.8 1.4c-.1.6-.2 1.2-.2 1.8s.1 1.2.2 1.8l-1.8 1.4 1.8 3.2 2.2-.8c.5.4 1.1.8 1.8 1L12.9 21h3.6l.7-2.2c.6-.1 1.2-.3 1.8-.6l2.2.8 1.8-3.2-1.8-1.4c.1-.6.2-1.2.2-1.8z',
  'logout': 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4m7 12l5-5m0 0l-5-5m5 5H9',
  // 订阅
  'add': 'M12 5v14M5 12h14',
  'check': 'M9 12l2 2 4-4m6 2a9 9 0 1 1-18 0 9 9 0 0 1 18 0z',
  'close': 'M18 6L6 18M6 6l12 12',
  'pause': 'M10 4H6v16h4V4zm8 0h-4v16h4V4z',
  'play': 'M8 5v14l11-7L8 5z',
  'warning': 'M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z',
  'refresh': 'M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15',
  'history': 'M12 8v4l2 2m6 0a8 8 0 1 1-4-7M1 4v4h4',
  // 设置 tab
  'antenna': 'M12 2a15.3 15.3 0 0 1 10.6 4.4M12 6a11.5 11.5 0 0 1 8 3.3M12 10a7.6 7.6 0 0 1 5.3 2.2M12 14a3.8 3.8 0 0 1 2.7 1.1M12 20v1',
  'folder': 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2v11z',
  'notification': 'M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9m-4.27 13a2 2 0 0 1-3.46 0M13.73 21a2 2 0 0 1-3.46 0',
  'cpu': 'M9 3v2m6-2v2M5 8h14M5 12h14M5 16h14M9 19v2m6-2v2M3 9v6m18-6v6M7 7h10v10H7V7z',
  'document': 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6zM14 2v6h6m-4 5H8m8 4H8m2-8H8',
  'more': 'M12 5v.01M12 12v.01M12 19v.01M12 12h.01M12 19h.01M12 5h.01',
  // 通用
  'chevron-left': 'M15 18l-6-6 6-6',
  'trash': 'M3 6h18m-2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2',
  'edit': 'M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z',
  'login': 'M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4m-5-4l5-5m0 0l-5-5m5 5H3',
  'user': 'M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2m8-10a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  'lock': 'M12 2a6 6 0 0 0-6 6v4a6 6 0 0 0 12 0V8a6 6 0 0 0-6-6zM4 12h16',
  'menu': 'M4 6h16M4 12h16M4 18h16',
}

function getPath(name: string): string {
  return icons[name] || icons['more']
}
</script>

<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    :stroke="color"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
  >
    <path :d="getPath(name)" />
  </svg>
</template>
