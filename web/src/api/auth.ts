import { api } from './client'

export interface User {
  steam64id: string
  display_name: string
  is_owner: boolean
  is_active: boolean
  must_reset_pw: boolean
}

export const authApi = {
  me: () => api.get<{ user: User }>('/api/auth/me', { silent401: true }),
  login: (steam64id: string, password: string) =>
    api.post<{ user: User }>('/api/auth/login', { steam64id, password }, { silent401: true }),
  register: (steam64id: string, password: string, display_name: string) =>
    api.post<{ user: User }>(
      '/api/auth/register',
      { steam64id, password, display_name },
      { silent401: true }
    ),
  logout: () => api.post<{ status: string }>('/api/auth/logout'),
}