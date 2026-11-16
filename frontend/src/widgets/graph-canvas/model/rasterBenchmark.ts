type Cam = { x: number; y: number; k: number }
export function benchmarkRaster(node: HTMLElement, get: () => Cam, set: (c: Cam) => void) {
  const original = { ...get() }
  let cancelled = false,
    mode = 'new'
  const apply = () => {
    for (const el of node.querySelectorAll<HTMLElement>('.gcard')) {
      const old = mode === 'old'
      el.style.willChange = old ? 'transform' : 'auto'
      const value = el.style.transform
      if (old && value.startsWith('translate('))
        el.style.transform = value.replace(/^translate\((.*)\)$/, 'translate3d($1, 0)')
      if (!old && value.startsWith('translate3d('))
        el.style.transform = value.replace(/^translate3d\((.*), 0(?:px)?\)$/, 'translate($1)')
    }
  }
  const observer = new MutationObserver(apply)
  observer.observe(node, { childList: true, subtree: true })
  const next = () => new Promise<number>((resolve) => requestAnimationFrame(resolve))
  const results: object[] = []
  const quantile = (x: number[], q: number) => [...x].sort((a, b) => a - b)[Math.floor((x.length - 1) * q)]
  const run = async (scenario: string, variant: string) => {
    mode = variant
    apply()
    const width = node.clientWidth,
      height = node.clientHeight
    const cx = (width / 2 - original.x) / original.k,
      cy = (height / 2 - original.y) / original.k
    const move = (t: number) => {
      const k =
        scenario === 'pan'
          ? original.k
          : Math.exp(Math.log(0.05) + Math.log(50) * (0.5 - 0.5 * Math.cos(t * Math.PI * 2)))
      set({
        k,
        x: width / 2 - cx * k + Math.sin(t * Math.PI * 2) * 140,
        y: height / 2 - cy * k + Math.cos(t * Math.PI * 2) * 80,
      })
      apply()
    }
    for (let i = 0; i < 60; i++) {
      if (cancelled) break
      move(i / 60)
      await next()
    }
    const frames: number[] = []
    let last = await next(),
      maxCards = 0
    for (let i = 0; i < 120; i++) {
      if (cancelled) break
      move(i / 120)
      const now = await next()
      frames.push(now - last)
      last = now
      maxCards = Math.max(maxCards, node.querySelectorAll('.gcard').length)
    }
    if (!frames.length) return
    const summary = {
      scenario,
      variant,
      frames: frames.length,
      median: quantile(frames, 0.5),
      p95: quantile(frames, 0.95),
      max: Math.max(...frames),
      over33: frames.filter((t) => t > 33.5).length,
      over50: frames.filter((t) => t > 50.5).length,
      maxCards,
    }
    results.push(summary)
    console.debug('RASTER_BENCH_SAMPLE', JSON.stringify(summary))
  }
  void (async () => {
    await next()
    await next()
    for (const variant of ['old', 'new', 'new', 'old'])
      for (const scenario of ['pan', 'zoom']) {
        if (cancelled) return
        await run(scenario, variant)
      }
    mode = 'new'
    apply()
    set(original)
    observer.disconnect()
    console.debug('RASTER_BENCH_DONE', JSON.stringify(results))
  })()
  return () => {
    cancelled = true
    observer.disconnect()
    set(original)
  }
}
