import type { FirewallStatus } from '@/types'
import type { ApiCtx } from './core'

// firewall 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface FirewallMethods {
    getFirewallStatus(agentId: string): Promise<FirewallStatus>
}

export const firewallMethods: FirewallMethods = {
  async getFirewallStatus(this: ApiCtx, agentId: string): Promise<FirewallStatus> {

    return this.client.get<unknown, FirewallStatus>(`/agents/${encodeURIComponent(agentId)}/firewall/status`)
  
  }
}
