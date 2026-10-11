<script lang="ts">
  import { tick } from 'svelte'

  /**
   * @typedef {{
   *   id: string,
   *   label?: string,
   *   danger?: boolean,
   *   sep?: boolean,
   *   checked?: boolean,
   *   disabled?: boolean,
   *   children?: CtxItem[],
   * }} CtxItem
   */
  type CtxItem = {
    id: string,
    label?: string,
    danger?: boolean,
    sep?: boolean,
    checked?: boolean,
    disabled?: boolean,
    children?: CtxItem[],
  }


  /**
   * @type {{ open: boolean, x: number, y: number, items: CtxItem[], onSelect: (id: string) => void, onClose: () => void }}
   */
  let { open = false, x = 0, y = 0, items = [], onSelect, onClose }: { open: boolean, x: number, y: number, items: CtxItem[], onSelect: (id: string) => void, onClose: () => void } = $props()

  let menuEl = $state((null as HTMLDivElement | null))
  let subEl = $state((null as HTMLDivElement | null))
  let posX = $state(0)
  let posY = $state(0)
  let placed = $state(false)
  let subX = $state(0)
  let subY = $state(0)
  let subPlaced = $state(false)
  /** @type {{ id: string, items: CtxItem[], left: number, right: number, top: number } | null} */
  let branch: { id: string, items: CtxItem[], left: number, right: number, top: number } | null = $state(null)

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

  /**
   * @param {CtxItem} item
   * @param {HTMLElement} el
   */
  function openBranch(item: CtxItem, el: HTMLElement) {
    if (!item.children?.length) return
    if (branch?.id === item.id) return
    const rect = el.getBoundingClientRect()
    subPlaced = false
    branch = {
      id: item.id,
      items: item.children,
      left: rect.left,
      right: rect.right,
      top: rect.top,
    }
  }

  function placeSub() {
    const el = subEl
    const anchor = branch
    if (!el || !anchor) return
    const { width, height } = el.getBoundingClientRect()
    const vw = window.innerWidth
    const vh = window.innerHeight
    let left = anchor.right - 6
    let top = anchor.top
    if (left + width > vw - PAD) left = anchor.left - width + 6
    if (left < PAD) left = PAD
    if (left + width > vw - PAD) left = Math.max(PAD, vw - width - PAD)
    if (top + height > vh - PAD) top = vh - height - PAD
    if (top < PAD) top = PAD
    subX = left
    subY = top
    subPlaced = true
  }

  /**
   * @param {CtxItem} item
   * @param {HTMLElement} el
   * @param {boolean} inSub
   */
  function onRowEnter(item: CtxItem, el: HTMLElement, inSub: boolean) {
    if (item.sep || item.disabled) return
    if (!inSub && item.children?.length) openBranch(item, el)
    else if (!inSub) branch = null
  }

  /** @param {CtxItem} item */
  function pick(item: CtxItem) {
    if (item.sep || item.disabled || item.children?.length) return
    onSelect(item.id)
    onClose()
  }

  $effect(() => {
    if (!open) {
      placed = false
      branch = null
      subPlaced = false
      return
    }
    const ax = x
    const ay = y
    void items.length
    posX = ax
    posY = ay
    placed = false
    branch = null
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
    if (!open || !branch) {
      subPlaced = false
      return
    }
    void branch.id
    void branch.items.length
    let cancelled = false
    tick().then(() => {
      if (cancelled) return
      if (!subEl) {
        requestAnimationFrame(() => {
          if (!cancelled) placeSub()
        })
        return
      }
      placeSub()
    })
    return () => {
      cancelled = true
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

{#snippet rows(list, inSub)}
  {#each list as item (item.id)}
    {#if item.sep}
      <div class="ctx-menu__sep" role="separator"></div>
    {:else if item.children?.length && !inSub}
      <button
        type="button"
        class="ctx-menu__item ctx-menu__item--branch"
        class:ctx-menu__item--open={branch?.id === item.id}
        role="menuitem"
        aria-haspopup="menu"
        aria-expanded={branch?.id === item.id}
        onmouseenter={(e) => onRowEnter(item, e.currentTarget, false)}
        onclick={(e) => openBranch(item, e.currentTarget)}
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
        onmouseenter={(e) => onRowEnter(item, e.currentTarget, inSub)}
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
    {@render rows(items, false)}
  </div>
  {#if branch}
    <div
      bind:this={subEl}
      class="ctx-menu ctx-menu--sub"
      class:ctx-menu--placed={subPlaced}
      style="left: {subX}px; top: {subY}px"
      role="menu"
    >
      {@render rows(branch.items, true)}
    </div>
  {/if}
{/if}

<style>
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
