// ========== ACME 证书自动签发（见 acme-design.md） ==========

// 一条证书签发配置（列表/详情响应视图：PEM 字段永不出响应，
// hasPrivateKey 标记私钥是否已入库）
export interface AcmeCertView {
  id: number
  domains: string[]
  primaryDomain: string
  caDirectory: 'staging' | 'production'
  status: 'pending' | 'issued' | 'failed'
  hasPrivateKey: boolean
  expiresAt: string
  renewBeforeDays: number
  autoRenew: boolean
  lastRenewAt: number
  lastStatus: 'never' | 'ok' | 'failed'
  lastError: string
  checkedAt: number
  // 部署目标与状态（D14 多目标，D-2026-10-12-3）
  deployTargets: AcmeDeployTargetView[]
  createdAt: string
  updatedAt: string
}

export interface AcmeDeployTargetView {
  id: number
  certId: number
  agentId: string
  certPath: string
  keyPath: string
  lastDeployAt: number
  lastDeployError: string
}

// 签发配置创建/更新入参
export interface AcmeCertInput {
  domains: string[]
  caDirectory: 'staging' | 'production'
  autoRenew: boolean
  renewBeforeDays: number
  deployTargets?: { agentId: string; certPath: string; keyPath: string }[]
}

// ACME 账户信息
export interface AcmeAccount {
  registered: boolean
  email?: string
  caDirectory?: string
  registrationURI?: string
}

// ACME 视角的 DNS provider 状态（D13：与 DNS 管理页的 Cloudflare 专属判定语义分叉）
export interface AcmeDnsStatus {
  provider: 'cloudflare' | 'dnspod' | 'alidns'
  configured: boolean
}

// 续期巡检配置（server 定时重签；0 = 关闭）
export interface AcmeScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
  dns?: AcmeDnsStatus
}
