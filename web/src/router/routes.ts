import type { RouteRecordRaw } from 'vue-router'

const pages = {
  Login: () => import('../views/Login.vue'),
  Layout: () => import('../views/Layout.vue'),
  Schedule: () => import('../views/Schedule.vue'),
  Subscriptions: () => import('../views/Subscriptions.vue'),
  Search: () => import('../views/Search.vue'),
  SubscriptionForm: () => import('../views/SubscriptionForm.vue'),
  SubscriptionDetail: () => import('../views/SubscriptionDetail.vue'),
  Downloads: () => import('../views/Downloads.vue'),
  SettingsPage: () => import('../views/SettingsPage.vue'),
}

function view(name: keyof typeof pages) {
  return { component: pages[name], meta: { sourceModule: `src/views/${name}.vue` } }
}

export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    ...view('Login'),
  },
  {
    path: '/',
    ...view('Layout'),
    children: [
      {
        path: '',
        redirect: '/schedule',
      },
      {
        path: 'schedule',
        name: 'schedule',
        ...view('Schedule'),
      },
      {
        path: 'subscriptions',
        name: 'subscriptions',
        ...view('Subscriptions'),
      },
      {
        path: 'search',
        name: 'search',
        ...view('Search'),
      },
      {
        path: 'subscriptions/new',
        name: 'subscription-create',
        ...view('SubscriptionForm'),
      },
      {
        path: 'subscriptions/:id',
        name: 'subscription-detail',
        ...view('SubscriptionDetail'),
      },
      {
        path: 'downloads',
        name: 'downloads',
        ...view('Downloads'),
      },
      {
        path: 'settings',
        name: 'settings',
        ...view('SettingsPage'),
      },
    ],
  },
]
