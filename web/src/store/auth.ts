import { create } from 'zustand'
import { setToken } from '../api/client'

export interface SessionUser {
  id: number
  username: string
  name: string
  role_code: string
}

interface AuthState {
  user: SessionUser | null
  setUser: (u: SessionUser | null) => void
  logout: () => void
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  setUser: (user) => set({ user }),
  logout: () => {
    setToken(null)
    set({ user: null })
  },
}))
