import { ref, watch } from 'vue'

const STORAGE_KEY = 'sevrtree-theme'

export const isDark = ref(localStorage.getItem(STORAGE_KEY) !== 'light')

function applyTheme(dark: boolean) {
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

applyTheme(isDark.value)
watch(isDark, (value) => {
  localStorage.setItem(STORAGE_KEY, value ? 'dark' : 'light')
  applyTheme(value)
})

export function toggleTheme() {
  isDark.value = !isDark.value
}
