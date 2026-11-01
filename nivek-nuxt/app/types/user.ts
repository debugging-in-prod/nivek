// Response of GET /profile.
export interface User {
  id: number
  username?: string
  created_at: string
  twitch_id: string | null
  twitch_login: string | null
  twitch_display_name: string | null
  bot_opt_in: boolean
  is_live: boolean
  // Whether this user may see the /overlay pairing page. Computed server-side
  // from the overlay allowlist (OVERLAY_DOWNLOAD_ALLOWLIST).
  overlay_access?: boolean
}
