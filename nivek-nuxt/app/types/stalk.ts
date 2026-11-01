// Response of GET /stalk (cmd/core-api/endpoints/stalk/get.go).
export interface StalkConfig {
  found: boolean
  target: string
  last_message: string
  set_by: string
}
