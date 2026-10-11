/** Семейства файлов для вложений. Классификация: content_type_detected, затем расширение; declared не используется. */

/** @type {Record<string, { label: string, color: string }>} */
export const FILE_KINDS: Record<string, { label: string; color: string }> = {
  word: { label: 'Текстовый документ', color: 'var(--ft-word, #3a63c8)' },
  doc: { label: 'Документ', color: 'var(--ft-doc, #d2605a)' },
  sheet: { label: 'Таблица', color: 'var(--ft-sheet, #4fa56b)' },
  image: { label: 'Изображение', color: 'var(--ft-image, #4f9fc7)' },
  code: { label: 'Код / текст', color: 'var(--ft-code, #b48ad6)' },
  archive: { label: 'Архив', color: 'var(--ft-archive, #c8a03c)' },
  db: { label: 'База данных', color: 'var(--ft-db, #d9894a)' },
  media: { label: 'Медиа', color: 'var(--ft-media, #d0699b)' },
  other: { label: 'Прочее', color: 'var(--color-fg-muted, #9a9a9a)' },
}

const EXT = {
  word: 'doc docx odt rtf',
  doc: 'pdf epub djvu ppt pptx odp',
  sheet: 'xls xlsx ods csv tsv',
  image: 'png jpg jpeg gif webp svg bmp ico tif tiff avif heic',
  code: 'sql py js ts go rs md txt log json yaml yml toml xml html css sh dot c h cpp java ini cfg conf svelte',
  archive: 'zip tar gz tgz bz2 xz 7z rar zst',
  db: 'sqlite sqlite3 db mdb',
  media: 'mp3 wav flac ogg m4a mp4 mkv webm mov avi',
}
const BY_EXT = new Map()
for (const [k, v] of Object.entries(EXT)) for (const e of v.split(' ')) BY_EXT.set(e, k)

/** @param {string} mime */
function byMime(mime: string) {
  if (!mime) return ''
  if (mime.startsWith('image/')) return 'image'
  if (mime.startsWith('audio/') || mime.startsWith('video/')) return 'media'
  if (mime === 'text/plain') return ''
  if (mime.startsWith('text/')) return mime === 'text/csv' ? 'sheet' : 'code'
  if (/sqlite|x-sqlite3/.test(mime)) return 'db'
  if (/zip|tar|gzip|7z|rar|bzip|xz|zstd/.test(mime)) return 'archive'
  if (/msword|wordprocessing|opendocument\.text|rtf/.test(mime)) return 'word'
  if (/pdf|presentation|epub/.test(mime)) return 'doc'
  if (/spreadsheet|ms-excel/.test(mime)) return 'sheet'
  if (/json|xml|yaml|javascript|x-sh|sql/.test(mime)) return 'code'
  return ''
}

/** @param {any} a attachment @returns {{ kind: string, label: string, color: string, ext: string }} */
export function fileKind(a: any): { kind: string; label: string; color: string; ext: string } {
  const ext = ((/\.([A-Za-z0-9]{1,8})$/.exec(a?.filename || '') || [])[1] || '').toLowerCase()
  const mime = (a?.content_type_detected || '').split(';')[0].trim().toLowerCase()
  let kind = byMime(mime)
  // octet-stream / неопознанное — решает расширение; расширение точнее для sqlite/dot/md и т.п.
  const fromExt = BY_EXT.get(ext)
  if (!kind || kind === 'code' || fromExt === 'db') kind = fromExt || kind
  kind = kind || 'other'
  return { kind, ...FILE_KINDS[kind], ext: ext.toUpperCase() }
}
