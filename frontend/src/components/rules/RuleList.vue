<template>
  <div class="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
    <!-- 表头 -->
    <div class="bg-gray-50 dark:bg-gray-700/50 px-4 py-2 grid grid-cols-12 gap-4 text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
      <div class="col-span-2">类型</div>
      <div class="col-span-5">匹配内容</div>
      <div class="col-span-4">目标节点</div>
      <div class="col-span-1 text-right">操作</div>
    </div>
    
    <!-- 规则列表 -->
    <div class="divide-y divide-gray-200 dark:divide-gray-700 max-h-48 overflow-y-auto">
      <div
        v-for="rule in rules"
        :key="rule.id"
        class="px-4 py-2.5 grid grid-cols-12 gap-4 items-center hover:bg-gray-50 dark:hover:bg-gray-700/30 transition-colors group"
      >
        <!-- 类型 -->
        <div class="col-span-2">
          <span :class="['inline-flex items-center px-2 py-0.5 rounded text-xs font-medium', getTypeStyle(rule.type)]">
            {{ getTypeLabel(rule.type) }}
          </span>
        </div>
        
        <!-- 匹配内容 -->
        <div class="col-span-5 font-mono text-sm text-gray-700 dark:text-gray-300 truncate" :title="rule.match">
          {{ rule.match }}
        </div>
        
        <!-- 目标 -->
        <div class="col-span-4 font-mono text-sm truncate" :title="rule.target">
          <span :class="getTargetStyle(rule.target)">
            {{ formatTarget(rule.target) }}
          </span>
        </div>
        
        <!-- 操作 -->
        <div class="col-span-1 flex justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
          <button
            @click="$emit('edit', rule)"
            class="p-1 text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 rounded"
            title="编辑"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
          </button>
          <button
            @click="$emit('delete', rule.id)"
            class="p-1 text-gray-400 hover:text-red-600 dark:hover:text-red-400 rounded"
            title="删除"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </button>
        </div>
      </div>
      
      <!-- 空状态 -->
      <div v-if="rules.length === 0" class="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
        <svg class="w-12 h-12 mx-auto mb-3 text-gray-300 dark:text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
        </svg>
        <p class="text-sm">暂无分流规则</p>
        <p class="text-xs mt-1 text-gray-400">点击"添加规则"创建第一条规则</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { RoutingRule } from '@/types'

defineProps<{
  rules: RoutingRule[]
}>()

defineEmits<{
  edit: [rule: RoutingRule]
  delete: [ruleId: string]
}>()

function getTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    '': '关键词',
    'domain:': '域名',
    'regexp:': '正则',
    'geosite:': 'Geosite',
    'geoip:': 'GeoIP'
  }
  return labels[type] || type
}

function getTypeStyle(type: string): string {
  const styles: Record<string, string> = {
    '': 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
    'domain:': 'bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300',
    'regexp:': 'bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300',
    'geosite:': 'bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300',
    'geoip:': 'bg-orange-100 text-orange-700 dark:bg-orange-900/50 dark:text-orange-300'
  }
  return styles[type] || styles['']
}

function getTargetStyle(target: string): string {
  const lowerTarget = target.toLowerCase()
  if (lowerTarget.includes('direct')) {
    return 'text-green-600 dark:text-green-400'
  }
  if (lowerTarget.includes('block')) {
    return 'text-red-600 dark:text-red-400'
  }
  return 'text-blue-600 dark:text-blue-400'
}

function formatTarget(target: string): string {
  const lowerTarget = target.toLowerCase()
  if (lowerTarget.includes('direct')) {
    return '⬅ 直连'
  }
  if (lowerTarget.includes('block')) {
    return '⛔ 拦截'
  }
  return '➡ ' + target
}
</script>
