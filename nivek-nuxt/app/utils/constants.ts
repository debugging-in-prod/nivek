// Entries are relative to the api() baseURL ('/api'), except the full-page
// navigation links (TwitchStart, OverlayDownload), which are used as plain
// hrefs and so include the '/api' prefix.
export const API_ROUTES = {
  TwitchStart: '/api/auth/twitch/start',
  OverlayDownload: '/api/overlay/download',
  Public: {
    Commands: '/commands',
    Stats: '/stats/public',
    DfSnapshot: '/df/snapshot',
  },
  Secure: {
    Profile: '/profile',
    Logout: '/logout',
    OverlayDevices: '/overlay/device',
    PutBotOptIn: '/bot-optin',
    Weather: '/weather',
    GetFishScore: '/fishing',
    GetAutoShoutChatters: '/auto-shout',
    PostCreateAutoShoutChatter: '/auto-shout',
    PutUpdateAutoShoutChatter: (id: number) => `/auto-shout/${id}`,
    DeleteAutoShoutChatter: (id: number) => `/auto-shout/${id}`,
    GetDadResponses: '/dad',
    PostCreateDadResponse: '/dad',
    DeleteDadResponse: (id: number) => `/dad/${id}`,
    GetPromos: '/promo',
    PostCreatePromo: '/promo',
    PutUpdatePromo: (id: number) => `/promo/${id}`,
    DeletePromo: (id: number) => `/promo/${id}`,
    GetChannelCommands: '/channel/commands',
    PostCreateChannelCommand: '/channel/commands',
    PutUpdateChannelCommand: (id: number) => `/channel/commands/${id}`,
    DeleteChannelCommand: (id: number) => `/channel/commands/${id}`,
    GetCommandSettings: '/channel/command-settings',
    PutCommandSetting: (id: number) => `/channel/command-settings/${id}`,
    DeleteCommandSetting: (id: number) => `/channel/command-settings/${id}`,
    GetStalk: '/stalk',
    PostStalk: '/stalk',
    DeleteStalk: '/stalk',
  },
} as const

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
