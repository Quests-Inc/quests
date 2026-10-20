<script lang="ts">
  type Health = { api: string, overlay: string, telegram: string, webdav?: string, db?: string, detail?: Record<string, any> }

  let { health, liveStatus }: { health: Health, liveStatus: string } = $props()

  type Level = 'ok' | 'warn' | 'offline' | 'unknown'
  type Row = { key: string, label: string, level: Level, text: string }

  const level = (s: string | undefined): Level => (s === 'ok' || s === 'warn' || s === 'offline' ? s : 'unknown')
  const STATE_TEXT: Record<Level, string> = { ok: 'в порядке', warn: 'предупреждение', offline: 'недоступно', unknown: 'не настроено или неизвестно' }

  let rows = $derived.by((): Row[] => {
    const live = String(liveStatus || 'off')
    const liveLevel: Level = live === 'live' ? 'ok' : live === 'connecting' || live === 'reconnect' ? 'warn' : 'offline'
    const dbDetail = health?.detail?.components?.db?.detail
    const mk = (key: string, label: string, lv: Level, detail?: string): Row => ({ key, label, level: lv, text: detail || STATE_TEXT[lv] })
    return [
      mk('live', 'Live', liveLevel, liveLevel === 'ok' ? 'WebSocket: live' : `WebSocket: ${live}`),
      mk('api', 'API', level(health?.api)),
      mk('db', 'DB', level(health?.db), level(health?.db) === 'unknown' ? 'статус неизвестен' : dbDetail),
      mk('hud', 'HUD', level(health?.overlay)),
      mk('bot', 'Bot', level(health?.telegram)),
      mk('dav', 'DAV', level(health?.webdav)),
    ]
  })

  /** Not-configured services stay grey: they are not a fault. */
  let bad = $derived(rows.filter((r) => r.level === 'warn' || r.level === 'offline'))
  let summary = $derived(bad.length ? `Сервисы: проблемы — ${bad.map((r) => r.label).join(', ')}` : 'Сервисы: всё в порядке')

  let open = $state(false)
  let root = $state<HTMLElement | null>(null)

  function onDocDown(e: PointerEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false
  }
  function onKey(e: KeyboardEvent) {
    if (open && e.key === 'Escape') open = false
  }
</script>

<svelte:window onpointerdown={onDocDown} onkeydown={onKey} />

<div class="hm" bind:this={root}>
  <button
    type="button"
    class="hm__btn"
    class:hm__btn--bad={bad.length > 0}
    title={summary}
    aria-label={summary}
    aria-haspopup="dialog"
    aria-expanded={open}
    onclick={() => (open = !open)}
  >
    {#each rows as r (r.key)}
      <span class="hm__bar" data-level={r.level} aria-hidden="true"></span>
    {/each}
  </button>
  {#if open}
    <div class="hm__pop" role="dialog" aria-label="Состояние сервисов">
      {#each rows as r (r.key)}
        <div class="hm__row" data-level={r.level}>
          <span class="hm__bar hm__bar--row" data-level={r.level} aria-hidden="true"></span>
          <span class="hm__name">{r.label}</span>
          <span class="hm__text">{r.text}</span>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .hm {
    position: relative;
    flex-shrink: 0;
  }
  .hm__btn {
    display: inline-flex;
    align-items: flex-end;
    gap: 3px;
    box-sizing: border-box;
    height: var(--nav-control-h, 2rem);
    padding: 0.5rem 0.45rem;
    border: 0;
    background: transparent;
    cursor: pointer;
  }
  .hm__btn:focus-visible {
    outline: 1px solid var(--color-fg-muted, #9a9a9a);
    outline-offset: 2px;
  }
  .hm__bar {
    display: block;
    width: 3px;
    height: 100%;
    border-radius: 1px;
    background: var(--color-fg-subtle, #6e6e6e);
    opacity: 0.55;
  }
  /* Level is shown by height as well as colour, so it does not depend on telling green from red. */
  .hm__bar[data-level='ok'] {
    background: var(--color-success, #7a9e3a);
    opacity: 1;
  }
  .hm__bar[data-level='warn'] {
    height: 60%;
    background: var(--color-warning, #c47a20);
    opacity: 1;
  }
  .hm__bar[data-level='offline'] {
    height: 30%;
    background: var(--color-danger, #b54a3a);
    opacity: 1;
  }
  .hm__bar[data-level='unknown'] {
    height: 18%;
  }
  .hm__bar--row {
    height: 0.9rem;
    align-self: center;
  }
  .hm__bar--row[data-level='warn'] {
    height: 0.55rem;
  }
  .hm__bar--row[data-level='offline'] {
    height: 0.3rem;
  }
  .hm__bar--row[data-level='unknown'] {
    height: 0.2rem;
  }
  .hm__pop {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 20;
    min-width: 15rem;
    max-width: min(22rem, calc(100vw - 1.5rem));
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--color-border, #333);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }
  .hm__row {
    display: grid;
    grid-template-columns: 3px 3rem 1fr;
    align-items: center;
    column-gap: 0.6rem;
    padding: 0.25rem 0;
    font-size: var(--text-sm, 0.875rem);
  }
  .hm__name {
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-fg, #e8e8e8);
  }
  .hm__text {
    color: var(--color-fg-muted, #9a9a9a);
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .hm__row[data-level='warn'] .hm__text {
    color: var(--color-warning, #c47a20);
  }
  .hm__row[data-level='offline'] .hm__text {
    color: var(--color-danger, #b54a3a);
  }
</style>
