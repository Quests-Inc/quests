<script>
  import HelpTip from './HelpTip.svelte'
  import Icon from './Icon.svelte'
  import { getNote, setNoteCheck, runNoteCheck } from '../js/api.js'
  import { loadRunners, runnersList } from '../js/runnersList.svelte.js'
  import { loadWorkspaceInfo, wsInfo } from '../js/workspaceInfo.svelte.js'

  /** A note's freshness check (quest=370): the same kind of command as a step's auto-check, but its answer
   * is "out of date?" — a bare 0/1 or {"outdated": 0|1, "reason": "…"}. The state follows the server;
   * this block polls it while open.
   * @type {{ noteId: number, onOpenSecrets?: () => void, onChanged?: () => void }} */
  let { noteId, onOpenSecrets, onChanged } = $props()

  let check = $state(/** @type {any | null} */ (null))
  let open = $state(false)
  let command = $state('')
  let interval = $state('')
  let runOn = $state('')
  let busy = $state(false)
  let error = $state('')
  let dirty = $state(false)

  function adopt(c) {
    check = c
    if (!dirty) {
      command = c?.command ?? ''
      interval = c?.interval_seconds != null ? String(c.interval_seconds) : ''
      runOn = c?.run_on ?? ''
    }
  }

  async function load() {
    try {
      const n = await getNote(noteId)
      adopt(n?.check ?? null)
    } catch {
      /* the editor above reports load failures */
    }
  }

  $effect(() => {
    void noteId
    dirty = false
    error = ''
    check = null
    void load()
  })

  // Follow the verdict while the block is open (a check may finish any moment).
  $effect(() => {
    if (!open) return
    void loadRunners()
    void loadWorkspaceInfo()
    const t = setInterval(load, 15000)
    return () => clearInterval(t)
  })

  const active = $derived(Boolean(check?.command))
  const missing = $derived(runOn && !runnersList.items.some((r) => r.label === runOn) ? runOn : '')

  async function run(fn) {
    busy = true
    error = ''
    try {
      await fn()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  const save = () =>
    run(async () => {
      const iv = interval === '' ? null : Number(interval)
      await setNoteCheck(noteId, { command: command.trim() || null, interval_seconds: iv, run_on: runOn || null })
      dirty = false
      await load()
      onChanged?.()
    })

  const remove = () =>
    run(async () => {
      await setNoteCheck(noteId, { command: null })
      dirty = false
      command = ''
      await load()
      onChanged?.()
    })

  const now = () =>
    run(async () => {
      await runNoteCheck(noteId)
      error = ''
      setTimeout(load, 20000) // the scheduler ticks every 15 s
    })

  const when = (iso) => (iso ? new Date(iso).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' }) : '—')
</script>

<div class="nc">
  <button type="button" class="nc__head" aria-expanded={open} onclick={() => (open = !open)}>
    <span class="nc__title">Проверка актуальности</span>
    <Icon name={open ? 'chevron-down' : 'chevron-right'} size={12} />
    {#if check?.outdated}
      <span class="nc__chip nc__chip--bad" title={check.reason}>устарела</span>
    {:else if active && check?.last_run_at && !check?.error}
      <span class="nc__chip nc__chip--ok">актуальна</span>
    {:else if active}
      <span class="nc__chip">ждёт проверки</span>
    {/if}
    {#if check?.error}<span class="nc__chip nc__chip--warn" title={check.error}>ошибка проверки</span>{/if}
  </button>

  {#if open}
    <div class="nc__body">
      <input
        type="text"
        class="mono nc__cmd"
        placeholder="команда: напечатает 0 или 1 (1 — устарела) либо JSON"
        aria-label="Команда проверки актуальности"
        spellcheck="false"
        bind:value={command}
        oninput={() => (dirty = true)}
      />
      <div class="nc__opts">
        <input
          type="number"
          class="nc__interval"
          min="15"
          step="15"
          placeholder="сек"
          title="Как часто проверять, сек (мин. 15; по умолчанию 3600)"
          aria-label="Интервал, сек"
          bind:value={interval}
          oninput={() => (dirty = true)}
        />
        <select class="nc__runon" aria-label="Устройство" bind:value={runOn} onchange={() => (dirty = true)}>
          {#if wsInfo.serverExec}
            <option value="">на сервере</option>
          {:else}
            <option value="" disabled>выберите устройство</option>
          {/if}
          {#each runnersList.items as r (r.label)}
            <option value={r.label}>{r.label}{r.online ? '' : r.registered ? ' (офлайн)' : ' (не подключён)'}</option>
          {/each}
          {#if missing}<option value={missing}>{missing} (нет такого раннера)</option>{/if}
        </select>
        <HelpTip label="Как работает проверка актуальности">
          <p>Команда работает как автопроверка шага: на сервере (в доверенном пространстве) или на раннере, с <b>секретами этой заметки</b> в окружении, раз в N секунд. Ставить команду может только владелец пространства.</p>
          <p>Ответ: последняя строка <code>0</code> / <code>1</code> (1 — устарела) или JSON <code>{`{"outdated": 1, "reason": "что разошлось"}`}</code>. Любой другой исход (код ≠ 0, таймаут, непонятный вывод) — это ошибка проверки: вердикт не меняется.</p>
          <p>Прошлое значение хранит сам скрипт — удобно строкой в тексте заметки (<code>&lt;!-- verified: abc1234 --&gt;</code>) и сравнением с <code>git log -1 --format=%h -- путь</code>. Заметку «актуализируют» правкой этой строки. В скрипте доступен <code>$QUESTS_NOTE_ID</code>.</p>
        </HelpTip>
      </div>
      <div class="nc__actions">
        <button type="button" class="btn btn--primary" disabled={busy || !command.trim()} onclick={save}>Сохранить</button>
        <button type="button" class="btn" disabled={busy || !active || dirty} onclick={now} title="Запустить на ближайшем такте (до 15 с)">Проверить сейчас</button>
        <button type="button" class="btn" onclick={() => onOpenSecrets?.()}>Секреты</button>
        {#if active}<button type="button" class="btn btn--danger-solid" disabled={busy} onclick={remove}>Убрать проверку</button>{/if}
      </div>
      {#if check?.reason}<p class="nc__line">Причина: {check.reason}</p>{/if}
      {#if active}<p class="nc__line nc__muted">Последний запуск: {when(check?.last_run_at)}</p>{/if}
      {#if check?.error}<p class="nc__line nc__err">Ошибка ({when(check.error_at)}): {check.error}</p>{/if}
      {#if error}<p class="nc__line nc__err">{error}</p>{/if}
    </div>
  {/if}
</div>

<style>
  .nc {
    display: grid;
    gap: 0.5rem;
  }
  .nc__head {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0;
    background: transparent;
    border: 0;
    line-height: 1;
    cursor: pointer;
    text-align: left;
    font: inherit;
    font-family: var(--font-ui, sans-serif);
    font-size: 0.68rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    font-variant: small-caps;
    font-weight: 500;
    color: var(--color-fg-subtle, #6e6e6e);
  }
  .nc__head:hover {
    color: var(--color-fg-muted, #9a9a9a);
  }
  .nc__chip {
    padding: 0.05rem 0.45rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 999px;
    font-size: 0.7rem;
    letter-spacing: 0;
    text-transform: none;
    font-variant: normal;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .nc__chip--bad {
    color: var(--color-danger, #b54a3a);
    border-color: color-mix(in srgb, var(--color-danger, #b54a3a) 55%, var(--color-border, #333));
  }
  .nc__chip--ok {
    color: var(--color-success, #8ec07c);
    border-color: color-mix(in srgb, var(--color-success, #8ec07c) 45%, var(--color-border, #333));
  }
  .nc__chip--warn {
    color: var(--color-warning, #d79921);
    border-color: color-mix(in srgb, var(--color-warning, #d79921) 50%, var(--color-border, #333));
  }
  .nc__body {
    display: grid;
    gap: 0.5rem;
  }
  .nc__cmd,
  .nc__interval,
  .nc__runon {
    box-sizing: border-box;
    padding: 0.4rem 0.55rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font-size: var(--text-sm, 0.875rem);
  }
  .nc__cmd {
    width: 100%;
    font-size: var(--text-xs, 0.75rem);
  }
  .nc__opts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
  }
  .nc__interval {
    width: 5rem;
  }
  .nc__actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .nc__line {
    margin: 0;
    font-size: 0.8rem;
    line-height: 1.4;
  }
  .nc__muted {
    color: var(--color-fg-muted, #9a9a9a);
  }
  .nc__err {
    color: var(--color-danger, #b54a3a);
    overflow-wrap: anywhere;
  }
</style>
