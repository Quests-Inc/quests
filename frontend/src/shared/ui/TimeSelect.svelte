<script lang="ts">
  /**
   * 24h HH:MM — no native time input (it follows the OS 12h/24h setting).
   * A button opens a popup with large hours and minutes, an arrow above and
   * below each (also the mouse wheel and ↑/↓ keys). With `allowNone`, an empty
   * hour means "no time".
   */
  let {
    hour = $bindable('00'),
    minute = $bindable('00'),
    allowNone = false,
    disabled = false,
    label = 'Время',
  }: { hour?: string, minute?: string, allowNone?: boolean, disabled?: boolean, label?: string } = $props()

  let open = $state(false)
  let root: HTMLElement | undefined = $state()
  let pop: HTMLElement | undefined = $state()
  let popStyle = $state('')

  const noTime = $derived(allowNone && !hour)
  const text = $derived(noTime ? '--:--' : `${hour}:${minute}`)

  function toggle() {
    if (open) return (open = false)
    const r = root.getBoundingClientRect()
    const below = window.innerHeight - r.bottom > 260
    popStyle = `left:${Math.max(8, Math.min(r.left, window.innerWidth - 200))}px;` +
      (below ? `top:${r.bottom + 4}px` : `bottom:${window.innerHeight - r.top + 4}px`)
    open = true
  }

  function wrap(v: number, size: number) {
    return String(((v % size) + size) % size).padStart(2, '0')
  }

  function nudgeHour(delta: number) {
    hour = wrap(noTime ? delta : +hour + delta, 24)
    if (!minute) minute = '00'
  }

  function nudgeMinute(delta: number) {
    if (noTime) return
    minute = wrap(+minute + delta, 60)
  }

  function onWheel(e: WheelEvent, step: (d: number) => void) {
    e.preventDefault()
    step(e.deltaY < 0 ? 1 : -1)
  }

  function onColKey(e: KeyboardEvent, step: (d: number) => void) {
    if (e.key === 'ArrowUp') { e.preventDefault(); step(1) }
    else if (e.key === 'ArrowDown') { e.preventDefault(); step(-1) }
    else onKey(e)
  }

  /** Moves the popup to <body>: an ancestor with transform would re-anchor `fixed`. */
  function portal(node: HTMLElement) {
    document.body.appendChild(node)
    return { destroy: () => node.remove() }
  }

  function onDocDown(e: PointerEvent) {
    const t = (e.target as Node)
    if (open && !root?.contains(t) && !pop?.contains(t)) open = false
  }

  function onKey(e: { key: string; stopPropagation: () => void }) {
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
      <div class="time-pick__dial">
        <div class="time-pick__col" role="spinbutton" tabindex="0" aria-label="Часы" aria-valuenow={noTime ? undefined : +hour}
          aria-valuemin="0" aria-valuemax="23" onwheel={(e) => onWheel(e, nudgeHour)} onkeydown={(e) => onColKey(e, nudgeHour)}>
          <button type="button" class="time-pick__arrow" tabindex="-1" aria-label="Час вперёд" onclick={() => nudgeHour(1)}>
            <svg viewBox="0 0 16 10" aria-hidden="true"><path d="M2 8l6-6 6 6" /></svg>
          </button>
          <span class="time-pick__num">{noTime ? '--' : hour}</span>
          <button type="button" class="time-pick__arrow" tabindex="-1" aria-label="Час назад" onclick={() => nudgeHour(-1)}>
            <svg viewBox="0 0 16 10" aria-hidden="true"><path d="M2 2l6 6 6-6" /></svg>
          </button>
        </div>
        <span class="time-pick__colon" aria-hidden="true">:</span>
        <div class="time-pick__col" class:inert={noTime} role="spinbutton" tabindex="0" aria-label="Минуты"
          aria-valuenow={noTime ? undefined : +minute} aria-valuemin="0" aria-valuemax="59"
          onwheel={(e) => onWheel(e, nudgeMinute)} onkeydown={(e) => onColKey(e, nudgeMinute)}>
          <button type="button" class="time-pick__arrow" tabindex="-1" aria-label="Минута вперёд" disabled={noTime} onclick={() => nudgeMinute(1)}>
            <svg viewBox="0 0 16 10" aria-hidden="true"><path d="M2 8l6-6 6 6" /></svg>
          </button>
          <span class="time-pick__num">{noTime ? '--' : minute}</span>
          <button type="button" class="time-pick__arrow" tabindex="-1" aria-label="Минута назад" disabled={noTime} onclick={() => nudgeMinute(-1)}>
            <svg viewBox="0 0 16 10" aria-hidden="true"><path d="M2 2l6 6 6-6" /></svg>
          </button>
        </div>
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
    width: 11.5rem;
    padding: 0.6rem 0.6rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg, #e8e8e8);
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.4);
    font-size: var(--text-sm, 0.875rem);
    font-variant-numeric: tabular-nums;
  }

  .time-pick__dial {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.3rem;
  }
  .time-pick__col {
    display: flex;
    flex-direction: column;
    align-items: center;
    border-radius: var(--radius-md, 4px);
    outline-offset: 2px;
  }
  .time-pick__col.inert { opacity: 0.4; }
  .time-pick__num {
    min-width: 2.6rem;
    text-align: center;
    font-family: var(--font-display, var(--font-ui, sans-serif));
    font-size: 2.2rem;
    line-height: 1.15;
  }
  .time-pick__colon {
    font-size: 2rem;
    line-height: 1;
    padding-bottom: 0.2rem;
    opacity: 0.6;
  }

  .time-pick__pop button {
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
    border-radius: var(--radius-sm, 3px);
  }
  .time-pick__arrow {
    display: grid;
    place-items: center;
    width: 2.4rem;
    height: 1.5rem;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .time-pick__arrow svg {
    width: 0.95rem;
    height: 0.6rem;
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .time-pick__pop button:hover:not(:disabled) { background: rgba(127, 127, 127, 0.22); color: var(--color-accent, #c9a227); }
  .time-pick__pop button:disabled { cursor: default; }

  .time-pick__foot {
    display: flex;
    justify-content: space-between;
    margin-top: 0.5rem;
    padding-top: 0.45rem;
    border-top: 1px solid var(--color-border, #333);
  }
  .time-pick__foot button { padding: 0.25rem 0.6rem; }
  .time-pick__foot .done { margin-left: auto; color: var(--color-accent, #c9a227); }
</style>
