/** Resolve a coarse image level from projected card size and its visible fraction. */
export function imagePixels(
  n: { x: number; y: number; w: number; h: number },
  cam: { x: number; y: number; k: number },
  stage: { w: number; h: number },
  dpr = 1
) {
  const w = n.w * cam.k,
    h = n.h * cam.k,
    x = (n.x - n.w / 2) * cam.k + cam.x,
    y = (n.y - n.h / 2) * cam.k + cam.y
  const visibleW = Math.max(0, Math.min(stage.w, x + w) - Math.max(0, x)),
    visibleH = Math.max(0, Math.min(stage.h, y + h) - Math.max(0, y))
  const fraction = w * h > 0 ? (visibleW * visibleH) / (w * h) : 0
  return Math.max(w, h) * dpr * Math.sqrt(Math.max(0.0625, fraction))
}

export const imageLevel = (pixels: number) => [128, 256, 512, 1024, 2048].find((size) => size >= pixels) ?? 0
