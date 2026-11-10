// ========== 服务域名绑定（见 domain-binding-design.md） ==========

// 一条绑定：「域名 → agent 上的目标服务」唯一事实源，apply 按 auto* 三联动
export interface DomainBinding {
  id: number
  domain: string
  agentId: string
  target: string // host:port 或 docker://服务名
  enabled: boolean
  autoDns: boolean
  autoProxy: boolean
  autoCert: boolean
  lastApplyStatus: string // never / ok / failed
  lastError: string
  appliedAt: number // Unix 秒，0 = 从未
  createdAt: string
  updatedAt: string
}

// 登记入参（POST /api/domains；* 为空时 server 默认 true）
export interface DomainBindingInput {
  domain: string
  agentId: string
  target: string
  enabled?: boolean
  autoDns?: boolean
  autoProxy?: boolean
  autoCert?: boolean
}

// 单路联动漂移（D6 三态：ok / missing|mismatch|foreign / error）
export interface DomainDriftCheck {
  checked: boolean
  ok: boolean
  status?: string
  expected?: string
  actual?: string
  error?: string
}

export interface DomainDriftReport {
  domain: string
  enabled: boolean
  dns: DomainDriftCheck
  proxy: DomainDriftCheck
  cert: DomainDriftCheck
}

export interface DomainDriftResponse {
  items: DomainDriftReport[]
  checkedAt: number
}

// apply 单项联动结果
export interface DomainApplyStep {
  name: string // dns / proxy / cert
  ok: boolean
  error?: string
}
