// Лёгкий оттенок карточки карты своим цветом: квест и шаг — цвет квестлайна, заметка и квестлайн — свой, вложение — цвет типа файла.
// Оттенок только смешивается с цветом самой сущности (TINT_SHARE), чтобы различать карточки на большой карте, а не перекрашивать их.
import { fileKind } from '@/shared/lib/fileKind.ts'

/** Цвет иконки по умолчанию: он ничего не говорит о карточке, оттенка нет. */
export const DEFAULT_ICON_COLOR = '#9a9a9a'
/** Доля оттенка в итоговом цвете карточки, %. */
export const TINT_SHARE = 30

const own = (c?: string | null) => (c && c.trim().toLowerCase() !== DEFAULT_ICON_COLOR ? c : '')

type Node = { key: string; type: string; title: string; mime?: string; color?: string }

/** Оттенок карточки `n`; `owner` — квест шага (по ребру принадлежности). Пустая строка — без оттенка. */
export function tintOf(n: Node, owner?: { color?: string } | null): string {
  if (n.type === 'quest' || n.type === 'note' || n.type === 'questline') return own(n.color)
  if (n.type === 'step') return own(owner?.color)
  if (n.type === 'attachment') {
    const k = fileKind({ filename: n.title, content_type_detected: n.mime })
    return k.kind === 'other' ? '' : k.color
  }
  return ''
}

/** Итоговый цвет карточки: цвет типа с долей оттенка. */
export const tinted = (base: string, tint: string) =>
  tint ? `color-mix(in srgb, ${base} ${100 - TINT_SHARE}%, ${tint} ${TINT_SHARE}%)` : base
