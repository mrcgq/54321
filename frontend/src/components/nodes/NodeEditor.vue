<template>
  <div v-if="loading" class="p-8 text-center text-gray-500">正在加载配置...</div>
  <div v-else class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
    <!-- Header -->
    <div class="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
      <div class="flex items-center gap-3">
        <div :class="['status-dot', status]"></div>
        <h3 class="text-lg font-semibold text-gray-800 dark:text-white">
          {{ localNode.name }}
        </h3>
        <button @click="editName" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" /></svg>
        </button>
      </div>
      
      <div class="flex items-center gap-2">
        <button @click="saveNode" class="btn-primary text-sm">保存配置</button>
        <button @click="exportNode" class="btn-secondary text-sm">导出</button>
        <button v-if="status !== 'running'" @click="startNode" class="btn-success text-sm">启动</button>
        <button v-else @click="stopNode" class="btn-danger text-sm">停止</button>
      </div>
    </div>
    
    <!-- Config Form -->
    <div class="p-6 space-y-6 max-h-[calc(100vh-400px)] overflow-y-auto">
      <section>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">基本配置</h4>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">本地监听</label>
            <input v-model="localNode.listen" type="text" class="input-base" />
          </div>
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">全局指定 IP</label>
            <input v-model="localNode.ip" type="text" class="input-base" />
          </div>
        </div>
        <div class="mt-4">
          <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">服务器地址池</label>
          <textarea v-model="localNode.server" rows="3" class="input-base font-mono text-sm resize-none"></textarea>
        </div>
        <div class="grid grid-cols-2 gap-4 mt-4">
          <div><label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">Token</label><input v-model="localNode.token" type="password" class="input-base" /></div>
          <div><label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">Secret Key</label><input v-model="localNode.secret_key" type="password" class="input-base" /></div>
        </div>
        <div class="grid grid-cols-2 gap-4 mt-4">
          <div><label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">回源 IP</label><input v-model="localNode.fallback_ip" type="text" class="input-base" /></div>
          <div><label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">上游 SOCKS5</label><input v-model="localNode.socks5" type="text" class="input-base" /></div>
        </div>
      </section>
      
      <section>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">路由与策略</h4>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">路由模式</label>
            <select v-model="localNode.routing_mode" class="input-base">
              <option :value="0">全局代理</option>
              <option :value="1">智能分流</option>
            </select>
          </div>
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">负载策略</label>
            <select v-model="localNode.strategy_mode" class="input-base">
              <option :value="0">随机</option>
              <option :value="1">轮询</option>
              <option :value="2">哈希</option>
            </select>
          </div>
        </div>
      </section>

      <section>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">DNS 与网络</h4>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">DNS 模式</label>
            <select v-model="localNode.dns_mode" class="input-base">
              <option :value="0">标准</option>
              <option :value="1">Fake-IP</option>
              <option :value="2">TUN</option>
            </select>
          </div>
          <div class="flex items-center">
            <label class="flex items-center gap-2 cursor-pointer">
              <input v-model="localNode.enable_sniffing" type="checkbox" class="w-4 h-4 text-primary-600 rounded" />
              <span class="text-sm text-gray-600 dark:text-gray-400">启用流量嗅探</span>
            </label>
          </div>
        </div>
        
        <!-- 🚀【核心 UI 新增】 -->
        <div class="mt-4">
          <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">IP 版本偏好</label>
          <select v-model="ipVersion" class="input-base">
            <option value="dual-ipv4">双栈 (IPv4 优先)</option>
            <option value="dual-ipv6">双栈 (IPv6 优先)</option>
            <option value="ipv4-only">仅 IPv4</option>
            <option value="ipv6-only">仅 IPv6</option>
          </select>
        </div>
      </section>

      <section>
        <div class="flex items-center justify-between mb-4">
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">分流规则 ({{ localNode.rules?.length || 0 }})</h4>
          <button @click="showRuleDialog = true" class="btn-primary text-sm py-1 px-3">+ 添加</button>
        </div>
        <RuleList v-if="localNode.rules" :rules="localNode.rules" @edit="editRule" @delete="deleteRule" />
      </section>
    </div>
    
    <RuleDialog v-if="showRuleDialog" :rule="editingRule" @save="saveRule" @close="closeRuleDialog" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'
import type { NodeConfig, RoutingRule } from '@/types'
import RuleList from '@/components/rules/RuleList.vue'
import RuleDialog from '@/components/rules/RuleDialog.vue'

const props = defineProps<{ nodeId: string }>()
const appStore = useAppStore()
const nodesStore = useNodesStore()
declare const window: any

const loading = ref(true)
const localNode = ref<NodeConfig>({} as NodeConfig)
const showRuleDialog = ref(false)
const editingRule = ref<RoutingRule | null>(null)

const status = computed(() => nodesStore.getNodeStatus(props.nodeId))

// 🚀【核心 UI 逻辑】
// 创建一个计算属性来双向绑定 IP 版本设置
const ipVersion = computed({
  get() {
    if (localNode.value.ipv6_only) return 'ipv6-only'
    if (localNode.value.disable_ipv6) return 'ipv4-only'
    if (localNode.value.prefer_ipv6) return 'dual-ipv6'
    return 'dual-ipv4' // 默认
  },
  set(value) {
    switch (value) {
      case 'ipv6-only':
        localNode.value.enable_ipv6 = true
        localNode.value.ipv6_only = true
        localNode.value.disable_ipv6 = false
        localNode.value.prefer_ipv6 = false
        break;
      case 'ipv4-only':
        localNode.value.enable_ipv6 = false
        localNode.value.ipv6_only = false
        localNode.value.disable_ipv6 = true
        localNode.value.prefer_ipv6 = false
        break;
      case 'dual-ipv6':
        localNode.value.enable_ipv6 = true
        localNode.value.ipv6_only = false
        localNode.value.disable_ipv6 = false
        localNode.value.prefer_ipv6 = true
        break;
      default: // dual-ipv4
        localNode.value.enable_ipv6 = true
        localNode.value.ipv6_only = false
        localNode.value.disable_ipv6 = false
        localNode.value.prefer_ipv6 = false
        break;
    }
  }
})

watch(() => props.nodeId, async (newId) => {
  if (newId) await fetchNodeData()
}, { immediate: true })

async function fetchNodeData() {
  loading.value = true
  try {
    const node = await window.go.main.App.GetNode(props.nodeId)
    if (node) {
      localNode.value = node
    }
  } catch (e: any) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

async function saveNode() {
  await nodesStore.updateNode(localNode.value)
  appStore.showToast('success', '已保存')
}

function editName() {
  const name = prompt('新名称:', localNode.value.name)
  if (name) {
    localNode.value.name = name
  }
}

async function exportNode() {
  await nodesStore.exportNode(props.nodeId)
  appStore.showToast('success', '已复制')
}

async function startNode() { 
  await saveNode()
  await nodesStore.startNode(props.nodeId) 
}

async function stopNode() { await nodesStore.stopNode(props.nodeId) }

function editRule(rule: RoutingRule) {
  editingRule.value = { ...rule }
  showRuleDialog.value = true
}

async function deleteRule(ruleId: string) {
  if(!confirm("删除?")) return
  await nodesStore.deleteRule(props.nodeId, ruleId)
  if (localNode.value.rules) {
    localNode.value.rules = localNode.value.rules.filter(r => r.id !== ruleId)
  }
}

async function saveRule(rule: RoutingRule) {
  if (editingRule.value?.id) {
    await nodesStore.updateRule(props.nodeId, rule)
  } else {
    await nodesStore.addRule(props.nodeId, rule)
  }
  closeRuleDialog()
  await fetchNodeData()
}

function closeRuleDialog() {
  showRuleDialog.value = false
  editingRule.value = null
}
</script>
