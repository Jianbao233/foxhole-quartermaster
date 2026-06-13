import { api } from './client'
import type { Item } from './items'

export interface AdminUser {
  steam64id: string
  display_name: string
  is_owner: boolean
  is_active: boolean
  must_reset_pw: boolean
  registered: boolean
  created_at: number
  registered_at: number
}

export interface AdminStats {
  user_count: number
  item_count: number
  placeholder_items: number
  stockpile_count: number
  snapshot_count: number
  active_war_label: string
  active_war_started: number
  db_path: string
  db_size_bytes: number
}

export interface UpsertItemReq {
  canonical_id: string
  name_zh: string
  name_en?: string
  faction?: string
  category?: string
  crate_size?: number | null
  wiki_url?: string
  aliases?: string[]
}

export const adminApi = {
  stats: () => api.get<AdminStats>('/api/admin/stats'),
  users: () => api.get<{ users: AdminUser[] }>('/api/admin/users'),
  addWhitelist: (steam64id: string, display_name: string) =>
    api.post<{ status: string }>('/api/admin/whitelist', { steam64id, display_name }),
  resetUser: (id: string) => api.post<{ status: string }>(`/api/admin/users/${id}/reset`),
  kickUser: (id: string) => api.post<{ status: string }>(`/api/admin/users/${id}/kick`),
  newWar: (label: string, notes: string) =>
    api.post<{ war_id: number }>('/api/admin/wars/new', { label, notes }),
  endWar: (id: number) => api.post<{ status: string }>(`/api/admin/wars/${id}/end`),
  deleteWar: (id: number, confirm_text: string) =>
    api.post<{ status: string }>(`/api/admin/wars/${id}/delete`, { confirm_text }),
  listItems: () => api.get<{ items: Item[] }>('/api/admin/items'),
  upsertItem: (req: UpsertItemReq) => api.post<{ status: string }>('/api/admin/items', req),
  exportWar: (id: number) => api.get<any>(`/api/admin/war/${id}/export`),
}