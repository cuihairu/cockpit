import { useCallback, useEffect, useState, ReactNode } from 'react'
import { api } from '@/services/api'
import type { LoginResponse } from '@/types'
import { TOTPRequiredError, type User } from './userTypes'
import { UserContext } from './userContextValue'

const getStoredUser = (): User | null => {
  const storedToken = localStorage.getItem('token')
  const storedUsername = localStorage.getItem('username')
  const storedRole = localStorage.getItem('role')

  if (!storedToken || !storedUsername) {
    return null
  }

  return {
    id: localStorage.getItem('userId') || '',
    username: storedUsername,
    email: localStorage.getItem('email') || undefined,
    phone: localStorage.getItem('phone') || undefined,
    department: localStorage.getItem('department') || undefined,
    role: storedRole || 'user',
    // permissions 不落 localStorage（D7：动态拉取，角色变更刷新即生效）
  }
}

export const UserProvider = ({ children }: { children: ReactNode }) => {
  const [user, setUser] = useState<User | null>(() => getStoredUser())
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'))
  // 无 token 无需拉取即就绪；有 token 时等 /api/me 回来（菜单/守卫用）
  const [permissionsReady, setPermissionsReady] = useState(() => !localStorage.getItem('token'))

  // 启动/换 token 时拉 /api/me 刷新用户资料与权限清单（失败静默：
  // 401 由 axios 拦截器跳登录，其余错误保持 permissions undefined，
  // 前端判定 fail-closed 全裁剪——与后端 RBAC 语义一致）
  useEffect(() => {
    if (!token) return
    let cancelled = false
    api
      .getCurrentUser()
      .then((me) => {
        if (cancelled) return
        setUser((prev) =>
          prev
            ? { ...prev, ...me, permissions: me.permissions ?? [] }
            : { ...me, permissions: me.permissions ?? [] }
        )
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setPermissionsReady(true)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  const login = async (username: string, password: string): Promise<LoginResponse> => {
    const res = await api.login(username, password)

    // 如果需要 TOTP 验证，抛出特殊错误
    if (res.requires_totp) {
      throw new TOTPRequiredError(res)
    }

    const { token, user_id, username: userName } = res

    if (!token) {
      throw new Error('No token returned from login')
    }

    localStorage.setItem('token', token)
    localStorage.setItem('userId', user_id)
    localStorage.setItem('username', userName)
    localStorage.setItem('role', res.role || 'user')
    localStorage.removeItem('email')
    localStorage.removeItem('phone')
    localStorage.removeItem('department')

    setToken(token)
    setUser({
      id: user_id,
      username: userName,
      role: res.role || 'user',
    })

    // 登录即拉一次权限清单（不等启动 effect——token 变化触发的那次）
    try {
      const me = await api.getCurrentUser()
      setUser((prev) => (prev ? { ...prev, ...me, permissions: me.permissions ?? [] } : prev))
    } catch {
      // 失败保持 undefined，启动 effect 的拉取会补上
    }
    setPermissionsReady(true)

    return res
  }

  const logout = useCallback(() => {
    localStorage.removeItem('token')
    localStorage.removeItem('userId')
    localStorage.removeItem('username')
    localStorage.removeItem('email')
    localStorage.removeItem('phone')
    localStorage.removeItem('department')
    localStorage.removeItem('role')
    setToken(null)
    setUser(null)
    setPermissionsReady(true)
  }, [])

  const updateUser = useCallback((updatedUser: User) => {
    setUser(updatedUser)
    setOptionalStorage('email', updatedUser.email)
    setOptionalStorage('phone', updatedUser.phone)
    setOptionalStorage('department', updatedUser.department)
    localStorage.setItem('role', updatedUser.role)
  }, [])

  return (
    <UserContext.Provider value={{ user, token, permissionsReady, login, logout, updateUser }}>
      {children}
    </UserContext.Provider>
  )
}

const setOptionalStorage = (key: string, value?: string) => {
  if (value && value.trim() !== '') {
    localStorage.setItem(key, value)
    return
  }
  localStorage.removeItem(key)
}
