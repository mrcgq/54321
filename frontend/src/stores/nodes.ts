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
      
      // 如果没有选中节点，选中第一个
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
    // 增删操作必须刷新列表，因为列表长度变了
    await fetchNodes()
    currentNodeId.value = node.id
    return node
  }

  // ⚠️【核心修复】updateNode 只负责向后端发送保存请求
  // 绝对不要在这里修改 nodes.value，也绝对不要调用 fetchNodes
  // 这样就切断了前端死循环的源头
  async function updateNode(node: NodeConfig) {
    await window.go.main.App.UpdateNode(node)
    // 结束。不做任何其他操作。
  }

  async function deleteNode(id: string) {
    await window.go.main.App.DeleteNode(id)
    await fetchNodes()
    if (currentNodeId.value === id) {
      currentNodeId.value = nodes.value[0]?.id || null
    }
  }

  async function duplicateNode(id: string) {
    const node = await window.go.main.App.DuplicateNode(id)
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
    // 规则变动不频繁，允许刷新
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
