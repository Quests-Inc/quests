<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { cancelRunnerJob, createRunner, deleteRunner, getRunnerJobs, getRunners } from '@/shared/api/client.ts'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import { formatLocal, parseApiDate } from '@/shared/lib/time.ts'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { openTarget } from '@/shared/lib/openTarget.ts'

  /**
   * Devices that run checks for us, and what they are doing: who is online,
   * what runs where, what waits in the queue, how the latest jobs ended.
   */
  let { active = true, onOpenQuest }: { active?: boolean, onOpenQuest?: (id: number) => void } = $props()

  let runners: any[] = $state([])
  let jobs: any[] = $state([])
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
  const groupKey = (j: any) =>
    j.step_id ? `step:${j.step_id}` : j.note_id ? `note:${j.note_id}` : j.template_id ? `template:${j.template_id}` : `title:${j.title}`
  let groups = $derived.by(() => {
    const m = new Map<string, any[]>()
    for (const j of finished) {
      const k = groupKey(j)
      const list = m.get(k)
      if (list) list.push(j)
      else m.set(k, [j])
    }
    return [...m.entries()].map(([key, list]) => ({ key, last: list[0], list, failed: list.filter((j) => j.state !== 'success' && j.state !== 'canceled').length }))
  })
  let openGroup = $state<string | null>(null)

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

  function presence(r: any) {
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

  function took(j: any) {
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

  async function revoke(r: any) {
    if (!confirm(`Отозвать раннер «${r.label}»? Его задания в очереди будут отменены.`)) return
    try {
      await deleteRunner(r.label)
      await load()
    } catch {
      /* toast already shown */
    }
  }

  async function cancel(j: any) {
    try {
      await cancelRunnerJob(j.id)
      await load()
    } catch {
      /* toast already shown */
    }
  }
</script>

<div class="runners">
  <section class="runners__block">
    <header class="runners__head">
      <h2>Устройства</h2>
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
    </header>

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
  </section>

  <section class="runners__block">
    <h2>Выполняется {#if running.length}<span class="count">{running.length}</span>{/if}</h2>
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
                {#if j.quest_id && onOpenQuest}
                  <button type="button" class="link" onclick={() => onOpenQuest?.(j.quest_id)}>{j.title || `задание ${j.id}`}</button>
                {:else}{j.title || `задание ${j.id}`}{/if}
              </span>
              <span class="job__meta mono">{j.runner_label} · {KIND[j.kind] ?? j.kind}{started ? ` · ${span(now - started)}` : ''}</span>
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
  </section>

  <section class="runners__block">
    <h2>Очередь {#if queued.length}<span class="count">{queued.length}</span>{/if}</h2>
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
                {#if j.quest_id && onOpenQuest}
                  <button type="button" class="link" onclick={() => onOpenQuest?.(j.quest_id)}>{j.title || `задание ${j.id}`}</button>
                {:else}{j.title || `задание ${j.id}`}{/if}
              </span>
              <span class="job__meta mono">
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
  </section>

  <section class="runners__block">
    <h2>Недавние</h2>
    {#if !finished.length}
      <p class="hint">Заданий ещё не было.</p>
    {:else}
      <ul class="jobs">
        {#each groups as g (g.key)}
          {@const last = g.last}
          {@const st = STATE[last.state] ?? { text: last.state, cls: 'off' }}
          <li class="job">
            <button type="button" class="job__row job__row--btn" onclick={() => (openGroup = openGroup === g.key ? null : g.key)} aria-expanded={openGroup === g.key}>
              <span class="pill pill--{st.cls}">{st.text}</span>
              <span class="job__title">{last.title || `задание ${last.id}`}</span>
              <span class="job__meta mono">
                {last.runner_label} · {KIND[last.kind] ?? last.kind}{g.list.length > 1 ? ` · ×${g.list.length}` : ''}{g.failed ? ` · ошибок ${g.failed}` : ''}
              </span>
            </button>
            {#if openGroup === g.key}
              <ul class="jobs jobs--inner">
                {#each g.list as j (j.id)}
                  {@const jst = STATE[j.state] ?? { text: j.state, cls: 'off' }}
                  <li class="job">
                    <button type="button" class="job__row job__row--btn" onclick={() => (openJob = openJob === j.id ? null : j.id)} aria-expanded={openJob === j.id}>
                      <span class="pill pill--{jst.cls}">{jst.text}</span>
                      <span class="job__title">{j.finished_at ? formatLocal(j.finished_at) : `задание ${j.id}`}</span>
                      <span class="job__meta mono">{j.attempt_no > 1 ? `попытка ${j.attempt_no}` : ''}{took(j) ? ` · ${took(j)}` : ''}</span>
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
  </section>
</div>

<style>
  .runners {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: 1rem 1.25rem 2rem;
    display: grid;
    /* minmax(0, …): a long title must shrink (ellipsis), not widen the whole page */
    grid-template-columns: minmax(0, 1fr);
    gap: 1.4rem;
    align-content: start;
    max-width: 62rem;
    width: 100%;
    box-sizing: border-box;
    margin: 0 auto;
  }

  .runners__block {
    min-width: 0;
  }

  .runners__block h2 {
    margin: 0 0 0.5rem;
    font-size: 0.95rem;
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .runners__head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    margin-bottom: 0.5rem;
  }

  .runners__head h2 {
    margin: 0;
  }

  .runners__add {
    display: flex;
    gap: 0.4rem;
    flex: 1 1 18rem;
    max-width: 26rem;
  }

  .runners__add .search {
    flex: 1 1 auto;
    min-width: 0;
  }

  .count {
    font-size: 0.72rem;
    padding: 0.05rem 0.45rem;
    border-radius: 999px;
    background: var(--pill-bg, #222);
    color: var(--color-fg-muted, #9a9a9a);
    font-weight: 500;
  }

  .hint {
    margin: 0.25rem 0;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    overflow-wrap: anywhere;
  }

  .runners__fresh {
    margin: 0.4rem 0 0.8rem;
    padding: 0.7rem 0.85rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: var(--pill-radius, 10px);
    background: var(--pill-bg, #1a1a1a);
    font-size: var(--text-sm, 0.85rem);
  }

  .runners__fresh p {
    margin: 0.25rem 0;
  }

  .runners__fresh pre {
    margin: 0.4rem 0;
    padding: 0.5rem 0.6rem;
    overflow-x: auto;
    border-radius: 6px;
    background: rgba(0, 0, 0, 0.3);
    font-size: 0.78rem;
    user-select: all;
  }

  .runners__fresh-actions {
    display: flex;
    gap: 0.4rem;
  }

  .runners__list,
  .jobs--inner {
    margin: 0.3rem 0 0 1rem;
  }
  .jobs {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 0.4rem;
  }

  .runner {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem 0.8rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 10px);
    background: var(--pill-bg, #151515);
  }

  .runner__label {
    font-weight: 600;
  }

  .runner__state {
    font-size: var(--text-xs, 0.78rem);
  }

  .runner__meta {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    flex: 1 1 auto;
    min-width: 0;
  }

  .runner__counts {
    display: flex;
    gap: 0.3rem;
  }

  .dot {
    width: 0.6rem;
    height: 0.6rem;
    border-radius: 50%;
    flex: 0 0 auto;
    background: #777;
  }

  .dot--on {
    background: #5cc46b;
    box-shadow: 0 0 6px rgba(92, 196, 107, 0.6);
  }

  .dot--off {
    background: #8a4040;
  }

  .dot--wait {
    background: #c9a227;
  }

  .job {
    min-width: 0;
    padding: 0.45rem 0.75rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 10px);
    background: var(--pill-bg, #151515);
  }

  .job--run {
    border-color: color-mix(in srgb, #4aa3df 50%, var(--color-border, #333));
  }

  .job__row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.2rem 0.6rem;
    min-width: 0;
  }

  .job__row--btn {
    width: 100%;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .job__title {
    flex: 1 1 14rem;
    min-width: 0;
    /* quest · step: the step name is the tail, so wrap instead of cutting it off */
    overflow-wrap: anywhere;
  }

  .job__meta {
    flex: 0 1 auto;
    min-width: 0;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.72rem;
    overflow-wrap: anywhere;
  }

  .link {
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-decoration: underline dotted;
    cursor: pointer;
    text-align: left;
  }

  .pill {
    flex: 0 0 auto;
    padding: 0.05rem 0.5rem;
    border-radius: 999px;
    font-size: 0.7rem;
    border: 1px solid currentColor;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .pill--run {
    color: #4aa3df;
  }

  .pill--wait {
    color: #c9a227;
  }

  .pill--ok {
    color: #5cc46b;
  }

  .pill--bad {
    color: #d96a6a;
  }

  .pill--warn {
    color: #e0933a;
  }

  .bar {
    margin-top: 0.4rem;
    height: 5px;
    border-radius: 3px;
    background: rgba(255, 255, 255, 0.08);
    overflow: hidden;
  }

  .bar span {
    display: block;
    height: 100%;
    background: #4aa3df;
    transition: width 0.3s ease;
  }

  .log {
    margin: 0.4rem 0 0;
    padding: 0.4rem 0.55rem;
    max-height: 12rem;
    overflow: auto;
    border-radius: 6px;
    background: rgba(0, 0, 0, 0.3);
    font-size: 0.74rem;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .log--err {
    color: #e0a0a0;
  }

  @media (max-width: 720px) {
    .job__meta {
      flex-basis: 100%;
    }
  }
</style>
