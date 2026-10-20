// Контекстные меню карты: по полю, по карточке, по шару вложений, по линии, по выделенным.
import { REF_FROM } from './links.ts'

export interface MenuItem {
  id: string
  label?: string
  sep?: boolean
  danger?: boolean
  children?: MenuItem[]
}

type Node = { key: string; type: string; id?: number | string; draft?: boolean; op?: number }

export interface MenuContext {
  node: Node | null
  group: boolean
  cluster?: { folded: boolean; key: string | null } | null
  edge?: { draft?: string; from: string; to: string; kind: string } | null
}

/** Things the map can be centred on. */
export const CENTERABLE = new Set(['quest', 'note', 'questline'])

/** What can be done to a line itself: light up its two ends, and (not for plain ownership of steps and files) change the link. */
export function edgeMenuItems(
  e: { draft?: string; from: string; to: string; kind: string },
  lit?: { from: string; to: string; kind: string } | null
): MenuItem[] {
  if (e.draft === 'remove' || !e.from || !e.to) return []
  const own = ownEdgeItems(e)
  const on = lit != null && lit.from === e.from && lit.to === e.to && lit.kind === e.kind
  return [
    { id: 'edge-light', label: on ? 'Снять подсветку' : 'Подсветить узлы' },
    ...(own.length ? [{ id: 'sep-edge', sep: true }, ...own] : []),
  ]
}

function ownEdgeItems(e: { draft?: string; from: string; to: string; kind: string }): MenuItem[] {
  if (e.kind === 'reference') return [{ id: 'edge-unref', label: 'Разорвать ссылку' }]
  if (e.kind === 'dependency' && e.from.startsWith('step:') && e.to.startsWith('step:')) {
    return [{ id: 'edge-nowait', label: 'Не ждать предыдущий шаг' }]
  }
  if (e.kind === 'structure' && e.from.startsWith('questline:') && e.to.startsWith('quest:') && !e.to.includes(':t')) {
    return [{ id: 'edge-unline', label: 'Вывести квест из квестлайна' }]
  }
  if (e.kind === 'structure' && e.from.startsWith('note:') && e.to.startsWith('note:')) {
    return [{ id: 'edge-unparent', label: 'Открепить заметку' }]
  }
  return []
}

export function graphMenuItems(
  ctx: MenuContext,
  env: {
    selectMode: boolean
    pickedCount: number
    lit?: { from: string; to: string; kind: string } | null
    rootKey?: string
    nodes: Node[]
    expandKeys: string[]
    openTarget: (n: Node) => unknown
    isExtraRoot: (n: Node) => boolean
    ownExpand: (key: string) => unknown[]
    outgoingRefs: (n: Node) => string[]
    childKeys: (n: Node, type: string) => string[]
    parentNote: (n: Node) => string | null
    titleOf: (key: string) => string
  }
): MenuItem[] {
  const n = ctx.node
  if (ctx.group)
    return [
      { id: 'hide-picked', label: `Убрать с карты (${env.pickedCount})` },
      { id: 'clear-picked', label: 'Снять выделение' },
    ]
  if (ctx.edge) return edgeMenuItems(ctx.edge, env.lit)
  if (ctx.cluster) {
    return [
      ctx.cluster.folded
        ? { id: 'cl-open', label: 'Развернуть' }
        : ctx.cluster.key
          ? { id: 'cl-fold', label: 'Свернуть' }
          : null,
      { id: 'cl-disband', label: 'Расформировать' },
    ].filter((x) => x !== null)
  }
  if (!n) {
    return [
      {
        id: 'create',
        label: 'Создать',
        children: [
          { id: 'new-quest', label: 'Квест' },
          { id: 'new-note', label: 'Заметку' },
          { id: 'new-step', label: 'Шаг' },
          { id: 'new-questline', label: 'Квестлайн' },
        ],
      },
      { id: 'upload', label: 'Загрузить файл…' },
      { id: 'find', label: 'Добавить на карту…' },
      { id: 'sep-sel', sep: true },
      { id: 'select-mode', label: env.selectMode ? 'Выйти из режима выделения' : 'Режим выделения' },
      { id: 'fit', label: 'Вписать в экран' },
    ]
  }
  const items: MenuItem[] = []
  if (!n.draft) {
    if (['quest', 'note', 'step', 'attachment', 'questline'].includes(n.type) && typeof n.id === 'number')
      items.push({ id: 'copy-ref', label: `Копировать ${n.type}=${n.id}` }, { id: 'sep-ref', sep: true })
    if (env.openTarget(n)) items.push({ id: 'open', label: 'Открыть' })
    if (CENTERABLE.has(n.type) && n.key !== env.rootKey) items.push({ id: 'center', label: 'Сделать центром' })
    if (n.key !== env.rootKey) items.push({ id: env.isExtraRoot(n) ? 'unroot' : 'hide', label: 'Убрать с карты' })
    else if (env.nodes.every((x) => x.key === n.key || x.draft)) items.push({ id: 'hide', label: 'Убрать с карты' })
  }
  // What can be changed, duplicated, or put into this card.
  const own: MenuItem[] = []
  if (n.type === 'quest' || n.type === 'step' || n.type === 'note' || (n.type === 'questline' && n.draft))
    own.push({ id: 'edit', label: 'Изменить…' })
  if (!n.draft && ['quest', 'note', 'step', 'attachment', 'questline'].includes(n.type))
    own.push({ id: 'copy', label: 'Копировать' })
  const add: MenuItem[] = []
  if (n.type === 'quest') add.push({ id: 'add-step', label: 'Шаг' })
  if (n.type === 'quest' || n.type === 'note') add.push({ id: 'add-file', label: 'Вложение…' })
  if (n.type === 'quest' || n.type === 'step' || n.type === 'questline') add.push({ id: 'add-note', label: 'Заметку' })
  if (n.type === 'questline') add.push({ id: 'add-quest-line', label: 'Квест' })
  if (!n.draft && (n.type === 'quest' || n.type === 'note'))
    add.push({ id: 'new-quest-ref', label: 'Квест со ссылкой сюда' })
  if (add.length) own.push({ id: 'add', label: 'Добавить', children: add })
  if (own.length) {
    if (items.length) items.push({ id: 'sep-own', sep: true })
    items.push(...own)
  }
  if (REF_FROM.has(n.type)) {
    if (items.length) items.push({ id: 'sep-link', sep: true })
    items.push({ id: 'link-to', label: 'Добавить ссылку на…' })
    items.push({ id: 'link-from', label: 'Отметить как ссылку для…' })
    const out = env.outgoingRefs(n)
    if (out.length) {
      items.push({
        id: 'unlink',
        label: 'Убрать ссылку…',
        children: out.map((k) => ({ id: `unlink:${k}`, label: `→ ${env.titleOf(k)}` })),
      })
    }
  }
  // Exploring: bring in what this card is linked to, or take its steps / quests off.
  if (!n.draft) {
    const explore: MenuItem[] = []
    if (env.ownExpand(n.key).length) explore.push({ id: 'collapse', label: 'Свернуть связи' })
    if (!env.expandKeys.includes(n.key)) explore.push({ id: 'expand', label: 'Показать все связи' })
    if (n.type === 'quest' && env.childKeys(n, 'step').length) explore.push({ id: 'drop-steps', label: 'Убрать шаги' })
    if (n.type === 'questline' && env.childKeys(n, 'quest').length)
      explore.push({ id: 'drop-quests', label: 'Убрать квесты' })
    if (items.length) items.push({ id: 'sep-explore', sep: true })
    items.push(...explore)
  }
  if (n.type === 'step') items.push({ id: 'attach', label: 'Прикрепить к квесту…' })
  if (n.type === 'attachment') items.push({ id: 'attach', label: 'Прикрепить к…' })
  if (n.type === 'note') {
    items.push({ id: 'attach', label: 'Прикрепить к заметке…' })
    const up = env.parentNote(n)
    if (up) items.push({ id: 'detach-note', label: `Открепить от «${env.titleOf(up)}»` })
  }
  if (n.draft) {
    items.push({ id: 'sep-drop', sep: true })
    items.push({ id: 'drop', label: 'Убрать из черновика', danger: true })
  }
  return items
}
