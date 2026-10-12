<script lang="ts">
  /**
   * Date as YYYY-MM-DD with a Monday-first month popup — no native date input,
   * whose first weekday follows the browser locale and cannot be overridden.
   */
  let { value = $bindable(''), label = 'Дата', clearable = false }: { value?: string, label?: string, clearable?: boolean } = $props()

  const MONTHS = ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь']
  const WEEKDAYS = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс']

  const pad = (n: number) => String(n).padStart(2, '0')
  const iso = (y: number, m: number, d: number) => `${y}-${pad(m + 1)}-${pad(d)}`

  function parse(v: string) {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v || '')
    return m ? { y: +m[1], m: +m[2] - 1, d: +m[3] } : null
  }

  let open = $state(false)
  let root: HTMLElement | undefined = $state()
  let pop: HTMLElement | undefined = $state()
  let popStyle = $state('')
  let viewY = $state(0)
  let viewM = $state(0)

  const text = $derived.by(() => {
    const p = parse(value)
    return p ? `${pad(p.d)}.${pad(p.m + 1)}.${p.y}` : 'дд.мм.гггг'
  })

  const today = $derived.by(() => {
    const n = new Date()
    return iso(n.getFullYear(), n.getMonth(), n.getDate())
  })

  const cells = $derived.by(() => {
    const lead = (new Date(viewY, viewM, 1).getDay() + 6) % 7
    const days = new Date(viewY, viewM + 1, 0).getDate()
    return [...Array(lead).fill(null), ...Array.from({ length: days }, (_, i) => i + 1)]
  })

  function toggle() {
    if (open) return (open = false)
    const p = parse(value) ?? parse(today)
    viewY = p.y
    viewM = p.m
    const r = root.getBoundingClientRect()
    const below = window.innerHeight - r.bottom > 300
    popStyle = `left:${Math.max(8, Math.min(r.left, window.innerWidth - 248))}px;` +
      (below ? `top:${r.bottom + 4}px` : `bottom:${window.innerHeight - r.top + 4}px`)
    open = true
  }

  function shift(delta: number) {
    const d = new Date(viewY, viewM + delta, 1)
    viewY = d.getFullYear()
    viewM = d.getMonth()
  }

  function pick(day: any) {
    value = iso(viewY, viewM, day)
    open = false
  }

  /** Moves the popup to <body>: an ancestor with transform would re-anchor `fixed`. */
  function portal(node: HTMLElement) {
    document.body.appendChild(node)
    return { destroy: () => node.remove() }
  }

  function onDocDown(e: { target: any }) {
    if (open && root && !root.contains(e.target) && !pop?.contains(e.target)) open = false
  }
</script>

<svelte:document onpointerdown={onDocDown} />

<span class="date-input" bind:this={root}>
  <button type="button" class="date-input__btn" aria-label={label} aria-haspopup="dialog" aria-expanded={open}
    onclick={toggle} onkeydown={(e) => e.key === 'Escape' && open && (open = false, e.stopPropagation())}>
    {text}
  </button>
  {#if open}
    <div class="date-input__pop" use:portal bind:this={pop} style={popStyle} role="dialog" tabindex="-1" aria-label={label}
      onkeydown={(e) => e.key === 'Escape' && (open = false, e.stopPropagation())}>
      <div class="date-input__head">
        <button type="button" onclick={() => shift(-1)} aria-label="Предыдущий месяц">‹</button>
        <span>{MONTHS[viewM]} {viewY}</span>
        <button type="button" onclick={() => shift(1)} aria-label="Следующий месяц">›</button>
      </div>
      <div class="date-input__grid">
        {#each WEEKDAYS as w, i}<span class="date-input__wd" class:weekend={i > 4}>{w}</span>{/each}
        {#each cells as day}
          {#if day === null}
            <span></span>
          {:else}
            {@const v = iso(viewY, viewM, day)}
            <button type="button" class="date-input__day" class:sel={v === value} class:today={v === today}
              onclick={() => pick(day)}>{day}</button>
          {/if}
        {/each}
      </div>
      {#if clearable && value}
        <div class="date-input__foot">
          <button type="button" onclick={() => { value = ''; open = false }}>Сбросить</button>
        </div>
      {/if}
    </div>
  {/if}
</span>

<style>
  .date-input { display: inline-block; }

  .date-input__btn {
    box-sizing: border-box;
    height: var(--hist-h, auto);
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
  .date-input__btn:hover,
  .date-input__btn[aria-expanded='true'] { border-color: var(--color-accent, #c9a227); }

  .date-input__pop {
    position: fixed;
    z-index: 1000;
    width: 15rem;
    padding: 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg, #e8e8e8);
    box-shadow: 0 6px 24px rgba(0, 0, 0, 0.4);
  }

  .date-input__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.35rem;
    font-size: var(--text-sm, 0.875rem);
  }

  .date-input__grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 2px;
    text-align: center;
  }

  .date-input__wd { font-size: 0.7rem; opacity: 0.6; padding: 0.15rem 0; }
  .date-input__wd.weekend { color: var(--color-danger, #e06c75); opacity: 0.8; }

  .date-input__pop button {
    border: 0;
    border-radius: var(--radius-sm, 3px);
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }

  .date-input__head button { padding: 0.1rem 0.6rem; font-size: 1.1rem; }
  .date-input__day { padding: 0.3rem 0; font-size: var(--text-sm, 0.875rem); }
  .date-input__pop button:hover { background: rgba(127, 127, 127, 0.22); }
  .date-input__day.today { outline: 1px solid var(--color-accent, #d79921); }
  .date-input__day.sel { background: var(--color-accent, #d79921); color: var(--color-bg, #111); }

  .date-input__foot {
    display: flex;
    justify-content: space-between;
    margin-top: 0.4rem;
    font-size: var(--text-sm, 0.875rem);
  }
  .date-input__foot button { padding: 0.2rem 0.5rem; }
</style>
