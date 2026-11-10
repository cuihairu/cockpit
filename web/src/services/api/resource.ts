import type { ComputeInstance, PaginatedResponse, Domain, Certificate, Service, Gateway, Storage } from '@/types'
import type { ApiCtx } from './core'

// resource 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface ResourceMethods {
    getComputeInstances(params?: {
    region?: string
    zone?: string
    type?: string
  }): Promise<PaginatedResponse<ComputeInstance>>
    getComputeInstance(id: string): Promise<ComputeInstance>
    getDomains(): Promise<PaginatedResponse<Domain>>
    getDomain(id: string): Promise<Domain>
    getCertificates(): Promise<PaginatedResponse<Certificate>>
    getCertificate(id: string): Promise<Certificate>
    getServices(): Promise<PaginatedResponse<Service>>
    getService(id: string): Promise<Service>
    getGateways(): Promise<PaginatedResponse<Gateway>>
    getGateway(id: string): Promise<Gateway>
    getStorages(): Promise<PaginatedResponse<Storage>>
    getStorage(id: string): Promise<Storage>
}

export const resourceMethods: ResourceMethods = {
  async getComputeInstances(this: ApiCtx, params?: {
    region?: string
    zone?: string
    type?: string
  }): Promise<PaginatedResponse<ComputeInstance>> {

    return this.client.get<unknown, PaginatedResponse<ComputeInstance>>('/resources/compute-instances', {
      params,
    })
  
  },

  async getComputeInstance(this: ApiCtx, id: string): Promise<ComputeInstance> {

    return this.client.get<unknown, ComputeInstance>(`/resources/compute-instances/${id}`)
  
  },

  async getDomains(this: ApiCtx): Promise<PaginatedResponse<Domain>> {

    return this.client.get<unknown, PaginatedResponse<Domain>>('/resources/domains')
  
  },

  async getDomain(this: ApiCtx, id: string): Promise<Domain> {

    return this.client.get<unknown, Domain>(`/resources/domains/${id}`)
  
  },

  async getCertificates(this: ApiCtx): Promise<PaginatedResponse<Certificate>> {

    return this.client.get<unknown, PaginatedResponse<Certificate>>('/resources/certificates')
  
  },

  async getCertificate(this: ApiCtx, id: string): Promise<Certificate> {

    return this.client.get<unknown, Certificate>(`/resources/certificates/${id}`)
  
  },

  async getServices(this: ApiCtx): Promise<PaginatedResponse<Service>> {

    return this.client.get<unknown, PaginatedResponse<Service>>('/resources/services')
  
  },

  async getService(this: ApiCtx, id: string): Promise<Service> {

    return this.client.get<unknown, Service>(`/resources/services/${id}`)
  
  },

  async getGateways(this: ApiCtx): Promise<PaginatedResponse<Gateway>> {

    return this.client.get<unknown, PaginatedResponse<Gateway>>('/resources/gateways')
  
  },

  async getGateway(this: ApiCtx, id: string): Promise<Gateway> {

    return this.client.get<unknown, Gateway>(`/resources/gateways/${id}`)
  
  },

  async getStorages(this: ApiCtx): Promise<PaginatedResponse<Storage>> {

    return this.client.get<unknown, PaginatedResponse<Storage>>('/resources/storages')
  
  },

  async getStorage(this: ApiCtx, id: string): Promise<Storage> {

    return this.client.get<unknown, Storage>(`/resources/storages/${id}`)
  
  }
}
