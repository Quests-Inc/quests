import { paintedRect } from '@/entities/graph'

/** Resolve a coarse image level from projected card size and its visible fraction. */
export function imagePixels(
  n: { x: number; y: number; w: number; h: number },
  cam: { x: number; y: number; k: number },
  stage: { w: number; h: number },
  dpr = 1
) {
  const box = paintedRect(n)
  const w = box.w * cam.k,
    h = box.h * cam.k,
    x = (box.x - box.w / 2) * cam.k + cam.x,
    y = (box.y - box.h / 2) * cam.k + cam.y
  const visibleW = Math.max(0, Math.min(stage.w, x + w) - Math.max(0, x)),
    visibleH = Math.max(0, Math.min(stage.h, y + h) - Math.max(0, y))
  const fraction = w * h > 0 ? (visibleW * visibleH) / (w * h) : 0
  return Math.max(w, h) * dpr * Math.sqrt(Math.max(0.0625, fraction))
}

export const imageLevel = (pixels: number) => [128, 256, 512, 1024, 2048].find((size) => size >= pixels) ?? 0

/** Small JPEG of an audio cover. Level 0 keeps the embedded picture; otherwise it is scaled down. */
export async function coverPreviewUrl(blob: Blob, level: number): Promise<string> {
  if (!level || typeof createImageBitmap !== 'function' || typeof document === 'undefined')
    return URL.createObjectURL(blob)
  try {
    const bmp = await createImageBitmap(blob, { resizeWidth: level, resizeQuality: 'low' })
    const canvas = document.createElement('canvas')
    canvas.width = bmp.width
    canvas.height = bmp.height
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      bmp.close?.()
      return URL.createObjectURL(blob)
    }
    ctx.drawImage(bmp, 0, 0)
    bmp.close?.()
    const out = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', 0.72))
    return URL.createObjectURL(out ?? blob)
  } catch {
    return URL.createObjectURL(blob)
  }
}
