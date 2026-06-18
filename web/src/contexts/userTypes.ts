import type { LoginResponse } from '@/types'

export interface User {
  id: string
  username: string
  email?: string
  role: string
}

export interface UserContextType {
  user: User | null
  token: string | null
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
