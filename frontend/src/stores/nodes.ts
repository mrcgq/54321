import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { NodeConfig, EngineStatus } from '@/types'

// Wails 绑定（将在运行时可用）
declare const window: {
  go: {
    main: {
      App: {
        GetNodes(): Promise<NodeConfig[]>
        GetNode(id: string): Promise<NodeConfig | null>
        AddNode(name: string): Promise<NodeConfig>
        UpdateNode(node: NodeConfig): Promise<void>
        DeleteNode(id: string): Promise<void>
        DuplicateNode(id: string): Promise<NodeConfig>
        StartNode(id: string): Promise<void>
        StopNode(id: string): Promise<void>
        StartAllNodes(): Promise<void>
        StopAllNodes(): Promise<void>
        PingTest(id: string): Promise<void>
        GetAllNodeStatuses(): Promise<Record<string, EngineStatus>>
        AddRule(nodeId: string, rule: any): Promise<void>
        UpdateRule(nodeId: string, rule: any): Promise<void>
        DeleteRule(nodeId: string, ruleId: string): Promise<void>
        ExportToClipboard(id: string): Promise<void>
        ImportFromClipboard(): Promise<number>
      }
    }
  }
}

export const useNodesStore = defineStore('nodes', () => {
  // 状态
  const nodes = ref<NodeConfig[]>([])
  const currentNodeId = ref<string | null>(null)
  const statuses = ref<Record<string, EngineStatus>>({})
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  // 计算属性
  const currentNode = computed(() => {
    if (!currentNodeId.value) return null
    return nodes.value.find(n => n.id === currentNodeId.value) || null
  })

  const runningNodes = computed(() => {
    return nodes.value.filter(n => {
      const status = statuses.value[n.id]
      return status?.status === 'running'
    })
  })

  const hasRunningNodes = computed(() => runningNodes.value.length > 0)

  // 方法
  async function fetchNodes() {
    isLoading.value = true
    error.value = null
    
    try {
      nodes.value = await window.go.main.App.GetNodes()
      await fetchStatuses()
      
      // 如果没有选中节点，选中第一个
      if (!currentNodeId.value && nodes.value.length > 0) {
        currentNodeId.value = nodes.value[0].id
      }
    } catch (e: any) {
      error.value = e.message || '加载节点失败'
      console.error('Failed to fetch nodes:', e)
    } finally {
      isLoading.value = false
    }
  }

  async function fetchStatuses() {
    try {
      statuses.value = await window.go.main.App.GetAllNodeStatuses()
    } catch (e) {
      console.error('Failed to fetch statuses:', e)
    }
  }

  function selectNode(id: string) {
    currentNodeId.value = id
  }

  async function addNode(name: string = '新节点') {
    try {
      const node = await window.go.main.App.AddNode(name)
      nodes.value.push(node)
      currentNodeId.value = node.id
      return node
    } catch (e: any) {
      error.value = e.message
      throw e
    }
  }

  async function updateNode(node: NodeConfig) {
    try {
      await window.go.main.App.UpdateNode(node)
      const index = nodes.value.findIndex(n => n.id === node.id)
      if (index !== -1) {
        nodes.value[index] = { ...node }
      }
    } catch (e: any) {
      error.value = e.message
      throw e
    }
  }

  async function deleteNode(id: string) {
    try {
      await window.go.main.App.DeleteNode(id)
      const index = nodes.value.findIndex(n => n.id === id)
      if (index !== -1) {
        nodes.value.splice(index, 1)
      }
      
      // 如果删除的是当前节点，选中第一个
      if (currentNodeId.value === id) {
        currentNodeId.value = nodes.value[0]?.id || null
      }
    } catch (e: any) {
      error.value = e.message
      throw e
    }
  }

  async function duplicateNode(id: string) {
    try {
      const node = await window.go.main.App.DuplicateNode(id)
      nodes.value.push(node)
      currentNodeId.value = node.id
      return node
    } catch (e: any) {
      error.value = e.message
      throw e
    }
  }

  async function startNode(id: string) {
    try {
      await window.go.main.App.StartNode(id)
      updateNodeStatus(id, 'running')
    } catch (e: any) {
      updateNodeStatus(id, 'error')
      throw e
    }
  }

  async function stopNode(id: string) {
    try {
      await window.go.main.App.StopNode(id)
      updateNodeStatus(id, 'stopped')
    } catch (e: any) {
      throw e
    }
  }

  async function startAllNodes() {
    try {
      await window.go.main.App.StartAllNodes()
      await fetchStatuses()
    } catch (e: any) {
      throw e
    }
  }

  async function stopAllNodes() {
    try {
      await window.go.main.App.StopAllNodes()
      await fetchStatuses()
    } catch (e: any) {
      throw e
    }
  }

  async function pingTest(id: string) {
    try {
      await window.go.main.App.PingTest(id)
    } catch (e: any) {
      throw e
    }
  }

  function updateNodeStatus(id: string, status: string) {
    if (statuses.value[id]) {
      statuses.value[id].status = status
    } else {
      statuses.value[id] = {
        node_id: id,
        status,
        start_time: '',
        pid: 0
      }
    }
  }

  function getNodeStatus(id: string): string {
    return statuses.value[id]?.status || 'stopped'
  }

  async function exportNode(id: string) {
    await window.go.main.App.ExportToClipboard(id)
  }

  async function importNodes() {
    const count = await window.go.main.App.ImportFromClipboard()
    if (count > 0) {
      await fetchNodes()
    }
    return count
  }

  // 规则操作
  async function addRule(nodeId: string, rule: any) {
    await window.go.main.App.AddRule(nodeId, rule)
    await fetchNodes()
  }

  async function updateRule(nodeId: string, rule: any) {
    await window.go.main.App.UpdateRule(nodeId, rule)
    await fetchNodes()
  }

  async function deleteRule(nodeId: string, ruleId: string) {
    await window.go.main.App.DeleteRule(nodeId, ruleId)
    await fetchNodes()
  }

  return {
    // 状态
    nodes,
    currentNodeId,
    statuses,
    isLoading,
    error,
    // 计算属性
    currentNode,
    runningNodes,
    hasRunningNodes,
    // 方法
    fetchNodes,
    fetchStatuses,
    selectNode,
    addNode,
    updateNode,
    deleteNode,
    duplicateNode,
    startNode,
    stopNode,
    startAllNodes,
    stopAllNodes,
    pingTest,
    updateNodeStatus,
    getNodeStatus,
    exportNode,
    importNodes,
    addRule,
    updateRule,
    deleteRule
  }
})
