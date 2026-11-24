import { createRouter, createWebHistory } from 'vue-router'
import { routes } from './routes'

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach((to) => {
  const token = localStorage.getItem('token')
  if (to.path !== '/login' && !token) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
})

export default router
