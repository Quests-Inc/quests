<script lang="ts">
  import { tick } from 'svelte'
  import { filterMenuItems } from '@/shared/lib/menuSearch.ts'

  type CtxItem = {
    id: string,
    label?: string,
    danger?: boolean,
    sep?: boolean,
    checked?: boolean,
    disabled?: boolean,
    children?: CtxItem[],
    /** Подменю с полем поиска: пока поле пустое видны `children`, с запросом — совпадения из этого полного списка. */
    search?: CtxItem[],
  }


  let { open = false, x = 0, y = 0, items = [], onSelect, onClose }: { open: boolean, x: number, y: number, items: CtxItem[], onSelect: (id: string) => void, onClose: () => void } = $props()

  type Branch = { id: string, items: CtxItem[], search?: CtxItem[], left: number, right: number, top: number, query: string }

  let menuEl = $state((null as HTMLDivElement | null))
  let posX = $state(0)
  let posY = $state(0)
  let placed = $state(false)
  /** Открытые подменю по глубине: [0] — от пункта корня, [1] — от пункта в нём и т.д. */
  let branches: Branch[] = $state([])
  const shownOf = (b: Branch) => (b.search && b.query.trim() ? filterMenuItems(b.search, b.query) : b.items)
  const focusOnMount = (node: HTMLElement) => node.focus({ preventScroll: true })

  const PAD = 8

  function place() {
    const el = menuEl
    if (!el) return
    const { width, height } = el.getBoundingClientRect()
    const vw = window.innerWidth
    const vh = window.innerHeight
    let left = x
    let top = y
    if (left + width > vw - PAD) left = x - width
    if (left < PAD) left = PAD
    if (left + width > vw - PAD) left = Math.max(PAD, vw - width - PAD)
    if (top + height > vh - PAD) top = y - height
    if (top < PAD) top = PAD
    if (top + height > vh - PAD) top = Math.max(PAD, vh - height - PAD)
    posX = left
    posY = top
    placed = true
  }

  function openBranch(item: CtxItem, el: HTMLElement, depth: number) {
    if (!item.children?.length) return
    if (branches[depth]?.id === item.id) {
      branches = branches.slice(0, depth + 1)
      return
    }
    const rect = el.getBoundingClientRect()
    branches = [
      ...branches.slice(0, depth),
      { id: item.id, items: item.children, search: item.search, left: rect.left, right: rect.right, top: rect.top, query: '' },
    ]
  }

  /** Ставит панель подменю рядом с пунктом, от которого оно открыто; при нехватке места — слева, внутрь окна. */
  function placeSub(node: HTMLElement, anchor: Branch) {
    const put = (a: Branch) => {
      const { width, height } = node.getBoundingClientRect()
      const vw = window.innerWidth
      const vh = window.innerHeight
      let left = a.right - 6
      let top = a.top
      if (left + width > vw - PAD) left = a.left - width + 6
      if (left < PAD) left = PAD
      if (left + width > vw - PAD) left = Math.max(PAD, vw - width - PAD)
      if (top + height > vh - PAD) top = vh - height - PAD
      if (top < PAD) top = PAD
      node.style.left = `${left}px`
      node.style.top = `${top}px`
      node.classList.add('ctx-menu--placed')
    }
    put(anchor)
    return { update: put }
  }

  function onRowEnter(item: CtxItem, el: HTMLElement, depth: number) {
    if (item.sep || item.disabled) return
    if (item.children?.length) openBranch(item, el, depth)
    else branches = branches.slice(0, depth)
  }

  function pick(item: CtxItem) {
    if (item.sep || item.disabled || item.children?.length) return
    onSelect(item.id)
    onClose()
  }

  $effect(() => {
    if (!open) {
      placed = false
      branches = []
      return
    }
    const ax = x
    const ay = y
    void items.length
    posX = ax
    posY = ay
    placed = false
    branches = []
    let cancelled = false
    tick().then(() => {
      if (cancelled) return
      if (!menuEl) {
        requestAnimationFrame(() => {
          if (!cancelled) place()
        })
        return
      }
      place()
    })
    const onResize = () => place()
    window.addEventListener('resize', onResize)
    return () => {
      cancelled = true
      window.removeEventListener('resize', onResize)
    }
  })

  $effect(() => {
    if (!open) return
    const onKey = (event: { key: string; preventDefault: () => void }) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
      }
    }
    const onPointer = (event: { target: any }) => {
      const el = event.target
      if (el instanceof Element && el.closest('.ctx-menu')) return
      onClose()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('pointerdown', onPointer, true)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('pointerdown', onPointer, true)
    }
  })
</script>

{#snippet rows(list: CtxItem[], depth: number)}
  {#each list as item (item.id)}
    {#if item.sep}
      <div class="ctx-menu__sep" role="separator"></div>
    {:else if item.children?.length}
      <button
        type="button"
        class="ctx-menu__item ctx-menu__item--branch"
        class:ctx-menu__item--open={branches[depth]?.id === item.id}
        role="menuitem"
        aria-haspopup="menu"
        aria-expanded={branches[depth]?.id === item.id}
        onmouseenter={(e) => onRowEnter(item, e.currentTarget, depth)}
        onclick={(e) => openBranch(item, e.currentTarget, depth)}
      >
        <span class="ctx-menu__label">{item.label}</span>
        <span class="ctx-menu__chev" aria-hidden="true">›</span>
      </button>
    {:else}
      <button
        type="button"
        class="ctx-menu__item"
        class:ctx-menu__item--danger={item.danger}
        class:ctx-menu__item--disabled={item.disabled}
        role={item.checked == null ? 'menuitem' : 'menuitemradio'}
        aria-checked={item.checked == null ? undefined : item.checked}
        disabled={item.disabled}
        onmouseenter={(e) => onRowEnter(item, e.currentTarget, depth)}
        onclick={() => pick(item)}
      >
        <span class="ctx-menu__label">{item.label}</span>
        {#if item.checked}
          <span class="ctx-menu__mark" aria-hidden="true">✓</span>
        {/if}
      </button>
    {/if}
  {/each}
{/snippet}

{#if open}
  <div
    bind:this={menuEl}
    class="ctx-menu"
    class:ctx-menu--placed={placed}
    style="left: {posX}px; top: {posY}px"
    role="menu"
  >
    {@render rows(items, 0)}
  </div>
  {#each branches as b, i (b.id)}
    {@const shown = shownOf(b)}
    <div class="ctx-menu ctx-menu--sub" use:placeSub={b} role="menu">
      {#if b.search}
        <input
          class="ctx-menu__search"
          type="search"
          placeholder="Найти…"
          aria-label="Поиск по пунктам"
          bind:value={b.query}
          use:focusOnMount
          onkeydown={(e) => {
            if (e.key === 'Enter' && shown[0]) pick(shown[0])
          }}
        />
      {/if}
      {#if b.search && !shown.length}
        <div class="ctx-menu__empty">Ничего не найдено</div>
      {/if}
      {@render rows(shown, i + 1)}
    </div>
  {/each}
{/if}

<style>
  .ctx-menu__search {
    box-sizing: border-box;
    width: 100%;
    margin: 0 0 0.25rem;
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 8px);
    background: var(--color-bg, #121212);
    color: inherit;
    font: inherit;
  }

  .ctx-menu__empty {
    padding: 0.4rem 0.6rem;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.875rem);
  }

  .ctx-menu {
    position: fixed;
    z-index: 80;
    min-width: 11.5rem;
    max-width: min(22rem, calc(100vw - 16px));
    max-height: calc(100vh - 16px);
    overflow-y: auto;
    padding: 0.25rem;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: var(--radius-lg, 12px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 10px 28px color-mix(in srgb, #000 40%, transparent);
    opacity: 0;
    visibility: hidden;
    pointer-events: none;
  }

  .ctx-menu--sub {
    z-index: 81;
  }

  .ctx-menu--placed {
    opacity: 1;
    visibility: visible;
    pointer-events: auto;
  }

  .ctx-menu__item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    width: 100%;
    border: 0;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    text-align: left;
    padding: 0.4rem 0.55rem;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    cursor: pointer;
    border-radius: var(--radius-md, 4px);
  }

  .ctx-menu__label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ctx-menu__item:hover,
  .ctx-menu__item--open {
    background: var(--color-bg-hover, #2a2a2a);
  }

  .ctx-menu__item--danger {
    color: var(--color-danger, #b54a3a);
  }

  .ctx-menu__item--danger:hover {
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 14%, transparent);
  }

  .ctx-menu__item--disabled,
  .ctx-menu__item--disabled:hover {
    opacity: 0.45;
    background: transparent;
    cursor: default;
  }

  .ctx-menu__chev,
  .ctx-menu__mark {
    flex: none;
    opacity: 0.6;
  }

  .ctx-menu__sep {
    height: 1px;
    margin: 0.3rem 0.35rem;
    background: var(--color-border, #333);
  }
</style>
