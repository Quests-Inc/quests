const cache = new Map<string, string>()
/** Approximate line rhythm without layout/Markdown parsing. One cached SVG path per text, at most 32 bars. */
export function outlinePath(text: string) {
  const source = text.slice(0, 4096)
  const hit = cache.get(source)
  if (hit !== undefined) return hit
  const lines = source
    .split('\n')
    .flatMap((line) => {
      if (!line.trim()) return ['']
      return line.match(/.{1,64}/gu) ?? []
    })
    .slice(0, 32)
  const height = 100 / Math.max(12, lines.length)
  const path = lines
    .map((line, i) => {
      if (!line) return ''
      const width = Math.max(3, Math.min(96, ([...line].length / 64) * 96))
      return `M2 ${i * height + 1}h${width}v${height * 0.45}H2z`
    })
    .join('')
  cache.set(source, path)
  if (cache.size > 400) cache.delete(cache.keys().next().value!)
  return path
}
