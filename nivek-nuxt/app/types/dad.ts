// Matches `dad.DadResponse` (internal/libraries/dad/const.go).
export interface DadResponse {
  id: number
  channelname: string | null
  response: string
  is_global: boolean
  use_count: number
  created_at: string
  updated_at: string
}
