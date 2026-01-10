<template>
  <header class="h-14 bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 flex items-center px-4 shrink-0 transition-colors duration-200">
    <!-- Logo 和标题 -->
    <div class="flex items-center gap-3">
      <div class="w-8 h-8 bg-gradient-to-br from-blue-500 to-purple-600 rounded-lg flex items-center justify-center shadow-sm">
        <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" 
                d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      </div>
      <h1 class="text-lg font-semibold text-gray-800 dark:text-white tracking-tight">
        Xlink 客户端
      </h1>
      <span class="text-xs text-gray-500 dark:text-gray-400 bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded border border-gray-200 dark:border-gray-600">
        v22.0
      </span>
    </div>
    
    <!-- 运行状态指示 -->
    <div class="ml-6 flex items-center gap-2 px-3 py-1 rounded-full bg-gray-50 dark:bg-gray-700/50">
      <div :class="['status-dot', hasRunningNodes ? 'running' : 'stopped']"></div>
      <span class="text-xs font-medium text-gray-600 dark:text-gray-300">
        {{ hasRunningNodes ? `${runningCount} 个节点运行中` : '未运行' }}
      </span>
    </div>
    
    <!-- 间隔 -->
    <div class="flex-1"></div>

    <!-- ✨ 系统代理开关 -->
    <div class="mr-4 flex items-center gap-2 border-r border-gray-200 dark:border-gray-700 pr-4">
      <label class="flex items-center cursor-pointer select-none" title="开启后将接管系统所有流量">
        <div class="relative">
          <input type="checkbox" class="sr-only" v-model="isSystemProxyEnabled" @change="toggleSystemProxy">
          <div :class="['block w-9 h-5 rounded-full transition-colors duration-200 ease-in-out', isSystemProxyEnabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-gray-600']"></div>
          <div :class="['absolute left-1 top-1 bg-white w-3 h-3 rounded-full transition-transform duration-200 ease-in-out', isSystemProxyEnabled ? 'translate-x-4' : '']"></div>
        </div>
        <span class="ml-2 text-xs font-medium text-gray-600 dark:text-gray-300">系统代理</span>
      </label>
    </div>
    
    <!-- 全局操作按钮 -->
    <div class="flex items-center gap-2">
      <button
        @click="startAll"
        :disabled="isAllRunning"
        class="btn-success flex items-center gap-1.5 text-xs py-1.5 shadow-sm active:scale-95 transition-transform"
        title="启动所有节点"
      >
        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        全部启动
      </button>
      
      <button
        @click="stopAll"
        :disabled="!hasRunningNodes"
        class="btn-danger flex items-center gap-1.5 text-xs py-1.5 shadow-sm active:scale-95 transition-transform"
        title="停止所有节点"
      >
        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
        </svg>
        全部停止
      </button>
    </div>
    
    <!-- 右侧工具栏 -->
    <div class="ml-4 flex items-center gap-1 pl-4 border-l border-gray-200 dark:border-gray-700">
      <!-- 主题切换 -->
      <button @click="toggleTheme" class="btn-icon group" title="切换主题">
        <svg v-if="isDark" class="w-5 h-5 text-yellow-400 group-hover:scale-110 transition-transform" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z" clip-rule="evenodd" />
        </svg>
        <svg v-else class="w-5 h-5 text-gray-500 group-hover:text-gray-700 group-hover:scale-110 transition-transform" fill="currentColor" viewBox="0 0 20 20">
          <path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z" />
        </svg>
      </button>
      
      <!-- 设置按钮 -->
      <button @click="showSettings" class="btn-icon group" title="设置">
        <svg class="w-5 h-5 text-gray-500 dark:text-gray-400 group-hover:text-gray-700 dark:group-hover:text-gray-200 group-hover:rotate-45 transition-transform duration-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
      </button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'

// 声明 Wails 绑定，防止 TypeScript 报错
declare const window: any

const appStore = useAppStore()
const nodesStore = useNodesStore()

// 定义事件：打开设置
const emit = defineEmits<{
  openSettings: []
}>()

// 计算属性
const isDark = computed(() => appStore.isDark)
const hasRunningNodes = computed(() => nodesStore.hasRunningNodes)
const runningCount = computed(() => nodesStore.runningNodes.length)
const isAllRunning = computed(() => {
  return nodesStore.nodes.length > 0 && 
         nodesStore.nodes.every(n => nodesStore.getNodeStatus(n.id) === 'running')
})

// 系统代理状态
const isSystemProxyEnabled = ref(false)

// 切换系统代理逻辑
async function toggleSystemProxy() {
  // 1. 开启代理
  if (isSystemProxyEnabled.value) {
    if (!nodesStore.currentNodeId) {
      appStore.showToast('warning', '请先选择一个节点')
      isSystemProxyEnabled.value = false
      return
    }
    
    try {
      // 调用后端: SetSystemProxy(nodeID)
      // 后端会根据该节点的监听端口来设置 IE 代理
      await window.go.main.App.SetSystemProxy(nodesStore.currentNodeId)
      appStore.showToast('success', '系统代理已开启')
    } catch (e: any) {
      appStore.showToast('error', '开启失败: ' + e.message)
      isSystemProxyEnabled.value = false
    }
  } 
  // 2. 关闭代理
  else {
    try {
      await window.go.main.App.ClearSystemProxy()
      appStore.showToast('success', '系统代理已清除')
    } catch (e: any) {
      console.error(e)
    }
  }
}

// 智能监听：如果所有节点都停止了，自动关闭系统代理
// 防止用户停止代理后忘记关系统设置导致断网
watch(hasRunningNodes, (running) => {
  if (!running && isSystemProxyEnabled.value) {
    isSystemProxyEnabled.value = false
    window.go.main.App.ClearSystemProxy().catch(console.error)
    appStore.showToast('info', '节点已停止，系统代理自动关闭')
  }
})

// 切换主题
function toggleTheme() {
  const themes: Array<'light' | 'dark' | 'system'> = ['light', 'dark', 'system']
  const currentIndex = themes.indexOf(appStore.theme)
  const nextIndex = (currentIndex + 1) % themes.length
  appStore.setTheme(themes[nextIndex])
}

// 启动所有
async function startAll() {
  try {
    await nodesStore.startAllNodes()
    appStore.showToast('success', '指令已发送：启动所有节点')
  } catch (e: any) {
    appStore.showToast('error', e.message || '启动失败')
  }
}

// 停止所有
async function stopAll() {
  try {
    await nodesStore.stopAllNodes()
    appStore.showToast('success', '所有节点已停止')
  } catch (e: any) {
    appStore.showToast('error', e.message || '停止失败')
  }
}

// 打开设置
function showSettings() {
  emit('openSettings')
}
</script>
