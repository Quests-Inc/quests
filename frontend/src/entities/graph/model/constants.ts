// Справочники карты связей: как выглядят узлы и рёбра каждого вида, размеры карточек, режимы раскладки.

export const TYPES: Record<string, any> = {
  quest: { label: 'Квест', glyph: '◆', color: '#c9a227' },
  step: { label: 'Шаг', glyph: '•', color: '#8c97a8' },
  note: { label: 'Заметка', glyph: '¶', color: '#b48ad6' },
  attachment: { label: 'Вложение', glyph: '⎘', color: '#4f9fc7' },
  questline: { label: 'Квестлайн', glyph: '☰', color: '#4fa56b' },
  template: { label: 'Шаблон', glyph: '⟳', color: '#d9894a' },
  cluster: { label: 'Вложения', glyph: '⎘', color: '#4f9fc7' },
}
export const EDGES: Record<string, any> = {
  structure: { label: 'Принадлежность', color: '#6e6e6e', dash: '' },
  reference: { label: 'Ссылка', color: '#c9a227', dash: '6 4' },
  dependency: { label: 'Зависимость', color: '#d0699b', dash: '' },
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
