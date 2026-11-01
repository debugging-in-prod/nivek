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

// Roles accepted for min_role. Mirrors `validRoles` in
// internal/libraries/commands/validate.go (and the CHECK constraint on
// nivek.command.min_role).
export const ROLES = ['everyone', 'sub', 'vip', 'mod', 'broadcaster'] as const

// Dashboard cooldown limits. Mirror `MinCooldownSecs`/`MaxCooldownSecs` in
// internal/libraries/commands/validate.go; the server enforces them.
export const MIN_COOLDOWN = 5
export const MAX_COOLDOWN = 3600
