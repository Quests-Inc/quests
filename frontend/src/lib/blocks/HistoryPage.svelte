<script>
  import { parseQuery, tokenValues, setToken, helpLine } from '../js/searchFlags.js'
  import FlagHints from '../ui/FlagHints.svelte'
  import Icon from '../ui/Icon.svelte'
  import DateInput from '../ui/DateInput.svelte'
  import { getQuestLog, listTags } from '../js/api.js'
  import { parseApiDate } from '../js/time.js'
  import { KINDS, detailOf } from '../js/historyKinds.ts'

  /** @type {{
   *   active?: boolean,
   *   nonce?: number,
   *   categories?: any[],
   *   questlines?: any[],
   *   onOpen?: (kind: string, id: number) => void,
   * }} */
  let { active = true, nonce = 0, categories = [], questlines = [], onOpen } = $props()

  const PAGE = 100


  const ENTITIES = [
    { id: '', label: 'Всё' },
    { id: 'quest', label: 'Квесты' },
    { id: 'note', label: 'Заметки' },
    { id: 'attachment', label: 'Вложения' },
  ]
  const KIND_GROUPS = [
    { id: '', label: 'Любые события', kinds: [] },
    { id: 'done', label: 'Выполнено', kinds: ['step_completed', 'quest_completed'] },
    {
      id: 'new',
      label: 'Создано',
      kinds: ['quest_created', 'quest_appeared', 'note_created', 'attachment_added'],
    },
    {
      id: 'edit',
      label: 'Правки',
      kinds: [
        'quest_updated',
        'status_changed',
        'pin_changed',
        'step_reopened',
        'note_updated',
        'attachment_version',
        'attachment_hidden',
      ],
    },
    { id: 'miss', label: 'Провалы и отсрочки', kinds: ['quest_failed', 'quest_expired', 'quest_delayed'] },
    {
      id: 'del',
      label: 'Удаления',
      kinds: ['quest_deleted', 'note_deleted', 'attachment_deleted'],
    },
  ]
  const PERIODS = [
    { id: '', label: 'За всё время' },
    { id: 'today', label: 'Сегодня' },
    { id: '7', label: '7 дней' },
    { id: '30', label: '30 дней' },
  ]

  /** @param {string} key @param {string} fallback */
  function param(key, fallback = '') {
    try {
      return new URL(location.href).searchParams.get(key) ?? fallback
    } catch {
      return fallback
    }
  }

  /** The query line is the single source of truth: every selector below reads its value from a token and
   * writes it back (`type:note act:edit line:3 cat:work tag:infra when:7 day:2026-10-09`). Links from before
   * (?et=…&kg=…) are turned into tokens once. */
  function initialQuery() {
    let q = param('q')
    const old = { type: param('et'), act: param('kg'), line: param('ql'), cat: param('cat'), tag: param('tag'), when: param('per'), day: param('day') }
    for (const [k, v] of Object.entries(old)) if (v && !new RegExp(`(^|\\s)${k}:`).test(q)) q = `${q} ${k}:${v}`.trim()
    return q
  }
  let search = $state(initialQuery())
  let tags = $state(/** @type {any[]} */ ([]))
  const histSpec = $derived({
    type: { label: 'что менялось', values: ENTITIES.filter((e) => e.id).map((e) => ({ id: e.id, label: e.label })), test: () => true },
    act: {
      label: 'вид события',
      values: [
        { id: 'done', label: 'выполнено' },
        { id: 'new', label: 'создано', aliases: ['create', 'created'] },
        { id: 'edit', label: 'правки', aliases: ['update', 'updated', 'edited'] },
        { id: 'miss', label: 'провалы и отсрочки', aliases: ['fail', 'failed', 'missed'] },
        { id: 'del', label: 'удаления', aliases: ['delete', 'deleted'] },
      ],
      test: () => true,
    },
    line: { label: 'квестлайн', hint: 'id', test: () => true },
    cat: { label: 'категория', values: categories.map((c) => ({ id: c.slug, label: c.label, aliases: [String(c.id)] })), test: () => true },
    tag: { label: 'тег', hint: 'infra', test: () => true },
    when: { label: 'период', values: [{ id: 'today', label: 'сегодня' }, { id: '7', label: '7 дней' }, { id: '30', label: '30 дней' }], test: () => true },
    day: { label: 'день', hint: 'ГГГГ-ММ-ДД', test: () => true },
  })
  let parsedSearch = $derived(parseQuery(search, histSpec))
  const first = (key) => tokenValues(parsedSearch, key)[0] ?? ''
  const put = (key, v) => (search = setToken(search, key, v ? [v] : []))
  let entity = $derived(first('type'))
  let kindGroup = $derived(first('act'))
  let questlineId = $derived(first('line'))
  let categoryId = $derived(String(categories.find((c) => c.slug === first('cat'))?.id ?? ''))
  let tagId = $derived(String(tags.find((t) => t.slug === first('tag') || String(t.id) === first('tag'))?.id ?? ''))
  let period = $derived(first('when'))
  let day = $derived(first('day'))

  let rows = $state(/** @type {any[]} */ ([]))
  let loading = $state(false)
  let more = $state(false)
  let error = $state('')
  let seq = 0

  let filtersOn = $derived(
    !!search.trim(),
  )

  /** Keep the filters in the address bar (tab=history stays) so a view can be linked. */
  function syncUrl() {
    try {
      const qs = new URLSearchParams({ tab: 'history' })
      const set = (k, v) => v && qs.set(k, v)
      set('q', search.trim())
      const next = `${location.pathname}?${qs}`
      if (next !== `${location.pathname}${location.search}`) history.replaceState(null, '', next)
    } catch {
      /* ignore */
    }
  }

  function dayStart(s) {
    const [y, m, d] = s.split('-').map(Number)
    return new Date(y, m - 1, d)
  }

  function range() {
    if (day) {
      const from = dayStart(day)
      return { from, to: new Date(from.getFullYear(), from.getMonth(), from.getDate() + 1) }
    }
    if (period) {
      const now = new Date()
      const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
      const back = period === 'today' ? 0 : Number(period) - 1
      return { from: new Date(today.getFullYear(), today.getMonth(), today.getDate() - back), to: null }
    }
    return { from: null, to: null }
  }

  /** @param {boolean} append */
  async function load(append = false) {
    const mine = ++seq
    loading = true
    error = ''
    try {
      const { from, to } = range()
      const last = append ? rows[rows.length - 1] : null
      const kinds = KIND_GROUPS.find((g) => g.id === kindGroup)?.kinds ?? []
      const data = await getQuestLog({
        limit: PAGE,
        entity_type: entity || 'all',
        kind: kinds.join(','),
        questline_id: questlineId,
        category_id: categoryId,
        tag_id: tagId,
        from: from?.toISOString(),
        to: to?.toISOString(),
        q: parsedSearch.text,
        before_id: last?.id,
        before_at: last?.at,
      })
      if (mine !== seq) return
      const list = Array.isArray(data) ? data : []
      rows = append ? [...rows, ...list] : list
      more = list.length === PAGE
    } catch (e) {
      if (mine === seq) error = e?.message || String(e)
    } finally {
      if (mine === seq) loading = false
    }
  }

  $effect(() => {
    if (!active) return
    void [parsedSearch.text, entity, kindGroup, questlineId, categoryId, tagId, period, day, nonce]
    syncUrl()
    // Typing in the search box shouldn't fire a request per keystroke.
    const t = setTimeout(() => load(false), search ? 250 : 0)
    return () => clearTimeout(t)
  })

  $effect(() => {
    if (!active || tags.length) return
    listTags()
      .then((t) => (tags = Array.isArray(t) ? t : (t?.items ?? [])))
      .catch(() => {})
  })

  function reset() {
    search = ''
  }

  /** @param {string} iso */
  function localDay(iso) {
    const d = parseApiDate(iso)
    if (!d) return ''
    const p = (n) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
  }

  /** @param {string} key */
  function dayLabel(key) {
    const d = dayStart(key)
    const now = new Date()
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const diff = Math.round((today.getTime() - d.getTime()) / 86400000)
    const base = d.toLocaleDateString('ru-RU', {
      day: 'numeric',
      month: 'long',
      year: d.getFullYear() === today.getFullYear() ? undefined : 'numeric',
      weekday: 'short',
    })
    if (diff === 0) return `Сегодня · ${base}`
    if (diff === 1) return `Вчера · ${base}`
    return base
  }

  /** @param {string} iso */
  function timeLabel(iso) {
    const d = parseApiDate(iso)
    return d ? d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) : ''
  }

  let groups = $derived.by(() => {
    /** @type {{ key: string, items: any[] }[]} */
    const out = []
    for (const r of rows) {
      const key = localDay(r.at)
      const g = out[out.length - 1]
      if (g && g.key === key) g.items.push(r)
      else out.push({ key, items: [r] })
    }
    return out
  })

  /** Entity the row points at, or null when it's gone / not openable. */
  function target(r) {
    if (r.kind.endsWith('_deleted')) return null
    if (r.entity_type === 'quest' && (r.entity_id ?? r.quest_id) != null) {
      return ['quest', r.entity_id ?? r.quest_id]
    }
    if (r.entity_type === 'note' && r.entity_id != null) return ['note', r.entity_id]
    if (r.entity_type === 'attachment' && r.entity_id != null) return ['attachment', r.entity_id]
    return null
  }

  /** @param {any} r */
  function open(r) {
    const t = target(r)
    if (t) onOpen?.(t[0], t[1])
  }

  /** Owner shown under attachment rows. */
  function parentLabel(r) {
    if (!r.parent_type || r.parent_id == null) return ''
    const name = { quest: 'квест', questline: 'квестлайн', note: 'заметка' }[r.parent_type] ?? r.parent_type
    return `${name} ${r.parent_id}`
  }
</script>

<div class="hist">
  <div class="hist__tools">
    <div class="hist__row">
      <input
        class="search hist__search"
        type="search"
        placeholder="Поиск по названию и описанию события…"
        bind:value={search}
        aria-label="Поиск по истории"
        title={`Фильтры: ${helpLine(histSpec)}`}
      />
      <select class="hist__select" bind:value={() => period, (v) => put('when', v)} aria-label="Период">
        {#each PERIODS as p (p.id)}
          <option value={p.id}>{p.label}</option>
        {/each}
      </select>
      <DateInput bind:value={() => day, (v) => put('day', v)} label="Конкретный день" clearable />
      {#if filtersOn}
        <button type="button" class="hist__reset" onclick={reset}>
          <Icon name="close" size={12} /> Сбросить
        </button>
      {/if}
    </div>
    <FlagHints value={search} spec={histSpec} parsed={parsedSearch} onPick={(v) => (search = v)} />
    <div class="hist__row">
      <div class="hist__seg" role="group" aria-label="Что менялось">
        {#each ENTITIES as e (e.id)}
          <button
            type="button"
            class="hist__pill"
            class:hist__pill--on={entity === e.id}
            onclick={() => put('type', e.id)}>{e.label}</button
          >
        {/each}
      </div>
      <select class="hist__select" bind:value={() => kindGroup, (v) => put('act', v)} aria-label="Вид события">
        {#each KIND_GROUPS as g (g.id)}
          <option value={g.id}>{g.label}</option>
        {/each}
      </select>
      <select class="hist__select" bind:value={() => questlineId, (v) => put('line', v)} aria-label="Квестлайн">
        <option value="">Все квестлайны</option>
        {#each questlines as l (l.id)}
          <option value={String(l.id)}>{l.title}</option>
        {/each}
      </select>
      <select class="hist__select" bind:value={() => categoryId, (v) => put('cat', categories.find((c) => String(c.id) === v)?.slug ?? '')} aria-label="Категория">
        <option value="">Все категории</option>
        {#each categories as c (c.id)}
          <option value={String(c.id)}>{c.label}</option>
        {/each}
      </select>
      {#if tags.length}
        <select class="hist__select" bind:value={() => tagId, (v) => put('tag', tags.find((t) => String(t.id) === v)?.slug ?? '')} aria-label="Тег">
          <option value="">Все теги</option>
          {#each tags as t (t.id)}
            <option value={String(t.id)}>{t.label || t.slug}</option>
          {/each}
        </select>
      {/if}
    </div>
  </div>

  <div class="hist__feed">
    {#if error}
      <p class="empty">{error}</p>
    {:else if rows.length === 0 && !loading}
      <p class="empty">{filtersOn ? 'Ничего не найдено' : 'История пока пуста'}</p>
    {:else}
      {#each groups as g (g.key)}
        <section class="hist__day">
          <h3 class="hist__dayhead">{dayLabel(g.key)} <span class="hist__n">{g.items.length}</span></h3>
          {#each g.items as r (r.id)}
            {@const info = KINDS[r.kind] ?? [r.kind.replaceAll('_', ' '), 'scroll', '']}
            {@const t = target(r)}
            <button
              type="button"
              class="hist__item hist__item--{info[2] || 'plain'}"
              disabled={!t}
              onclick={() => open(r)}
            >
              <span class="hist__time">{timeLabel(r.at)}</span>
              <span class="hist__icon"><Icon name={info[1]} size={14} /></span>
              <span class="hist__body">
                <span class="hist__title">{r.title || '—'}</span>
                <span class="hist__what">
                  {info[0]}{#if detailOf(r)} · {detailOf(r)}{/if}
                  {#if parentLabel(r)}<span class="hist__parent"> · {parentLabel(r)}</span>{/if}
                </span>
              </span>
            </button>
          {/each}
        </section>
      {/each}
      {#if more}
        <button type="button" class="hist__more" disabled={loading} onclick={() => load(true)}>
          {loading ? 'Загрузка…' : 'Показать ещё'}
        </button>
      {/if}
    {/if}
  </div>
</div>

<style>
  .hist {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  .hist__tools {
    --hist-h: 2.4rem;
    display: grid;
    gap: 0.55rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--color-border, #333);
  }

  .hist__row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .hist__search {
    box-sizing: border-box;
    height: var(--hist-h);
    flex: 1 1 14rem;
    min-width: 0;
  }

  .hist__select {
    flex: 0 0 auto;
    max-width: 100%;
    box-sizing: border-box;
    height: var(--hist-h);
    padding: 0 0.6rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: inherit;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
  }

  .hist__select option {
    background: var(--color-bg-raised, #1a1a1a);
  }

  .hist__seg {
    display: inline-flex;
    box-sizing: border-box;
    height: var(--hist-h);
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    overflow: hidden;
  }

  .hist__pill {
    padding: 0 0.9rem;
    border: 0;
    border-right: 1px solid var(--color-border, #333);
    background: transparent;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .hist__pill:last-child {
    border-right: 0;
  }

  .hist__pill:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .hist__pill--on {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 18%, transparent);
  }

  .hist__reset {
    box-sizing: border-box;
    height: var(--hist-h);
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0 0.8rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .hist__feed {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: 0.25rem 1rem 1.5rem;
  }

  .hist__dayhead {
    position: sticky;
    top: 0;
    z-index: 1;
    margin: 0;
    padding: 0.7rem 0 0.3rem;
    background: var(--color-bg, #121212);
    font-size: var(--text-xs, 0.75rem);
    font-weight: 500;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .hist__n {
    margin-left: 0.3rem;
    opacity: 0.7;
    font-variant-numeric: tabular-nums;
  }

  .hist__item {
    display: grid;
    grid-template-columns: 3rem 1.4rem minmax(0, 1fr);
    align-items: start;
    gap: 0.5rem;
    width: 100%;
    padding: 0.4rem 0.3rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    font: inherit;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .hist__item:disabled {
    cursor: default;
    opacity: 0.7;
  }

  .hist__item:not(:disabled):hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 6%, transparent);
  }

  .hist__time {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-subtle, #6e6e6e);
    padding-top: 0.15rem;
  }

  .hist__icon {
    display: inline-flex;
    padding-top: 0.15rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .hist__item--ok .hist__icon {
    color: var(--color-success, #4fa56b);
  }
  .hist__item--bad .hist__icon {
    color: var(--color-danger, #b54a3a);
  }
  .hist__item--warn .hist__icon {
    color: var(--color-warning, #c8a03c);
  }
  .hist__item--new .hist__icon {
    color: var(--color-accent, #c9a227);
  }

  .hist__body {
    display: grid;
    gap: 0.1rem;
    min-width: 0;
  }

  .hist__title {
    font-size: var(--text-sm, 0.875rem);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hist__what {
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .hist__parent {
    font-family: var(--font-mono, monospace);
  }

  .hist__more {
    display: block;
    margin: 1rem auto 0;
    padding: 0.4rem 1.2rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    font: inherit;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  @media (max-width: 40rem) {
    .hist__tools {
      padding: 0.6rem 0.75rem;
    }
    .hist__feed {
      padding: 0.25rem 0.75rem 1.5rem;
    }
    .hist__item {
      grid-template-columns: 2.6rem 1.2rem minmax(0, 1fr);
    }
  }
</style>
