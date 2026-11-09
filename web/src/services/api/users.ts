import type { ManagedUser, RoleInfo } from '@/types'
import type { ApiCtx } from './core'

// users 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface UsersMethods {
    listUsers(): Promise<ManagedUser[]>
    createUser(data: { username: string; password: string; email?: string; role?: string }): Promise<ManagedUser>
    updateUser(id: string, data: { email?: string; role?: string }): Promise<ManagedUser>
    deleteUser(id: string): Promise<{ message: string }>
    changeUserPassword(id: string, newPassword: string): Promise<{ message: string }>
    listRoles(): Promise<RoleInfo[]>
    createRole(data: { name: string; permissions: string[] }): Promise<RoleInfo>
    updateRole(name: string, permissions: string[]): Promise<RoleInfo>
    deleteRole(name: string): Promise<{ message: string }>
}

export const usersMethods: UsersMethods = {
  async listUsers(this: ApiCtx): Promise<ManagedUser[]> {

    return this.client.get<unknown, ManagedUser[]>('/users')
  
  },

  async createUser(this: ApiCtx, data: { username: string; password: string; email?: string; role?: string }): Promise<ManagedUser> {

    return this.client.post<unknown, ManagedUser>('/users', data)
  
  },

  async updateUser(this: ApiCtx, id: string, data: { email?: string; role?: string }): Promise<ManagedUser> {

    return this.client.put<unknown, ManagedUser>(`/users/${encodeURIComponent(id)}`, data)
  
  },

  async deleteUser(this: ApiCtx, id: string): Promise<{ message: string }> {

    return this.client.delete<unknown, { message: string }>(`/users/${encodeURIComponent(id)}`)
  
  },

  async changeUserPassword(this: ApiCtx, id: string, newPassword: string): Promise<{ message: string }> {

    return this.client.post<unknown, { message: string }>(
      `/users/${encodeURIComponent(id)}/password`,
      { new_password: newPassword }
    )
  
  },

  async listRoles(this: ApiCtx): Promise<RoleInfo[]> {

    return this.client.get<unknown, RoleInfo[]>('/roles')
  
  },

  async createRole(this: ApiCtx, data: { name: string; permissions: string[] }): Promise<RoleInfo> {

    return this.client.post<unknown, RoleInfo>('/roles', data)
  
  },

  async updateRole(this: ApiCtx, name: string, permissions: string[]): Promise<RoleInfo> {

    return this.client.put<unknown, RoleInfo>(`/roles/${encodeURIComponent(name)}`, { permissions })
  
  },

  async deleteRole(this: ApiCtx, name: string): Promise<{ message: string }> {

    return this.client.delete<unknown, { message: string }>(`/roles/${encodeURIComponent(name)}`)
  
  }
}
