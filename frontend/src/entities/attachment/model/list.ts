// Список вложений: как отличить картинку, подпись размера и даты, фильтры поиска, сортировка и группировка по владельцу.

export const ownerKind = (a: any): string => a.owner_type || 'quest'

export function isRaster(a: any) {
  const t = (a.content_type_detected || '').split(';')[0].trim().toLowerCase()
  return t === 'image/png' || t === 'image/jpeg' || t === 'image/gif' || t === 'image/webp'
}

export function thumbSrc(a: any) {
  return `/api/attachments/${a.id}/image?preview=1&revision=${a.revision || 1}`
}

export function extOf(a: any) {
  const m = /\.([A-Za-z0-9]{1,6})$/.exec(a.filename || '')
  return m ? m[1].toUpperCase() : ''
}

/** Filters typed into the search box; the segment buttons below read and write the same tokens. */
export const ATT_SPEC = {
  flag: {
    label: 'флаг',
    all: true,
    values: [{ id: 'hidden', label: 'скрыто' }],
    test: (a: { hidden: any }) => !!a.hidden,
  },
  show: { label: 'показать', values: [{ id: 'all', label: 'и скрытые тоже' }], test: () => true },
  owner: {
    label: 'владелец',
    values: [
      { id: 'quest', label: 'квест' },
      { id: 'questline', label: 'квестлайн' },
      { id: 'note', label: 'заметка' },
    ],
    test: (a: any, v: any) => ownerKind(a) === v,
  },
  ftype: {
    label: 'тип файла',
    hint: 'pdf|png|image',
    test: (a: { content_type_detected: any; content_type_declared: any; filename: any }, v: string) => {
      const want = v.toLowerCase().replace(/^\./, '')
      const mime = (a.content_type_detected || a.content_type_declared || '').split(';')[0].trim().toLowerCase()
      const ext = ((/\.([A-Za-z0-9]{1,8})$/.exec(a.filename || '') || [])[1] || '').toLowerCase()
      return want === ext || want === mime || want === mime.split('/')[0] || want === mime.split('/')[1]
    },
  },
  rev: {
    label: 'число версий',
    hint: 'N|>N|<N',
    test: (a: { revision_count: number }, v: string) => {
      const m = /^(>=|<=|>|<|=)?(\d+)$/.exec(v)
      if (!m) return false
      const n = Number(m[2])
      const c = a.revision_count || 1
      return { '>': c > n, '<': c < n, '>=': c >= n, '<=': c <= n }[m[1]] ?? c === n
    },
  },
}

export function formatSize(n: number) {
  const bytes = Number(n) || 0
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function formatWhen(iso: string, full: boolean = false) {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleString(
      'ru-RU',
      full
        ? { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }
        : { day: '2-digit', month: 'short', year: '2-digit' }
    )
  } catch {
    return iso
  }
}

export type AttachmentSort = 'new' | 'old' | 'name' | 'size'

/** Sorted copy: newest / oldest upload, by name, by size (biggest first). */
export function sortAttachments(list: any[], sort: AttachmentSort | string): any[] {
  const byWhen = (x: { uploaded_at: any; id: number }, y: { uploaded_at: any; id: number }) =>
    (y.uploaded_at || '').localeCompare(x.uploaded_at || '') || y.id - x.id
  const cmp: Record<string, (x: any, y: any) => number> = {
    new: byWhen,
    old: (x, y) => -byWhen(x, y),
    name: (x, y) => (x.filename || '').localeCompare(y.filename || '', 'ru'),
    size: (x, y) => (y.size_bytes || 0) - (x.size_bytes || 0),
  }
  return [...list].sort(cmp[sort] ?? byWhen)
}

/** Files grouped by owner, groups ordered by owner title. */
export function groupByOwner(files: any[], ownerLabel: (a: any) => string) {
  const m = new Map<string, { key: string; kind: string; id: number; title: string; files: any[] }>()
  for (const a of files) {
    const kind = ownerKind(a)
    const key = `${kind}:${a.owner_id}`
    let g = m.get(key)
    if (!g) {
      g = { key, kind, id: Number(a.owner_id), title: ownerLabel(a), files: [] }
      m.set(key, g)
    }
    g.files.push(a)
  }
  return [...m.values()].sort((x, y) => x.title.localeCompare(y.title, 'ru'))
}
