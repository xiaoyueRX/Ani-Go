import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
    },
    {
      path: '/',
      component: () => import('../views/Layout.vue'),
      children: [
        {
          path: '',
          redirect: '/subscriptions',
        },
        {
          path: 'subscriptions',
          name: 'subscriptions',
          component: () => import('../views/Subscriptions.vue'),
        },
        {
          path: 'subscriptions/new',
          name: 'subscription-create',
          component: () => import('../views/SubscriptionForm.vue'),
        },
        {
          path: 'subscriptions/:id',
          name: 'subscription-detail',
          component: () => import('../views/SubscriptionDetail.vue'),
        },
        {
          path: 'downloads',
          name: 'downloads',
          component: () => import('../views/Downloads.vue'),
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('../views/SettingsPage.vue'),
        },
      ],
    },
  ],
})

router.beforeEach((to) => {
  const token = localStorage.getItem('token')
  if (to.path !== '/login' && !token) {
    return '/login'
  }
})

export default router
