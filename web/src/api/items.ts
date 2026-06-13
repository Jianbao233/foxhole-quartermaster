import { api } from './client'

export interface Item {
  canonical_id: string
  name_zh: string
  name_en: string | null
  faction: string | null
  category: string | null
  crate_size: number | null
  icon_source: string
  icon_path: string | null
  wiki_url: string | null
}

export interface ItemAggregate {
  stockpile_id: number
  region: string
  building_name: string
  player_alias: string
  loose: number
  crates: number
  captured_at: number
}

export interface ItemDetail {
  item: Item
  war_id: number
  stockpiles: ItemAggregate[] | null
}

export const itemsApi = {
  list: () => api.get<{ items: Item[] }>('/api/items'),
  detail: (cid: string, warID?: number) =>
    api.get<ItemDetail>(
      warID ? `/api/items/${encodeURIComponent(cid)}?war_id=${warID}` : `/api/items/${encodeURIComponent(cid)}`
    ),
}