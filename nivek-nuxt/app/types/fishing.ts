// Matches `fishing.FishScore` and `fishing.Fish`
// (internal/libraries/fishing/const.go).
export interface Fish {
  value: number
  name: string
  scarcity: number
}

export interface FishScore {
  id: number
  channelname: string
  chattername: string
  score: number
  fish: Fish[]
  trash_caught: number
  times_fished: number
  created_at: string
  updated_at: string
}

export type FishScoresResponse = { as_channel: FishScore[]; as_chatter: FishScore[] }
