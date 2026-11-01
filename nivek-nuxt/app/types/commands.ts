// Command shapes match `commands.Commands` and `commands.CommandSetting` in
// internal/libraries/commands (const.go, editor.go).

// A row of nivek.command. Returned by the public GET /commands (global,
// enabled) and by GET /channel/commands (the session channel's custom ones).
export interface Command {
  id: number
  trigger: string
  kind: string
  handler_key: string | null
  response_tmpl: string | null
  description: string
  scope: string
  min_role: string
  cooldown_secs: number
  enabled: boolean
}

export type CommandsResponse = { commands: Command[] }

// A global command merged with the channel's override. The non-default
// fields are null when the channel inherits the default.
export interface CommandSetting {
  command_id: number
  trigger: string
  description: string
  default_enabled: boolean
  default_min_role: string
  default_cooldown_secs: number
  enabled: boolean | null
  min_role: string | null
  cooldown_secs: number | null
}

export type CommandSettingsResponse = { settings: CommandSetting[] }
