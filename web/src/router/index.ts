import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { useUserStore } from '@/stores/user'

const routes: RouteRecordRaw[] = [
  { path: '/login', component: () => import('@/views/LoginView.vue'), meta: { guest: true } },
  { path: '/register', component: () => import('@/views/RegisterView.vue'), meta: { guest: true } },
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', name: 'dashboard', component: () => import('@/views/DashboardView.vue') },
      { path: 'upload', name: 'upload', component: () => import('@/views/UploadView.vue') },
      { path: 'wars', name: 'wars', component: () => import('@/views/WarsView.vue') },
      {
        path: 'stockpiles/:id',
        name: 'stockpile',
        component: () => import('@/views/StockpileDetailView.vue'),
        props: true,
      },
      {
        path: 'items',
        name: 'items',
        component: () => import('@/views/ItemsView.vue'),
      },
      {
        path: 'items/:cid',
        name: 'item',
        component: () => import('@/views/ItemDetailView.vue'),
        props: true,
      },
      {
        path: 'admin',
        component: () => import('@/views/admin/AdminLayout.vue'),
        meta: { requiresOwner: true },
        children: [
          { path: '', redirect: { name: 'admin-stats' } },
          { path: 'stats', name: 'admin-stats', component: () => import('@/views/admin/StatsView.vue') },
          {
            path: 'users',
            name: 'admin-users',
            component: () => import('@/views/admin/WhitelistView.vue'),
          },
          {
            path: 'wars',
            name: 'admin-wars',
            component: () => import('@/views/admin/WarManageView.vue'),
          },
          {
            path: 'items',
            name: 'admin-items',
            component: () => import('@/views/admin/ItemManageView.vue'),
          },
        ],
      },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const store = useUserStore()
  if (!store.ready) {
    await store.refresh()
  }
  if (to.meta.requiresAuth && !store.isLoggedIn) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.requiresOwner && !store.isOwner) {
    return { path: '/' }
  }
  if (to.meta.guest && store.isLoggedIn) {
    return { path: '/' }
  }
  return true
})