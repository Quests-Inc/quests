<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import RefText from '@/shared/ui/RefText.svelte'
  import { refIcons } from '@/shared/lib/refIcons.svelte.ts'
  import { refPreview } from '@/shared/lib/refPreview.ts'
  import type { RunnerJob } from '@/shared/api/execution.gen.ts'
  import type { RunnerView } from '@/shared/api/execution_http.gen.ts'
  import { cancelRunnerJob, createRunner, deleteRunner, getRunnerJobs, getRunners } from '@/shared/api/client.ts'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import { formatLocal, parseApiDate } from '@/shared/lib/time.ts'
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

  const OPEN_STATES = ['queued', 'assigned', 'running']
  let running = $derived(jobs.filter((j) => j.state === 'assigned' || j.state === 'running'))
  let queued = $derived(jobs.filter((j) => j.state === 'queued'))
  let finished = $derived(jobs.filter((j) => !OPEN_STATES.includes(j.state)))
  /** Finished jobs by what queued them (a step, a note, a template, else the title): a check every 5 minutes is one line. */
  const groupKey = (j: RunnerJob) =>
    j.step_id ? `step:${j.step_id}` : j.note_id ? `note:${j.note_id}` : j.template_id ? `template:${j.template_id}` : `title:${j.title}`
  let groups = $derived.by(() => {
    const m = new Map<string, RunnerJob[]>()
    for (const j of finished) {
      const k = groupKey(j)
      const list = m.get(k)
      if (list) list.push(j)
      else m.set(k, [j])
    }
    return [...m.entries()].map(([key, list]) => ({ key, last: list[0], list, failed: list.filter((j) => j.state !== 'success' && j.state !== 'canceled').length }))
  })
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

  function span(ms: number) {
    const s = Math.max(0, Math.round(ms / 1000))
    if (s < 60) return `${s} с`
    const m = Math.floor(s / 60)
    if (m < 60) return `${m} мин`
    const h = Math.floor(m / 60)
    if (h < 48) return `${h} ч ${m % 60} мин`
    return `${Math.floor(h / 24)} дн`
  }

  const at = (iso: string | null | undefined) => (iso ? parseApiDate(iso)?.getTime() ?? null : null)

  function presence(r: RunnerView) {
    if (r.revoked) return { cls: 'off', text: 'отозван' }
    if (!r.registered) return { cls: 'wait', text: 'ещё не подключён' }
    const seen = at(r.last_seen_at)
    if (r.online) return { cls: 'on', text: 'в сети' }
    return { cls: 'off', text: seen ? `офлайн ${span(now - seen)}` : 'офлайн' }
  }

  const STATE: Record<string, { text: string; cls: string }> = {
    queued: { text: 'в очереди', cls: 'wait' },
    assigned: { text: 'взято', cls: 'run' },
    running: { text: 'идёт', cls: 'run' },
    success: { text: 'готово', cls: 'ok' },
    fail: { text: 'ошибка', cls: 'bad' },
    timeout: { text: 'таймаут', cls: 'bad' },
    lost: { text: 'связь потеряна', cls: 'warn' },
    skipped: { text: 'устройство не появилось', cls: 'warn' },
    abandoned: { text: 'результат неизвестен', cls: 'bad' },
    canceled: { text: 'отменено', cls: 'off' },
  }
  const KIND: Record<string, string> = { poll: 'опрос', once: 'разово', emit_pool: 'шаблон' }

  function took(j: RunnerJob) {
    const a = at(j.started_at) ?? at(j.assigned_at)
    const b = at(j.finished_at)
    return a && b ? span(b - a) : ''
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

<div class="runners">
  <header class="runners__intro">
    <h1>Раннеры</h1>
    <p>Устройства, очередь и результаты выполнения заданий.</p>
  </header>
  <details class="runners__block" open>
    <summary>
      <h2><Icon name="chevron-right" size={14} /> Устройства <span class="count">{runners.length}</span></h2>
    </summary>
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
      <p class="hint">Загрузка…</p>
    {:else if !runners.length}
      <p class="hint">Раннеров пока нет. Раннер выполняет автопроверки шагов и команды шаблонов на другом устройстве (ноут, домашний ПК) — он сам ходит на сервер, открывать порты не нужно.</p>
    {:else}
      <ul class="runners__list">
        {#each runners as r (r.id)}
          {@const p = presence(r)}
          <li class="runner">
            <span class="dot dot--{p.cls}" aria-hidden="true"></span>
            <span class="runner__label mono">{r.label}</span>
            <span class="runner__state">{p.text}</span>
            <span class="runner__meta">
              {#if r.hostname}{r.hostname}{/if}
              {#if r.version} · {r.version}{/if}
            </span>
            <span class="runner__counts">
              {#if r.running}<span class="pill pill--run">идёт {r.running}</span>{/if}
              {#if r.queued}<span class="pill pill--wait">в очереди {r.queued}</span>{/if}
            </span>
            {#if !r.revoked}
              <button type="button" class="btn btn--ghost runner__rm" title="Отозвать раннер" onclick={() => revoke(r)}>
                <Icon name="delete" size={14} />
              </button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </details>

  <details class="runners__block" open>
    <summary>
      <h2><Icon name="chevron-right" size={14} /> Выполняется <span class="count">{running.length}</span></h2>
    </summary>
    {#if !running.length}
      <p class="hint">Сейчас ничего не выполняется.</p>
    {:else}
      <ul class="jobs">
        {#each running as j (j.id)}
          {@const started = at(j.started_at) ?? at(j.assigned_at)}
          <li class="job job--run">
            <div class="job__row">
              <span class="pill pill--run">{STATE[j.state]?.text}</span>
              <span class="job__title">
                {@render jobTitle(j)}
              </span>
              <span class="job__meta">{j.runner_label} · {KIND[j.kind] ?? j.kind}{started ? ` · ${span(now - started)}` : ''}</span>
              <button type="button" class="btn btn--ghost" title="Остановить" onclick={() => cancel(j)}>
                <Icon name="close" size={14} />
              </button>
            </div>
            {#if j.progress_total}
              <div class="bar" title="{j.progress_current ?? 0} / {j.progress_total}">
                <span style:width="{Math.min(100, ((j.progress_current ?? 0) / j.progress_total) * 100)}%"></span>
              </div>
            {/if}
            {#if j.log_tail}<pre class="log mono">{j.log_tail}</pre>{/if}
            {#if j.cancel}<p class="hint">Остановка запрошена, раннер услышит её при следующем сигнале.</p>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </details>

  <details class="runners__block" open>
    <summary>
      <h2><Icon name="chevron-right" size={14} /> Очередь <span class="count">{queued.length}</span></h2>
    </summary>
    {#if !queued.length}
      <p class="hint">Очередь пуста.</p>
    {:else}
      <ul class="jobs">
        {#each queued as j (j.id)}
          {@const waited = at(j.created_at)}
          {@const until = at(j.valid_until)}
          {@const dev = runners.find((r) => r.label === j.runner_label)}
          <li class="job">
            <div class="job__row">
              <span class="pill pill--wait">ждёт</span>
              <span class="job__title">
                {@render jobTitle(j)}
              </span>
              <span class="job__meta">
                {j.runner_label} · {KIND[j.kind] ?? j.kind}{waited ? ` · ждёт ${span(now - waited)}` : ''}{until ? ` · до ${new Date(until).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}` : ''}
              </span>
              <button type="button" class="btn btn--ghost" title="Убрать из очереди" onclick={() => cancel(j)}>
                <Icon name="close" size={14} />
              </button>
            </div>
            {#if dev && !dev.online}<p class="hint">Устройство «{j.runner_label}» офлайн — задание дождётся его.</p>{/if}
            {#if !dev}<p class="hint">Раннера «{j.runner_label}» нет: добавьте его выше, иначе задание истечёт.</p>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </details>

  <details class="runners__block" open>
    <summary>
      <h2><Icon name="chevron-right" size={14} /> Недавние <span class="count">{finished.length}</span></h2>
    </summary>
    {#if !finished.length}
      <p class="hint">Заданий ещё не было.</p>
    {:else}
      <ul class="jobs">
        {#each groups as g (g.key)}
          {@const last = g.last}
          {@const st = STATE[last.state] ?? { text: last.state, cls: 'off' }}
          <li class="job">
            <div class="job__row">
              <span class="pill pill--{st.cls}">{st.text}</span>
              <span class="job__title">{@render jobTitle(last)}</span>
              <span class="job__meta">
                {last.runner_label} · {KIND[last.kind] ?? last.kind}{g.failed ? ` · ошибок ${g.failed}` : ''}
              </span>
              <button
                type="button"
                class="job__toggle"
                onclick={() => (openGroup = openGroup === g.key ? null : g.key)}
                aria-expanded={openGroup === g.key}
                aria-label={`История заданий: ${last.title || last.id}`}
              >
                <span>{g.list.length}</span>
                <Icon name={openGroup === g.key ? 'chevron-down' : 'chevron-right'} size={14} />
              </button>
            </div>
            {#if openGroup === g.key}
              <ul class="jobs jobs--inner">
                {#each g.list as j (j.id)}
                  {@const jst = STATE[j.state] ?? { text: j.state, cls: 'off' }}
                  <li class="job">
                    <button type="button" class="job__row job__row--btn" onclick={() => (openJob = openJob === j.id ? null : j.id)} aria-expanded={openJob === j.id}>
                      <span class="pill pill--{jst.cls}">{jst.text}</span>
                      <span class="job__title">{j.finished_at ? formatLocal(j.finished_at) : `задание ${j.id}`}</span>
                      <span class="job__meta">{j.attempt_no > 1 ? `попытка ${j.attempt_no}` : ''}{took(j) ? ` · ${took(j)}` : ''}</span>
                      <Icon name={openJob === j.id ? 'chevron-down' : 'chevron-right'} size={14} />
                    </button>
                    {#if j.reason}<p class="hint">{j.reason}</p>{/if}
                    {#if openJob === j.id}
                      <div class="job__out">
                        <p class="hint mono">{j.command}</p>
                        {#if j.stdout}<pre class="log mono">{j.stdout}</pre>{/if}
                        {#if j.stderr}<pre class="log log--err mono">{j.stderr}</pre>{/if}
                        {#if !j.stdout && !j.stderr}<p class="hint">Вывода нет.</p>{/if}
                        {#if j.run_id}
                          <button type="button" class="btn btn--ghost" onclick={() => openTarget({ kind: 'runs', run: j.run_id })}>Весь запуск скрипта</button>
                        {/if}
                      </div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </details>
</div>

<style>
  .runners {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    align-content: start;
    gap: 1.25rem;
    width: 100%;
    max-width: 72rem;
    margin: 0 auto;
    padding: 1.5rem 2rem 2rem;
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .runners__intro h1 {
    margin: 0;
    font-family: var(--font-display, serif);
    font-size: var(--text-xl, 1.6rem);
    font-weight: 600;
  }

  .runners__intro p {
    margin: 0.35rem 0 0;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .runners__block {
    min-width: 0;
    padding: 1rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-lg, 12px);
    background: var(--color-bg-raised, #1a1a1a);
  }

  .runners__block h2 {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    font-weight: 600;
  }

  .runners__block > summary {
    list-style: none;
    cursor: pointer;
    border-radius: var(--radius-md, 4px);
  }

  .runners__block > summary::-webkit-details-marker {
    display: none;
  }

  .runners__block[open] > summary {
    margin-bottom: 0.75rem;
  }

  .runners__block > summary:hover {
    color: var(--color-accent, #c9a227);
  }

  .runners__block > summary:focus-visible {
    outline: 2px solid var(--color-accent, #c9a227);
    outline-offset: 4px;
  }

  .runners__block[open] > summary :global(.icon) {
    transform: rotate(90deg);
  }

  .runners__add {
    display: flex;
    flex: 1 1 18rem;
    gap: 0.5rem;
    max-width: 27rem;
    margin: 0 0 0.75rem;
  }

  .runners__add .search {
    flex: 1;
    min-width: 0;
  }

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

  .runners__fresh {
    margin: 0.75rem 0;
    padding: 0.75rem;
    border: 1px solid color-mix(in srgb, var(--color-accent, #c9a227) 45%, var(--color-border, #333));
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 5%, var(--color-bg-raised, #1a1a1a));
  }

  .runners__fresh p {
    margin: 0.25rem 0;
  }

  .runners__fresh pre {
    margin: 0.6rem 0;
    padding: 0.6rem;
    overflow-x: auto;
    border-radius: var(--radius-md, 4px);
    background: var(--pill-bg, #151515);
    font-size: var(--text-xs, 0.75rem);
    user-select: all;
  }

  .runners__fresh-actions {
    display: flex;
    gap: 0.5rem;
  }

  .runners__list,
  .jobs {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .runner {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem 0.75rem;
    min-width: 0;
    padding: 0.75rem 0;
  }

  .runner + .runner,
  .job + .job {
    border-top: 1px solid var(--color-border, #333);
  }

  .runner__label {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .runner__state,
  .runner__meta {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }

  .runner__meta {
    flex: 1 1 auto;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .runner__counts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
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
    padding: 0.8rem 0;
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
    .runners {
      padding: 1rem 0.75rem;
      gap: 0.75rem;
    }

    .runners__block {
      padding: 0.75rem;
    }

    .runners__add {
      max-width: none;
      flex-basis: 100%;
    }

    .runner {
      display: grid;
      grid-template-columns: 0.5rem minmax(0, 1fr) auto;
    }

    .runner__label {
      grid-area: 1 / 2;
    }

    .runner__state {
      grid-area: 1 / 3;
    }

    .runner__meta {
      grid-area: 2 / 2;
    }

    .runner__counts {
      grid-area: 3 / 2;
    }

    .runner__rm {
      grid-area: 2 / 3 / 4;
      justify-self: end;
    }

    .job__title {
      flex-basis: calc(100% - 7rem);
    }

    .job__meta {
      flex: 1 1 12rem;
    }
  }
</style>
