import type { CloudAccount, CloudAccountInput, CloudInstance, CloudOperation, CloudInstanceSpecInput } from '@/types'
import type { ApiCtx } from './core'

// cloud 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface CloudMethods {
    getCloudAccounts(): Promise<CloudAccount[]>
    saveCloudAccount(input: CloudAccountInput & { confirmKey: string }): Promise<CloudAccount>
    deleteCloudAccount(id: string): Promise<void>
    getCloudInstances(accountId: string): Promise<CloudInstance[]>
    cloudInstanceAction(accountId: string, action: 'start' | 'stop' | 'delete', instanceId: string): Promise<CloudOperation>
    cloudInstanceCreate(accountId: string, spec: CloudInstanceSpecInput): Promise<CloudOperation>
    getCloudOperation(accountId: string, operationId: string): Promise<CloudOperation>
}

export const cloudMethods: CloudMethods = {
  async getCloudAccounts(this: ApiCtx): Promise<CloudAccount[]> {

    const resp = await this.client.get<unknown, { data: CloudAccount[] }>('/cloud/accounts')
    return resp.data ?? []
  
  },

  async saveCloudAccount(this: ApiCtx, input: CloudAccountInput & { confirmKey: string }): Promise<CloudAccount> {

    return this.client.put<unknown, CloudAccount>('/cloud/accounts', {
      vendor: input.vendor,
      alias: input.alias,
      secret: input.secret,
      username: input.username,
      region: input.region,
      scopes: input.scopes,
      enabled: input.enabled,
      confirm_key: input.confirmKey,
    })
  
  },

  async deleteCloudAccount(this: ApiCtx, id: string): Promise<void> {

    await this.client.delete(`/cloud/accounts/${encodeURIComponent(id)}`, {
      data: { confirm_id: id },
    })
  
  },

  async getCloudInstances(this: ApiCtx, accountId: string): Promise<CloudInstance[]> {

    const resp = await this.client.get<unknown, { data: CloudInstance[] }>(
      `/cloud/accounts/${encodeURIComponent(accountId)}/instances`,
    )
    return resp.data ?? []
  
  },

  async cloudInstanceAction(this: ApiCtx, accountId: string, action: 'start' | 'stop' | 'delete', instanceId: string): Promise<CloudOperation> {

    return this.client.post<unknown, CloudOperation>(`/cloud/instances/${encodeURIComponent(accountId)}/actions`, {
      action,
      instance_id: instanceId,
      confirm_key: instanceId,
    })
  
  },

  async cloudInstanceCreate(this: ApiCtx, accountId: string, spec: CloudInstanceSpecInput): Promise<CloudOperation> {

    return this.client.post<unknown, CloudOperation>(`/cloud/instances/${encodeURIComponent(accountId)}/actions`, {
      action: 'create',
      ...spec,
    })
  
  },

  async getCloudOperation(this: ApiCtx, accountId: string, operationId: string): Promise<CloudOperation> {

    return this.client.post<unknown, CloudOperation>(`/cloud/instances/${encodeURIComponent(accountId)}/operations`, {
      operation_id: operationId,
    })
  
  }
}
