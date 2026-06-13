import { api } from './client'

export interface War {
  id: number
  label: string
  started_at: number
  ended_at: number | null
  is_active: boolean
  notes: string
}

export interface Stockpile {
  id: number
  war_id: number
  region: string
  sub_region: string
  building_name: string
  visibility: string
  player_alias: string
  first_seen_at: number
  last_seen_at: number
  last_uploader: string
  last_captured: number
  snapshot_id: number
  item_count: number
}

export interface StockpileItem {
  canonical_id: string
  name_zh: string
  name_en: string | null
  category: string | null
  crate_size: number | null
  icon_source: string
  loose: number
  crates: number
  total: number
}

export interface StockpileDetail {
  stockpile: Stockpile & { captured_at: number }
  items: StockpileItem[]
}

export interface TimelineSeries {
  name: string
  data: [number, number][]
}

export interface TimelineResp {
  stockpile_id: number
  canonical_id: string
  series: TimelineSeries[]
}

export interface RecentUpload {
  id: number
  stockpile_id: number
  region: string
  building_name: string
  player_alias: string
  uploader_id: string
  uploader_name: string
  captured_at: number
  uploaded_at: number
  item_count: number
}

export interface IngestResult {
  stockpile_id: number
  snapshot_id: number
  war_id: number
  item_count: number
  duplicate: boolean
  captured_at: number
  region: string
  sub_region: string
  building: string
  visibility: string
  player_alias: string
}

export const warsApi = {
  list: () => api.get<{ wars: War[] }>('/api/wars'),
  active: () => api.get<{ war: War | null }>('/api/wars/active'),
}

export const stockpilesApi = {
  list: (warID?: number) =>
    api.get<{ war_id: number; stockpiles: Stockpile[] }>(
      warID ? `/api/stockpiles?war_id=${warID}` : '/api/stockpiles'
    ),
  detail: (id: number) => api.get<StockpileDetail>(`/api/stockpiles/${id}`),
  timeline: (id: number, item: string) =>
    api.get<TimelineResp>(`/api/stockpiles/${id}/timeline?item=${encodeURIComponent(item)}`),
  recent: (limit = 50) => api.get<{ uploads: RecentUpload[] }>(`/api/uploads?limit=${limit}`),
}

export const ingestApi = {
  submit: (raw_text: string, player_alias: string, is_new_alias: boolean) =>
    api.post<IngestResult>('/api/ingest', { raw_text, player_alias, is_new_alias }),
}