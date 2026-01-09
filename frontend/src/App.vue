<template>
  <div class="h-full flex flex-col bg-gray-100 dark:bg-gray-900">
    <!-- 顶部标题栏 -->
    <AppHeader @open-settings="showSettings = true" />
    
    <!-- 主内容区域 -->
    <div class="flex-1 flex overflow-hidden">
      <!-- 左侧节点列表 -->
      <AppSidebar />
      
      <!-- 右侧内容区 -->
      <main class="flex-1 flex flex-col overflow-hidden">
        <!-- 节点配置编辑器 -->
        <div class="flex-1 overflow-auto p-4">
          <!-- ⚠️ KEY FIX: :key forces component re-creation on node change -->
          <NodeEditor v-if="currentNode" :node="currentNode" :key="currentNode.id" />
          
          <div v-else class="h-full flex items-center justify-center text-gray-500 dark:text-gray-400">
            <div class="text-center">
              <svg class="w-16 h-16 mx-auto mb-4 text-gray-300 dark:text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" 
                      d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2" />
              </svg>
              <p class="text-lg font-medium">请选择或创建一个节点</p>
              <p class="text-sm mt-2 opacity-70">在左侧列表点击 "+ 新建" 按钮开始使用</p>
            </div>
          </div>
        </div>
        
        <!-- 底部日志区域 -->
        <div class="h-64 border-t border-gray-200 dark:border-gray-700 shrink-0">
          <LogViewer />
        </div>
      </main>
    </div>
    
    <!-- 全局设置对话框 -->
    <GeneralSettings v-if="showSettings" @close="showSettings = false" />
    
    <!-- 全局 Toast 通知 -->
    <div class="fixed bottom-4 right-4 z-50 space-y-2 pointer-events-none">
      <TransitionGroup name="fade">
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="[
            'px-4 py-3 rounded-lg shadow-lg text-white text-sm flex items-center gap-2 pointer-events-auto',
            toast.type === 'success' && 'bg-green-500',
            toast.type === 'error' && 'bg-red-500',
            toast.type === 'warning' && 'bg-yellow-500',
            toast.type === 'info' && 'bg-blue-500'
          ]"
        >
          <span v-if="toast.type === 'success'">✓</span>
          <span v-else-if="toast.type === 'error'">✗</span>
          <span v-else-if="toast.type === 'warning'">⚠</span>
          <span v-else>ℹ</span>
          {{ toast.message }}
        </div>
      </TransitionGroup>
    </div>
    
    <!-- 全局加载遮罩 -->
    <Transition name="fade">
      <div v-if="isLoading" class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 backdrop-blur-sm">
        <div class="bg-white dark:bg-gray-800 rounded-lg p-6 shadow-xl text-center">
          <div class="animate-spin w-10 h-10 border-4 border-primary-500 border-t-transparent rounded-full mx-auto"></div>
          <p class="mt-4 text-sm font-medium text-gray-600 dark:text-gray-300">加载中...</p>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'
import { useLogsStore } from '@/stores/logs'
import { useWailsEvent } from '@/composables/useWails'

import AppHeader from '@/components/layout/AppHeader.vue'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import NodeEditor from '@/components/nodes/NodeEditor.vue'
import LogViewer from '@/components/logs/LogViewer.vue'
import GeneralSettings from '@/components/settings/GeneralSettings.vue'

const appStore = useAppStore()
const nodesStore = useNodesStore()
const logsStore = useLogsStore()

const showSettings = ref(false)
const currentNode = computed(() => nodesStore.currentNode)
const toasts = computed(() => appStore.toasts)
const isLoading = computed(() => appStore.isLoading)

// Events
useWailsEvent('node:status', (data: { node_id: string; status: string }) => {
  nodesStore.updateNodeStatus(data.node_id, data.status)
})

useWailsEvent('log:append', (entry: any) => {
  logsStore.addLog(entry)
})

useWailsEvent('config:changed', () => {
  nodesStore.fetchNodes()
})

useWailsEvent('ping:result', (result: any) => {
  // console.debug('Ping result:', result)
})

onMounted(async () => {
  appStore.setLoading(true)
  try {
    await Promise.all([
      nodesStore.fetchNodes(),
      logsStore.fetchLogs()
    ])
  } catch (e: any) {
    appStore.showToast('error', '应用初始化失败: ' + e.message)
    console.error('Initialization failed:', e)
  } finally {
    appStore.setLoading(false)
  }
})
</script>

<style>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
