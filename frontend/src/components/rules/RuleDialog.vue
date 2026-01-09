<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" @click.self="$emit('close')">
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md mx-4 overflow-hidden">
      <!-- 标题 -->
      <div class="px-6 py-4 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
        <h3 class="text-lg font-semibold text-gray-800 dark:text-white">
          {{ isEdit ? '编辑规则' : '添加规则' }}
        </h3>
        <button @click="$emit('close')" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      
      <!-- 表单 -->
      <form @submit.prevent="handleSubmit" class="p-6 space-y-4">
        <!-- 类型选择 -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            匹配类型
          </label>
          <select v-model="form.type" class="input-base">
            <option value="">关键词 (Keyword)</option>
            <option value="domain:">精准域名 (Domain)</option>
            <option value="regexp:">正则表达式 (Regexp)</option>
            <option value="geosite:">Geosite (域名分类)</option>
            <option value="geoip:">GeoIP (IP地理位置)</option>
          </select>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ getTypeHint(form.type) }}
          </p>
        </div>
        
        <!-- 匹配内容 -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            匹配内容
          </label>
          <input
            v-model="form.match"
            type="text"
            class="input-base"
            :placeholder="getMatchPlaceholder(form.type)"
            required
          />
        </div>
        
        <!-- 目标节点 -->
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            目标节点
          </label>
          <div class="space-y-2">
            <div class="flex gap-2">
              <button
                type="button"
                @click="form.target = 'direct'"
                :class="[
                  'flex-1 py-2 px-3 rounded-lg border text-sm font-medium transition-colors',
                  form.target === 'direct'
                    ? 'border-green-500 bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                    : 'border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700'
                ]"
              >
                ⬅ 直连
              </button>
              <button
                type="button"
                @click="form.target = 'block'"
                :class="[
                  'flex-1 py-2 px-3 rounded-lg border text-sm font-medium transition-colors',
                  form.target === 'block'
                    ? 'border-red-500 bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                    : 'border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700'
                ]"
              >
                ⛔ 拦截
              </button>
              <button
                type="button"
                @click="form.target = ''"
                :class="[
                  'flex-1 py-2 px-3 rounded-lg border text-sm font-medium transition-colors',
                  form.target !== 'direct' && form.target !== 'block'
                    ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                    : 'border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700'
                ]"
              >
                ➡ 代理
              </button>
            </div>
            
            <input
              v-if="form.target !== 'direct' && form.target !== 'block'"
              v-model="form.target"
              type="text"
              class="input-base"
              placeholder="如: us-node:443"
              required
            />
          </div>
        </div>
        
        <!-- 预设规则 -->
        <div v-if="!isEdit">
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            快速添加预设
          </label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in presets"
              :key="preset.match"
              type="button"
              @click="applyPreset(preset)"
              class="text-xs px-2 py-1 rounded bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors"
            >
              {{ preset.label }}
            </button>
          </div>
        </div>
        
        <!-- 按钮 -->
        <div class="flex gap-3 pt-4">
          <button type="button" @click="$emit('close')" class="flex-1 btn-secondary">
            取消
          </button>
          <button type="submit" class="flex-1 btn-primary">
            {{ isEdit ? '保存' : '添加' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import type { RoutingRule } from '@/types'

const props = defineProps<{
  rule?: RoutingRule | null
}>()

const emit = defineEmits<{
  save: [rule: RoutingRule]
  close: []
}>()

const isEdit = computed(() => !!props.rule?.id)

const form = ref({
  id: '',
  type: '',
  match: '',
  target: ''
})

// 预设规则
const presets = [
  { label: 'Google', type: 'geosite:', match: 'google', target: 'proxy' },
  { label: 'YouTube', type: 'geosite:', match: 'youtube', target: 'proxy' },
  { label: 'Twitter', type: 'geosite:', match: 'twitter', target: 'proxy' },
  { label: 'Telegram', type: 'geosite:', match: 'telegram', target: 'proxy' },
  { label: '广告拦截', type: 'geosite:', match: 'category-ads-all', target: 'block' },
  { label: '中国直连', type: 'geosite:', match: 'cn', target: 'direct' },
]

onMounted(() => {
  if (props.rule) {
    form.value = { ...props.rule }
  }
})

function getTypeHint(type: string): string {
  const hints: Record<string, string> = {
    '': '匹配包含指定关键词的域名',
    'domain:': '精确匹配域名及其子域名',
    'regexp:': '使用正则表达式匹配',
    'geosite:': '使用 Xray 的域名分类数据库',
    'geoip:': '根据 IP 地理位置匹配'
  }
  return hints[type] || ''
}

function getMatchPlaceholder(type: string): string {
  const placeholders: Record<string, string> = {
    '': '如: youtube, google',
    'domain:': '如: google.com, youtube.com',
    'regexp:': '如: .*\\.google\\..*',
    'geosite:': '如: google, youtube, cn',
    'geoip:': '如: cn, us, jp'
  }
  return placeholders[type] || ''
}

function applyPreset(preset: typeof presets[0]) {
  form.value.type = preset.type
  form.value.match = preset.match
  form.value.target = preset.target
}

function handleSubmit() {
  if (!form.value.match.trim()) {
    return
  }
  
  if (!form.value.target.trim() && form.value.target !== 'direct' && form.value.target !== 'block') {
    return
  }
  
  emit('save', {
    id: form.value.id || '',
    type: form.value.type,
    match: form.value.match.trim(),
    target: form.value.target.trim()
  })
}
</script>
