import type { ProxyStatus, ProxySite, ProxySiteDetail } from '@/types'
import type { ApiCtx } from './core'

// proxy 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface ProxyMethods {
    getProxyStatus(agentId: string): Promise<ProxyStatus>
    getProxySites(agentId: string): Promise<{ sites: ProxySite[] }>
    getProxySite(agentId: string, name: string): Promise<ProxySiteDetail>
    applyProxySite(agentId: string, site: ProxySite): Promise<{ name: string; file: string }>
    deleteProxySite(agentId: string, name: string): Promise<void>
}

export const proxyMethods: ProxyMethods = {
  async getProxyStatus(this: ApiCtx, agentId: string): Promise<ProxyStatus> {

    return this.client.get<unknown, ProxyStatus>(`/agents/${encodeURIComponent(agentId)}/proxy/status`)
  
  },

  async getProxySites(this: ApiCtx, agentId: string): Promise<{ sites: ProxySite[] }> {

    return this.client.get<unknown, { sites: ProxySite[] }>(
      `/agents/${encodeURIComponent(agentId)}/proxy/sites`,
    )
  
  },

  async getProxySite(this: ApiCtx, agentId: string, name: string): Promise<ProxySiteDetail> {

    return this.client.get<unknown, ProxySiteDetail>(
      `/agents/${encodeURIComponent(agentId)}/proxy/sites/${encodeURIComponent(name)}`,
    )
  
  },

  async applyProxySite(this: ApiCtx, agentId: string, site: ProxySite): Promise<{ name: string; file: string }> {

    return this.client.put<unknown, { name: string; file: string }>(
      `/agents/${encodeURIComponent(agentId)}/proxy/sites/${encodeURIComponent(site.name)}`,
      site,
    )
  
  },

  async deleteProxySite(this: ApiCtx, agentId: string, name: string): Promise<void> {

    await this.client.delete(`/agents/${encodeURIComponent(agentId)}/proxy/sites/${encodeURIComponent(name)}`)
  
  }
}
