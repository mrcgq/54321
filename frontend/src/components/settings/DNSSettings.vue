<template>
  <div class="space-y-6">
    <!-- DNS 模式选择 -->
    <div>
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">DNS 防泄露模式</h4>
      
      <div class="space-y-3">
        <label
          v-for="mode in dnsModes"
          :key="mode.value"
          :class="[
            'flex items-start p-4 rounded-lg border cursor-pointer transition-all',
            selectedMode === mode.value
              ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20'
              : 'border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600'
          ]"
        >
          <input
            type="radio"
            :value="mode.value"
            v-model="selectedMode"
            class="mt-0.5 h-4 w-4 text-primary-600 focus:ring-primary-500"
          />
          <div class="ml-3">
            <div class="flex items-center gap-2">
              <span class="font-medium text-gray-800 dark:text-white">{{ mode.label }}</span>
              <span v-if="mode.recommended" class="text-xs px-1.5 py-0.5 rounded bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400">
                推荐
              </span>
            </div>
            <p class="text-sm text-gray-500 dark:text-gray-400 mt-1">{{ mode.description }}</p>
          </div>
        </label>
      </div>
    </div>
    
    <!-- 高级选项 -->
    <div>
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">高级选项</h4>
      
      <div class="space-y-4">
        <label class="flex items-center gap-3">
          <input
            type="checkbox"
            v-model="enableSniffing"
            class="h-4 w-4 text-primary-600 rounded focus:ring-primary-500"
          />
          <div>
            <span class="text-sm text-gray-700 dark:text-gray-300">启用流量嗅探</span>
            <p class="text-xs text-gray-500 dark:text-gray-400">从TLS/HTTP流量中提取真实域名</p>
          </div>
        </label>
        
        <label class="flex items-center gap-3">
          <input
            type="checkbox"
            v-model="blockAds"
            class="h-4 w-4 text-primary-600 rounded focus:ring-primary-500"
          />
          <div>
            <span class="text-sm text-gray-700 dark:text-gray-300">广告拦截</span>
            <p class="text-xs text-gray-500 dark:text-gray-400">使用 geosite 数据库拦截广告域名</p>
          </div>
        </label>
      </div>
    </div>
    
    <!-- DNS 泄露测试 -->
    <div>
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">泄露检测</h4>
      
      <div class="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
        <div v-if="!testResult" class="text-center">
          <button
            @click="runLeakTest"
            :disabled="isTesting"
            class="btn-primary"
          >
            <span v-if="isTesting" class="flex items-center gap-2">
              <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              检测中...
            </span>
            <span v-else>开始 DNS 泄露测试</span>
          </button>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-2">
            检测您的DNS请求是否被正确代理
          </p>
        </div>
        
        <div v-else>
          <div :class="['text-center p-4 rounded-lg', testResult.leaked ? 'bg-red-50 dark:bg-red-900/20' : 'bg-green-50 dark:bg-green-900/20']">
            <div :class="['text-3xl mb-2', testResult.leaked ? 'text-red-500' : 'text-green-500']">
              {{ testResult.leaked ? '⚠️' : '✓' }}
            </div>
            <p :class="['font-medium', testResult.leaked ? 'text-red-700 dark:text-red-400' : 'text-green-700 dark:text-green-400']">
              {{ testResult.conclusion }}
            </p>
          </div>
          
          <div v-if="testResult.detected_dns?.length" class="mt-4">
            <p class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">检测到的DNS服务器:</p>
            <div class="space-y-2">
              <div
                v-for="dns in testResult.detected_dns"
                :key="dns.ip"
                class="flex items-center justify-between text-sm p-2 bg-white dark:bg-gray-700 rounded"
              >
                <span class="font-mono">{{ dns.ip }}</span>
                <span :class="dns.is_china ? 'text-red-500' : 'text-green-500'">
                  {{ dns.country }} {{ dns.isp }}
                </span>
              </div>
            </div>
          </div>
          
          <button
            @click="testResult = null"
            class="mt-4 w-full btn-secondary"
          >
            重新测试
          </button>
        </div>
      </div>
    </div>
    
    <!-- TUN 模式状态 -->
    <div v-if="selectedMode === 2">
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">TUN 模式状态</h4>
      
      <div :class="['p-4 rounded-lg', tunSupported ? 'bg-green-50 dark:bg-green-900/20' : 'bg-yellow-50 dark:bg-yellow-900/20']">
        <div class="flex items-start gap-3">
          <div :class="['text-2xl', tunSupported ? 'text-green-500' : 'text-yellow-500']">
            {{ tunSupported ? '✓' : '⚠️' }}
          </div>
          <div>
            <p :class="['font-medium', tunSupported ? 'text-green-700 dark:text-green-400' : 'text-yellow-700 dark:text-yellow-400']">
              {{ tunStatus.message }}
            </p>
            <ul class="text-sm text-gray-600 dark:text-gray-400 mt-2 space-y-1">
              <li>管理员权限: {{ tunStatus.is_admin ? '✓ 是' : '✗ 否' }}</li>
              <li>Wintun 驱动: {{ tunStatus.driver_exists ? '✓ 已安装' : '✗ 未安装' }}</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import type { DNSLeakResult } from '@/types'

// Wails 绑定
declare const window: {
  go: {
    main: {
      App: {
        GetDNSModes(): Promise<any[]>
        TestDNSLeak(): Promise<DNSLeakResult>
        IsTUNSupported(): Promise<any>
        UpdateDNSConfig(nodeId: string, mode: number, sniffing: boolean): Promise<void>
      }
    }
  }
}

const props = defineProps<{
  nodeId: string
  initialMode: number
  initialSniffing: boolean
}>()

const emit = defineEmits<{
  change: [mode: number, sniffing: boolean]
}>()

const selectedMode = ref(props.initialMode)
const enableSniffing = ref(props.initialSniffing)
const blockAds = ref(true)
const isTesting = ref(false)
const testResult = ref<DNSLeakResult | null>(null)
const tunStatus = ref<any>({})

const dnsModes = ref([
  { value: 0, label: '标准模式', description: '使用系统默认DNS，可能导致DNS泄露', recommended: false },
  { value: 1, label: 'Fake-IP 模式', description: '本地返回虚假IP，域名通过代理解析，有效防止DNS泄露', recommended: true },
  { value: 2, label: 'TUN 全局接管', description: '创建虚拟网卡接管所有流量，需要管理员权限', recommended: false }
])

const tunSupported = computed(() => tunStatus.value?.supported === true)

onMounted(async () => {
  try {
    // 获取TUN状态
    tunStatus.value = await window.go.main.App.IsTUNSupported()
  } catch (e) {
    console.error('Failed to get TUN status:', e)
  }
})

// 监听变化
import { watch } from 'vue'

watch([selectedMode, enableSniffing], ([mode, sniffing]) => {
  emit('change', mode, sniffing)
})

async function runLeakTest() {
  isTesting.value = true
  testResult.value = null
  
  try {
    testResult.value = await window.go.main.App.TestDNSLeak()
  } catch (e: any) {
    testResult.value = {
      leaked: true,
      tested_at: new Date().toISOString(),
      local_dns: [],
      detected_dns: [],
      conclusion: '测试失败: ' + (e.message || '未知错误')
    }
  } finally {
    isTesting.value = false
  }
}
</script>
