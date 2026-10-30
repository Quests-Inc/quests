/** A world-space annotation, independent of entity cards and their layout. */
export type GraphZone = { id: string; title: string; color: string; x: number; y: number; w: number; h: number }

export function readZones(raw: unknown): GraphZone[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  return raw.slice(0, 200).flatMap((v) => {
    if (
      !v ||
      typeof v.id !== 'string' ||
      !v.id ||
      seen.has(v.id) ||
      ![v.x, v.y, v.w, v.h].every((n) => typeof n === 'number' && Number.isFinite(n)) ||
      v.w < 40 ||
      v.h < 40
    )
      return []
    seen.add(v.id)
    return [
      {
        id: v.id,
        title: String(v.title ?? '').slice(0, 120),
        color: /^#[\da-f]{6}$/i.test(v.color) ? v.color : '#6a8fbf',
        x: v.x,
        y: v.y,
        w: v.w,
        h: v.h,
      },
    ]
  })
}
