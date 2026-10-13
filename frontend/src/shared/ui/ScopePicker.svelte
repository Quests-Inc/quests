<script lang="ts">
  import { tick } from 'svelte'
  import Icon from './Icon.svelte'
  import { watchPopover } from '@/shared/lib/popover.ts'

  type Cat = number | 'none' | null
  type Line = { id: number; title: string }
  type RowKey = number | 'none' | 'all'
  const toCat = (k: RowKey): Cat => (k === 'all' ? null : k)

  /**
   * A two-level filter: categories first (plus «all» and «no category»), and a side menu of the hovered
   * category's questlines. Choosing a category alone stays possible; a questline is optional. The side menu
   * scrolls when it is long and gets a search box when it has many entries.
   */
  let {
    categories = [],
    category = null,
    questline = null,
    linesFor,
    onPick,
  }: {
    categories?: { id: number; label: string; color?: string }[]
    category?: Cat
    questline?: number | null
    linesFor: (cat: number | 'none') => Line[]
    onPick: (category: Cat, questline: number | null) => void
  } = $props()

  const SEARCH_FROM = 8
  const EDGE = 8

  let open = $state(false)
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popEl = $state<HTMLDivElement | null>(null)
  let subEl = $state<HTMLDivElement | null>(null)
  let searchEl = $state<HTMLInputElement | null>(null)
  let popStyle = $state('')
  let subStyle = $state('')
  let sub = $state<{ cat: number | 'none'; top: number } | null>(null)
  let query = $state('')
  let idx = $state(0)
  let subIdx = $state(0)
  let level = $state<'main' | 'sub'>('main')

  const rows = $derived([
    { key: 'all' as RowKey, label: 'Все разделы', color: undefined as string | undefined },
    ...categories.map((c) => ({ key: c.id as RowKey, label: c.label, color: c.color })),
    { key: 'none' as RowKey, label: 'Без раздела', color: undefined as string | undefined },
  ])

  const catLabel = (c: Cat) => (c == null ? 'Все разделы' : c === 'none' ? 'Без раздела' : (categories.find((x) => x.id === c)?.label ?? `раздел ${c}`))

  const triggerText = $derived.by(() => {
    if (category == null) return 'Все разделы'
    const base = catLabel(category)
    if (questline == null) return base
    const line = linesFor(category).find((l) => l.id === questline)
    return `${base} › ${line?.title ?? `квестлайн ${questline}`}`
  })

  const subLines = $derived.by(() => {
    if (!sub) return []
    const all = linesFor(sub.cat)
    const q = query.trim().toLowerCase()
    return q ? all.filter((l) => l.title.toLowerCase().includes(q)) : all
  })
  const subTotal = $derived(sub ? linesFor(sub.cat).length : 0)

  function place() {
    if (!triggerEl || !popEl) return
    const r = triggerEl.getBoundingClientRect()
    const vw = window.innerWidth
    const vh = window.innerHeight
    popEl.style.minWidth = `${Math.round(r.width)}px`
    const w = popEl.offsetWidth
    const h = popEl.offsetHeight
    const left = Math.max(EDGE, Math.min(r.left, vw - w - EDGE))
    let top = r.bottom + 2
    if (top + h > vh - EDGE) top = Math.max(EDGE, r.top - 2 - h)
    popStyle = `left:${Math.round(left)}px;top:${Math.round(top)}px`
  }

  async function placeSub(rowTop: number) {
    await tick()
    if (!subEl || !popEl) return
    const p = popEl.getBoundingClientRect()
    const vw = window.innerWidth
    const vh = window.innerHeight
    const w = subEl.offsetWidth
    const h = subEl.offsetHeight
    let left = p.right + 2
    if (left + w > vw - EDGE) left = Math.max(EDGE, p.left - w - 2)
    const top = Math.max(EDGE, Math.min(rowTop - 4, vh - EDGE - h))
    subStyle = `left:${Math.round(left)}px;top:${Math.round(top)}px`
  }

  async function openMenu() {
    open = true
    level = 'main'
    sub = null
    idx = Math.max(0, rows.findIndex((r) => r.key === (category ?? 'all')))
    await tick()
    place()
    popEl?.focus()
  }

  function close(refocus = true) {
    open = false
    sub = null
    query = ''
    if (refocus) triggerEl?.focus()
  }

  function pick(cat: Cat, line: number | null = null) {
    onPick(cat, line)
    close()
  }

  function hasLines(key: RowKey) {
    return key !== 'all' && linesFor(key).length > 0
  }

  async function showSub(i: number, rowEl: HTMLElement, focus = false) {
    const key = rows[i]?.key
    idx = i
    if (key === 'all' || !hasLines(key)) {
      sub = null
      level = 'main'
      return
    }
    if (sub?.cat !== key) {
      query = ''
      subIdx = 0
    }
    sub = { cat: key, top: rowEl.getBoundingClientRect().top }
    await placeSub(sub.top)
    if (focus) {
      level = 'sub'
      if (subTotal >= SEARCH_FROM) searchEl?.focus()
      else subEl?.focus()
    }
  }

  function onMainKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      close()
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      idx = (idx + (e.key === 'ArrowDown' ? 1 : -1) + rows.length) % rows.length
      const el = popEl?.querySelectorAll<HTMLElement>('[data-row]')[idx]
      if (el) void showSub(idx, el)
    } else if (e.key === 'ArrowRight') {
      const el = popEl?.querySelectorAll<HTMLElement>('[data-row]')[idx]
      if (el && hasLines(rows[idx].key)) {
        e.preventDefault()
        void showSub(idx, el, true)
      }
    } else if (e.key === 'Enter') {
      e.preventDefault()
      pick(toCat(rows[idx]?.key ?? 'all'))
    }
  }

  function onSubKey(e: KeyboardEvent) {
    // index 0 of the side menu is «the whole category»
    const last = subLines.length
    if (e.key === 'Escape' || e.key === 'ArrowLeft') {
      if (e.key === 'ArrowLeft' && e.target === searchEl && searchEl && searchEl.selectionStart) return
      e.preventDefault()
      e.stopPropagation()
      level = 'main'
      popEl?.focus()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      subIdx = Math.min(subIdx + 1, last)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      subIdx = Math.max(subIdx - 1, 0)
    } else if (e.key === 'Enter' && sub) {
      e.preventDefault()
      pick(sub.cat, subIdx === 0 ? null : (subLines[subIdx - 1]?.id ?? null))
    }
  }

  $effect(() => {
    if (!open) return
    return watchPopover(
      () => [popEl, subEl, triggerEl],
      () => close(false),
      () => {
        place()
        if (sub) void placeSub(sub.top)
      },
    )
  })

  /** Moves the menus to <body>: an ancestor with a transform or overflow would clip or re-anchor `fixed`. */
  function portal(node: HTMLElement) {
    document.body.appendChild(node)
    return { destroy: () => node.remove() }
  }

  function onTriggerKey(e: KeyboardEvent) {
    if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault()
      void openMenu()
    }
  }
</script>

<button
  type="button"
  bind:this={triggerEl}
  class="scope__btn"
  aria-haspopup="menu"
  aria-expanded={open}
  aria-label="Сузить до раздела или квестлайна"
  title={triggerText}
  onclick={() => (open ? close() : openMenu())}
  onkeydown={onTriggerKey}
>
  <span class="scope__text">{triggerText}</span>
</button>

{#if open}
  <div
    use:portal
    bind:this={popEl}
    class="scope__pop"
    style={popStyle}
    role="menu"
    tabindex="-1"
    onkeydown={onMainKey}
  >
    {#each rows as r, i (r.key)}
      {#if r.key === 'none'}<div class="scope__sep" role="separator"></div>{/if}
      <button
        type="button"
        role="menuitem"
        data-row
        class="scope__row"
        class:scope__row--active={level === 'main' && idx === i}
        class:scope__row--open={sub != null && sub.cat === r.key}
        tabindex="-1"
        onpointerenter={(e) => showSub(i, e.currentTarget)}
        onclick={() => pick(toCat(r.key))}
      >
        {#if r.color}<span class="scope__dot" style:background={r.color}></span>{/if}
        <span class="scope__label">{r.label}</span>
        {#if (category ?? 'all') === r.key && questline == null}<span class="scope__mark" aria-hidden="true">✓</span>{/if}
        {#if hasLines(r.key)}<span class="scope__chev" aria-hidden="true"><Icon name="chevron-right" size={12} /></span>{/if}
      </button>
    {/each}
  </div>
  {#if sub}
    <div
      use:portal
      bind:this={subEl}
      class="scope__pop scope__sub"
      style={subStyle}
      role="menu"
      tabindex="-1"
      onkeydown={onSubKey}
    >
      <button
        type="button"
        role="menuitem"
        class="scope__row"
        class:scope__row--active={level === 'sub' && subIdx === 0}
        tabindex="-1"
        onclick={() => pick(sub!.cat)}
      >
        <span class="scope__label">Весь раздел</span>
        {#if category === sub.cat && questline == null}<span class="scope__mark" aria-hidden="true">✓</span>{/if}
      </button>
      {#if subTotal >= SEARCH_FROM}
        <input
          bind:this={searchEl}
          class="scope__search"
          type="search"
          placeholder="Найти квестлайн…"
          aria-label="Найти квестлайн"
          bind:value={query}
          oninput={() => (subIdx = 0)}
        />
      {/if}
      <div class="scope__list">
        {#each subLines as l, j (l.id)}
          <button
            type="button"
            role="menuitem"
            class="scope__row"
            class:scope__row--active={level === 'sub' && subIdx === j + 1}
            tabindex="-1"
            title={l.title}
            onclick={() => pick(sub!.cat, l.id)}
          >
            <span class="scope__label">{l.title}</span>
            {#if category === sub.cat && questline === l.id}<span class="scope__mark" aria-hidden="true">✓</span>{/if}
          </button>
        {:else}
          <p class="scope__empty">Ничего не найдено</p>
        {/each}
      </div>
    </div>
  {/if}
{/if}

<style>
  .scope__btn {
    display: flex;
    align-items: center;
    justify-content: center;
    box-sizing: border-box;
    width: 100%;
    min-width: 0;
    height: 2.25rem;
    padding: 0 var(--space-3, 0.75rem);
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: var(--radius-lg, 12px);
    background: var(--color-bg-muted, #242424);
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
    line-height: 1;
    text-align: center;
    cursor: pointer;
  }
  .scope__btn:hover,
  .scope__btn[aria-expanded='true'] {
    border-color: var(--color-accent, #c9a227);
  }
  .scope__text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .scope__pop {
    position: fixed;
    z-index: 90;
    display: flex;
    flex-direction: column;
    min-width: 11rem;
    max-width: min(22rem, calc(100vw - 16px));
    max-height: calc(100vh - 16px);
    padding: 0.25rem;
    overflow-y: auto;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: var(--radius-lg, 12px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 10px 28px color-mix(in srgb, #000 40%, transparent);
    outline: none;
  }
  .scope__sub {
    z-index: 91;
    overflow: hidden;
    width: 15rem;
  }
  .scope__list {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
  }
  .scope__sep {
    height: 1px;
    margin: 0.3rem 0.35rem;
    background: var(--color-border, #333);
  }
  .scope__row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.4rem 0.55rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }
  .scope__row:hover,
  .scope__row--open,
  .scope__row--active {
    background: var(--color-bg-hover, #2a2a2a);
  }
  .scope__label {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .scope__dot {
    flex: none;
    width: 0.55rem;
    height: 0.55rem;
    border-radius: 50%;
  }
  .scope__mark,
  .scope__chev {
    flex: none;
    opacity: 0.6;
  }
  .scope__search {
    margin: 0.2rem 0.2rem 0.3rem;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
  }
  .scope__search:focus {
    outline: 1px solid var(--color-accent, #c9a227);
    outline-offset: 0;
  }
  .scope__empty {
    margin: 0.5rem;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.875rem);
  }
</style>
