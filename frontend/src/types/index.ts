// ============================================
// 节点相关类型
// ============================================

export interface RoutingRule {
  id: string
  type: string
  match: string
  target: string
}

export interface NodeConfig {
  id: string
  name: string
  listen: string
  server: string
  ip: string
  token: string
  secret_key: string
  fallback_ip: string
  socks5: string
  routing_mode: number
  strategy_mode: number
  dns_mode: number
  enable_sniffing: boolean
  
  // 🚀【核心修复】同步后端的模型变更
  enable_ipv6: boolean
  prefer_ipv6: boolean
  ipv6_only: boolean
  disable_ipv6: boolean

  rules: RoutingRule[]
  status?: string
}

// ============================================
// 应用配置
// ============================================

export interface AppConfig {
  nodes: NodeConfig[]
  auto_start: boolean
  minimize_to_tray: boolean
  theme: 'light' | 'dark' | 'system'
  language: string
  global_dns_mode: number
  tun_interface_name: string
}

// ============================================
// 引擎状态
// ============================================

export interface EngineStatus {
  node_id: string
  status: string
  start_time: string
  pid: number
  xray_pid?: number
  error_message?: string
}

// ============================================
// 日志
// ============================================

export interface LogEntry {
  timestamp: string
  node_id: string
  node_name: string
  level: 'debug' | 'info' | 'warn' | 'error'
  category: string
  message: string
}

// ============================================
// Ping测试
// ============================================

export interface PingResult {
  server: string
  latency: number
  error?: string
}

export interface PingReport {
  node_id: string
  node_name: string
  start_time: string
  end_time: string
  duration: number
  total_count: number
  success_count: number
  fail_count: number
  avg_latency: number
  min_latency: number
  max_latency: number
  results: PingResult[]
}

// ============================================
// DNS相关
// ============================================

export interface DNSMode {
  value: number
  label: string
  description: string
  recommended: boolean
}

export interface DNSLeakResult {
  leaked: boolean
  tested_at: string
  local_dns: string[]
  detected_dns: DNSServerInfo[]
  conclusion: string
}

export interface DNSServerInfo {
  ip: string
  country: string
  city: string
  isp: string
  is_china: boolean
}

// ============================================
// 常量
// ============================================

export const RoutingModes = [
  { value: 0, label: '全局代理' },
  { value: 1, label: '智能分流' }
] as const

export const StrategyModes = [
  { value: 0, label: '随机 (Random)' },
  { value: 1, label: '轮询 (Round Robin)' },
  { value: 2, label: '哈希 (Hash)' }
] as const

export const DNSModes = [
  { value: 0, label: '标准模式', desc: '可能泄露DNS' },
  { value: 1, label: 'Fake-IP 模式', desc: '推荐，防泄露' },
  { value: 2, label: 'TUN 全局', desc: '需管理员权限' }
] as const

export const RuleTypes = [
  { value: '', label: '关键词 (Keyword)' },
  { value: 'domain:', label: '精准域名 (Domain)' },
  { value: 'regexp:', label: '正则 (Regexp)' },
  { value: 'geosite:', label: 'Geosite' },
  { value: 'geoip:', label: 'GeoIP' }
] as const

export const NodeStatus = {
  STOPPED: 'stopped',
  STARTING: 'starting',
  RUNNING: 'running',
  ERROR: 'error'
} as const
