// ========== 防火墙观测（iptables/nftables 只读快照，见 firewall-design.md） ==========

// firewall.status 规则（白名单摘要：text 人读文本 + counters）
export interface FirewallRule {
  handle?: number
  text: string
  packets?: number
  bytes?: number
  ownedByCockpit: boolean
}

export interface FirewallChain {
  name: string
  policy?: string
  rules: FirewallRule[]
}

// nft 后端 family 为 inet/ip/ip6；iptables 后端为 ipv4/ipv6
export interface FirewallTable {
  family: string
  name: string
  chains: FirewallChain[]
}

// firewall.status 返回：available=false 时 tables 空且 error 带原因；
// available=true 时 error 仍可能非空（IPv6 尽力而为失败、超限说明）
export interface FirewallStatus {
  available: boolean
  backend: string
  backendVersion?: string
  iptablesVariant?: string
  totalRules: number
  tables: FirewallTable[]
  truncated: boolean
  error?: string
}
