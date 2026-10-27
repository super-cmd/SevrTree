import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'

interface MePayload {
  username: string
  authenticated: boolean
  version: string
}

export const useAuthStore = defineStore('auth', () => {
  const username = ref('')
  const version = ref('')
  const authenticated = ref(false)
  const checked = ref(false)

  async function fetchMe(): Promise<boolean> {
    try {
      const data = await api.get<MePayload>('/api/auth/me')
      username.value = data.username
      version.value = data.version
      authenticated.value = data.authenticated
    } catch {
      authenticated.value = false
    } finally {
      checked.value = true
    }
    return authenticated.value
  }

  async function login(password: string): Promise<void> {
    const data = await api.post<{ username: string }>('/api/auth/login', { password })
    username.value = data.username
    authenticated.value = true
    checked.value = true
  }

  async function logout(): Promise<void> {
    try {
      await api.post('/api/auth/logout')
    } finally {
      authenticated.value = false
    }
  }

  return { username, version, authenticated, checked, fetchMe, login, logout }
})
