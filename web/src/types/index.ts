// 资源类型定义
export type ResourceType =
  | 'compute-instance'
  | 'container-service'
  | 'domain'
  | 'certificate'
  | 'service'
  | 'gateway'
  | 'ci-service'
  | 'storage'

// 位置信息
export interface Location {
  region: string
  zone: string
}

// Agent 状态
export interface Agent {
  id: string
  location: Location
  capabilities: Capability[]
  hostname: string
  ip: string
  status: 'online' | 'offline'
  lastSeen: string
  // 虚拟化信息
  virtType?: string  // kvm, vmware, qemu, docker, none
  virtRole?: string  // guest (虚拟机), host (物理机)
  // 标签（支持复杂类型）
  labels?: Record<string, any>
}

// 能力定义
export interface Capability {
  type: string
  endpoint?: string
  version?: string
  metadata?: Record<string, any>
}

// 计算实例
export interface ComputeInstance {
  id: string
  name: string
  displayName: string
  location: Location
  type: 'bare-metal' | 'vm' | 'container' | 'vps'
  region?: string
  zone?: string
  cpuCores?: number
  memoryMb?: number
  diskGb?: number
  agentId?: string
  ipv4?: string
  platform?: string
  platformUrl?: string
  hardware?: {
    cpu?: {
      model?: string
      cores?: number
      threads?: number
    }
    memory?: {
      capacity: string
    }
    disk?: Array<{
      device: string
      capacity: string
      type: string
    }>
  }
  access?: {
    web?: {
      url: string
    }
    ssh?: {
      host: string
      port: number
      user: string
    }
  }
  monitoring?: {
    enabled: boolean
    nezhaAgentId?: number
  }
  status?: 'running' | 'stopped' | 'unknown'
  tags?: Record<string, string>
}

// 域名
export interface Domain {
  id: string
  name: string
  displayName: string
  registrar: string
  registeredDate: string
  expiryDate: string
  autoRenew: boolean
  registrarConsoleUrl: string
  dnsProvider: string
  dnsConsoleUrl: string
  certificates: string[]
  subdomains: string[]
  status?: 'valid' | 'expiring' | 'expired'
  tags?: Record<string, string>
}

// 证书
export interface Certificate {
  id: string
  name: string
  displayName: string
  type: 'letsencrypt' | 'commercial' | 'self-signed' | 'cloudflare-origin'
  commonName: string
  sans: string[]
  issuedDate: string
  expiryDate: string
  autoRenew: boolean
  acmeProvider?: string
  deployedOn: Array<{
    computeRef: string
    path: string
  }>
  monitoring?: {
    enabled: boolean
    checkUrl?: string
  }
  daysRemaining?: number
  status?: 'valid' | 'expiring' | 'expired'
  tags?: Record<string, string>
}

// 服务
export interface Service {
  id: string
  name: string
  displayName: string
  type: 'web' | 'api' | 'database' | 'cache' | 'message-queue' | 'other'
  description?: string
  urls: string[]
  domainRef?: string
  certificateRef?: string
  computeRef?: string
  containerRef?: string
  healthCheck?: {
    enabled: boolean
    method: string
    url: string
    interval: string
    expectedStatus: number
  }
  repository?: {
    type: string
    url: string
    branch: string
  }
  dependsOn?: Array<{
    serviceRef: string
  }>
  status?: 'running' | 'stopped' | 'unhealthy' | 'unknown'
  health?: 'healthy' | 'unhealthy' | 'unknown'
  tags?: Record<string, string>
}

// Gateway / OpenWrt
export interface Gateway {
  id: string
  name: string
  displayName: string
  location: Location
  type: 'openwrt'
  model?: string
  firmware?: string
  access?: {
    web?: {
      url: string
    }
    ssh?: {
      host: string
      port: number
      user: string
    }
  }
  role: 'main-router' | 'ap' | 'bypass-gateway' | 'client-gateway'
  tunnels?: Tunnel[]
  monitoring?: {
    enabled: boolean
    nezhaAgentId?: number
    publicIp: boolean
  }
  status?: 'online' | 'offline'
  tags?: Record<string, string>
}

// 隧道
export interface Tunnel {
  type: 'wireguard' | 'cloudflare-tunnel' | 'vxlan' | 'gre'
  name: string
  peerEndpoint?: string
  subnet?: string
  allowedIPs?: string[]
  keepalive?: number
  tunnelId?: string
  tunnelUrl?: string
  services?: Array<{
    serviceRef: string
    publicUrl: string
  }>
}

// 存储设备
export interface Storage {
  id: string
  name: string
  displayName: string
  location: Location
  type: 'nas' | 'san' | 'object-storage'
  provider?: string
  access?: {
    web?: {
      url: string
    }
    ssh?: {
      host: string
      port: number
      user: string
    }
    smb?: string[]
    nfs?: string[]
  }
  capacity?: {
    total: string
    used: string
    available: string
  }
  usage?: Array<{
    type: string
    description: string
  }>
  monitoring?: {
    enabled: boolean
    diskHealth: boolean
    nezhaAgentId?: number
  }
  status?: 'online' | 'offline'
  tags?: Record<string, string>
}

// API 响应
export interface ApiResponse<T> {
  data?: T
  error?: string
  message?: string
}

// 分页参数
export interface PaginationParams {
  current: number
  pageSize: number
}

// 分页响应（与后端 api.go 返回格式一致）
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}
