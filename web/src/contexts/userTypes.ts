import type { LoginResponse } from '@/types'

export interface User {
  id: string
  username: string
  email?: string
  phone?: string
  department?: string
  role: string
  /** /api/me 展开的权限点清单；undefined = 尚未加载（判定一律 fail-closed） */
  permissions?: string[]
}

export interface UserContextType {
  user: User | null
  token: string | null
  /** /api/me 已完成（含失败）——菜单/路由守卫等它就绪再渲染，避免闪烁 */
  permissionsReady: boolean
  login: (username: string, password: string) => Promise<LoginResponse>
  logout: () => void
  updateUser: (user: User) => void
}

export class TOTPRequiredError extends Error {
  public response: LoginResponse

  constructor(response: LoginResponse) {
    super('TOTP verification required')
    this.name = 'TOTPRequiredError'
    this.response = response
  }
}
