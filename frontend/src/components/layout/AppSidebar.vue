<template>
  <aside class="w-64 bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 flex flex-col shrink-0">
    <!-- 标题和操作 -->
    <div class="p-4 border-b border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between mb-3">
        <h2 class="font-semibold text-gray-800 dark:text-white">节点列表</h2>
        <span class="text-xs text-gray-500">{{ nodes.length }}/50</span>
      </div>
      
      <div class="flex gap-2">
        <button @click="addNode" class="flex-1 btn-primary text-sm py-1.5">
          + 新建
        </button>
        <button @click="importNodes" class="flex-1 btn-secondary text-sm py-1.5">
          导入
        </button>
      </div>
    </div>
    
    <!-- 节点列表 -->
    <div class="flex-1 overflow-y-auto">
      <div
        v-for="node in nodes"
        :key="node.id"
        @click="selectNode(node.id)"
        :class="[
          'px-4 py-3 cursor-pointer border-b border-gray-100 dark:border-gray-700',
          'hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors',
          node.id === currentNodeId && 'bg-primary-50 dark:bg-primary-900/30 border-l-2 border-l-primary-500'
        ]"
      >
        <div class="flex items-center gap-3">
          <!-- 状态指示 -->
          <div :class="['status-dot', getNodeStatus(node.id)]"></div>
          
          <!-- 节点信息 -->
          <div class="flex-1 min-w-0">
            <div class="font-medium text-gray-800 dark:text-white truncate">
              {{ node.name }}
            </div>
            <div class="text-xs text-gray-500 dark:text-gray-400 truncate">
              {{ node.listen }}
            </div>
          </div>
          
          <!-- 快捷操作 -->
          <div class="flex items-center gap-1 opacity-0 group-hover:opacity-100">
            <button
              v-if="getNodeStatus(node.id) !== 'running'"
              @click.stop="startNode(node.id)"
              class="p-1 hover:bg-green-100 dark:hover:bg-green-900/30 rounded text-green-600"
              title="启动"
            >
              <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM9.555 7.168A1 1 0 008 8v4a1 1 0 001.555.832l3-2a1 1 0 000-1.664l-3-2z" clip-rule="evenodd" />
              </svg>
            </button>
            <button
              v-else
              @click.stop="stopNode(node.id)"
              class="p-1 hover:bg-red-100 dark:hover:bg-red-900/30 rounded text-red-600"
              title="停止"
            >
              <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8 7a1 1 0 00-1 1v4a1 1 0 001 1h4a1 1 0 001-1V8a1 1 0 00-1-1H8z" clip-rule="evenodd" />
              </svg>
            </button>
          </div>
        </div>
      </div>
      
      <!-- 空状态 -->
      <div v-if="nodes.length === 0" class="p-8 text-center text-gray-500">
        <p>暂无节点</p>
        <p class="text-sm mt-1">点击"新建"创建节点</p>
      </div>
    </div>
    
    <!-- 底部操作 -->
    <div class="p-4 border-t border-gray-200 dark:border-gray-700 space-y-2">
      <button
        @click="pingTest"
        :disabled="!currentNodeId"
        class="w-full btn-secondary text-sm py-1.5 flex items-center justify-center gap-2"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
        延迟测速
      </button>
      
      <div class="flex gap-2">
        <button
          @click="duplicateNode"
          :disabled="!currentNodeId"
          class="flex-1 btn-secondary text-sm py-1.5"
          title="复制节点"
        >
          复制
        </button>
        <button
          @click="deleteNode"
          :disabled="!currentNodeId"
          class="flex-1 btn-danger text-sm py-1.5"
          title="删除节点"
        >
          删除
        </button>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'

const appStore = useAppStore()
const nodesStore = useNodesStore()

const nodes = computed(() => nodesStore.nodes)
const currentNodeId = computed(() => nodesStore.currentNodeId)

function getNodeStatus(id: string) {
  return nodesStore.getNodeStatus(id)
}

function selectNode(id: string) {
  nodesStore.selectNode(id)
}

async function addNode() {
  try {
    const name = prompt('请输入节点名称:', '新节点')
    if (name) {
      await nodesStore.addNode(name)
      appStore.showToast('success', '节点已创建')
    }
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function duplicateNode() {
  if (!currentNodeId.value) return
  
  try {
    await nodesStore.duplicateNode(currentNodeId.value)
    appStore.showToast('success', '节点已复制')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function deleteNode() {
  if (!currentNodeId.value) return
  
  const node = nodesStore.currentNode
  if (!confirm(`确定要删除节点 "${node?.name}" 吗？`)) return
  
  try {
    await nodesStore.deleteNode(currentNodeId.value)
    appStore.showToast('success', '节点已删除')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function startNode(id: string) {
  try {
    await nodesStore.startNode(id)
    appStore.showToast('success', '节点已启动')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function stopNode(id: string) {
  try {
    await nodesStore.stopNode(id)
    appStore.showToast('success', '节点已停止')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function importNodes() {
  try {
    const count = await nodesStore.importNodes()
    if (count > 0) {
      appStore.showToast('success', `成功导入 ${count} 个节点`)
    } else {
      appStore.showToast('warning', '未找到有效的节点配置')
    }
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function pingTest() {
  if (!currentNodeId.value) return
  
  try {
    await nodesStore.pingTest(currentNodeId.value)
    appStore.showToast('info', '开始测速...')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}
</script>

<style scoped>
/* 鼠标悬停时显示操作按钮 */
aside > div > div:hover .opacity-0 {
  opacity: 1;
}
</style>
