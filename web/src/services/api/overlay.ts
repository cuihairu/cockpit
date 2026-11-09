import type { OverlayStatus, OverlayDaemonStatus, OverlayChangeResult, OverlayServiceResult, OverlayCloudStatus } from '@/types'
import type { ApiCtx } from './core'

// overlay 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface OverlayMethods {
    getOverlayStatus(agentId: string): Promise<OverlayStatus>
    getOverlayDaemon(agentId: string): Promise<OverlayDaemonStatus>
    joinAgentOverlayNetwork(agentId: string, netId: string, tool: string): Promise<OverlayChangeResult>
    leaveAgentOverlayNetwork(agentId: string, netId: string, tool: string): Promise<OverlayChangeResult>
    agentOverlayService(agentId: string, tool: string, action: string): Promise<OverlayServiceResult>
    getOverlayCloud(): Promise<OverlayCloudStatus>
    setOverlayZTMemberAuthorized(networkId: string, memberId: string, authorized: boolean): Promise<void>
    removeOverlayZTMember(networkId: string, memberId: string): Promise<void>
    authorizeOverlayTSDevice(deviceId: string): Promise<void>
    removeOverlayTSDevice(deviceId: string): Promise<void>
}

export const overlayMethods: OverlayMethods = {
  async getOverlayStatus(this: ApiCtx, agentId: string): Promise<OverlayStatus> {

    return this.client.get<unknown, OverlayStatus>(`/agents/${encodeURIComponent(agentId)}/overlay/status`)
  
  },

  async getOverlayDaemon(this: ApiCtx, agentId: string): Promise<OverlayDaemonStatus> {

    return this.client.get<unknown, OverlayDaemonStatus>(`/agents/${encodeURIComponent(agentId)}/overlay/daemon`)
  
  },

  async joinAgentOverlayNetwork(this: ApiCtx, agentId: string, netId: string, tool: string): Promise<OverlayChangeResult> {

    return this.client.post<unknown, OverlayChangeResult>(
      `/agents/${encodeURIComponent(agentId)}/overlay/networks/${encodeURIComponent(netId)}/join`,
      { tool },
    )
  
  },

  async leaveAgentOverlayNetwork(this: ApiCtx, agentId: string, netId: string, tool: string): Promise<OverlayChangeResult> {

    return this.client.post<unknown, OverlayChangeResult>(
      `/agents/${encodeURIComponent(agentId)}/overlay/networks/${encodeURIComponent(netId)}/leave`,
      { tool },
    )
  
  },

  async agentOverlayService(this: ApiCtx, agentId: string, tool: string, action: string): Promise<OverlayServiceResult> {

    return this.client.post<unknown, OverlayServiceResult>(`/agents/${encodeURIComponent(agentId)}/overlay/service`, {
      tool,
      action,
    })
  
  },

  async getOverlayCloud(this: ApiCtx): Promise<OverlayCloudStatus> {

    return this.client.get<unknown, OverlayCloudStatus>('/overlay/cloud')
  
  },

  async setOverlayZTMemberAuthorized(this: ApiCtx, networkId: string, memberId: string, authorized: boolean): Promise<void> {

    await this.client.post(
      `/overlay/cloud/zerotier/networks/${encodeURIComponent(networkId)}/members/${encodeURIComponent(memberId)}`,
      { authorized },
    )
  
  },

  async removeOverlayZTMember(this: ApiCtx, networkId: string, memberId: string): Promise<void> {

    await this.client.delete(
      `/overlay/cloud/zerotier/networks/${encodeURIComponent(networkId)}/members/${encodeURIComponent(memberId)}`,
    )
  
  },

  async authorizeOverlayTSDevice(this: ApiCtx, deviceId: string): Promise<void> {

    await this.client.post(`/overlay/cloud/tailscale/devices/${encodeURIComponent(deviceId)}/authorize`)
  
  },

  async removeOverlayTSDevice(this: ApiCtx, deviceId: string): Promise<void> {

    await this.client.delete(`/overlay/cloud/tailscale/devices/${encodeURIComponent(deviceId)}`)
  
  }
}
