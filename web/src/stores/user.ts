import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi, type User } from '@/api/auth'

export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)
  const ready = ref(false)

  const isLoggedIn = computed(() => user.value != null && user.value.is_active)
  const isOwner = computed(() => user.value?.is_owner === true)

  async function refresh() {
    try {
      const r = await authApi.me()
      user.value = r.user
    } catch {
      user.value = null
    } finally {
      ready.value = true
    }
  }

  function setUser(u: User | null) {
    user.value = u
    ready.value = true
  }

  async function logout() {
    try {
      await authApi.logout()
    } catch {
      // ignore
    }
    user.value = null
  }

  return { user, ready, isLoggedIn, isOwner, refresh, setUser, logout }
})