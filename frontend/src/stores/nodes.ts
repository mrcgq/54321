import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { NodeConfig, EngineStatus } from '@/types'

// Wails 绑定声明
declare const window: any

export const useNodesStore = defineStore('nodes', () => {
  const nodes = ref<NodeConfig[]>([])
  const currentNodeId = ref<string | null>(null)
  const statuses = ref<Record<string, EngineStatus>>({})
  const isLoading = ref(false)
  const error = ref<string | null>(null)

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

  async function fetchNodes() {
    isLoading.value = true
    try {
      nodes.value = await window.go.main.App.GetNodes()
      await fetchStatuses()
      if (!currentNodeId.value && nodes.value.length > 0) {
        currentNodeId.value = nodes.value[0].id
      }
    } catch (e: any) {
      console.error(e)
    } finally {
      isLoading.value = false
    }
  }

  async function fetchStatuses() {
    try {
      statuses.value = await window.go.main.App.GetAllNodeStatuses()
    } catch (e) {}
  }

  function selectNode(id: string) {
    currentNodeId.value = id
  }

  async function addNode(name: string) {
    const node = await window.go.main.App.AddNode(name)
    await fetchNodes()
    currentNodeId.value = node.id
    return node
  }

  // ⚠️【核心修复】
  async function updateNode(node: NodeConfig) {
    // 1. 告诉后端保存
    await window.go.main.App.UpdateNode(node)
    
    // 2. 手动、安全地更新前端 Pinia Store 的状态
    // 这样可以确保侧边栏等其他组件能看到最新的名称等信息
    const index = nodes.value.findIndex(n => n.id === node.id)
    if (index !== -1) {
        // 使用 Object.assign 只更新属性，不替换整个对象引用，
        // 这样做最安全，不会触发不必要的 Vue 响应式重绘。
        Object.assign(nodes.value[index], node)
    }
  }

  async function deleteNode(id: string) {
    await window.go.main.App.DeleteNode(id)
    await fetchNodes()
    if (currentNodeId.value === id) currentNodeId.value = null
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
    if (count > 0) await fetchNodes()
    return count
  }
  
  async function addRule(nodeId: string, rule: any) {
    await window.go.main.App.AddRule(nodeId, rule);
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
