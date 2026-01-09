#  Wails 客户端

<div align="center">

![Xlink Logo](build/appicon.png)

**一个功能强大的代理客户端，支持智能分流和DNS防泄露**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![Vue Version](https://img.shields.io/badge/Vue-3.4+-4FC08D?style=flat-square&logo=vue.js)](https://vuejs.org/)
[![Wails Version](https://img.shields.io/badge/Wails-2.8+-00ACD7?style=flat-square)](https://wails.io/)
[![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](LICENSE)

</div>

---

## ✨ 功能特性

### 🚀 核心功能
- **多节点管理** - 支持最多50个节点配置
- **智能分流** - 基于域名/IP的路由规则
- **负载均衡** - Random/RR/Hash 三种策略
- **延迟测速** - 快速测试节点连接质量

### 🔒 DNS防泄露
- **Fake-IP模式** - 本地返回虚假IP，远端解析真实域名
- **流量嗅探** - 从TLS/HTTP流量中提取真实域名
- **TUN模式** - 虚拟网卡全局接管（需管理员权限）
- **泄露检测** - 一键检测DNS是否泄露

### 💻 系统集成
- **开机自启** - 支持Windows/macOS/Linux
- **系统托盘** - 最小化到托盘运行
- **系统代理** - 自动配置系统代理设置
- **深色模式** - 跟随系统或手动切换

### 📦 其他功能
- **配置加密** - AES-256-GCM加密存储敏感信息
- **导入导出** - 支持 xlink:// 协议链接
- **实时日志** - 详细的运行日志和过滤功能
- **自动备份** - 配置文件自动备份

---

## 📋 系统要求

| 平台 | 最低版本 | 备注 |
|------|----------|------|
| Windows | Windows 10 1809+ | 需要 WebView2 运行时 |
| macOS | macOS 10.15+ | Intel 和 Apple Silicon |
| Linux | Ubuntu 20.04+ | 需要 WebKitGTK |

---

## 🚀 快速开始

### 下载安装

从 [Releases](https://github.com/xlink/xlink-wails/releases) 页面下载适合您系统的版本。

### 首次运行

1. 解压下载的文件
2. 确保以下文件在同一目录：
   - `xlink-client.exe` (主程序)
   - `xlink-cli-binary.exe` (核心引擎)
   - `xray.exe` (智能分流需要)
   - `geosite.dat` (域名规则库)
   - `geoip.dat` (IP规则库)
   - `wintun.dll` (TUN模式需要, 仅Windows)

3. 双击运行 `xlink-client.exe`

### 基本配置

1. **添加节点**: 点击左侧"新建"按钮
2. **配置服务器**: 填写服务器地址、Token等信息
3. **启动连接**: 点击"启动"按钮
4. **设置代理**: 配置浏览器或系统代理为 `127.0.0.1:10808`

---

## 🛡️ DNS防泄露指南

### 什么是DNS泄露？

当你使用代理时，如果DNS请求没有通过代理发送，而是直接发送给本地ISP的DNS服务器，这就是DNS泄露。泄露会暴露你访问的网站域名。

### 推荐配置

| 场景 | 推荐模式 | 说明 |
|------|----------|------|
| 日常使用 | Fake-IP | 平衡安全性和兼容性 |
| 高隐私需求 | TUN模式 | 完全杜绝泄露 |
| 兼容性优先 | 标准模式 | 可能存在泄露风险 |

### Fake-IP 模式原理
应用请求 google.com
DNS请求被拦截
返回 Fake-IP: 198.18.0.1
应用连接 198.18.0.1:443
代理嗅探 TLS 获取真实域名: google.com
真实域名通过加密隧道发送到远端
远端服务器解析并转发
DNS泄露被完全阻止 ✓



---

## ⌨️ 快捷键

| 快捷键 | 功能 |
|--------|------|
| `Ctrl + N` | 新建节点 |
| `Ctrl + S` | 保存配置 |
| `Ctrl + Q` | 退出程序 |
| `F5` | 刷新节点状态 |
| `Esc` | 关闭对话框 |

---

## 🔧 开发指南

### 环境准备


# 安装 Go 1.21+
# 安装 Node.js 18+

# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 检查环境
wails doctor
克隆项目


git clone https://github.com/xlink/xlink-wails.git
cd xlink-wails
开发模式


# 安装依赖
make install-deps

# 启动开发服务器
make dev
# 或
wails dev
构建发布
Bash

# Windows
make build-windows

# macOS
make build-darwin

# Linux
make build-linux

# 所有平台
make build
📁 项目结构


xlink-wails/
├── main.go                     # 应用入口
├── app.go                      # 主应用逻辑 (所有API)
├── wails.json                  # Wails配置
├── go.mod / go.sum            # Go依赖
├── Makefile                   # 构建脚本
├── README.md                  # 说明文档
│
├── internal/                  # Go内部包
│   ├── models/               # 数据模型
│   │   └── models.go        # 节点/规则/日志结构
│   ├── config/              # 配置管理
│   │   ├── config.go       # 加载/保存/加密
│   │   ├── dpapi_windows.go # Windows DPAPI
│   │   └── dpapi_other.go  # 跨平台兼容
│   ├── engine/              # 进程管理
│   │   ├── engine.go       # 启动/停止/监控
│   │   ├── engine_windows.go
│   │   └── engine_other.go
│   ├── generator/           # 配置生成
│   │   ├── generator.go    # Xlink/Xray配置
│   │   └── templates.go    # 配置模板
│   ├── logger/              # 日志系统
│   │   ├── logger.go       # 日志管理
│   │   ├── ping.go         # Ping测试
│   │   └── ping_windows.go
│   ├── dns/                 # DNS防泄露
│   │   ├── dns.go          # DNS配置生成
│   │   ├── leaktest.go     # 泄露检测
│   │   ├── tun_windows.go  # TUN管理
│   │   └── tun_other.go
│   └── system/              # 系统功能
│       ├── autostart.go    # 开机自启
│       ├── autostart_windows.go
│       ├── autostart_other.go
│       ├── tray.go         # 系统托盘
│       ├── notification.go # 系统通知
│       ├── proxy.go        # 系统代理
│       └── utils.go        # 工具函数
│
├── frontend/                 # Vue3前端
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── tsconfig.json
│   └── src/
│       ├── main.ts          # 入口
│       ├── App.vue          # 根组件
│       ├── style.css        # 全局样式
│       ├── types/           # 类型定义
│       │   └── index.ts
│       ├── stores/          # Pinia状态
│       │   ├── app.ts
│       │   ├── nodes.ts
│       │   └── logs.ts
│       ├── composables/     # 组合式函数
│       │   └── useWails.ts
│       └── components/      # Vue组件
│           ├── layout/
│           │   ├── AppHeader.vue
│           │   └── AppSidebar.vue
│           ├── nodes/
│           │   └── NodeEditor.vue
│           ├── rules/
│           │   ├── RuleList.vue
│           │   └── RuleDialog.vue
│           ├── logs/
│           │   └── LogViewer.vue
│           ├── settings/
│           │   ├── DNSSettings.vue
│           │   └── GeneralSettings.vue
│           └── common/
│               └── Modal.vue
│
├── build/                   # 构建资源
│   ├── appicon.png         # 应用图标
│   └── windows/
│       ├── icon.ico
│       └── wails.exe.manifest
│
└── resources/               # 运行时资源
    ├── xlink-cli-binary.exe
    ├── xray.exe
    ├── wintun.dll
    ├── geosite.dat
    └── geoip.dat
    
📊 API 参考

节点管理
方法	参数	返回值	说明
GetNodes()	-	[]NodeConfig	获取所有节点
GetNode(id)	string	NodeConfig	获取单个节点
AddNode(name)	string	NodeConfig	添加节点
UpdateNode(node)	NodeConfig	error	更新节点
DeleteNode(id)	string	error	删除节点
DuplicateNode(id)	string	NodeConfig	复制节点

节点控制
方法	参数	返回值	说明
StartNode(id)	string	error	启动节点
StopNode(id)	string	error	停止节点
StartAllNodes()	-	error	启动全部
StopAllNodes()	-	error	停止全部
PingTest(id)	string	error	延迟测试

DNS防泄露
方法	参数	返回值	说明
GetDNSModes()	-	[]DNSMode	获取DNS模式
TestDNSLeak()	-	LeakResult	泄露测试
IsTUNSupported()	-	map	TUN支持检查
ClearFakeIPCache()	-	-	清空缓存
FlushDNSCache()	-	error	刷新系统DNS

日志系统
方法	参数	返回值	说明
GetLogs(limit)	int	[]LogEntry	获取日志
ClearLogs()	-	-	清空日志
ExportLogs(format)	string	string	导出日志
OpenLogFolder()	-	error	打开日志目录

🐛 常见问题
Q: 程序无法启动？
A: 确保安装了 WebView2 运行时。Windows 10 1809+ 通常已预装。

Q: 连接失败？
A: 检查服务器地址、Token是否正确，以及防火墙设置。

Q: DNS仍然泄露？
A: 确保启用了 Fake-IP 模式，并开启流量嗅探功能。

Q: TUN模式无法启用？
A: 需要以管理员身份运行程序，并确保 wintun.dll 存在。

Q: 如何更新 geosite/geoip？
A: 从 v2ray-rules-dat 下载最新版本。

📄 开源协议
本项目采用 MIT License 开源协议。

🙏 致谢
Wails - Go + Web 桌面应用框架
Vue.js - 渐进式 JavaScript 框架
Tailwind CSS - 实用优先的 CSS 框架
Xray-core - 代理核心引擎
v2ray-rules-dat - 规则数据库
<div align="center">
如果这个项目对您有帮助，请给一个 ⭐ Star！

Made with ❤️ by Xlink Team

</div> 
