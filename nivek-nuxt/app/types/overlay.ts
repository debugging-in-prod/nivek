// Matches `overlayrelay.Device` (internal/libraries/overlayrelay/const.go).
export interface OverlayDevice {
  id: number
  user_id: number
  label: string
  created_at: string
  last_seen_at?: string | null
  revoked_at?: string | null
  connected: boolean
}

export type OverlayDevicesResponse = { devices: OverlayDevice[] }

// The plaintext token exists only in this create response.
export type CreateOverlayDeviceResponse = { device: OverlayDevice; token: string }
