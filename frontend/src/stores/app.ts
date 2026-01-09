import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useAppStore = defineStore('app', () => {
  // 状态
  const theme = ref<'light' | 'dark' | 'system'>('system')
  const language = ref('zh-CN')
  const isLoading = ref(false)
  const toasts = ref<{ id: number; type: string; message: string }[]>([])
  
  let toastId = 0

  // 计算属性
  const isDark = computed(() => {
    if (theme.value === 'system') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches
    }
    return theme.value === 'dark'
  })

  // 方法
  function setTheme(newTheme: 'light' | 'dark' | 'system') {
    theme.value = newTheme
    applyTheme()
  }

  function applyTheme() {
    if (isDark.value) {
      document.documentElement.classList.add('dark')
    } else {
      document.documentElement.classList.remove('dark')
    }
  }

  function showToast(type: 'success' | 'error' | 'warning' | 'info', message: string, duration = 3000) {
    const id = ++toastId
    toasts.value.push({ id, type, message })
    
    setTimeout(() => {
      const index = toasts.value.findIndex(t => t.id === id)
      if (index !== -1) {
        toasts.value.splice(index, 1)
      }
    }, duration)
  }

  function setLoading(loading: boolean) {
    isLoading.value = loading
  }

  // 初始化主题
  applyTheme()

  // 监听系统主题变化
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (theme.value === 'system') {
      applyTheme()
    }
  })

  return {
    theme,
    language,
    isLoading,
    toasts,
    isDark,
    setTheme,
    showToast,
    setLoading
  }
})
