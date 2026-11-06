import { api } from './api'
export interface MusicQuery { title: string; artist: string; album: string; duration: number }
export interface OnlineMusic { matched: boolean; source?: string; lyrics?: string; cover?: string }
export function matchOnlineMusic(query: MusicQuery, signal: AbortSignal): Promise<OnlineMusic> {
  return api.music.metadata(query, signal)
}
