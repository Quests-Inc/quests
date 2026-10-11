<script lang="ts">
  /**
   * 24h HH:MM — no native time input (it follows the OS 12h/24h setting).
   * A button opens a popup: hours as a 6×4 grid, minutes in steps of 5 with
   * ±1 for the exact minute. With `allowNone`, "—" means "no time" and the
   * minutes go inert.
   * @type {{ hour?: string, minute?: string, allowNone?: boolean, disabled?: boolean, label?: string }}
   */
  let {
    hour = $bindable('00'),
    minute = $bindable('00'),
    allowNone = false,
    disabled = false,
    label = 'Время',
  }: { hour?: string, minute?: string, allowNone?: boolean, disabled?: boolean, label?: string } = $props()

  const HOURS = Array.from({ length: 24 }, (_, i) => String(i).padStart(2, '0'))
  const MINUTES = Array.from({ length: 12 }, (_, i) => String(i * 5).padStart(2, '0'))

  let open = $state(false)
  let root = $state()
  let pop = $state()
  let popStyle = $state('')

  const noTime = $derived(allowNone && !hour)
  const text = $derived(noTime ? '--:--' : `${hour}:${minute}`)

  function toggle() {
    if (open) return (open = false)
    const r = root.getBoundingClientRect()
    const below = window.innerHeight - r.bottom > 320
    popStyle = `left:${Math.max(8, Math.min(r.left, window.innerWidth - 248))}px;` +
      (below ? `top:${r.bottom + 4}px` : `bottom:${window.innerHeight - r.top + 4}px`)
    open = true
  }

  /** @param {number} delta */
  function nudge(delta: number) {
    const total = (((+minute + delta) % 60) + 60) % 60
    minute = String(total).padStart(2, '0')
  }

  /** Moves the popup to <body>: an ancestor with transform would re-anchor `fixed`. */
  function portal(/** @type {HTMLElement} */ node: { remove: () => any }) {
    document.body.appendChild(node)
    return { destroy: () => node.remove() }
  }

  function onDocDown(/** @type {PointerEvent} */ e: { target: Node }) {
    const t = (e.target as Node)
    if (open && !root?.contains(t) && !pop?.contains(t)) open = false
  }

  function onKey(/** @type {KeyboardEvent} */ e: { key: string; stopPropagation: () => void }) {
    if (e.key === 'Escape' && open) {
      open = false
      e.stopPropagation()
    }
  }
</script>

<svelte:document onpointerdown={onDocDown} />

<span class="time-pick" bind:this={root}>
  <button type="button" class="time-pick__btn" {disabled} aria-label={label} aria-haspopup="dialog"
    aria-expanded={open} onclick={toggle} onkeydown={onKey}>{text}</button>
  {#if open}
    <div class="time-pick__pop" use:portal bind:this={pop} style={popStyle} role="dialog" tabindex="-1"
      aria-label={label} onkeydown={onKey}>
      <div class="time-pick__title">Часы</div>
      <div class="time-pick__grid">
        {#each HOURS as h}
          <button type="button" class:sel={h === hour} onclick={() => (hour = h)}>{h}</button>
        {/each}
      </div>
      <div class="time-pick__title">Минуты</div>
      <div class="time-pick__grid" class:inert={noTime}>
        {#each MINUTES as m}
          <button type="button" class:sel={m === minute} disabled={noTime} onclick={() => (minute = m)}>{m}</button>
        {/each}
      </div>
      <div class="time-pick__fine">
        <button type="button" disabled={noTime} onclick={() => nudge(-1)} aria-label="Минута назад">−1</button>
        <span>{noTime ? '--' : minute}</span>
        <button type="button" disabled={noTime} onclick={() => nudge(1)} aria-label="Минута вперёд">+1</button>
      </div>
      <div class="time-pick__foot">
        {#if allowNone}
          <button type="button" onclick={() => { hour = ''; open = false }}>Без времени</button>
        {/if}
        <button type="button" class="done" onclick={() => (open = false)}>Готово</button>
      </div>
    </div>
  {/if}
</span>

<style>
  .time-pick { display: inline-block; }

  .time-pick__btn {
    box-sizing: border-box;
    padding: 0.45rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
    font-variant-numeric: tabular-nums;
    cursor: pointer;
  }
  .time-pick__btn:disabled { opacity: 0.5; cursor: default; }
  .time-pick__btn:hover:not(:disabled),
  .time-pick__btn[aria-expanded='true'] { border-color: var(--color-accent, #c9a227); }

  .time-pick__pop {
    position: fixed;
    z-index: 1000;
    width: 15rem;
    padding: 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg, #e8e8e8);
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.4);
    font-size: var(--text-sm, 0.875rem);
    font-variant-numeric: tabular-nums;
  }

  .time-pick__title {
    margin: 0.2rem 0 0.25rem;
    font-size: 0.7rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    opacity: 0.6;
  }

  .time-pick__grid {
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    gap: 2px;
  }
  .time-pick__grid.inert { opacity: 0.4; }

  .time-pick__pop button {
    padding: 0.35rem 0;
    border: 0;
    border-radius: var(--radius-sm, 3px);
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }
  .time-pick__pop button:hover:not(:disabled) { background: rgba(127, 127, 127, 0.22); }
  .time-pick__pop button:disabled { cursor: default; }
  .time-pick__pop button.sel { background: var(--color-accent, #c9a227); color: var(--color-bg, #111); }

  .time-pick__fine {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.6rem;
    margin-top: 0.3rem;
  }
  .time-pick__fine button { padding: 0.25rem 0.7rem; border: 1px solid var(--color-border, #333); }

  .time-pick__foot {
    display: flex;
    justify-content: space-between;
    margin-top: 0.4rem;
  }
  .time-pick__foot button { padding: 0.25rem 0.6rem; }
  .time-pick__foot .done { margin-left: auto; }
</style>
