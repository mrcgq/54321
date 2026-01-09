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
