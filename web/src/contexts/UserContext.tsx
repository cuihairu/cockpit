import { useState, ReactNode } from 'react'
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
    role: storedRole || 'user',
  }
}

export const UserProvider = ({ children }: { children: ReactNode }) => {
  const [user, setUser] = useState<User | null>(() => getStoredUser())
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'))

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

    setToken(token)
    setUser({
      id: user_id,
      username: userName,
      role: res.role || 'user',
    })

    return res
  }

  const logout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('userId')
    localStorage.removeItem('username')
    localStorage.removeItem('email')
    localStorage.removeItem('role')
    setToken(null)
    setUser(null)
  }

  const updateUser = (updatedUser: User) => {
    setUser(updatedUser)
    if (updatedUser.email) {
      localStorage.setItem('email', updatedUser.email)
    }
    localStorage.setItem('role', updatedUser.role)
  }

  return (
    <UserContext.Provider value={{ user, token, login, logout, updateUser }}>
      {children}
    </UserContext.Provider>
  )
}

