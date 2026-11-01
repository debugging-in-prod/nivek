// Matches `promo.Promo` (internal/libraries/promo/const.go).
export interface Promo {
  id: number
  channelname: string
  message: string
  interval_seconds: number
  enabled: boolean
  created_at: string
  updated_at: string
}
