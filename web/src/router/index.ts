import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('@/layouts/AppLayout.vue'),
      children: [
        { path: '', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: '总览' } },
        { path: 'processes', name: 'processes', component: () => import('@/views/ProcessesView.vue'), meta: { title: '进程与依赖' } },
        { path: 'tasks', name: 'tasks', component: () => import('@/views/TasksView.vue'), meta: { title: '托管任务' } },
        { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { title: '设置与审计' } },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) {
    return true
  }
  const auth = useAuthStore()
  if (!auth.checked) {
    const ok = await auth.fetchMe()
    if (!ok) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
  }
  if (!auth.authenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  return true
})

export default router
