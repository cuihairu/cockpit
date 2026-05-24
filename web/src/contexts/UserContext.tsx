import { createContext, useContext, useState, useEffect, ReactNode } from 'react'
import { api } from '@/services/api'

interface User {
  id: string
  username: string
  email?: string
  role: string
}

interface UserContextType {
  user: User | null
  token: string | null
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  updateUser: (user: User) => void
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

  const login = async (username: string, password: string) => {
    const res = await api.login(username, password)
    const { token, user_id, username: userName } = res

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
