package generator

// =============================================================================
// 配置模板（用于特殊场景）
// =============================================================================

// XrayFullTemplate 完整Xray配置模板（包含所有可选项）
const XrayFullTemplate = `{
  "log": {
    "loglevel": "{{.LogLevel}}",
    "access": "",
    "error": ""
  },
  "dns": {
    "hosts": {
      "localhost": "127.0.0.1"
    },
    "servers": [
      {{range $i, $s := .DNSServers}}{{if $i}},{{end}}
      {{$s}}{{end}}
    ],
    "queryStrategy": "{{.QueryStrategy}}",
    "disableCache": {{.DisableCache}},
    "disableFallback": {{.DisableFallback}},
    "tag": "dns-internal"
  },
  "fakedns": [
    {
      "ipPool": "198.18.0.0/15",
      "poolSize": 65535
    }
  ],
  "inbounds": [
    {
      "tag": "socks-in",
      "listen": "{{.ListenHost}}",
      "port": {{.ListenPort}},
      "protocol": "socks",
      "settings": {
        "auth": "noauth",
        "udp": true,
        "ip": "127.0.0.1"
      },
      "sniffing": {
        "enabled": {{.SniffingEnabled}},
        "destOverride": ["http", "tls", "quic"],
        "metadataOnly": false,
        "routeOnly": false
      }
    }
  ],
  "outbounds": [
    {
      "tag": "proxy_out",
      "protocol": "socks",
      "settings": {
        "servers": [
          {
            "address": "127.0.0.1",
            "port": {{.ProxyPort}}
          }
        ]
      }
    },
    {
      "tag": "direct",
      "protocol": "freedom",
      "settings": {}
    },
    {
      "tag": "block",
      "protocol": "blackhole",
      "settings": {}
    },
    {
      "tag": "dns-out",
      "protocol": "dns",
      "settings": {}
    }
  ],
  "routing": {
    "domainStrategy": "{{.DomainStrategy}}",
    "domainMatcher": "hybrid",
    "rules": [
      {
        "type": "field",
        "inboundTag": ["socks-in"],
        "port": 53,
        "outboundTag": "dns-out"
      },
      {{range $i, $r := .Rules}}{{if $i}},{{end}}
      {{$r}}{{end}}
    ]
  }
}`

// XlinkSimpleTemplate 简化版Xlink配置模板
const XlinkSimpleTemplate = `{
  "inbounds": [
    {
      "tag": "socks-in",
      "listen": "{{.Listen}}",
      "protocol": "socks"
    }
  ],
  "outbounds": [
    {
      "tag": "proxy",
      "protocol": "ech-proxy",
      "settings": {
        "server": "{{.Server}}",
        "server_ip": "{{.ServerIP}}",
        "token": "{{.Token}}",
        "strategy": "{{.Strategy}}",
        "rules": "{{.Rules}}",
        "global_keep_alive": false,
        "s5": "{{.S5}}"
      }
    }
  ]
}`

// =============================================================================
// DNS模式说明模板
// =============================================================================

// DNSModeDescriptions DNS模式描述
var DNSModeDescriptions = map[int]string{
	0: `标准模式 (可能泄露DNS)
- 使用系统默认DNS
- 分流依赖IP规则
- 适合对隐私要求不高的场景`,

	1: `Fake-IP 模式 (推荐)
- 本地返回虚假IP
- 真实域名通过代理解析
- 有效防止DNS泄露
- 可能影响部分本地应用`,

	2: `TUN 全局接管 (最安全)
- 创建虚拟网卡接管所有流量
- 完全杜绝DNS泄露
- 需要管理员权限
- 性能略有损耗`,
}

// =============================================================================
// 预设规则模板
// =============================================================================

// PresetRules 预设规则集
var PresetRules = map[string][]string{
	// 广告拦截
	"block-ads": {
		"geosite:category-ads-all,block",
		"geosite:category-ads,block",
	},

	// 中国直连
	"direct-cn": {
		"geosite:cn,direct",
		"geoip:cn,direct",
		"geosite:geolocation-cn,direct",
	},

	// 常用代理
	"proxy-common": {
		"geosite:google,proxy",
		"geosite:youtube,proxy",
		"geosite:twitter,proxy",
		"geosite:facebook,proxy",
		"geosite:telegram,proxy",
		"geosite:github,proxy",
	},

	// 流媒体
	"proxy-streaming": {
		"geosite:netflix,proxy",
		"geosite:disney,proxy",
		"geosite:hbo,proxy",
		"geosite:spotify,proxy",
	},

	// 隐私保护
	"privacy": {
		"geosite:category-porn,block",
		"geosite:category-gambling,block",
	},
}

// GetPresetRules 获取预设规则
func GetPresetRules(presetName string) []string {
	if rules, ok := PresetRules[presetName]; ok {
		return rules
	}
	return nil
}
