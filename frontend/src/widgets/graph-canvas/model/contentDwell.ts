/** How long a card must stay in the strict viewport before its heavy body may start. 0 starts it as soon as the camera is still. */
export const CONTENT_DWELL_MS = 0

/**
 * Heavy card bodies (markdown, photos, audio covers) start only after the camera
 * has been still and the card has stayed inside the strict viewport for `dwellMs`.
 * Leaving the mounted area drops a warm card so the next visit waits again.
 */
export function planContent({
  now,
  idleSince,
  pointer,
  camMoved,
  visible,
  mounted,
  clocks,
  warm,
  dwellMs = CONTENT_DWELL_MS,
}: {
  now: number
  idleSince: number
  pointer: boolean
  camMoved: boolean
  visible: readonly string[]
  mounted: readonly string[]
  clocks: ReadonlyMap<string, number>
  warm: ReadonlySet<string>
  dwellMs?: number
}): { idleSince: number; clocks: Map<string, number>; warm: Set<string>; idle: boolean; wait: number } {
  const moving = pointer || camMoved
  const since = moving ? now : idleSince
  const onMap = new Set(mounted)
  const nextClocks = new Map<string, number>()
  for (const key of visible) nextClocks.set(key, clocks.get(key) ?? now)
  const idle = !moving && now - since >= dwellMs
  const nextWarm = new Set<string>()
  for (const key of warm) if (onMap.has(key)) nextWarm.add(key)
  if (idle) for (const [key, at] of nextClocks) if (now - at >= dwellMs) nextWarm.add(key)
  let wait = idle ? Number.POSITIVE_INFINITY : dwellMs - (now - since)
  if (idle) {
    for (const [key, at] of nextClocks) {
      if (!nextWarm.has(key)) wait = Math.min(wait, dwellMs - (now - at))
    }
  }
  return {
    idleSince: since,
    clocks: nextClocks,
    warm: nextWarm,
    idle,
    wait: Number.isFinite(wait) ? Math.max(0, wait) : 0,
  }
}
