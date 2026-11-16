import { isAudioFile, type AudioFile } from './audioFile.ts'

export type PlayerTrack = {
  id: number
  filename: string
  hidden: boolean
  revision: number
}

const prefsKey = 'quests.player'

export const player = $state({
  track: null as PlayerTrack | null,
  playing: false,
  pinned: false,
  loop: false,
  muted: false,
  volume: 1,
  current: 0,
  duration: 0,
  title: '',
  artist: '',
  cover: '',
  error: '',
})

function loadPrefs() {
  try {
    const raw = localStorage.getItem(prefsKey)
    if (!raw) return
    const p = JSON.parse(raw) as { pinned?: boolean; loop?: boolean; muted?: boolean; volume?: number }
    if (typeof p.pinned === 'boolean') player.pinned = p.pinned
    if (typeof p.loop === 'boolean') player.loop = p.loop
    if (typeof p.muted === 'boolean') player.muted = p.muted
    if (typeof p.volume === 'number' && p.volume >= 0 && p.volume <= 1) player.volume = p.volume
  } catch {
    /* A broken preference must not stop playback. */
  }
}

function savePrefs() {
  try {
    localStorage.setItem(
      prefsKey,
      JSON.stringify({
        pinned: player.pinned,
        loop: player.loop,
        muted: player.muted,
        volume: player.volume,
      })
    )
  } catch {
    /* Private mode can refuse the write; the session still plays. */
  }
}

loadPrefs()

export function audioSrc(track: PlayerTrack) {
  const q = new URLSearchParams()
  if (track.hidden) q.set('preview', '1')
  if (track.revision > 0) q.set('revision', String(track.revision))
  const s = q.toString()
  return `/api/attachments/${track.id}/audio${s ? `?${s}` : ''}`
}

/** off — another file; paused — this file is loaded; playing — this file is sounding. */
export function trackOf(id: number): 'off' | 'paused' | 'playing' {
  if (!player.track || player.track.id !== id) return 'off'
  return player.playing ? 'playing' : 'paused'
}

export function playAttachment(file: AudioFile) {
  if (!isAudioFile(file) || file.id == null) return
  const id = Number(file.id)
  if (!Number.isFinite(id)) return
  if (player.track?.id === id) {
    player.playing = !player.playing
    player.error = ''
    return
  }
  if (player.cover) URL.revokeObjectURL(player.cover)
  player.track = {
    id,
    filename: file.filename || file.title || `attachment=${id}`,
    hidden: !!file.hidden,
    revision: Number(file.revision) || 0,
  }
  player.playing = true
  player.current = 0
  player.duration = 0
  player.title = ''
  player.artist = ''
  player.cover = ''
  player.error = ''
}

export function stopPlayback() {
  if (player.cover) URL.revokeObjectURL(player.cover)
  player.track = null
  player.playing = false
  player.current = 0
  player.duration = 0
  player.title = ''
  player.artist = ''
  player.cover = ''
  player.error = ''
}

export function setPinned(pinned: boolean) {
  player.pinned = pinned
  savePrefs()
}

export function setLoop(loop: boolean) {
  player.loop = loop
  savePrefs()
}

export function setMuted(muted: boolean) {
  player.muted = muted
  savePrefs()
}

export function setVolume(volume: number) {
  player.volume = Math.min(1, Math.max(0, volume))
  if (player.volume > 0) player.muted = false
  savePrefs()
}
