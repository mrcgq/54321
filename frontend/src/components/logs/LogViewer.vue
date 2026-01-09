<template>
  <div class="h-full flex flex-col bg-gray-900">
    <!-- 工具栏 -->
    <div class="flex items-center gap-2 px-4 py-2 bg-gray-800 border-b border-gray-700 shrink-0">
      <span class="text-sm font-medium text-gray-300">运行日志</span>
      
      <!-- 过滤器 -->
      <div class="flex-1 flex items-center gap-2 ml-4">
        <!-- 级别过滤 -->
        <select
          v-model="filter.level"
          class="text-xs bg-gray-700 border-gray-600 text-gray-300 rounded px-2 py-1 focus:ring-primary-500"
        >
          <option value="">全部级别</option>
          <option value="error">错误</option>
          <option value="warn">警告</option>
          <option value="info">信息</option>
          <option value="debug">调试</option>
        </select>
        
        <!-- 类别过滤 -->
        <select
          v-model="filter.category"
          class="text-xs bg-gray-700 border-gray-600 text-gray-300 rounded px-2 py-1 focus:ring-primary-500"
        >
          <option value="">全部类别</option>
          <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
        </select>
        
        <!-- 搜索 -->
        <div class="relative flex-1 max-w-xs">
          <input
            v-model="filter.search"
            type="text"
            placeholder="搜索日志..."
            class="w-full text-xs bg-gray-700 border-gray-600 text-gray-300 rounded pl-8 pr-3 py-1 focus:ring-primary-500"
          />
          <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </div>
      </div>
      
      <!-- 操作按钮 -->
      <div class="flex items-center gap-1">
        <button
          @click="toggleAutoScroll"
          :class="['p-1.5 rounded text-xs', autoScroll ? 'bg-primary-600 text-white' : 'bg-gray-700 text-gray-400']"
          title="自动滚动"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
          </svg>
        </button>
        
        <button
          @click="exportLogs"
          class="p-1.5 rounded bg-gray-700 text-gray-400 hover:text-gray-200"
          title="导出日志"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
          </svg>
        </button>
        
        <button
          @click="clearLogs"
          class="p-1.5 rounded bg-gray-700 text-gray-400 hover:text-gray-200"
          title="清空日志"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
        </button>
      </div>
    </div>
    
    <!-- 日志内容 -->
    <div
      ref="logContainer"
      class="flex-1 overflow-y-auto overflow-x-hidden px-4 py-2 font-mono text-xs"
      @scroll="handleScroll"
    >
      <div
        v-for="(log, index) in filteredLogs"
        :key="index"
        :class="['log-entry', `level-${log.level}`]"
      >
        <span class="text-gray-500">[{{ formatTime(log.timestamp) }}]</span>
        <span class="text-cyan-400 ml-1">[{{ log.node_name }}]</span>
        <span :class="getCategoryColor(log.category)" class="ml-1">[{{ log.category }}]</span>
        <span class="ml-1 selectable">{{ log.message }}</span>
      </div>
      
      <!-- 空状态 -->
      <div v-if="filteredLogs.length === 0" class="h-full flex items-center justify-center text-gray-500">
        <div class="text-center">
          <svg class="w-12 h-12 mx-auto mb-2 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
          <p>暂无日志</p>
        </div>
      </div>
    </div>
    
    <!-- 底部状态栏 -->
    <div class="px-4 py-1 bg-gray-800 border-t border-gray-700 flex items-center justify-between text-xs text-gray-500 shrink-0">
      <span>共 {{ filteredLogs.length }} 条日志</span>
      <span v-if="filter.level || filter.category || filter.search">已过滤</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted } from 'vue'
import { useLogsStore } from '@/stores/logs'
import { useAppStore } from '@/stores/app'

const logsStore = useLogsStore()
const appStore = useAppStore()

const logContainer = ref<HTMLElement | null>(null)

const filter = computed({
  get: () => logsStore.filter,
  set: (val) => {
    Object.keys(val).forEach(key => {
      logsStore.setFilter(key as any, val[key as keyof typeof val])
    })
  }
})

const autoScroll = computed(() => logsStore.autoScroll)
const filteredLogs = computed(() => logsStore.filteredLogs)
const categories = computed(() => logsStore.categories)

// 监听日志变化，自动滚动
watch(filteredLogs, () => {
  if (autoScroll.value) {
    nextTick(() => {
      scrollToBottom()
    })
  }
}, { deep: true })

onMounted(() => {
  scrollToBottom()
})

function formatTime(timestamp: string): string {
  try {
    const date = new Date(timestamp)
    return date.toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    })
  } catch {
    return timestamp
  }
}

function getCategoryColor(category: string): string {
  const colors: Record<string, string> = {
    '系统': 'text-blue-400',
    '内核': 'text-green-400',
    '隧道': 'text-purple-400',
    '规则': 'text-yellow-400',
    '负载': 'text-orange-400',
    '统计': 'text-pink-400',
    '测速': 'text-cyan-400',
    'Xray': 'text-indigo-400',
    'DNS': 'text-teal-400'
  }
  return colors[category] || 'text-gray-400'
}

function toggleAutoScroll() {
  logsStore.autoScroll = !logsStore.autoScroll
  if (logsStore.autoScroll) {
    scrollToBottom()
  }
}

function scrollToBottom() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

function handleScroll() {
  if (!logContainer.value) return
  
  const { scrollTop, scrollHeight, clientHeight } = logContainer.value
  const isAtBottom = scrollHeight - scrollTop - clientHeight < 50
  
  // 如果用户手动滚动到非底部，禁用自动滚动
  if (!isAtBottom && logsStore.autoScroll) {
    logsStore.autoScroll = false
  }
}

async function clearLogs() {
  if (confirm('确定要清空所有日志吗？')) {
    await logsStore.clearLogs()
    appStore.showToast('success', '日志已清空')
  }
}

async function exportLogs() {
  try {
    const path = await logsStore.exportLogs('txt')
    if (path) {
      appStore.showToast('success', `日志已导出到: ${path}`)
    }
  } catch (e: any) {
    appStore.showToast('error', e.message || '导出失败')
  }
}
</script>

<style scoped>
.log-entry {
  white-space: nowrap;
}

.log-entry.level-error {
  @apply text-red-400;
}

.log-entry.level-warn {
  @apply text-yellow-400;
}

.log-entry.level-info {
  @apply text-gray-300;
}

.log-entry.level-debug {
  @apply text-gray-500;
}
</style>
