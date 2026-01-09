import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { LogEntry } from '@/types'

declare const window: {
  go: {
    main: {
      App: {
        GetLogs(limit: number): Promise<LogEntry[]>
        ClearLogs(): Promise<void>
        ExportLogs(format: string): Promise<string>
      }
    }
  }
  runtime: {
    EventsOn(event: string, callback: (data: any) => void): () => void
  }
}

export const useLogsStore = defineStore('logs', () => {
  // 状态
  const logs = ref<LogEntry[]>([])
  const maxLogs = ref(1000)
  const autoScroll = ref(true)
  const filter = ref({
    level: '' as string,
    category: '' as string,
    nodeId: '' as string,
    search: ''
  })

  // 计算属性
  const filteredLogs = computed(() => {
    return logs.value.filter(log => {
      if (filter.value.level && log.level !== filter.value.level) {
        return false
      }
      if (filter.value.category && log.category !== filter.value.category) {
        return false
      }
      if (filter.value.nodeId && log.node_id !== filter.value.nodeId) {
        return false
      }
      if (filter.value.search) {
        const search = filter.value.search.toLowerCase()
        return log.message.toLowerCase().includes(search) ||
               log.category.toLowerCase().includes(search) ||
               log.node_name.toLowerCase().includes(search)
      }
      return true
    })
  })

  const categories = computed(() => {
    const cats = new Set<string>()
    logs.value.forEach(log => cats.add(log.category))
    return Array.from(cats)
  })

  // 方法
  function addLog(log: LogEntry) {
    logs.value.push(log)
    
    // 限制日志数量
    if (logs.value.length > maxLogs.value) {
      logs.value = logs.value.slice(-maxLogs.value)
    }
  }

  async function fetchLogs(limit = 500) {
    try {
      logs.value = await window.go.main.App.GetLogs(limit)
    } catch (e) {
      console.error('Failed to fetch logs:', e)
    }
  }

  async function clearLogs() {
    try {
      await window.go.main.App.ClearLogs()
      logs.value = []
    } catch (e) {
      console.error('Failed to clear logs:', e)
    }
  }

  async function exportLogs(format: 'txt' | 'json' | 'csv') {
    try {
      return await window.go.main.App.ExportLogs(format)
    } catch (e) {
      console.error('Failed to export logs:', e)
      throw e
    }
  }

  function setFilter(key: keyof typeof filter.value, value: string) {
    filter.value[key] = value
  }

  function clearFilter() {
    filter.value = {
      level: '',
      category: '',
      nodeId: '',
      search: ''
    }
  }

  // 初始化事件监听
  function initEventListener() {
    if (typeof window.runtime?.EventsOn === 'function') {
      window.runtime.EventsOn('log:append', (data: LogEntry) => {
        addLog(data)
      })
    }
  }

  return {
    logs,
    maxLogs,
    autoScroll,
    filter,
    filteredLogs,
    categories,
    addLog,
    fetchLogs,
    clearLogs,
    exportLogs,
    setFilter,
    clearFilter,
    initEventListener
  }
})
