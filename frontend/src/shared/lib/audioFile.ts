const AUDIO_EXT = new Set(['mp3', 'wav', 'flac', 'ogg', 'oga', 'm4a', 'aac', 'opus', 'weba'])

export type AudioFile = {
  id?: number | string
  filename?: string
  title?: string
  hidden?: boolean
  revision?: number
  content_type_detected?: string
  mime?: string
}

/** A sound recording the player can load. Video stays a download. */
export function isAudioFile(a: AudioFile | null | undefined) {
  if (!a) return false
  const mime = (a.content_type_detected || a.mime || '').split(';')[0].trim().toLowerCase()
  if (mime.startsWith('audio/')) return true
  const name = a.filename || a.title || ''
  const ext = ((/\.([A-Za-z0-9]{1,8})$/.exec(name) || [])[1] || '').toLowerCase()
  return AUDIO_EXT.has(ext)
}
