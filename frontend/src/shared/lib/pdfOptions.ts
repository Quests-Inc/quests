export interface PdfOptions {
  width: number
  height: number
  landscape: boolean
  top: number
  right: number
  bottom: number
  left: number
  font: 'serif' | 'sans' | 'mono' | 'gothic'
  font_size: number
  line_spacing: number
  text_color: string
  background_color: string
  background_image: number
  background_opacity: number
  background_fit: 'contain' | 'cover'
  number_format: 'none' | 'page' | 'total'
  number_position: 'top' | 'bottom'
  number_align: 'left' | 'center' | 'right'
  chapter_new_page: boolean
  mirror_margins: boolean
  binding: number
}
export const defaultPdfOptions: PdfOptions = {
  width: 210,
  height: 297,
  landscape: false,
  top: 16,
  right: 16,
  bottom: 18,
  left: 16,
  font: 'serif',
  font_size: 10,
  line_spacing: 1.45,
  text_color: '#1a1a1a',
  background_color: '#ffffff',
  background_image: 0,
  background_opacity: 1,
  background_fit: 'contain',
  number_format: 'total',
  number_position: 'bottom',
  number_align: 'center',
  chapter_new_page: true,
  mirror_margins: false,
  binding: 0,
}
export interface PdfPreset {
  id: string
  name: string
  options: PdfOptions
}
export const builtinPdfPresets: PdfPreset[] = [
  { id: 'standard', name: 'Стандартный', options: { ...defaultPdfOptions } },
  {
    id: 'compact',
    name: 'Компактный',
    options: { ...defaultPdfOptions, top: 12, right: 12, bottom: 12, left: 12, font_size: 9, line_spacing: 1.2 },
  },
  { id: 'reading', name: 'Для чтения', options: { ...defaultPdfOptions, font_size: 12, line_spacing: 1.65 } },
  { id: 'print', name: 'Для печати', options: { ...defaultPdfOptions, text_color: '#000000' } },
  {
    id: 'book-a5',
    name: 'Книга A5',
    options: {
      ...defaultPdfOptions,
      width: 148,
      height: 210,
      top: 14,
      right: 12,
      bottom: 14,
      left: 14,
      mirror_margins: true,
      binding: 6,
    },
  },
]
export function pdfOptionsError(o: PdfOptions): string {
  const range = (n: number, min: number, max: number) => Number.isFinite(n) && n >= min && n <= max
  if (!range(o.width, 80, 600) || !range(o.height, 80, 600)) return 'Ширина и высота: от 80 до 600 мм.'
  if ([o.top, o.right, o.bottom, o.left, o.binding].some((n) => !range(n, 0, 100)))
    return 'Поля и переплёт: от 0 до 100 мм.'
  const w = o.landscape ? Math.max(o.width, o.height) : Math.min(o.width, o.height)
  const h = o.landscape ? Math.min(o.width, o.height) : Math.max(o.width, o.height)
  if (w - o.left - o.right - o.binding < 40 || h - o.top - o.bottom < 40)
    return 'Для содержимого должно оставаться не менее 40 × 40 мм.'
  if (!range(o.font_size, 6, 24) || !range(o.line_spacing, 1, 2.5))
    return 'Шрифт: 6–24 пт; межстрочный интервал: 1–2,5.'
  if (!['serif', 'sans', 'mono', 'gothic'].includes(o.font)) return 'Выберите шрифт.'
  if (!/^#[\da-f]{6}$/i.test(o.text_color) || !/^#[\da-f]{6}$/i.test(o.background_color))
    return 'Цвет должен иметь формат #rrggbb.'
  if (
    !['none', 'page', 'total'].includes(o.number_format) ||
    !['top', 'bottom'].includes(o.number_position) ||
    !['left', 'center', 'right'].includes(o.number_align)
  )
    return 'Выберите формат и положение нумерации.'
  if (
    o.number_format !== 'none' &&
    ((o.number_position === 'top' && o.top < 10) || (o.number_position === 'bottom' && o.bottom < 10))
  )
    return 'Со стороны номера страницы нужно поле не менее 10 мм.'
  if (
    !Number.isSafeInteger(o.background_image) ||
    o.background_image < 0 ||
    !range(o.background_opacity, 0, 1) ||
    !['contain', 'cover'].includes(o.background_fit)
  )
    return 'Проверьте параметры фонового изображения.'
  if ([o.landscape, o.chapter_new_page, o.mirror_margins].some((v) => typeof v !== 'boolean'))
    return 'Неверный формат настроек.'
  return ''
}
export function restorePdfOptions(value: unknown): PdfOptions | null {
  if (!value || typeof value !== 'object') return null
  const o = { ...defaultPdfOptions, ...value } as PdfOptions
  return pdfOptionsError(o) ? null : o
}
export function savePdfBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
