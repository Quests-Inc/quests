<script lang="ts">
  import { untrack } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import SidebarLayout from '@/shared/ui/SidebarLayout.svelte'
  import { getRun, getScriptNodes, listRuns, type RunDetail, type RunSourceKind } from '@/shared/api/client.ts'
  import type { RunSummary } from '@/shared/api/execution.gen.ts'
  import type { NodeInfo } from '@/shared/api/scriptgraph.gen.ts'
  import { replaceSearch } from '@/shared/lib/urlSearch.ts'
  import { openTarget } from '@/shared/lib/openTarget.ts'
  import { ago, formatLocal } from '@/shared/lib/time.ts'
  import { JOB_STATE_WORDS, NODE_STATE_WORDS, SOURCE_WORDS, STATE_WORDS, filterFromUrl, isLong, shortValue } from '../model/runWords.ts'

  /**
   * The run log (quest=489): every run of a script — a bound check, a hook, a text's embedding, a test from the editor — with
   * what each node did and what went over its outputs. Other tabs open it filtered (a script, a hook, a step) or on one run.
   */
  let { active = true }: { active?: boolean } = $props()

  const PAGE = 50
  let runs = $state<RunSummary[]>([])
  let loading = $state(false)
  let more = $state(false)
  let error = $state('')
  let stateFilter = $state<'' | 'failed' | 'waiting' | 'done'>('')
  let source = $state<RunSourceKind | ''>('')
  let sourceMenu = $state(false)
  let sourceX = $state(0)
  let sourceY = $state(0)
  let sourceButton = $state<HTMLButtonElement | null>(null)
  const sourceChoices = $derived([
    { id: '', label: 'Любой источник', checked: !source || !!pinned },
    ...Object.entries(SOURCE_WORDS).map(([id, label]) => ({ id, label, checked: !pinned && source === id })),
  ])

  function showSources() {
    if (!sourceButton) return
    const rect = sourceButton.getBoundingClientRect()
    sourceX = rect.left
    sourceY = rect.bottom + 4
    sourceMenu = true
  }

  /** A narrower filter that came from another tab: one script, or one source (a hook, a step…), with its title once known. */
  let script = $state<number | undefined>(undefined)
  let pinned = $state<{ kind: RunSourceKind; id: number } | undefined>(undefined)
  let selected = $state<number | null>(null)
  let detail = $state<RunDetail | null>(null)
  let detailError = $state('')
  let infos = $state<NodeInfo[]>([])
  let poll: ReturnType<typeof setTimeout> | undefined

  const filter = () => ({
    script_id: script,
    source_kind: pinned?.kind ?? source,
    source_id: pinned?.id,
    state: stateFilter,
    limit: PAGE,
  })

  async function load(append = false) {
    loading = true
    try {
      const before = append && runs.length ? runs[runs.length - 1].id : undefined
      const page = await listRuns({ ...filter(), before })
      runs = append ? [...runs, ...page] : page
      more = page.length === PAGE
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  async function open(id: number | null, { push = true } = {}) {
    clearTimeout(poll)
    selected = id
    detail = null
    detailError = ''
    if (push) writeUrl()
    if (id == null) return
    try {
      const d = await getRun(id)
      if (selected !== id) return
      detail = d
      // A run that waits for its runner is read again until it settles.
      if (d.state === 'waiting') poll = setTimeout((): void => void refreshOpen(id), 1500)
    } catch (e) {
      if (selected === id) detailError = e instanceof Error ? e.message : String(e)
    }
  }

  async function refreshOpen(id: number) {
    if (selected !== id || !active) return
    await open(id, { push: false })
    const i = runs.findIndex((r) => r.id === id)
    if (detail && i !== -1 && runs[i].state !== detail.state) void load()
  }

  function writeUrl() {
    replaceSearch({
      tab: 'runs',
      run: selected ?? undefined,
      script,
      src: pinned ? `${pinned.kind}:${pinned.id}` : undefined,
      state: stateFilter || undefined,
    })
  }

  /** The filter and the open run as the address says (opening the tab, or a jump from another tab). */
  function fromUrl() {
    const f = filterFromUrl()
    script = f.script
    pinned = f.source
    stateFilter = f.state
    source = ''
    void load()
    void open(f.run ?? null, { push: false })
  }

  $effect(() => {
    if (!active) return
    untrack(() => {
      fromUrl()
      if (!infos.length) void getScriptNodes().then((x) => (infos = x)).catch(() => {})
    })
    const onRoute = () => untrack(fromUrl)
    window.addEventListener('quests:route', onRoute)
    return () => {
      window.removeEventListener('quests:route', onRoute)
      clearTimeout(poll)
    }
  })

  function setState(v: typeof stateFilter) {
    stateFilter = v
    writeUrl()
    void load()
  }
  function setSource(v: typeof source) {
    source = v
    pinned = undefined
    writeUrl()
    void load()
  }
  function clearNarrow() {
    script = undefined
    pinned = undefined
    writeUrl()
    void load()
  }

  const narrowLabel = $derived.by(() => {
    if (pinned) {
      const r = runs.find((x) => x.source_kind === pinned!.kind && x.source_id === pinned!.id)
      return `${SOURCE_WORDS[pinned.kind] ?? pinned.kind} ${r?.source_title ? `«${r.source_title}»` : pinned.id}`
    }
    if (script) {
      const r = runs.find((x) => x.script_id === script)
      return `скрипт ${r?.script_title ? `«${r.script_title}»` : script}`
    }
    return ''
  })

  const nodeTitle = (type: string) => infos.find((i) => i.name === type)?.title ?? type

  /** Where a click on the source of a run leads. */
  function openSource(r: { source_kind: string; source_id: number; script_id?: number }) {
    if (r.source_kind === 'hook') openTarget({ kind: 'hook', id: r.source_id })
    else if (r.source_kind === 'embed' || r.source_kind === 'test') openTarget({ kind: 'script', id: r.source_id })
    else openTarget({ kind: 'ref', ref: r.source_kind, id: r.source_id })
  }

  const SUBJECT_WORDS: Record<string, string> = { step: 'шаг', note: 'заметка', quest: 'квест', template: 'шаблон', questline: 'квестлайн', workspace: 'пространство', text: 'текст' }
  const canOpenSubject = (t: string) => ['step', 'note', 'quest', 'template', 'questline'].includes(t)
  const summary = $derived(runs.find((r) => r.id === selected) ?? null)

  const duration = (d: RunDetail) => {
    if (!d.finished_at) return ''
    const ms = new Date(d.finished_at).getTime() - new Date(d.created_at).getTime()
    return ms < 1000 ? `${Math.max(ms, 0)} мс` : `${(ms / 1000).toFixed(1)} с`
  }
</script>

<SidebarLayout key="runs" class="runs">
  {#snippet sidebar()}
    <aside class="runs__side" aria-label="Запуски">
      <div class="runs__filters">
        <div class="seg" role="group" aria-label="Состояние">
          {#each [{ id: '', label: 'Все' }, { id: 'failed', label: 'Ошибки' }, { id: 'waiting', label: 'Ждут' }] as s (s.id)}
            <button type="button" class="seg__pill" class:seg__pill--on={stateFilter === s.id} onclick={() => setState(s.id as typeof stateFilter)}>{s.label}</button>
          {/each}
        </div>
        <button
          bind:this={sourceButton}
          type="button"
          class="runs__source"
          aria-label="Что запустило"
          aria-haspopup="menu"
          aria-expanded={sourceMenu}
          onclick={() => sourceMenu ? (sourceMenu = false) : showSources()}
          onkeydown={(event) => {
            if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
              event.preventDefault()
              showSources()
            }
          }}
        >
          <span>{pinned || !source ? 'Любой источник' : SOURCE_WORDS[source]}</span>
          <Icon name="chevron-down" size={12} />
        </button>
        {#if narrowLabel}
          <span class="runs__narrow" title="Показаны только эти запуски">
            {narrowLabel}
            <button type="button" class="btn btn--ghost btn--icon" aria-label="Показать все запуски" onclick={clearNarrow}><Icon name="close" size={12} /></button>
          </span>
        {/if}
      </div>
      <ul class="runs__list">
        {#each runs as r (r.id)}
          <li>
            <button type="button" class="run" class:run--on={r.id === selected} onclick={() => void open(r.id)}>
              <span class="run__dot run__dot--{r.state}" title={STATE_WORDS[r.state] ?? r.state}></span>
              <span class="run__text">
                <span class="run__title">{r.script_title || r.graph_ref}</span>
                <small>
                  {SOURCE_WORDS[r.source_kind as RunSourceKind] ?? r.source_kind}{r.source_title && r.source_kind !== 'test' && r.source_kind !== 'embed' ? ` «${r.source_title}»` : ''}
                  · {ago(r.created_at)}{r.dry ? ' · без записи' : ''}
                </small>
                {#if r.state === 'failed' && r.error}<small class="run__err">{r.error}</small>{/if}
              </span>
            </button>
          </li>
        {:else}
          <li class="runs__empty">{loading ? 'Загрузка…' : error || 'Запусков нет. Они хранятся не меньше трёх дней и не меньше 50 последних на каждый источник.'}</li>
        {/each}
        {#if more}
          <li><button type="button" class="btn btn--ghost runs__more" disabled={loading} onclick={() => void load(true)}>Ещё</button></li>
        {/if}
      </ul>
    </aside>
  {/snippet}

  <section class="runs__main">
    {#if selected == null}
      <div class="runs__intro">
        <h2>Запуски</h2>
        <p class="explain">
          Каждый запуск скрипта: проверка шага или заметки, эмит шаблона, хук, вставка в тексте и проверка из редактора. Выберите
          запуск слева: видно, какая нода сработала, что ушло по каждому выходу и что ответил раннер.
        </p>
      </div>
    {:else if detailError}
      <p class="runs__error" role="alert">{detailError}</p>
    {:else if !detail}
      <p class="runs__loading">Загрузка запуска…</p>
    {:else}
      <header class="rd__head">
        <h2>
          <span class="run__dot run__dot--{detail.state}"></span>
          {summary?.script_title || detail.graph_ref}
          <span class="rd__state">{STATE_WORDS[detail.state] ?? detail.state}</span>
          {#if detail.dry}<span class="rd__badge" title="Приёмники ничего не записали: это проверка">без записи</span>{/if}
        </h2>
        <dl class="rd__meta">
          <dt>Запустил</dt>
          <dd>
            <button type="button" class="link" onclick={() => openSource(detail!)}>
              {SOURCE_WORDS[detail.source_kind] ?? detail.source_kind}{summary?.source_title ? ` «${summary.source_title}»` : ` ${detail.source_id}`}
            </button>
          </dd>
          <dt>Читал</dt>
          <dd>
            {#if canOpenSubject(detail.subject_type)}
              <button type="button" class="link" onclick={() => openTarget({ kind: 'ref', ref: detail!.subject_type, id: detail!.subject_id })}>
                {SUBJECT_WORDS[detail.subject_type] ?? detail.subject_type} {summary?.subject_title ? `«${summary.subject_title}»` : detail.subject_id}
              </button>
            {:else}
              {SUBJECT_WORDS[detail.subject_type] ?? detail.subject_type}
            {/if}
          </dd>
          <dt>Начат</dt>
          <dd>{formatLocal(detail.created_at)}{duration(detail) ? ` · ${duration(detail)}` : ''}</dd>
          {#if detail.script_id}
            <dt>Скрипт</dt>
            <dd>
              <button type="button" class="link" onclick={() => openTarget({ kind: 'script', id: detail!.script_id! })}>открыть в редакторе</button>
              · <button type="button" class="link" onclick={() => openTarget({ kind: 'runs', script: detail!.script_id! })}>все его запуски</button>
            </dd>
          {/if}
        </dl>
        {#if detail.error}<p class="runs__error">{detail.error}</p>{/if}
        {#if detail.result}<p class="rd__result"><span>Результат:</span> <code>{shortValue(detail.result, 400)}</code></p>{/if}
      </header>

      {#if detail.graph_error}
        <p class="explain">Граф не прочитать: {detail.graph_error}</p>
      {:else}
        <ol class="rd__nodes">
          {#each detail.nodes as n (n.id)}
            <li class="node node--{n.state ?? 'idle'}">
              <div class="node__head">
                <span class="node__id">{n.id}</span>
                <span class="node__title">{nodeTitle(n.type)}</span>
                <span class="node__state">{n.state ? (NODE_STATE_WORDS[n.state] ?? n.state) : 'не дошло'}</span>
                {#if detail.dry && n.type.startsWith('sink.') && n.state === 'ran'}<span class="rd__badge">не записал</span>{/if}
              </div>
              {#if n.failure}<p class="node__fail">{n.failure}</p>{/if}
              {#if n.outputs}
                <dl class="node__out">
                  {#each Object.entries(n.outputs) as [port, v] (port)}
                    <dt>{port}</dt>
                    <dd>
                      {#if isLong(v)}
                        <details><summary><code>{shortValue(v, 80)}</code></summary><pre>{typeof v === 'string' ? v : JSON.stringify(v, null, 2)}</pre></details>
                      {:else}
                        <code>{shortValue(v)}</code>
                      {/if}
                    </dd>
                  {/each}
                </dl>
              {/if}
              {#if n.exits?.length}<p class="node__exits">без значения: {n.exits.join(', ')}</p>{/if}
            </li>
          {/each}
        </ol>
      {/if}

      {#if detail.jobs.length}
        <h3 class="rd__sub">Задания раннера</h3>
        {#each detail.jobs as j (j.id)}
          <div class="job">
            <div class="job__head">
              <span>{j.runner_label || 'раннер'}</span>
              <span class="job__state">{JOB_STATE_WORDS[j.state] ?? j.state}{j.exit_code != null ? ` · код ${j.exit_code}` : ''}{j.timed_out ? ' · таймаут' : ''}</span>
            </div>
            {#if j.reason}<p class="node__fail">{j.reason}</p>{/if}
            {#if j.stdout}<details><summary>stdout</summary><pre>{j.stdout}</pre></details>{/if}
            {#if j.stderr}<details open={j.state !== 'success'}><summary>stderr</summary><pre>{j.stderr}</pre></details>{/if}
          </div>
        {/each}
      {/if}
    {/if}
  </section>
</SidebarLayout>

<ContextMenu
  open={sourceMenu}
  anchor={sourceButton}
  x={sourceX}
  y={sourceY}
  items={sourceChoices}
  onSelect={(id) => setSource(id as typeof source)}
  onClose={() => {
    sourceMenu = false
    sourceButton?.focus()
  }}
/>

<style>
  .runs__side {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }
  .runs__filters {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    padding: 0.5rem;
    border-bottom: 1px solid var(--color-border, #333);
  }
  .runs__source {
    display: inline-flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex: 1 1 8rem;
    min-width: 0;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-xs, 0.75rem);
    cursor: pointer;
  }
  .runs__source:hover,
  .runs__source[aria-expanded='true'] {
    border-color: var(--color-accent, #c9a227);
  }
  .runs__source:focus-visible {
    outline: 2px solid var(--color-accent, #c9a227);
    outline-offset: 2px;
  }
  .runs__narrow {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    max-width: 100%;
    padding: 0.1rem 0.15rem 0.1rem 0.5rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: var(--radius-md, 4px);
    font-size: var(--text-xs, 0.75rem);
  }
  .seg {
    display: inline-flex;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    overflow: hidden;
  }
  .seg__pill {
    padding: 0.28rem 0.6rem;
    border: 0;
    border-right: 1px solid var(--color-border, #333);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    cursor: pointer;
  }
  .seg__pill:last-child {
    border-right: 0;
  }
  .seg__pill--on {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 18%, transparent);
  }
  .runs__list {
    flex: 1 1 auto;
    overflow: auto;
    margin: 0;
    padding: 0.25rem;
    list-style: none;
  }
  .run {
    display: flex;
    gap: 0.5rem;
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 1px solid transparent;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .run:hover {
    background: var(--color-bg-muted, #1e1e1e);
  }
  .run--on {
    border-color: var(--color-accent, #c9a227);
  }
  .run__text {
    display: grid;
    min-width: 0;
    gap: 0.1rem;
  }
  .run__title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .run small {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .run small.run__err {
    overflow: hidden;
    color: var(--color-danger, #b54a3a);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .run__dot {
    flex: 0 0 auto;
    display: inline-block;
    width: 0.55rem;
    height: 0.55rem;
    margin-top: 0.4rem;
    border-radius: 50%;
    background: var(--color-fg-muted, #9a9a9a);
  }
  .run__dot--done {
    background: var(--color-success, #7a9e3a);
  }
  .run__dot--failed {
    background: var(--color-danger, #b54a3a);
  }
  .run__dot--waiting {
    background: var(--color-accent, #c9a227);
  }
  .runs__empty {
    padding: 0.75rem 0.5rem;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.85rem);
  }
  .runs__more {
    width: 100%;
  }
  .runs__main {
    overflow: auto;
    padding: 1rem 1.25rem;
  }
  .runs__intro {
    max-width: 40rem;
  }
  .runs__error {
    color: var(--color-danger, #b54a3a);
    white-space: pre-wrap;
  }
  .runs__loading,
  .explain {
    color: var(--color-fg-muted, #9a9a9a);
  }
  .rd__head h2 {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    margin: 0 0 0.5rem;
  }
  .rd__head h2 .run__dot {
    margin-top: 0;
  }
  .rd__state {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.85rem);
    font-weight: normal;
  }
  .rd__badge {
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--color-border-strong, #555);
    border-radius: var(--radius-sm, 3px);
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    font-weight: normal;
  }
  .rd__meta {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 0.2rem 0.75rem;
    margin: 0 0 0.75rem;
    font-size: var(--text-sm, 0.85rem);
  }
  .rd__meta dt {
    color: var(--color-fg-muted, #9a9a9a);
  }
  .rd__meta dd {
    margin: 0;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent, #c9a227);
    cursor: pointer;
    font: inherit;
  }
  .link:hover {
    text-decoration: underline;
  }
  .rd__result span {
    color: var(--color-fg-muted, #9a9a9a);
  }
  .rd__nodes {
    display: grid;
    gap: 0.4rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .node {
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--color-border, #333);
    border-left-width: 3px;
    border-radius: var(--radius-md, 4px);
  }
  .node--ran {
    border-left-color: var(--color-success, #7a9e3a);
  }
  .node--failed {
    border-left-color: var(--color-danger, #b54a3a);
  }
  .node--waiting {
    border-left-color: var(--color-accent, #c9a227);
  }
  .node--skipped,
  .node--idle {
    opacity: 0.7;
  }
  .node__head {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }
  .node__id {
    color: var(--color-fg-muted, #9a9a9a);
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
  }
  .node__title {
    font-weight: 600;
  }
  .node__state {
    margin-left: auto;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .node__fail {
    margin: 0.3rem 0 0;
    color: var(--color-danger, #b54a3a);
    font-size: var(--text-sm, 0.85rem);
    white-space: pre-wrap;
  }
  .node__out {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 0.15rem 0.6rem;
    margin: 0.35rem 0 0;
    font-size: var(--text-sm, 0.85rem);
  }
  .node__out dt {
    color: var(--color-fg-muted, #9a9a9a);
    font-family: var(--font-mono, monospace);
  }
  .node__out dd {
    min-width: 0;
    margin: 0;
    overflow-wrap: anywhere;
  }
  .node__exits {
    margin: 0.3rem 0 0;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  pre {
    max-height: 20rem;
    margin: 0.3rem 0 0;
    overflow: auto;
    padding: 0.4rem;
    background: var(--color-bg-muted, #1e1e1e);
    font-size: var(--text-xs, 0.75rem);
    white-space: pre-wrap;
  }
  .rd__sub {
    margin: 1rem 0 0.4rem;
  }
  .job {
    margin-bottom: 0.4rem;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
  }
  .job__head {
    display: flex;
    justify-content: space-between;
    font-size: var(--text-sm, 0.85rem);
  }
  .job__state {
    color: var(--color-fg-muted, #9a9a9a);
  }
</style>
