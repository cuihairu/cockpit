// 认证与访问控制域类型
// ========== TOTP 二次验证相关类型 ==========

// TOTP 生成响应
export interface TOTPGenerateResponse {
  secret: string
  qr_code: string
  backup_codes: string[]
}

// TOTP 验证请求
export interface TOTPVerifyRequest {
  code: string
  tmp_token?: string
}

// TOTP 验证响应
export interface TOTPVerifyResponse {
  token: string
  expires_at: number
  user_id: string
  username: string
  role: string
}

// TOTP 启用/禁用请求
export interface TOTPCodeRequest {
  code: string
}

// 登录响应（扩展支持 TOTP）
export interface LoginResponse {
  token?: string
  expires_at?: number
  user_id: string
  username: string
  role?: string
  requires_totp: boolean
  tmp_token?: string
}

// 用户信息（包含 TOTP 状态）
export interface UserInfo {
  id: string
  username: string
  email?: string
  phone?: string
  department?: string
  role: string
  /** 角色表展开的权限点（RBAC P1 笔 5）；幽灵角色/库故障为空数组 */
  permissions: string[]
  totp_enabled: boolean
  totp_setup_at?: string
}

// ========== 访问控制（RBAC P1 笔 7） ==========

// 用户管理列表项（GET /users 返回的 storage.User，密码永不返回）
export interface ManagedUser {
  id: string
  username: string
  email?: string
  role: string
  totp_enabled: boolean
  created_at?: string
  updated_at?: string
}

// 角色定义（GET /roles 返回的 storage.Role）
export interface RoleInfo {
  name: string
  permissions: string[]
  builtin?: boolean
}
