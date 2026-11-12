<script lang="ts">
  import { createEntitySearch } from '@/shared/lib/entitySearch.svelte.ts'
  import SearchState from '@/shared/ui/SearchState.svelte'
  import PageToolbar from '@/shared/ui/PageToolbar.svelte'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import FlagHints from '@/shared/ui/FlagHints.svelte'
  import { helpLine } from '@/shared/lib/searchFlags.ts'
  import { STATE, KIND, span, at, took } from '../model/runnerWords.ts'
  import { runnerGroups, OPEN_STATES } from '../model/runnerGroups.ts'
  import { filterRunners, normalizeRunnerQuery, runnerView, setRunnerView } from '../model/runnerSearch.ts'
  import PageLayout from '@/shared/ui/PageLayout.svelte'
  import { onMount } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import RefText from '@/shared/ui/RefText.svelte'
  import { refIcons } from '@/shared/lib/refIcons.svelte.ts'
  import { refPreview } from '@/shared/lib/refPreview.ts'
  import type { RunnerJob } from '@/shared/api/execution.gen.ts'
  import type { RunnerView } from '@/shared/api/execution_http.gen.ts'
  import { cancelRunnerJob, createRunner, deleteRunner, getRunnerJobs, getRunners } from '@/shared/api/client.ts'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import { formatLocal } from '@/shared/lib/time.ts'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { openTarget } from '@/shared/lib/openTarget.ts'

  /**
   * Devices that run checks for us, and what they are doing: who is online,
   * what runs where, what waits in the queue, how the latest jobs ended.
   */
  let { active = true }: { active?: boolean } = $props()

  let runners: RunnerView[] = $state([])
  let jobs: RunnerJob[] = $state([])
  let loaded = $state(false)
  let now = $state(Date.now())
  /** id of a finished job whose output is open */
  let openJob = $state((null as number | null))

  // --- adding a runner ---
  let newLabel = $state('')
  let adding = $state(false)
  let fresh: { label: string, token: string, hours: number } | null = $state(null)

  let search = $state('')
  const filtered = $derived(filterRunners(search, runners, jobs,{text:false}))
  const jobFilter = $derived(runnerView(filtered.parsed))
  const remoteSearch = createEntitySearch(() => filtered.parsed.text ? {q:filtered.parsed.text,types:['runner','job'],allowed:{runner:filtered.devices.map(d=>d.id),job:filtered.jobs.map(j=>j.id)}} : null,{all:true})
  const searched = $derived.by(() => {
    if (remoteSearch.hits == null) return filtered
    const devices = remoteSearch.select(filtered.devices,'runner')
    const labels = new Set(devices.map(d=>d.label))
    const jobIds = new Set(remoteSearch.select(filtered.jobs,'job').map(j=>j.id))
    const foundJobs = filtered.jobs.filter(j=>jobIds.has(j.id)||labels.has(j.runner_label))
    for (const j of foundJobs) labels.add(j.runner_label)
    return {devices:filtered.devices.filter(d=>labels.has(d.label)),jobs:foundJobs}
  })
  const groups = $derived(runnerGroups(searched.devices, searched.jobs, jobFilter))
  let showAdd = $state(false)
  let openGroup = $state<string | null>(null)

  function jobRef(j: RunnerJob) {
    if (j.step_id) return `step=${j.step_id}`
    if (j.note_id) return `note=${j.note_id}`
    if (j.template_id) return `template=${j.template_id}`
    if (j.quest_id) return `quest=${j.quest_id}`
    return null
  }

  const onRef = (ref: string, id: number) => openTarget({ kind: 'ref', ref, id })

  async function load() {
    try {
      const [r, j] = await Promise.all([getRunners(), getRunnerJobs({ limit: 150 })])
      runners = r ?? []
      jobs = j ?? []
    } catch {
      /* toast already shown */
    } finally {
      loaded = true
    }
  }

  onMount(() => {
    const tick = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(tick)
  })

  // Poll while the tab is shown.
  $effect(() => {
    if (!active) return
    void load()
    const t = setInterval(load, 4000)
    return () => clearInterval(t)
  })

  function presence(r: RunnerView) {
    if (r.revoked) return { cls: 'off', text: 'отозван' }
    if (!r.registered) return { cls: 'wait', text: 'ещё не подключён' }
    const seen = at(r.last_seen_at)
    if (r.online) return { cls: 'on', text: 'в сети' }
    return { cls: 'off', text: seen ? `офлайн ${span(now - seen)}` : 'офлайн' }
  }

  async function add() {
    const label = newLabel.trim().toLowerCase()
    if (!label || adding) return
    adding = true
    try {
      const r = await createRunner(label)
      fresh = { label: r.label, token: r.registration_token, hours: r.valid_hours }
      newLabel = ''
      await load()
    } catch {
      /* toast already shown */
    } finally {
      adding = false
    }
  }

  let command = $derived(
    fresh ? `quests-runner register --server ${location.origin} --token ${fresh.token}` : '',
  )

  async function copyCommand() {
    try {
      await copyText(command)
      toast('Команда скопирована', { kind: 'success' })
    } catch {
      /* ignore */
    }
  }

  async function revoke(r: RunnerView) {
    if (!confirm(`Отозвать раннер «${r.label}»? Его задания в очереди будут отменены.`)) return
    try {
      await deleteRunner(r.label)
      await load()
    } catch {
      /* toast already shown */
    }
  }

  async function cancel(j: RunnerJob) {
    try {
      await cancelRunnerJob(j.id)
      await load()
    } catch {
      /* toast already shown */
    }
  }
</script>

{#snippet jobTitle(j: RunnerJob)}
  {@const ref = jobRef(j)}
  {#if ref}
    <span use:refIcons use:refPreview={onRef}>
      <a
        class="md-ref"
        href={`?${ref}`}
        onclick={(event) => {
          if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
          event.preventDefault()
          const [kind, id] = ref.split('=')
          onRef(kind, Number(id))
        }}
      >{j.title || ref}</a>
    </span>
  {:else}
    <RefText source={j.title || `задание ${j.id}`} {onRef} />
  {/if}
{/snippet}

<PageLayout>
  {#snippet toolbar()}
    <PageToolbar>
      <input class="search" type="search" bind:value={search} aria-label="Поиск устройств и заданий" placeholder="Устройство, статус, название… device=main-pc status=queued" title={`Фильтры: ${helpLine(filtered.spec)}; также поддерживается key=value`} />
      <button type="button" class="btn btn--accent btn--icon-only" title="Добавить раннер" aria-label="Добавить раннер" aria-expanded={showAdd} onclick={() => showAdd = !showAdd}><Icon name="add" size={16}/></button>
    </PageToolbar>
    <PageToolbar>
      <OptionPills options={[{id:'all',label:'Все задания'}, {id:'active',label:'В работе'}, {id:'recent',label:'Недавние'}]} value={jobFilter} onToggle={v => search = setRunnerView(search,v)} label="Задания" compact />
      {#if search}<button type="button" class="btn btn--ghost" onclick={() => search = ''}>Сбросить</button>{/if}
    </PageToolbar>
    {#if showAdd}
    <form
      class="runners__add"
      onsubmit={(e) => {
        e.preventDefault()
        void add()
      }}
    >
      <input
        class="search"
        type="text"
        placeholder="метка нового раннера, напр. home"
        maxlength="32"
        bind:value={newLabel}
        aria-label="Метка нового раннера"
        spellcheck="false"
      />
      <button type="submit" class="btn" disabled={!newLabel.trim() || adding}>
        <Icon name="add" size={14} />
        <span>Добавить</span>
      </button>
    </form>
    {/if}
    <SearchState pending={remoteSearch.pending} error={remoteSearch.error} />
    <FlagHints value={normalizeRunnerQuery(search)} spec={filtered.spec} parsed={filtered.parsed} onPick={v => search = v} />
  {/snippet}
<div class="runners">

    {#if fresh}
      <div class="runners__fresh">
        <p>
          Раннер <b>{fresh.label}</b> создан. На устройстве выполните (токен одноразовый, действует {fresh.hours} ч):
        </p>
        <pre class="mono">{command}</pre>
        <p class="runners__fresh-actions">
          <button type="button" class="btn" onclick={copyCommand}>
            <Icon name="copy" size={14} />
            <span>Копировать</span>
          </button>
          <button type="button" class="btn btn--ghost" onclick={() => (fresh = null)}>Скрыть</button>
        </p>
        <p class="hint">Затем <code>quests-runner run</code> (или юнит <code>quests-runner.service</code>). Метку указывают в поле «устройство» автопроверки шага.</p>
      </div>
    {/if}

  {#if !loaded}
    <p class="hint runners__empty">Загрузка…</p>
  {:else if !groups.length}
    <p class="hint runners__empty">{search ? 'Устройства и задания не найдены. Измените поиск или фильтры.' : 'Раннеров пока нет. Добавьте устройство кнопкой «+».'}</p>
  {:else}
    {#each groups as deviceGroup (deviceGroup.label)}
      {@const r = deviceGroup.device}
      {@const p = r ? presence(r) : {cls:'off',text:'устройство не зарегистрировано'}}
      <section class="runner-group" aria-label={`Раннер ${deviceGroup.label}`}>
        <header class="runner-group__head">
          <span class="dot dot--{p.cls}" aria-hidden="true"></span>
          <h2 class="mono">{deviceGroup.label}</h2>
          <span class="runner__state">{p.text}</span>
          <span class="runner__meta">{r?.hostname || ''}{r?.version ? ` · ${r.version}` : ''}</span>
          <span class="count" title="Заданий по текущим фильтрам">{deviceGroup.total}</span>
          {#if r && !r.revoked}<button type="button" class="btn btn--ghost" title="Отозвать раннер" aria-label={`Отозвать раннер ${r.label}`} onclick={() => revoke(r)}><Icon name="delete" size={14}/></button>{/if}
        </header>
        <ul class="jobs">
          {#each deviceGroup.rows as g (g.key)}
            {@const last = g.last}
            {@const st = STATE[last.state] ?? {text:last.state,cls:'off'}}
            {@const activeJob = OPEN_STATES.includes(last.state)}
            {@const started = at(last.started_at) ?? at(last.assigned_at)}
            <li class="job" class:job--run={activeJob}>
              <div class="job__row">
                <span class="pill pill--{st.cls}">{st.text}</span>
                <span class="job__title">{@render jobTitle(last)}</span>
                <span class="job__meta">{KIND[last.kind] ?? last.kind}{last.state === 'queued' && at(last.created_at) ? ` · ждёт ${span(now-at(last.created_at)!)}` : activeJob && started ? ` · ${span(now-started)}` : last.finished_at ? ` · ${formatLocal(last.finished_at)}` : ''}{g.failed && !activeJob ? ` · ошибок ${g.failed}` : ''}</span>
                {#if activeJob}
                  <button type="button" class="btn btn--ghost" title={last.state === 'queued' ? 'Убрать из очереди' : 'Остановить'} aria-label={last.state === 'queued' ? 'Убрать из очереди' : 'Остановить'} onclick={() => cancel(last)}><Icon name="close" size={14}/></button>
                {:else}
                  <button type="button" class="job__toggle" onclick={() => openGroup = openGroup === g.key ? null : g.key} aria-expanded={openGroup === g.key} aria-label={`История заданий: ${last.title || last.id}`}><span>{g.list.length}</span><Icon name={openGroup === g.key ? 'chevron-down' : 'chevron-right'} size={14}/></button>
                {/if}
              </div>
              {#if activeJob}
                {#if last.progress_total}<div class="bar" title={`${last.progress_current ?? 0} / ${last.progress_total}`}><span style:width={`${Math.min(100,((last.progress_current ?? 0)/last.progress_total)*100)}%`}></span></div>{/if}
                {#if last.log_tail}<pre class="log mono">{last.log_tail}</pre>{/if}
                {#if last.cancel}<p class="hint">Остановка запрошена, раннер услышит её при следующем сигнале.</p>{/if}
                {#if last.state === 'queued' && (!r || !r.online)}<p class="hint">{r ? 'Устройство офлайн — задание дождётся его.' : 'Устройство не зарегистрировано — задание ожидает подключения.'}</p>{/if}
              {:else if openGroup === g.key}
                <ul class="jobs jobs--inner">
                  {#each g.list as j (j.id)}
                    {@const jst = STATE[j.state] ?? {text:j.state,cls:'off'}}
                    <li class="job">
                      <button type="button" class="job__row job__row--btn" onclick={() => openJob = openJob === j.id ? null : j.id} aria-expanded={openJob === j.id}>
                        <span class="pill pill--{jst.cls}">{jst.text}</span><span class="job__title">{j.finished_at ? formatLocal(j.finished_at) : `задание ${j.id}`}</span><span class="job__meta">{j.attempt_no > 1 ? `попытка ${j.attempt_no}` : ''}{took(j) ? ` · ${took(j)}` : ''}</span><Icon name={openJob === j.id ? 'chevron-down' : 'chevron-right'} size={14}/>
                      </button>
                      {#if j.reason}<p class="hint">{j.reason}</p>{/if}
                      {#if openJob === j.id}<div class="job__out"><p class="hint mono">{j.command}</p>{#if j.stdout}<pre class="log mono">{j.stdout}</pre>{/if}{#if j.stderr}<pre class="log log--err mono">{j.stderr}</pre>{/if}{#if !j.stdout && !j.stderr}<p class="hint">Вывода нет.</p>{/if}{#if j.run_id}<button type="button" class="btn btn--ghost" onclick={() => openTarget({kind:'runs',run:j.run_id})}>Весь запуск скрипта</button>{/if}</div>{/if}
                    </li>
                  {/each}
                </ul>
              {/if}
            </li>
          {:else}<li class="hint runners__empty">{search || jobFilter !== 'all' ? 'Нет заданий по текущим фильтрам.' : 'Заданий ещё не было.'}</li>{/each}
        </ul>
      </section>
    {/each}
  {/if}
</div>
</PageLayout>

<style>
  .runners { display:grid; align-content:start; gap:1rem; padding:0.5rem 0 1rem; min-width:0; font-size:var(--text-sm); }
  .jobs { display:grid; grid-template-columns:minmax(0,1fr); list-style:none; margin:0; padding:0; }
  .job + .job { border-top:1px solid var(--color-border); }
  .runner__meta { flex:1 1 auto; min-width:0; color:var(--color-fg-muted); font-size:var(--text-xs); overflow-wrap:anywhere; }
  .runner__state { color:var(--color-fg-muted); font-size:var(--text-xs); }
  .runners__add { display:flex; gap:0.5rem; width:100%; max-width:27rem; }
  .runners__fresh { margin:0 1rem; padding:0.75rem; border:1px solid var(--color-border); border-radius:var(--radius-md); }
  .runners__fresh pre { overflow:auto; }
  .runners__fresh-actions { display:flex; flex-wrap:wrap; gap:0.5rem; }

  .runner-group { min-width:0; }
  .runner-group__head { display:flex; flex-wrap:wrap; align-items:center; gap:0.4rem 0.65rem; padding:0.55rem 1rem; border-block:1px solid var(--color-border); background:color-mix(in srgb,var(--color-fg) 4%,var(--color-bg)); }
  .runner-group__head h2 { margin:0; font:600 var(--text-sm) var(--font-ui); }
  .runner-group > .jobs { padding-inline:1rem; }
  .runners__empty { padding:0.75rem 1rem; }
  @media(max-width:40rem) { .runner-group__head { padding-inline:0.75rem; } .runner-group > .jobs { padding-inline:0.75rem; } }

  .count {
    padding: 0.05rem 0.4rem;
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-muted, #242424);
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    font-weight: 400;
    font-variant-numeric: tabular-nums;
  }

  .hint {
    margin: 0.4rem 0;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    overflow-wrap: anywhere;
  }

  .dot {
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 50%;
    flex: 0 0 auto;
    background: var(--color-fg-subtle, #6e6e6e);
  }

  .dot--on {
    background: var(--color-success, #7a9e3a);
  }

  .dot--wait {
    background: var(--color-warning, #c47a20);
  }

  .job {
    min-width: 0;
    padding:0.5rem 0;
  }

  .job__row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem 0.75rem;
    min-width: 0;
  }

  .job__row--btn {
    width: 100%;
    padding: 0.35rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .job__row--btn:hover,
  .job__toggle:hover {
    background: var(--color-bg-hover, #2a2a2a);
  }

  .job__row--btn:focus-visible,
  .job__toggle:focus-visible {
    outline: 2px solid var(--color-accent, #c9a227);
    outline-offset: 2px;
  }

  .job__title {
    flex: 1 1 16rem;
    min-width: 0;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }

  .job__title :global(.md-ref) {
    color: inherit;
    text-decoration: none;
    border-bottom: 1px dotted var(--color-border-strong, #4a4a4a);
  }

  .job__title :global(.md-ref:hover) {
    color: var(--color-accent, #c9a227);
    border-bottom-color: currentColor;
  }

  .job__meta {
    flex: 0 1 auto;
    min-width: 0;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    font-variant-numeric: tabular-nums;
    overflow-wrap: anywhere;
  }

  .job__toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 2rem;
    padding: 0.25rem 0.4rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font: inherit;
    font-size: var(--text-xs, 0.75rem);
    font-variant-numeric: tabular-nums;
  }

  .pill {
    white-space:nowrap;
    flex: 0 0 auto;
    min-width: 4.5rem;
    padding: 0.15rem 0.45rem;
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, currentColor 9%, transparent);
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    text-align: center;
  }

  .pill--run {
    color: var(--color-info, #5a8a7a);
  }

  .pill--wait {
    color: var(--color-warning, #c47a20);
  }

  .pill--ok {
    color: var(--color-success, #7a9e3a);
  }

  .pill--bad,
  .log--err {
    color: var(--color-danger, #b54a3a);
  }

  .pill--warn {
    color: var(--color-warning, #c47a20);
  }

  .bar {
    margin-top: 0.65rem;
    height: 4px;
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg-muted, #242424);
    overflow: hidden;
  }

  .bar span {
    display: block;
    height: 100%;
    background: var(--color-info, #5a8a7a);
    transition: width 0.3s ease;
  }

  .jobs--inner {
    margin-top: 0.75rem;
    padding: 0 0.75rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--pill-bg, #151515);
  }

  .job__out {
    margin-top: 0.5rem;
  }

  .log {
    margin: 0.5rem 0 0;
    padding: 0.65rem;
    max-height: 16rem;
    overflow: auto;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--pill-bg, #151515);
    font-size: var(--text-xs, 0.75rem);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  @media (max-width: 720px) {

    .job__title {
      flex-basis: calc(100% - 7rem);
    }

    .job__meta {
      flex: 1 1 12rem;
    }
  }
</style>
