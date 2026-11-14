import { serverLimits } from '@/shared/lib/serverLimits.ts'

/** Compatible with an older API during a rolling update; such an API uses the defaults. */
export async function refreshServerLimits() {
  const response = await fetch('/api/capabilities', { credentials: 'same-origin' })
  if (!response.ok) return
  const result = (await response.json()) as { limits: Record<string, number> }
  for (const key of Object.keys(serverLimits) as (keyof typeof serverLimits)[]) {
    const value = result.limits?.[key]
    if (Number.isInteger(value) && value >= 1 && value <= 1_048_576) serverLimits[key] = value
  }
}
