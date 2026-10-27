<script setup lang="ts">
import { computed, h } from 'vue'
import type { Component } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { NDropdown, NIcon, NMenu, NTag } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import {
  GitNetworkOutline,
  GridOutline,
  LayersOutline,
  LogOutOutline,
  MoonOutline,
  SettingsOutline,
  SunnyOutline,
} from '@vicons/ionicons5'
import { useAuthStore } from '@/stores/auth'
import { isDark, toggleTheme } from '@/theme'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

function icon(component: Component) {
  return () => h(NIcon, null, { default: () => h(component) })
}

const menuOptions: MenuOption[] = [
  {
    key: 'dashboard',
    label: () => h(RouterLink, { to: '/' }, { default: () => '总览' }),
    icon: icon(GridOutline),
  },
  {
    key: 'processes',
    label: () => h(RouterLink, { to: '/processes' }, { default: () => '进程与依赖' }),
    icon: icon(GitNetworkOutline),
  },
  {
    key: 'tasks',
    label: () => h(RouterLink, { to: '/tasks' }, { default: () => '托管任务' }),
    icon: icon(LayersOutline),
  },
  {
    key: 'settings',
    label: () => h(RouterLink, { to: '/settings' }, { default: () => '设置与审计' }),
    icon: icon(SettingsOutline),
  },
]

const activeKey = computed(() => String(route.name ?? 'dashboard'))
const pageTitle = computed(() => String(route.meta.title ?? '总览'))

const userOptions = [
  { key: 'logout', label: '退出登录', icon: icon(LogOutOutline) },
]

async function handleUserAction(key: string) {
  if (key === 'logout') {
    await auth.logout()
    await router.replace({ name: 'login' })
  }
}
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="brand">
        <span class="brand-dot" />
        <div>
          <div class="brand-name">SevrTree</div>
          <div class="brand-sub">进程依赖管理</div>
        </div>
      </div>

      <n-menu :value="activeKey" :options="menuOptions" :indent="18" />

      <div class="sidebar-foot">
        <n-tag size="small" :bordered="false" type="success">v{{ auth.version || '0.1.0' }}</n-tag>
        <span class="muted">仅本机访问</span>
      </div>
    </aside>

    <div class="main">
      <header class="topbar">
        <div>
          <div class="topbar-title">{{ pageTitle }}</div>
          <div class="topbar-sub muted">{{ route.path }}</div>
        </div>
        <div class="topbar-actions">
          <n-button quaternary circle @click="toggleTheme">
            <template #icon>
              <n-icon><component :is="isDark ? SunnyOutline : MoonOutline" /></n-icon>
            </template>
          </n-button>
          <n-dropdown trigger="click" :options="userOptions" @select="handleUserAction">
            <n-tag :bordered="false" type="info">{{ auth.username || 'admin' }}</n-tag>
          </n-dropdown>
        </div>
      </header>

      <main class="content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  min-height: 100vh;
  background: var(--app-bg);
}

.sidebar {
  width: 224px;
  flex-shrink: 0;
  padding: 18px 12px;
  border-right: 1px solid var(--app-border);
  background: var(--app-bg-soft);
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 10px 10px;
}

.brand-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: linear-gradient(135deg, #4f9cf9, #47d6b2);
  box-shadow: 0 0 12px rgba(79, 156, 249, 0.6);
}

.brand-name {
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.brand-sub {
  font-size: 11px;
  color: var(--app-text-muted);
}

.sidebar-foot {
  margin-top: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px 2px;
  font-size: 11px;
}

.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topbar {
  height: 60px;
  flex-shrink: 0;
  padding: 0 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--app-border);
  background: var(--app-bg-soft);
}

.topbar-title {
  font-size: 15px;
  font-weight: 600;
}

.topbar-sub {
  font-size: 11px;
  margin-top: 2px;
}

.topbar-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.content {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

@media (max-width: 860px) {
  .sidebar {
    width: 68px;
    padding: 14px 6px;
  }

  .brand {
    justify-content: center;
    padding-bottom: 6px;
  }

  .brand > div,
  .sidebar-foot span {
    display: none;
  }
}
</style>
