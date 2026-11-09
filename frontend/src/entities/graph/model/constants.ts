import { ENTITY_COLORS } from '@/shared/lib/entityColors.ts'

// Справочники карты связей: как выглядят узлы и рёбра каждого вида, размеры карточек, режимы раскладки.

export const TYPES: Record<string, any> = {
  quest: { label: 'Квест', glyph: '◆', color: ENTITY_COLORS.quest },
  step: { label: 'Шаг', glyph: '•', color: ENTITY_COLORS.step },
  note: { label: 'Заметка', glyph: '¶', color: ENTITY_COLORS.note },
  attachment: { label: 'Вложение', glyph: '⎘', color: ENTITY_COLORS.attachment },
  questline: { label: 'Квестлайн', glyph: '☰', color: ENTITY_COLORS.questline },
  template: { label: 'Шаблон', glyph: '⟳', color: ENTITY_COLORS.template },
  cluster: { label: 'Вложения', glyph: '⎘', color: ENTITY_COLORS.attachment },
}
export const EDGES: Record<string, any> = {
  structure: { label: 'Принадлежность', color: '#6e6e6e' },
  reference: { label: 'Ссылка', color: '#c9a227' },
  dependency: { label: 'Зависимость', color: '#d0699b' },
}
/** Attachments of one owner collapse into a cluster beyond this many. */
export const CLUSTER_MIN = 6
/** Card width / max height (px) by kind; text beyond the height scrolls. */
export const CARD: Record<string, any> = {
  quest: { w: 300, max: 260 },
  note: { w: 320, max: 300 },
  stepBody: { w: 240, max: 120 },
}
export const SQUARE = 96
export const RASTER = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp'])

export const LAYOUTS = {
  tree: 'Дерево',
  down: 'Дерево сверху вниз',
  radial: 'Радиальная',
  layers: 'Слоями по типам',
  free: 'Свободно',
}

/** Виды сущностей, которые можно показывать на карте. */
export const ENTITY_TYPES = ['questline', 'quest', 'step', 'note', 'attachment']
export const ENTITY_LABELS: Record<string, string> = {
  questline: 'Квестлайны',
  quest: 'Квесты',
  step: 'Шаги',
  note: 'Заметки',
  attachment: 'Вложения',
}
