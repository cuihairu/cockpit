import type { BackupConfig, BackupConfigInput, BackupRun, BackupFile, BackupTask } from '@/types'
import type { ApiCtx } from './core'

// backup 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface BackupMethods {
    getBackupConfigs(): Promise<{ configs: BackupConfig[] }>
    createBackupConfig(input: BackupConfigInput): Promise<BackupConfig>
    updateBackupConfig(id: number, input: BackupConfigInput): Promise<BackupConfig>
    deleteBackupConfig(id: number): Promise<void>
    runBackup(id: number): Promise<{ status: string }>
    getBackupRuns(limit?: number): Promise<{ runs: BackupRun[] }>
    getBackupConfigRuns(id: number, limit?: number): Promise<{ runs: BackupRun[] }>
    getBackupFiles(id: number): Promise<{ files: BackupFile[] }>
    deleteBackupFile(id: number, name: string): Promise<void>
    syncBackupFileRemote(id: number, name: string): Promise<{ status: string }>
    restoreBackup(id: number, input: { file: string; dest_dir: string; confirm_name: string }): Promise<{ taskId: string; status: string }>
    getBackupTask(id: number, taskId: string): Promise<BackupTask>
    downloadBackupFile(id: number, name: string): Promise<Blob>
}

export const backupMethods: BackupMethods = {
  async getBackupConfigs(this: ApiCtx): Promise<{ configs: BackupConfig[] }> {

    return this.client.get<unknown, { configs: BackupConfig[] }>('/backups/configs')
  
  },

  async createBackupConfig(this: ApiCtx, input: BackupConfigInput): Promise<BackupConfig> {

    return this.client.post<unknown, BackupConfig>('/backups/configs', input)
  
  },

  async updateBackupConfig(this: ApiCtx, id: number, input: BackupConfigInput): Promise<BackupConfig> {

    return this.client.put<unknown, BackupConfig>(`/backups/configs/${id}`, input)
  
  },

  async deleteBackupConfig(this: ApiCtx, id: number): Promise<void> {

    await this.client.delete(`/backups/configs/${id}`)
  
  },

  async runBackup(this: ApiCtx, id: number): Promise<{ status: string }> {

    return this.client.post<unknown, { status: string }>(`/backups/configs/${id}/run`)
  
  },

  async getBackupRuns(this: ApiCtx, limit = 50): Promise<{ runs: BackupRun[] }> {

    return this.client.get<unknown, { runs: BackupRun[] }>(`/backups/runs?limit=${limit}`)
  
  },

  async getBackupConfigRuns(this: ApiCtx, id: number, limit = 50): Promise<{ runs: BackupRun[] }> {

    return this.client.get<unknown, { runs: BackupRun[] }>(`/backups/configs/${id}/runs?limit=${limit}`)
  
  },

  async getBackupFiles(this: ApiCtx, id: number): Promise<{ files: BackupFile[] }> {

    return this.client.get<unknown, { files: BackupFile[] }>(`/backups/configs/${id}/files`)
  
  },

  async deleteBackupFile(this: ApiCtx, id: number, name: string): Promise<void> {

    await this.client.post(`/backups/configs/${id}/files/delete`, { name })
  
  },

  async syncBackupFileRemote(this: ApiCtx, id: number, name: string): Promise<{ status: string }> {

    return this.client.post<unknown, { status: string }>(`/backups/configs/${id}/files/sync-remote`, { name })
  
  },

  async restoreBackup(this: ApiCtx, id: number, input: { file: string; dest_dir: string; confirm_name: string }): Promise<{ taskId: string; status: string }> {

    return this.client.post<unknown, { taskId: string; status: string }>(`/backups/configs/${id}/restore`, input)
  
  },

  async getBackupTask(this: ApiCtx, id: number, taskId: string): Promise<BackupTask> {

    return this.client.get<unknown, BackupTask>(`/backups/configs/${id}/tasks/${encodeURIComponent(taskId)}`)
  
  },

  async downloadBackupFile(this: ApiCtx, id: number, name: string): Promise<Blob> {

    const resp = await this.client.get(`/backups/configs/${id}/files/download`, {
      params: { name },
      responseType: 'blob',
    })
    return resp as unknown as Blob
  
  }
}
