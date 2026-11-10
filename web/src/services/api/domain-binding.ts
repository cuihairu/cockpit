import type { DomainBinding, DomainBindingInput, DomainApplyStep, DomainDriftResponse } from '@/types'
import type { ApiCtx } from './core'

// domain-binding 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface DomainBindingMethods {
    getDomainBindings(agentId?: string): Promise<DomainBinding[]>
    saveDomainBinding(input: DomainBindingInput): Promise<DomainBinding>
    deleteDomainBinding(domain: string): Promise<void>
    applyDomainBinding(domain: string): Promise<{ domain: string; status: string; steps: DomainApplyStep[] }>
    getDomainDrift(agentId?: string): Promise<DomainDriftResponse>
    getAgentDomains(agentId: string): Promise<DomainBinding[]>
    getAgentDomainsSnippet(agentId: string): Promise<string>
}

export const domainBindingMethods: DomainBindingMethods = {
  async getDomainBindings(this: ApiCtx, agentId?: string): Promise<DomainBinding[]> {

    const resp = await this.client.get<unknown, { data: DomainBinding[] }>('/domains', {
      params: agentId ? { agent: agentId } : undefined,
    })
    return resp.data ?? []
  
  },

  async saveDomainBinding(this: ApiCtx, input: DomainBindingInput): Promise<DomainBinding> {

    return this.client.post<unknown, DomainBinding>('/domains', input)
  
  },

  async deleteDomainBinding(this: ApiCtx, domain: string): Promise<void> {

    await this.client.delete(`/domains/${encodeURIComponent(domain)}`)
  
  },

  async applyDomainBinding(this: ApiCtx, domain: string): Promise<{ domain: string; status: string; steps: DomainApplyStep[] }> {

    return this.client.post<unknown, { domain: string; status: string; steps: DomainApplyStep[] }>(
      `/domains/${encodeURIComponent(domain)}/apply`,
    )
  
  },

  async getDomainDrift(this: ApiCtx, agentId?: string): Promise<DomainDriftResponse> {

    return this.client.get<unknown, DomainDriftResponse>('/domains/drift', {
      params: agentId ? { agent: agentId } : undefined,
    })
  
  },

  async getAgentDomains(this: ApiCtx, agentId: string): Promise<DomainBinding[]> {

    const resp = await this.client.get<unknown, { data: DomainBinding[] }>(
      `/agents/${encodeURIComponent(agentId)}/domains`,
    )
    return resp.data ?? []
  
  },

  async getAgentDomainsSnippet(this: ApiCtx, agentId: string): Promise<string> {

    const resp = await this.client.get<unknown, string>(
      `/agents/${encodeURIComponent(agentId)}/domains/snippet`,
      { responseType: 'text' },
    )
    return resp
  
  }
}
