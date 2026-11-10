import type { LoginResponse, UserInfo, TOTPGenerateResponse, TOTPVerifyResponse } from '@/types'
import type { ApiCtx } from './core'
import type { AxiosResponse } from 'axios'

// auth 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface AuthMethods {
    login(username: string, password: string): Promise<LoginResponse>
    logout(): Promise<void>
    refreshToken(): Promise<AxiosResponse<{ token: string }>>
    getCurrentUser(): Promise<UserInfo>
    forgotPassword(username: string): Promise<{ message: string }>
    resetPassword(token: string, code: string, newPassword: string): Promise<{ message: string }>
    verifyResetCode(token: string, code: string): Promise<{ valid: boolean; message?: string }>
    generateTOTP(): Promise<TOTPGenerateResponse>
    enableTOTP(code: string): Promise<{ status: string; message: string }>
    disableTOTP(code: string): Promise<{ status: string; message: string }>
    verifyTOTP(code: string, tmpToken: string): Promise<TOTPVerifyResponse>
}

export const authMethods: AuthMethods = {
  async login(this: ApiCtx, username: string, password: string): Promise<LoginResponse> {

    const data = await this.client.post<unknown, LoginResponse>('/auth/login', { username, password })
    return data
  
  },

  async logout(this: ApiCtx) {

    localStorage.removeItem('token')
    localStorage.removeItem('username')
    window.location.href = '/login'
  
  },

  async refreshToken(this: ApiCtx) {

    return this.client.post<{ token: string }>('/auth/refresh')
  
  },

  async getCurrentUser(this: ApiCtx): Promise<UserInfo> {

    return this.client.get<unknown, UserInfo>('/me')
  
  },

  async forgotPassword(this: ApiCtx, username: string): Promise<{ message: string }> {

    return this.client.post<unknown, { message: string }>('/auth/forgot-password', { username })
  
  },

  async resetPassword(this: ApiCtx, token: string, code: string, newPassword: string): Promise<{ message: string }> {

    return this.client.post<unknown, { message: string }>('/auth/reset-password', {
      token,
      code,
      newPassword,
    })
  
  },

  async verifyResetCode(this: ApiCtx, token: string, code: string): Promise<{ valid: boolean; message?: string }> {

    return this.client.post<unknown, { valid: boolean; message?: string }>('/auth/verify-reset-code', { token, code })
  
  },

  async generateTOTP(this: ApiCtx): Promise<TOTPGenerateResponse> {

    return this.client.post<unknown, TOTPGenerateResponse>('/auth/totp/generate')
  
  },

  async enableTOTP(this: ApiCtx, code: string): Promise<{ status: string; message: string }> {

    return this.client.post<unknown, { status: string; message: string }>('/auth/totp/enable', { code })
  
  },

  async disableTOTP(this: ApiCtx, code: string): Promise<{ status: string; message: string }> {

    return this.client.post<unknown, { status: string; message: string }>('/auth/totp/disable', { code })
  
  },

  async verifyTOTP(this: ApiCtx, code: string, tmpToken: string): Promise<TOTPVerifyResponse> {

    return this.client.post<unknown, TOTPVerifyResponse>('/auth/totp/verify', { code, tmp_token: tmpToken })
  
  }
}
