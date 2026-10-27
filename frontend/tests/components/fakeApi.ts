// Фальшивый бэкенд для сквозных проверок интерфейса: перехватывает fetch и отвечает данными по адресу.
// Данные — одна квест-цепочка с шагами, заметка с вложенной и вложение, чтобы вкладки было что рисовать.

export const fixtureData = () => ({
  categories: [{ id: 1, slug: 'work', label: 'Работа', sort_order: 0, color: '#c47a20' }],
  questlines: [{ id: 1, title: 'Квестлайн', category_id: 1, color: '#5a8a9a', icon: 'document', icon_url: null }],
  quests: [
    {
      id: 1,
      title: 'Первый квест',
      description: 'Описание note=1',
      status: 'active',
      significance: 'common',
      pinned: false,
      overlay_hidden: false,
      automated: false,
      sort_order: 0,
      deadline_at: null,
      duration_seconds: null,
      category_id: 1,
      category_slug: 'work',
      category_label: 'Работа',
      questline_id: 1,
      questline_title: 'Квестлайн',
      tags: [],
      created_at: '2026-03-01T10:00:00Z',
      updated_at: '2026-03-01T10:00:00Z',
      steps_total: 3,
      progress_label: '1/3',
      steps: [
        {
          id: 11,
          quest_id: 1,
          title: 'Шаг один',
          description: '',
          progress_current: 1,
          progress_total: 3,
          sort_order: 0,
          wait_previous: false,
          run_mode: 'poll',
        },
      ],
    },
  ],
  notes: [
    {
      id: 1,
      title: 'Корневая',
      description: 'Текст quest=1',
      parent_id: null,
      pinned: false,
      is_readme: false,
      is_private: false,
      is_category: false,
      sort_order: 0,
      outdated: false,
      icon: null,
      color: '#9a9a9a',
    },
    {
      id: 2,
      title: 'Вложенная',
      description: '',
      parent_id: 1,
      pinned: false,
      is_readme: false,
      is_private: false,
      is_category: false,
      sort_order: 0,
      outdated: false,
      icon: null,
      color: '#9a9a9a',
    },
  ],
  attachments: {
    quest: {
      1: [
        {
          id: 5,
          owner_type: 'quest',
          owner_id: 1,
          filename: 'схема.png',
          content_type_detected: 'image/png',
          size_bytes: 2048,
          uploaded_at: '2026-03-01T10:00:00Z',
          revision: 1,
          revision_count: 1,
          hidden: false,
          comment: '',
        },
      ],
    },
    questline: {},
    note: {},
  },
})

export type Fake = ReturnType<typeof installFakeApi>

/** Ставит перехват fetch и заглушку WebSocket; возвращает журнал запросов и снятие перехвата. */
export function installFakeApi(data = fixtureData(), opts: { authRequired?: boolean } = {}) {
  const log: string[] = []
  const bodies: Record<string, unknown[]> = {}
  const realFetch = globalThis.fetch
  const realWS = (globalThis as any).WebSocket
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  globalThis.fetch = (async (input: any, init: any = {}) => {
    const url = new URL(String(input), 'http://localhost')
    const method = (init.method || 'GET').toUpperCase()
    log.push(`${method} ${url.pathname}${url.search}`)
    if (init.body) (bodies[`${method} ${url.pathname}`] ??= []).push(JSON.parse(init.body))
    const p = url.pathname
    const body = init.body && typeof init.body === 'string' ? JSON.parse(init.body) : null
    if (method === 'POST' && p === '/api/quests') {
      const q = { ...data.quests[0], id: data.quests.length + 1, steps: [], ...body }
      data.quests.push(q)
      return json(q, 201)
    }
    const stepPatch = /^\/api\/quests\/(\d+)\/steps\/(\d+)$/.exec(p)
    if (method === 'PATCH' && stepPatch) {
      const q = data.quests.find((x) => x.id === Number(stepPatch[1]))!
      const s = q.steps.find((x) => x.id === Number(stepPatch[2]))!
      Object.assign(s, body)
      return json(q)
    }
    const questPatch = /^\/api\/quests\/(\d+)$/.exec(p)
    if (method === 'PATCH' && questPatch) {
      const q = data.quests.find((x) => x.id === Number(questPatch[1]))!
      Object.assign(q, body)
      return json(q)
    }
    const notePatch = /^\/api\/notes\/(\d+)$/.exec(p)
    if (method === 'PATCH' && notePatch) {
      const n = data.notes.find((x) => x.id === Number(notePatch[1]))!
      Object.assign(n, body)
      return json({ ...n, refs: [], backlinks: [] })
    }
    if (p === '/api/auth/state')
      return json({ auth_required: !!opts.authRequired, authenticated: true, username: 'alice', workspace_id: 1 })
    if (p === '/api/health')
      return json({ api: { status: 'ok' }, components: { db: { status: 'ok' }, webdav: { status: 'disabled' } } })
    if (p === '/api/quests') return json(data.quests)
    if (/^\/api\/quests\/\d+$/.test(p)) return json(data.quests[0])
    if (p === '/api/categories') return json(data.categories)
    if (p === '/api/questlines') return json(data.questlines)
    if (p === '/api/notes') return json(data.notes)
    if (/^\/api\/notes\/\d+$/.test(p))
      return json({ ...data.notes.find((n) => `/api/notes/${n.id}` === p), refs: [], backlinks: [] })
    if (p === '/api/attachments') return json(data.attachments)
    if (p === '/api/templates' || p.startsWith('/api/trash/')) return json([])
    if (p === '/api/runners' || p === '/api/runner-jobs') return json([])
    if (p === '/api/quest-log') return json([])
    if (p === '/api/graph-draft') return json({ ops: [], roots: [] })
    if (p === '/api/graph') {
      return json({
        root: 'quest:1',
        roots: ['quest:1'],
        truncated: false,
        depth: 2,
        nodes: [
          { key: 'quest:1', type: 'quest', id: 1, title: 'Первый квест', depth: 0, links: 0, status: 'active' },
          { key: 'note:1', type: 'note', id: 1, title: 'Корневая', depth: 1, links: 1 },
        ],
        edges: [{ from: 'quest:1', to: 'note:1', kind: 'reference' }],
      })
    }
    if (/\/(attachments|secrets|tags|stats|revisions|steak|streak|emit-attempts)$/.test(p) || p === '/api/tags')
      return json([])
    if (p.startsWith('/api/workspaces')) return json({ workspaces: [{ id: 1, name: 'Моё' }], current: 1 })
    return json({})
  }) as typeof fetch
  class QuietSocket extends EventTarget {
    static CONNECTING = 0
    static OPEN = 1
    static CLOSED = 3
    readyState = 0
    send() {}
    close() {
      this.readyState = 3
    }
  }
  ;(globalThis as any).WebSocket = QuietSocket
  return {
    log,
    bodies,
    data,
    restore() {
      globalThis.fetch = realFetch
      ;(globalThis as any).WebSocket = realWS
    },
  }
}
