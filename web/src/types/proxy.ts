// ============ 反向代理（Nginx 站点管理） ============

// proxy.status：nginx 安装状态概览
export interface ProxyStatus {
  backend?: 'nginx' | 'traefik' // M2 起区分后端；旧 agent 无此字段视为 nginx
  installed: boolean
  version: string
  confDir: string
  siteCount: number
  reloadMode: 'systemctl' | 'signal' | 'hot'
}

// 一个反代站点的声明（与 agent 侧 ProxySite 同构）
export interface ProxySite {
  name: string
  serverNames: string[]
  upstream: string
  scheme: 'http' | 'https'
  tlsCert?: string
  tlsKey?: string
  websocket?: boolean
  extra?: string
}

// proxy.site.get：站点元数据 + 渲染后的配置全文
export interface ProxySiteDetail {
  name: string
  site: ProxySite
  content: string
}
