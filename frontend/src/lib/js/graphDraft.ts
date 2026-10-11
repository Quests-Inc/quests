// Overlays the map draft (a log of pending operations) on the graph the server
// returned, so the page can show what saving would produce. The server stays
// the judge of validity; this only mirrors it for display.

import { parseRefs } from './refs.ts'

const tmpKey = (type: string, opId: number) => `${type}:t${opId}`

/**
 * @param {{ nodes: any[], edges: any[], roots?: string[], root?: string }} graph
 * @param {{ id: number, kind: string, payload: any }[]} ops
 * @returns {{ nodes: any[], edges: any[], ownerless: Set<string>, touched: Map<string, number[]> }}
 */
export function applyDraft(
  graph: { nodes: any[]; edges: any[]; roots?: string[]; root?: string },
  ops: { id: number; kind: string; payload: any }[]
): { nodes: any[]; edges: any[]; ownerless: Set<string>; touched: Map<string, number[]> } {
  const nodes = graph.nodes.map((n) => ({ ...n }))
  const edges = graph.edges.map((e) => ({ ...e }))
  const byKey = new Map(nodes.map((n) => [n.key, n]))
  const has = (from: string, to: string, kind: string) =>
    edges.some((e) => e.from === from && e.to === to && e.kind === kind)
  const addEdge = (from: string, to: string, kind: string, draft: string, opId: number) => {
    if (from === to || has(from, to, kind)) return
    edges.push({ from, to, kind, draft, op: opId })
  }
  const ownerOf = new Map() // node key → owner key (new steps / files)
  /** node key → ops that write into it (the source of a link, the moved item) */
  const touched = new Map()
  const touch = (key: any, opId: number) => {
    if (!touched.has(key)) touched.set(key, [])
    touched.get(key).push(opId)
  }

  const stub = (type: string, opId: number, title: string, extra = {}) => {
    const key = tmpKey(type, opId)
    const n = {
      key,
      type,
      id: `t${opId}`,
      title: title || '(без названия)',
      depth: -1,
      links: 0,
      draft: true,
      op: opId,
      ...extra,
    }
    nodes.push(n)
    byKey.set(key, n)
    return n
  }
  // Tokens typed into a new thing's own text are real links once saved.
  const textRefs = (from: string, text: any) => {
    for (const r of parseRefs(text || '')) {
      if (
        r.kind === 'quest' ||
        r.kind === 'note' ||
        r.kind === 'step' ||
        r.kind === 'attachment' ||
        r.kind === 'questline'
      ) {
        addEdge(from, `${r.kind}:${r.id}`, 'reference', 'add', null)
      }
    }
  }

  for (const op of ops) {
    const p = op.payload || {}
    switch (op.kind) {
      case 'create_quest': {
        const f = p.fields || {}
        stub('quest', op.id, f.title, { desc: f.description || '', status: f.status || 'active', pos: p.pos })
        textRefs(tmpKey('quest', op.id), f.description)
        // A quest made inside a questline hangs off it.
        if (f.questline_id != null && byKey.has(`questline:${f.questline_id}`)) {
          edges.push({
            from: `questline:${f.questline_id}`,
            to: tmpKey('quest', op.id),
            kind: 'structure',
            draft: 'add',
            op: op.id,
          })
        }
        break
      }
      case 'create_note': {
        const f = p.fields || {}
        stub('note', op.id, f.title, { desc: f.description || '', pos: p.pos })
        textRefs(tmpKey('note', op.id), f.description)
        break
      }
      case 'create_questline': {
        const f = p.fields || {}
        stub('questline', op.id, f.title, { desc: f.description || '', pos: p.pos })
        textRefs(tmpKey('questline', op.id), f.description)
        break
      }
      case 'create_step': {
        const f = p.fields || {}
        const n = stub('step', op.id, f.title, {
          desc: f.description || '',
          progress: '0/1',
          status: 'open',
          pos: p.pos,
        })
        textRefs(n.key, f.description)
        if (p.quest) ownerOf.set(n.key, p.quest)
        break
      }
      case 'upload_file': {
        stub('attachment', op.id, p.filename, { mime: p.mime, pos: p.pos })
        if (p.owner) ownerOf.set(tmpKey('attachment', op.id), p.owner)
        break
      }
      case 'add_ref':
        addEdge(p.from, p.to, 'reference', 'add', op.id)
        touch(p.from, op.id)
        break
      case 'remove_ref': {
        const e = edges.find((x) => x.from === p.from && x.to === p.to && x.kind === 'reference')
        if (e) {
          e.draft = 'remove'
          e.op = op.id
        }
        touch(p.from, op.id)
        break
      }
      case 'attach':
        ownerOf.set(p.item, p.owner)
        touch(p.item, op.id)
        break
      case 'edit_quest': {
        const n = byKey.get(p.id)
        const f = p.fields || {}
        // Taken out of its questline: the ownership line is struck out.
        if ('questline_id' in f) {
          for (const e of edges) {
            if (
              e.kind === 'structure' &&
              e.to === p.id &&
              e.from.startsWith('questline:') &&
              e.draft !== 'add' &&
              e.draft !== 'remove'
            ) {
              e.draft = 'remove'
              e.op = op.id
            }
          }
        }
        if (n) {
          if (f.title != null) n.title = f.title
          if (f.description != null) n.desc = f.description
          if (f.status != null) n.status = f.status
          touch(p.id, op.id)
        }
        break
      }
      case 'edit_note': {
        const n = byKey.get(p.id)
        const f = p.fields || {}
        if (n) {
          if (f.title != null) n.title = f.title
          if (f.description != null) n.desc = f.description
          touch(p.id, op.id)
        }
        break
      }
      case 'set_parent': {
        // Replaces whatever parent the note had; a removed link stays drawn (struck out).
        for (let i = edges.length - 1; i >= 0; i--) {
          const e = edges[i]
          if (e.kind !== 'structure' || e.to !== p.id || !e.from.startsWith('note:') || e.draft === 'remove') continue
          if (e.draft === 'add') edges.splice(i, 1)
          else {
            e.draft = 'remove'
            e.op = op.id
          }
        }
        if (p.parent) edges.push({ from: p.parent, to: p.id, kind: 'structure', draft: 'add', op: op.id })
        touch(p.id, op.id)
        break
      }
      case 'edit_step': {
        const n = byKey.get(p.id)
        const f = p.fields || {}
        // No longer waits for the previous step: the dependency line is struck out.
        if (f.wait_previous === false) {
          for (const e of edges) {
            if (e.kind === 'dependency' && e.to === p.id && e.from.startsWith('step:') && e.draft !== 'remove') {
              e.draft = 'remove'
              e.op = op.id
            }
          }
        }
        if (n) {
          if (f.title != null) n.title = f.title
          if (f.description != null) n.desc = f.description
          if (f.progress_current != null || f.progress_total != null) {
            const [c0, t0] = String(n.progress || '0/1')
              .split('/')
              .map(Number)
            const c = f.progress_current ?? c0
            const t = f.progress_total ?? t0
            n.progress = `${c}/${t}`
            n.status = c >= t ? 'done' : 'open'
          }
          touch(p.id, op.id)
        }
        break
      }
      case 'copy': {
        const src = byKey.get(p.source)
        if (!src) break
        const key = `${src.type}:t${op.id}`
        const n = stub(src.type, op.id, `${src.title} (копия)`, {
          desc: src.desc,
          icon: src.icon,
          icon_url: src.icon_url,
          color: src.color,
          mime: src.mime,
          progress: src.type === 'step' ? `0/${String(src.progress || '0/1').split('/')[1] || 1}` : undefined,
          status: src.type === 'step' ? 'open' : src.status,
          pos: p.pos,
        })
        // Copies of steps and files stay with the original's owner; a copied
        // quest stays in its questline.
        const own = edges.find((e) => e.kind === 'structure' && e.to === p.source)
        if (own && (src.type === 'step' || src.type === 'attachment')) ownerOf.set(key, own.from)
        if (own && src.type === 'quest' && own.from.startsWith('questline:')) {
          edges.push({ from: own.from, to: key, kind: 'structure', draft: 'add', op: op.id })
        }
        void n
        break
      }
    }
  }

  // Owners: a new thing's owner edge, or a re-parented file's / step's new one.
  /** quest key → change in its number of steps */
  const stepDelta = new Map()
  const bump = (k: any, d: number) => stepDelta.set(k, (stepDelta.get(k) ?? 0) + d)
  for (const [item, owner] of ownerOf) {
    const n = byKey.get(item)
    if (!n) continue
    if (!n.draft) {
      // A saved attachment / step moves: its old owner edge goes, the new one is a draft.
      for (let i = edges.length - 1; i >= 0; i--) {
        if (edges[i].to === item && edges[i].kind === 'structure') {
          if (n.type === 'step' && edges[i].from.startsWith('quest:') && edges[i].from !== owner) {
            bump(edges[i].from, -1)
            bump(owner, +1)
          }
          edges.splice(i, 1)
        }
      }
    } else if (n.type === 'step') {
      bump(owner, +1)
    }
    if (byKey.has(owner)) edges.push({ from: owner, to: item, kind: 'structure', draft: 'add', op: null })
  }

  // Invalid until fixed: a new step or file with no owner cannot exist on its
  // own, and a quest must keep at least one step.
  const ownerless = new Set<string>()
  for (const n of nodes) {
    if (n.draft && (n.type === 'step' || n.type === 'attachment') && !ownerOf.has(n.key)) ownerless.add(n.key)
    if (n.type === 'quest' && !n.draft && n.step_count != null && n.step_count + (stepDelta.get(n.key) ?? 0) < 1) {
      ownerless.add(n.key)
    }
  }

  // Draft things hang where they are linked: depth one past their neighbour.
  const adj = new Map()
  for (const e of edges) {
    if (!adj.has(e.from)) adj.set(e.from, [])
    if (!adj.has(e.to)) adj.set(e.to, [])
    adj.get(e.from).push(e.to)
    adj.get(e.to).push(e.from)
  }
  const queue = nodes.filter((n) => n.depth >= 0).map((n) => n.key)
  for (let i = 0; i < queue.length; i++) {
    const cur = byKey.get(queue[i])
    for (const k of adj.get(cur.key) ?? []) {
      const nb = byKey.get(k)
      if (nb && nb.depth < 0) {
        nb.depth = cur.depth + 1
        queue.push(k)
      }
    }
  }
  return { nodes, edges, ownerless, touched }
}

/** One line for the draft log. @param {any} op @param {(key: string) => string} title */
export function describeOp(op: any, title: (key: string) => string) {
  const p = op.payload || {}
  switch (op.kind) {
    case 'create_quest':
      return `Создать квест «${p.fields?.title ?? ''}»`
    case 'create_note':
      return `Создать заметку «${p.fields?.title ?? ''}»`
    case 'create_questline':
      return `Создать квестлайн «${p.fields?.title ?? ''}»`
    case 'create_step':
      return `Создать шаг «${p.fields?.title ?? ''}»`
    case 'upload_file':
      return `Загрузить файл «${p.filename ?? ''}»`
    case 'add_ref':
      return `Ссылка: «${title(p.from)}» → «${title(p.to)}»`
    case 'remove_ref':
      return `Убрать ссылку: «${title(p.from)}» → «${title(p.to)}»`
    case 'attach':
      return `Прикрепить «${title(p.item)}» к «${title(p.owner)}»`
    case 'edit_quest':
      if (
        p.fields &&
        Object.keys(p.fields).length === 1 &&
        'questline_id' in p.fields &&
        p.fields.questline_id == null
      ) {
        return `Вывести квест «${title(p.id)}» из квестлайна`
      }
      return `Изменить квест «${title(p.id)}»: ${Object.keys(p.fields || {}).join(', ')}`
    case 'edit_note':
      return `Изменить заметку «${title(p.id)}»: ${Object.keys(p.fields || {}).join(', ')}`
    case 'edit_step':
      if (p.fields?.wait_previous === false && Object.keys(p.fields).length === 1)
        return `Шаг «${title(p.id)}» больше не ждёт предыдущий`
      return `Изменить шаг «${title(p.id)}»: ${Object.keys(p.fields || {}).join(', ')}`
    case 'set_parent':
      return p.parent
        ? `Заметка «${title(p.id)}» → под «${title(p.parent)}»`
        : `Заметка «${title(p.id)}» — открепить${p.was ? ` от «${title(p.was)}»` : ''}`
    case 'copy':
      return `Копия «${title(p.source)}»`
    default:
      return op.kind
  }
}
