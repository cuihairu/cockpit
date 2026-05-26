import { createContext, useContext, useState, useEffect, ReactNode } from 'react'
import { api } from '@/services/api'
import type { LoginResponse } from '@/types'

interface User {
  id: string
  username: string
  email?: string
  role: string
}

interface UserContextType {
  user: User | null
  token: string | null
  login: (username: string, password: string) => Promise<LoginResponse>
  logout: () => void
  updateUser: (user: User) => void
}

// TOTP 要求错误类
export class TOTPRequiredError extends Error {
  public response: LoginResponse

  constructor(response: LoginResponse) {
    super('TOTP verification required')
    this.name = 'TOTPRequiredError'
    this.response = response
  }
}

const UserContext = createContext<UserContextType | undefined>(undefined)

export const UserProvider = ({ children }: { children: ReactNode }) => {
  const [user, setUser] = useState<User | null>(null)
  const [token, setToken] = useState<string | null>(null)

  useEffect(() => {
    const storedToken = localStorage.getItem('token')
    const storedUsername = localStorage.getItem('username')
    const storedRole = localStorage.getItem('role')

    if (storedToken) {
      setToken(storedToken)
      if (storedUsername) {
        setUser({
          id: localStorage.getItem('userId') || '',
          username: storedUsername,
          email: localStorage.getItem('email') || undefined,
          role: storedRole || 'user',
        })
      }
    }
  }, [])

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

export const useUser = () => {
  const context = useContext(UserContext)
  if (!context) {
    throw new Error('useUser must be used within a UserProvider')
  }
  return context
}
