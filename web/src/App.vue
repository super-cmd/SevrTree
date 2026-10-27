<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  NNotificationProvider,
  darkTheme,
  dateZhCN,
  zhCN,
} from 'naive-ui'
import type { GlobalThemeOverrides } from 'naive-ui'
import { isDark } from './theme'

const router = useRouter()

const theme = computed(() => (isDark.value ? darkTheme : null))

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#4f9cf9',
    primaryColorHover: '#71b2ff',
    primaryColorPressed: '#3b86e0',
    primaryColorSuppl: '#71b2ff',
    borderRadius: '6px',
  },
  Card: {
    borderRadius: '10px',
  },
}

function handleUnauthorized() {
  const current = router.currentRoute.value
  if (current.name !== 'login') {
    void router.push({ name: 'login', query: { redirect: current.fullPath } })
  }
}

onMounted(() => window.addEventListener('sevrtree:unauthorized', handleUnauthorized))
onUnmounted(() => window.removeEventListener('sevrtree:unauthorized', handleUnauthorized))
</script>

<template>
  <n-config-provider
    :theme="theme"
    :theme-overrides="themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-message-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <router-view />
        </n-notification-provider>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
