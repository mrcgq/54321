<template>
  <div class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 overflow-hidden">
    <!-- 头部 -->
    <div class="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
      <div class="flex items-center gap-3">
        <div :class="['status-dot', status]"></div>
        <h3 class="text-lg font-semibold text-gray-800 dark:text-white">
          {{ localNode.name }}
        </h3>
        <button @click="editName" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
          </svg>
        </button>
      </div>
      
      <div class="flex items-center gap-2">
        <button @click="exportNode" class="btn-secondary text-sm">
          导出
        </button>
        <button
          v-if="status !== 'running'"
          @click="startNode"
          class="btn-success text-sm"
        >
          启动
        </button>
        <button
          v-else
          @click="stopNode"
          class="btn-danger text-sm"
        >
          停止
        </button>
      </div>
    </div>
    
    <!-- 配置表单 -->
    <div class="p-6 space-y-6 max-h-[calc(100vh-400px)] overflow-y-auto">
      <!-- 基本配置 -->
      <section>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4 flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
          </svg>
          基本配置
        </h4>
        
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">本地监听</label>
            <input
              v-model="localNode.listen"
              type="text"
              class="input-base"
              placeholder="127.0.0.1:10808"
              @change="saveNode"
            />
          </div>
          
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">全局指定 IP</label>
            <input
              v-model="localNode.ip"
              type="text"
              class="input-base"
              placeholder="可选，优先使用此IP"
              @change="saveNode"
            />
          </div>
        </div>
        
        <div class="mt-4">
          <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">
            服务器地址池 <span class="text-gray-400">(多个地址用换行分隔)</span>
          </label>
          <textarea
            v-model="localNode.server"
            rows="3"
            class="input-base font-mono text-sm resize-none"
            placeholder="cdn.worker.dev:443&#10;cdn2.worker.dev:443"
            @change="saveNode"
          ></textarea>
        </div>
        
        <div class="grid grid-cols-2 gap-4 mt-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">Token</label>
            <input
              v-model="localNode.token"
              type="password"
              class="input-base"
              placeholder="认证密码"
              @change="saveNode"
            />
          </div>
          
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">Secret Key</label>
            <input
              v-model="localNode.secret_key"
              type="password"
              class="input-base"
              placeholder="加密密钥"
              @change="saveNode"
            />
          </div>
        </div>
        
        <div class="grid grid-cols-2 gap-4 mt-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">回源 IP</label>
            <input
              v-model="localNode.fallback_ip"
              type="text"
              class="input-base"
              placeholder="可选"
              @change="saveNode"
            />
          </div>
          
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">上游 SOCKS5</label>
            <input
              v-model="localNode.socks5"
              type="text"
              class="input-base"
              placeholder="可选，如 127.0.0.1:1080"
              @change="saveNode"
            />
          </div>
        </div>
      </section>
      
      <!-- 路由配置 -->
      <section>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4 flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
          </svg>
          路由配置
        </h4>
        
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">路由模式</label>
            <select v-model="localNode.routing_mode" class="input-base" @change="saveNode">
              <option :value="0">全局代理</option>
              <option :value="1">智能分流 (需Xray)</option>
            </select>
          </div>
          
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">负载策略</label>
            <select v-model="localNode.strategy_mode" class="input-base" @change="saveNode">
              <option :value="0">随机 (Random)</option>
              <option :value="1">轮询 (Round Robin)</option>
              <option :value="2">哈希 (Hash)</option>
            </select>
          </div>
        </div>
      </section>
      
      <!-- DNS 防泄露 -->
      <section>
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4 flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
          DNS 防泄露
        </h4>
        
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-sm text-gray-600 dark:text-gray-400 mb-1">DNS 模式</label>
            <select v-model="localNode.dns_mode" class="input-base" @change="saveNode">
              <option :value="0">标准模式</option>
              <option :value="1">Fake-IP 模式 (推荐)</option>
              <option :value="2">TUN 全局接管</option>
            </select>
          </div>
          
          <div class="flex items-center">
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                v-model="localNode.enable_sniffing"
                type="checkbox"
                class="w-4 h-4 text-primary-600 rounded focus:ring-primary-500"
                @change="saveNode"
              />
              <span class="text-sm text-gray-600 dark:text-gray-400">启用流量嗅探</span>
            </label>
          </div>
        </div>
        
        <div class="mt-3 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg text-sm text-blue-700 dark:text-blue-300">
          <p v-if="localNode.dns_mode === 0">⚠️ 标准模式可能导致DNS泄露</p>
          <p v-else-if="localNode.dns_mode === 1">✓ Fake-IP模式可有效防止DNS泄露</p>
          <p v-else>🔒 TUN模式提供最高级别的隐私保护（需管理员权限）</p>
        </div>
      </section>
      
      <!-- 分流规则 -->
      <section>
        <div class="flex items-center justify-between mb-4">
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" />
            </svg>
            分流规则
            <span class="text-gray-400 font-normal">({{ localNode.rules?.length || 0 }})</span>
          </h4>
          
          <button @click="showRuleDialog = true" class="btn-primary text-sm py-1 px-3">
            + 添加规则
          </button>
        </div>
        
        <RuleList
          :rules="localNode.rules || []"
          @edit="editRule"
          @delete="deleteRule"
        />
      </section>
    </div>
    
    <!-- 规则编辑对话框 -->
    <RuleDialog
      v-if="showRuleDialog"
      :rule="editingRule"
      @save="saveRule"
      @close="closeRuleDialog"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'
import type { NodeConfig, RoutingRule } from '@/types'
import RuleList from '@/components/rules/RuleList.vue'
import RuleDialog from '@/components/rules/RuleDialog.vue'

const props = defineProps<{
  node: NodeConfig
}>()

const appStore = useAppStore()
const nodesStore = useNodesStore()

// 本地副本
const localNode = ref<NodeConfig>({ ...props.node })

// 规则对话框
const showRuleDialog = ref(false)
const editingRule = ref<RoutingRule | null>(null)

// 状态
const status = computed(() => nodesStore.getNodeStatus(props.node.id))

// 监听 prop 变化
watch(() => props.node, (newNode) => {
  localNode.value = { ...newNode }
}, { deep: true })

// 保存节点
async function saveNode() {
  try {
    await nodesStore.updateNode(localNode.value)
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

// 编辑名称
function editName() {
  const name = prompt('请输入新的节点名称:', localNode.value.name)
  if (name && name !== localNode.value.name) {
    localNode.value.name = name
    saveNode()
  }
}

// 导出节点
async function exportNode() {
  try {
    await nodesStore.exportNode(props.node.id)
    appStore.showToast('success', '已复制到剪贴板')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

// 启动/停止
async function startNode() {
  try {
    await nodesStore.startNode(props.node.id)
    appStore.showToast('success', '节点已启动')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function stopNode() {
  try {
    await nodesStore.stopNode(props.node.id)
    appStore.showToast('success', '节点已停止')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

// 规则操作
function editRule(rule: RoutingRule) {
  editingRule.value = { ...rule }
  showRuleDialog.value = true
}

async function deleteRule(ruleId: string) {
  if (!confirm('确定要删除此规则吗？')) return
  
  try {
    await nodesStore.deleteRule(props.node.id, ruleId)
    appStore.showToast('success', '规则已删除')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

async function saveRule(rule: RoutingRule) {
  try {
    if (editingRule.value?.id) {
      await nodesStore.updateRule(props.node.id, rule)
    } else {
      await nodesStore.addRule(props.node.id, rule)
    }
    closeRuleDialog()
    appStore.showToast('success', '规则已保存')
  } catch (e: any) {
    appStore.showToast('error', e.message)
  }
}

function closeRuleDialog() {
  showRuleDialog.value = false
  editingRule.value = null
}
</script>
