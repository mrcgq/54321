<template>
  <div v-if="loading" class="p-8 text-center">加载中...</div>
  <div v-else class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
    <!-- 头部 -->
    <div class="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
      <h3 class="text-lg font-semibold">{{ localNode.name }}</h3>
      <div class="flex gap-2">
        <!-- 移除实时保存，改为手动保存按钮，这是最稳妥的 -->
        <button @click="saveNode" class="btn-primary text-sm">保存配置</button>
      </div>
    </div>
    
    <!-- 表单 (移除 @change="saveNode") -->
    <!-- 我们改回手动保存模式，彻底杜绝输入死循环 -->
    <div class="p-6 space-y-6">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="block text-sm mb-1">名称</label>
          <input v-model="localNode.name" class="input-base" />
        </div>
        <div>
          <label class="block text-sm mb-1">监听地址</label>
          <input v-model="localNode.listen" class="input-base" />
        </div>
        <div>
          <label class="block text-sm mb-1">服务器</label>
          <input v-model="localNode.server" class="input-base" />
        </div>
        <!-- 其他字段省略，逻辑一样... -->
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useNodesStore } from '@/stores/nodes'
import type { NodeConfig } from '@/types'

// 接收 ID
const props = defineProps<{ nodeId: string }>()
const nodesStore = useNodesStore()

const loading = ref(true)
const localNode = ref<NodeConfig>({} as NodeConfig)

// ⚠️ 关键：组件挂载时，自己去后端拿最新的数据
// 不依赖父组件传进来的可能带有响应式污染的数据
onMounted(async () => {
  loading.value = true
  try {
    // 直接调用 Wails 后端 API 获取单个节点数据
    // 注意：需要在 stores/nodes.ts 或 types 中确认 GetNode 可以在 window.go... 中访问
    // 如果 store 里没封装，直接用 store 的 getter 找也行，但要深拷贝
    const node = await window.go.main.App.GetNode(props.nodeId)
    if (node) {
      localNode.value = node // 这是一个纯净的后端数据，没有 Vue 响应式包裹
    }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
})

async function saveNode() {
  // 保存时，把数据发给后端
  await nodesStore.updateNode(localNode.value)
  alert('保存成功') // 简单反馈
}
</script>
