import type { AcmeCertView, AcmeCertInput, AcmeAccount, AcmeScanConfig } from '@/types'
import type { ApiCtx } from './core'

// acme 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface AcmeMethods {
    getAcmeCerts(): Promise<AcmeCertView[]>
    createAcmeCert(input: AcmeCertInput): Promise<AcmeCertView>
    updateAcmeCert(id: number, input: AcmeCertInput): Promise<AcmeCertView>
    deleteAcmeCert(id: number): Promise<void>
    issueAcmeCert(id: number): Promise<AcmeCertView>
    downloadAcmeCert(id: number, part: 'cert' | 'issuer' | 'key'): Promise<string>
    getAcmeAccount(): Promise<AcmeAccount>
    deployAcmeCert(id: number): Promise<AcmeCertView>
    putAcmeAccount(email: string): Promise<void>
    getAcmeScanConfig(): Promise<AcmeScanConfig>
    putAcmeScanConfig(scanIntervalSeconds: number): Promise<void>
}

export const acmeMethods: AcmeMethods = {
  async getAcmeCerts(this: ApiCtx): Promise<AcmeCertView[]> {

    return this.client.get<unknown, AcmeCertView[]>('/acme/certs')
  
  },

  async createAcmeCert(this: ApiCtx, input: AcmeCertInput): Promise<AcmeCertView> {

    return this.client.post<unknown, AcmeCertView>('/acme/certs', input)
  
  },

  async updateAcmeCert(this: ApiCtx, id: number, input: AcmeCertInput): Promise<AcmeCertView> {

    return this.client.put<unknown, AcmeCertView>(`/acme/certs/${id}`, input)
  
  },

  async deleteAcmeCert(this: ApiCtx, id: number): Promise<void> {

    await this.client.delete(`/acme/certs/${id}`)
  
  },

  async issueAcmeCert(this: ApiCtx, id: number): Promise<AcmeCertView> {

    return this.client.post<unknown, AcmeCertView>(`/acme/certs/${id}/issue`)
  
  },

  async downloadAcmeCert(this: ApiCtx, id: number, part: 'cert' | 'issuer' | 'key'): Promise<string> {

    const resp = await this.client.get<unknown, string>(`/acme/certs/${id}/download`, { params: { part }, responseType: 'text' })
    return resp
  
  },

  async getAcmeAccount(this: ApiCtx): Promise<AcmeAccount> {

    return this.client.get<unknown, AcmeAccount>('/acme/account')
  
  },

  async deployAcmeCert(this: ApiCtx, id: number): Promise<AcmeCertView> {

    return this.client.post<unknown, AcmeCertView>(`/acme/certs/${id}/deploy`)
  
  },

  async putAcmeAccount(this: ApiCtx, email: string): Promise<void> {

    await this.client.put('/acme/account', { email })
  
  },

  async getAcmeScanConfig(this: ApiCtx): Promise<AcmeScanConfig> {

    return this.client.get<unknown, AcmeScanConfig>('/acme/config')
  
  },

  async putAcmeScanConfig(this: ApiCtx, scanIntervalSeconds: number): Promise<void> {

    await this.client.put('/acme/config', { scan_interval_seconds: scanIntervalSeconds })
  
  }
}
