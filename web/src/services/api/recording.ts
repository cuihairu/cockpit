import type { TerminalRecording, RecordingConfig, ServerBackupFile, ServerBackupConfig } from '@/types'
import type { ApiCtx } from './core'

// recording 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface RecordingMethods {
    getRecordings(): Promise<TerminalRecording[]>
    getRecordingBlob(sessionId: string): Promise<Blob>
    getRecordingCast(sessionId: string): Promise<string>
    downloadRecording(sessionId: string): Promise<Blob>
    deleteRecording(sessionId: string): Promise<void>
    getRecordingsConfig(): Promise<RecordingConfig>
    putRecordingsConfig(input: {
    enabled?: boolean
    retention_days?: number
    remote_dest?: string
  }): Promise<void>
    syncRecordingRemote(sessionId: string): Promise<{ status: string }>
    getServerBackups(): Promise<ServerBackupFile[]>
    getServerBackupConfig(): Promise<ServerBackupConfig>
    putServerBackupConfig(input: {
    interval_hours?: number
    retention_days?: number
    remote_dest?: string
  }): Promise<void>
    runServerBackup(): Promise<{ name: string }>
    syncServerBackupRemote(name: string): Promise<{ status: string }>
    downloadServerBackup(name: string): Promise<Blob>
    deleteServerBackup(name: string): Promise<void>
}

export const recordingMethods: RecordingMethods = {
  async getRecordings(this: ApiCtx): Promise<TerminalRecording[]> {

    const resp = await this.client.get<unknown, { data: TerminalRecording[] }>('/recordings')
    return resp.data ?? []
  
  },

  async getRecordingBlob(this: ApiCtx, sessionId: string): Promise<Blob> {

    return this.client.get(`/recordings/${encodeURIComponent(sessionId)}/cast`, {
      responseType: 'blob',
    })
  
  },

  async getRecordingCast(this: ApiCtx, sessionId: string): Promise<string> {

    const resp = await this.client.get(`/recordings/${encodeURIComponent(sessionId)}/cast`, {
      responseType: 'text',
      transformResponse: [(data: string) => data],
    })
    return resp as unknown as string
  
  },

  async downloadRecording(this: ApiCtx, sessionId: string): Promise<Blob> {

    const resp = await this.client.get(`/recordings/${encodeURIComponent(sessionId)}/cast`, {
      responseType: 'blob',
    })
    return resp as unknown as Blob
  
  },

  async deleteRecording(this: ApiCtx, sessionId: string): Promise<void> {

    await this.client.delete(`/recordings/${encodeURIComponent(sessionId)}`)
  
  },

  async getRecordingsConfig(this: ApiCtx): Promise<RecordingConfig> {

    return this.client.get<unknown, RecordingConfig>('/recordings/config')
  
  },

  async putRecordingsConfig(this: ApiCtx, input: {
    enabled?: boolean
    retention_days?: number
    remote_dest?: string
  }): Promise<void> {

    await this.client.put('/recordings/config', input)
  
  },

  async syncRecordingRemote(this: ApiCtx, sessionId: string): Promise<{ status: string }> {

    return this.client.post<unknown, { status: string }>(
      `/recordings/${encodeURIComponent(sessionId)}/sync-remote`,
    )
  
  },

  async getServerBackups(this: ApiCtx): Promise<ServerBackupFile[]> {

    const resp = await this.client.get<unknown, { data: ServerBackupFile[] }>('/server-backups')
    return resp.data ?? []
  
  },

  async getServerBackupConfig(this: ApiCtx): Promise<ServerBackupConfig> {

    return this.client.get<unknown, ServerBackupConfig>('/server-backups/config')
  
  },

  async putServerBackupConfig(this: ApiCtx, input: {
    interval_hours?: number
    retention_days?: number
    remote_dest?: string
  }): Promise<void> {

    await this.client.put('/server-backups/config', input)
  
  },

  async runServerBackup(this: ApiCtx): Promise<{ name: string }> {

    return this.client.post<unknown, { name: string }>('/server-backups/run')
  
  },

  async syncServerBackupRemote(this: ApiCtx, name: string): Promise<{ status: string }> {

    return this.client.post<unknown, { status: string }>(
      `/server-backups/${encodeURIComponent(name)}/sync-remote`,
    )
  
  },

  async downloadServerBackup(this: ApiCtx, name: string): Promise<Blob> {

    const resp = await this.client.get(
      `/server-backups/${encodeURIComponent(name)}/download`,
      { responseType: 'blob' },
    )
    return resp as unknown as Blob
  
  },

  async deleteServerBackup(this: ApiCtx, name: string): Promise<void> {

    await this.client.delete(`/server-backups/${encodeURIComponent(name)}`)
  
  }
}
