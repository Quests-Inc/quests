export type GalleryImage = { src: string; title: string }
export const imageViewer = $state({ images: [] as GalleryImage[], index: 0, open: false })
export function showImages(images: GalleryImage[], src: string) {
  const unique = images.filter((image, i) => image.src && images.findIndex((x) => x.src === image.src) === i)
  const index = unique.findIndex((image) => image.src === src)
  if (index < 0) return
  imageViewer.images = unique
  imageViewer.index = index
  imageViewer.open = true
}
export function attachmentImage(a: { id: number | string; filename?: string; revision?: number }): GalleryImage {
  return {
    src: `/api/attachments/${a.id}/image?preview=1${a.revision ? `&revision=${a.revision}` : ''}`,
    title: a.filename || `attachment=${a.id}`,
  }
}
