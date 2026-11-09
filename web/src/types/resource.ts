// 台账资源域类型（计算实例/域名/证书/服务/网关/存储）

// 计算实例
export interface ComputeInstance {
  id: string
  name: string
  displayName: string
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
  type: 'openwrt'
  agentId?: string
  ipv4?: string
  ipv6?: string
  upstream?: string
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
  type: 'nas' | 'san' | 'object-storage' | 'local' | 'nfs' | 'iscsi' | 'ceph'
  agentId?: string
  path?: string
  totalGb?: number
  usedGb?: number
  availableGb?: number
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
