import type { DNSStatus, DNSZone, DNSOrphan, DNSRecordsPage, DNSRecord, DNSRecordInput, DNSExport, DNSImportResult } from '@/types'
import type { ApiCtx } from './core'

// dns 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface DnsMethods {
    getDNSStatus(): Promise<DNSStatus>
    getDNSZones(): Promise<{ zones: DNSZone[]; orphans: DNSOrphan[] }>
    registerDNSZoneCMDB(zoneID: string): Promise<{ registered: boolean; id: string }>
    unregisterDNSZoneCMDB(zoneID: string): Promise<{ unregistered: boolean; records_removed: number }>
    getDNSRecords(zoneID: string, type?: string, page?: number): Promise<DNSRecordsPage>
    createDNSRecord(zoneID: string, input: DNSRecordInput): Promise<DNSRecord>
    updateDNSRecord(zoneID: string, recordID: string, input: DNSRecordInput): Promise<DNSRecord>
    deleteDNSRecord(zoneID: string, recordID: string): Promise<void>
    exportDNSRecords(zoneID: string): Promise<DNSExport>
    importDNSRecords(zoneID: string, records: DNSRecordInput[]): Promise<DNSImportResult>
}

export const dnsMethods: DnsMethods = {
  async getDNSStatus(this: ApiCtx): Promise<DNSStatus> {

    return this.client.get<unknown, DNSStatus>('/dns/status')
  
  },

  async getDNSZones(this: ApiCtx): Promise<{ zones: DNSZone[]; orphans: DNSOrphan[] }> {

    const resp = await this.client.get<unknown, { data: DNSZone[]; orphans?: DNSOrphan[] }>('/dns/zones')
    return { zones: resp.data ?? [], orphans: resp.orphans ?? [] }
  
  },

  async registerDNSZoneCMDB(this: ApiCtx, zoneID: string): Promise<{ registered: boolean; id: string }> {

    return this.client.post<unknown, { registered: boolean; id: string }>(
      `/dns/zones/${encodeURIComponent(zoneID)}/cmdb`,
    )
  
  },

  async unregisterDNSZoneCMDB(this: ApiCtx, zoneID: string): Promise<{ unregistered: boolean; records_removed: number }> {

    return this.client.delete<unknown, { unregistered: boolean; records_removed: number }>(
      `/dns/zones/${encodeURIComponent(zoneID)}/cmdb`,
    )
  
  },

  async getDNSRecords(this: ApiCtx, zoneID: string, type?: string, page?: number): Promise<DNSRecordsPage> {

    const params: Record<string, string> = {}
    if (type) params.type = type
    if (page) params.page = String(page)
    const resp = await this.client.get<unknown, { data: DNSRecordsPage }>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records`,
      { params },
    )
    return resp.data
  
  },

  async createDNSRecord(this: ApiCtx, zoneID: string, input: DNSRecordInput): Promise<DNSRecord> {

    return this.client.post<unknown, DNSRecord>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records`,
      input,
    )
  
  },

  async updateDNSRecord(this: ApiCtx, zoneID: string, recordID: string, input: DNSRecordInput): Promise<DNSRecord> {

    return this.client.put<unknown, DNSRecord>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records/${encodeURIComponent(recordID)}`,
      input,
    )
  
  },

  async deleteDNSRecord(this: ApiCtx, zoneID: string, recordID: string): Promise<void> {

    await this.client.delete(
      `/dns/zones/${encodeURIComponent(zoneID)}/records/${encodeURIComponent(recordID)}`,
    )
  
  },

  async exportDNSRecords(this: ApiCtx, zoneID: string): Promise<DNSExport> {

    return this.client.get<unknown, DNSExport>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records/export`,
    )
  
  },

  async importDNSRecords(this: ApiCtx, zoneID: string, records: DNSRecordInput[]): Promise<DNSImportResult> {

    return this.client.post<unknown, DNSImportResult>(
      `/dns/zones/${encodeURIComponent(zoneID)}/records/import`,
      { records },
    )
  
  }
}
