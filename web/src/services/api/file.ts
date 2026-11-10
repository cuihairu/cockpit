import type { FileEntry, FileReadResult, FileSearchResult } from '@/types'
import type { ApiCtx } from './core'

// file 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface FileMethods {
    listFiles(agentId: string, dir: string): Promise<{ dir: string; entries: FileEntry[] }>
    readFileChunk(agentId: string, path: string, offset: number, length: number): Promise<FileReadResult>
    writeFile(agentId: string, path: string, data: string, truncate?: boolean): Promise<{ size: number }>
    createRemoteDir(agentId: string, path: string): Promise<void>
    deleteRemoteFile(agentId: string, path: string): Promise<void>
    renameRemoteFile(agentId: string, path: string, name: string): Promise<void>
    chmodFile(agentId: string, path: string, mode: number): Promise<void>
    chownFile(agentId: string, path: string, uid: number, gid: number): Promise<void>
    searchFiles(agentId: string, dir: string, query: string, caseSensitive?: boolean): Promise<FileSearchResult>
    downloadRemoteFile(agentId: string, path: string): Promise<Blob>
}

export const fileMethods: FileMethods = {
  async listFiles(this: ApiCtx, agentId: string, dir: string): Promise<{ dir: string; entries: FileEntry[] }> {

    return this.client.post<unknown, { dir: string; entries: FileEntry[] }>(
      `/agents/${encodeURIComponent(agentId)}/files/list`,
      { dir },
    )
  
  },

  async readFileChunk(this: ApiCtx, agentId: string, path: string, offset: number, length: number): Promise<FileReadResult> {

    return this.client.post<unknown, FileReadResult>(
      `/agents/${encodeURIComponent(agentId)}/files/read`,
      { path, offset, length },
    )
  
  },

  async writeFile(this: ApiCtx, agentId: string, path: string, data: string, truncate = true): Promise<{ size: number }> {

    return this.client.post<unknown, { size: number }>(
      `/agents/${encodeURIComponent(agentId)}/files/write`,
      { path, data, truncate },
    )
  
  },

  async createRemoteDir(this: ApiCtx, agentId: string, path: string): Promise<void> {

    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/mkdir`, { path })
  
  },

  async deleteRemoteFile(this: ApiCtx, agentId: string, path: string): Promise<void> {

    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/delete`, { path })
  
  },

  async renameRemoteFile(this: ApiCtx, agentId: string, path: string, name: string): Promise<void> {

    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/rename`, { path, name })
  
  },

  async chmodFile(this: ApiCtx, agentId: string, path: string, mode: number): Promise<void> {

    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/chmod`, { path, mode })
  
  },

  async chownFile(this: ApiCtx, agentId: string, path: string, uid: number, gid: number): Promise<void> {

    await this.client.post(`/agents/${encodeURIComponent(agentId)}/files/chown`, { path, uid, gid })
  
  },

  async searchFiles(this: ApiCtx, agentId: string, dir: string, query: string, caseSensitive = false): Promise<FileSearchResult> {

    return this.client.post<unknown, FileSearchResult>(
      `/agents/${encodeURIComponent(agentId)}/files/search`,
      { dir, query, caseSensitive },
    )
  
  },

  async downloadRemoteFile(this: ApiCtx, agentId: string, path: string): Promise<Blob> {

    const resp = await this.client.get(`/agents/${encodeURIComponent(agentId)}/files/download`, {
      params: { path },
      responseType: 'blob',
    })
    return resp as unknown as Blob
  
  }
}
