<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" @click.self="$emit('close')">
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg mx-4 overflow-hidden">
      <!-- 标题 -->
      <div class="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
        <h3 class="text-lg font-semibold text-gray-800 dark:text-white">设置</h3>
        <button @click="$emit('close')" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      
      <!-- 设置内容 -->
      <div class="p-6 space-y-6 max-h-[70vh] overflow-y-auto">
        <!-- 外观 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">外观</h4>
          
          <div class="space-y-4">
            <div>
              <label class="block text-sm text-gray-600 dark:text-gray-400 mb-2">主题</label>
              <div class="flex gap-2">
                <button
                  v-for="t in themes"
                  :key="t.value"
                  @click="theme = t.value"
                  :class="[
                    'flex-1 py-2 px-4 rounded-lg border text-sm font-medium transition-colors',
                    theme === t.value
                      ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                      : 'border-gray-200 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700'
                  ]"
                >
                  {{ t.label }}
                </button>
              </div>
            </div>
          </div>
        </section>
        
        <!-- 行为 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">行为</h4>
          
          <div class="space-y-4">
            <label class="flex items-center justify-between">
              <div>
                <span class="text-sm text-gray-700 dark:text-gray-300">开机自动启动</span>
                <p class="text-xs text-gray-500 dark:text-gray-400">系统启动时自动运行所有节点</p>
              </div>
              <input
                type="checkbox"
                v-model="autoStart"
                class="h-4 w-4 text-primary-600 rounded focus:ring-primary-500"
              />
            </label>
            
            <label class="flex items-center justify-between">
              <div>
                <span class="text-sm text-gray-700 dark:text-gray-300">最小化到托盘</span>
                <p class="text-xs text-gray-500 dark:text-gray-400">关闭窗口时最小化到系统托盘</p>
              </div>
              <input
                type="checkbox"
                v-model="minimizeToTray"
                class="h-4 w-4 text-primary-600 rounded focus:ring-primary-500"
              />
            </label>
          </div>
        </section>
        
        <!-- 数据管理 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">数据管理</h4>
          
          <div class="space-y-3">
            <button @click="openConfigFolder" class="w-full btn-secondary text-left flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
              </svg>
              打开配置文件夹
            </button>
            
            <button @click="openLogFolder" class="w-full btn-secondary text-left flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
              打开日志文件夹
            </button>
            
            <button @click="clearFakeIPCache" class="w-full btn-secondary text-left flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              清空 Fake-IP 缓存
            </button>
            
            <button @click="flushDNSCache" class="w-full btn-secondary text-left flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
              刷新系统 DNS 缓存
            </button>
          </div>
        </section>
        
        <!-- 关于 -->
        <section>
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">关于</h4>
          
          <div class="p-4 bg-gray-50 dark:bg-gray-700/50 rounded-lg text-sm text-gray-600 dark:text-gray-400">
            <p><strong>Xlink 客户端</strong> v22.0.0</p>
            <p class="mt-1">Wails + Vue3 + Go</p>
            <p class="mt-2 text-xs">
              一个强大的代理客户端，支持智能分流、DNS防泄露等功能。
            </p>
          </div>
        </section>
      </div>
      
      <!-- 底部按钮 -->
      <div class="px-6 py-4 border-t border-gray-200 dark:border-gray-700 flex justify-end gap-3">
        <button @click="$emit('close')" class="btn-secondary">
          关闭
        </button>
        <button @click="saveSettings" class="btn-primary">
          保存设置
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAppStore } from '@/stores/app'

// Wails 绑定
declare const window: {
  go: {
    main: {
      App: {
        GetSettings(): Promise<any>
        UpdateSettings(settings: any): Promise<void>
        SetAutoStart(enabled: boolean): Promise<void>
        OpenConfigFolder(): Promise<void>
        OpenLogFolder(): Promise<void>
        ClearFakeIPCache(): Promise<void>
        FlushDNSCache(): Promise<void>
      }
    }
  }
}

const emit = defineEmits<{
  close: []
}>()

const appStore = useAppStore()

// 定义 Theme 类型
type Theme = 'light' | 'dark' | 'system'

const theme = ref<Theme>('system')
const autoStart = ref(false)
const minimizeToTray = ref(true)

// 【修复 1】显式声明数组类型，解决模板中 theme = t.value 的类型报错
const themes: { value: Theme; label: string }[] = [
  { value: 'light', label: '浅色' },
  { value: 'dark', label: '深色' },
  { value: 'system', label: '跟随系统' }
]

onMounted(async () => {
  try {
    const settings = await window.go.main.App.GetSettings()
    // 【修复 2】保留类型断言，防止后端返回值类型检查失败
    theme.value = (settings.theme as Theme) || 'system'
    autoStart.value = settings.auto_start || false
    minimizeToTray.value = settings.minimize_to_tray !== false
  } catch (e) {
    console.error('Failed to load settings:', e)
  }
})

async function saveSettings() {
  try {
    // 更新主题
    appStore.setTheme(theme.value)
    
    // 保存到后端
    await window.go.main.App.UpdateSettings({
      theme: theme.value,
      auto_start: autoStart.value,
      minimize_to_tray: minimizeToTray.value
    })
    
    // 设置开机自启
    await window.go.main.App.SetAutoStart(autoStart.value)
    
    appStore.showToast('success', '设置已保存')
    emit('close')
  } catch (e: any) {
    appStore.showToast('error', e.message || '保存失败')
  }
}

async function openConfigFolder() {
  try {
    await window.go.main.App.OpenConfigFolder()
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function openLogFolder() {
  try {
    await window.go.main.App.OpenLogFolder()
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function clearFakeIPCache() {
  try {
    await window.go.main.App.ClearFakeIPCache()
    appStore.showToast('success', 'Fake-IP 缓存已清空')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function flushDNSCache() {
  try {
    await window.go.main.App.FlushDNSCache()
    appStore.showToast('success', 'DNS 缓存已刷新')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}
</script>
