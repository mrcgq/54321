import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { NodeConfig, EngineStatus } from '@/types'

// Wails 绑定
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
      
      if (!currentNodeId.value && nodes.value.length > 0) {
        currentNodeId.value = nodes.value[0].id
      }
    } catch (e: any) {
      error.value = e.message || '加载节点失败'
    } finally {
      isLoading.value = false
    }
  }

  async function fetchStatuses() {
    try {
      statuses.value = await window.go.main.App.GetAllNodeStatuses()
    } catch (e) {
      // ignore
    }
  }

  function selectNode(id: string) {
    currentNodeId.value = id
  }

  async function addNode(name: string = '新节点') {
    const node = await window.go.main.App.AddNode(name)
    // 对于增删操作，需要重新获取列表
    await fetchNodes()
    currentNodeId.value = node.id
    return node
  }

  async function updateNode(node: NodeConfig) {
    // 🛑【核心修复】只调用后端，不修改本地 state
    // 修改本地 state 会导致无限循环
    await window.go.main.App.UpdateNode(node)
    
    // 【可选优化】可以手动更新单个节点的属性，但不替换整个对象
    const index = nodes.value.findIndex(n => n.id === node.id)
    if (index !== -1) {
        // 使用 Object.assign 保持引用不变，只更新属性
        Object.assign(nodes.value[index], node)
    }
  }

  async function deleteNode(id: string) {
    await window.go.main.App.DeleteNode(id)
    // 重新获取列表
    await fetchNodes()
    if (currentNodeId.value === id) {
      currentNodeId.value = nodes.value[0]?.id || null
    }
  }

  async function duplicateNode(id: string) {
    const node = await window.go.main.App.DuplicateNode(id)
    // 重新获取列表
    await fetchNodes()
    currentNodeId.value = node.id
    return node
  }

  async function startNode(id: string) {
    await window.go.main.App.StartNode(id)
    updateNodeStatus(id, 'starting')
  }

  async function stopNode(id: string) {
    await window.go.main.App.StopNode(id)
    updateNodeStatus(id, 'stopped')
  }

  async function startAllNodes() {
    await window.go.main.App.StartAllNodes()
    await fetchStatuses()
  }

  async function stopAllNodes() {
    await window.go.main.App.StopAllNodes()
    await fetchStatuses()
  }

  async function pingTest(id: string) {
    await window.go.main.App.PingTest(id)
  }

  function updateNodeStatus(id: string, status: string) {
    if (statuses.value[id]) {
      statuses.value[id].status = status
    } else {
      statuses.value[id] = { node_id: id, status, start_time: '', pid: 0 }
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
    await window.go.main.App.AddRule(nodeId, rule);
    // 重新获取数据以更新
    await fetchNodes();
  }
  
  async function updateRule(nodeId: string, rule: any) {
    await window.go.main.App.UpdateRule(nodeId, rule);
    await fetchNodes();
  }
  
  async function deleteRule(nodeId: string, ruleId: string) {
    await window.go.main.App.DeleteRule(nodeId, ruleId);
    await fetchNodes();
  }


  return {
    nodes, currentNodeId, statuses, isLoading, error,
    currentNode, runningNodes, hasRunningNodes,
    fetchNodes, fetchStatuses, selectNode, addNode, updateNode,
    deleteNode, duplicateNode, startNode, stopNode, startAllNodes,
    stopAllNodes, pingTest, updateNodeStatus, getNodeStatus,
    exportNode, importNodes, addRule, updateRule, deleteRule
  }
})
