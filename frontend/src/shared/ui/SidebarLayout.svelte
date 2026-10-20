<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { sidebars, SIDEBAR_MIN, SIDEBAR_MAX } from '@/shared/lib/sidebar.svelte.ts'

  /** A list on the left and the work on the right: one grid for every tab that has a list.
   * Owns the fold (the handle on the border), the shared width (drag the border) and the one-pane-at-a-time phone layout.
   * `sidebar` renders the list (one root element), `children` the work area (one root element). */
  let { key, class: className = '', sidebar, children }: { key: string, class?: string, sidebar: Snippet, children: Snippet } = $props()

  let root = $state<HTMLElement | null>(null)
  let dragging = $state(false)
  let collapsed = $derived(sidebars.isCollapsed(key))
  const label = $derived(collapsed ? 'Показать боковую панель' : 'Скрыть боковую панель')

  function dragTo(e: PointerEvent) {
    if (root) sidebars.setWidth(e.clientX - root.getBoundingClientRect().left, false)
  }
  function startDrag(e: PointerEvent) {
    if (e.button !== 0) return
    e.preventDefault()
    dragging = true
    const stop = () => {
      dragging = false
      sidebars.setWidth(sidebars.width)
      window.removeEventListener('pointermove', dragTo)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
    }
    window.addEventListener('pointermove', dragTo)
    window.addEventListener('pointerup', stop)
    window.addEventListener('pointercancel', stop)
  }
  function onResizeKey(e: KeyboardEvent) {
    const step = e.shiftKey ? 48 : 16
    if (e.key === 'ArrowLeft') sidebars.setWidth(sidebars.width - step)
    else if (e.key === 'ArrowRight') sidebars.setWidth(sidebars.width + step)
    else if (e.key === 'Home') sidebars.resetWidth()
    else return
    e.preventDefault()
  }
</script>

<div
  class="sbl {className}"
  class:sbl--collapsed={collapsed}
  class:sbl--dragging={dragging}
  style:--sbl-w="{sidebars.width}px"
  bind:this={root}
>
  {@render sidebar()}
  {@render children()}

  {#if !collapsed}
    <!-- A window splitter is focusable and takes arrow keys (WAI-ARIA separator pattern). -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div
      class="sbl__resize"
      role="separator"
      aria-orientation="vertical"
      aria-label="Ширина боковой панели"
      aria-valuemin={SIDEBAR_MIN}
      aria-valuemax={SIDEBAR_MAX}
      aria-valuenow={sidebars.width}
      tabindex="0"
      title="Потяните, чтобы изменить ширину; двойной клик — по умолчанию"
      onpointerdown={startDrag}
      ondblclick={() => sidebars.resetWidth()}
      onkeydown={onResizeKey}
    ></div>
  {/if}
  <button
    type="button"
    class="sbl__handle"
    onclick={() => sidebars.toggle(key)}
    title={label}
    aria-label={label}
    aria-pressed={collapsed}
  >
    <Icon name="chevron-right" size={12} />
  </button>
</div>

<style>
  .sbl {
    --sbl-col: min(var(--sbl-w), 45%);
    position: relative;
    flex: 1 1 auto;
    display: grid;
    grid-template-columns: var(--sbl-col) minmax(0, 1fr);
    min-height: 0;
    overflow: hidden;
  }
  .sbl--collapsed {
    grid-template-columns: 0 minmax(0, 1fr);
  }
  .sbl--dragging {
    user-select: none;
    cursor: col-resize;
  }
  .sbl > :global(:first-child) {
    position: relative;
    min-width: 0;
    border-right: 1px solid var(--color-border, #333);
  }
  .sbl--collapsed > :global(:first-child) {
    border-right: 0;
  }

  .sbl__resize {
    position: absolute;
    top: 0;
    bottom: 0;
    left: calc(var(--sbl-col) - 3px);
    z-index: 4;
    width: 6px;
    cursor: col-resize;
    touch-action: none;
  }
  .sbl__resize::after {
    content: '';
    position: absolute;
    top: 0;
    bottom: 0;
    left: 2px;
    width: 2px;
    background: transparent;
    transition: background 0.12s;
  }
  .sbl__resize:hover::after,
  .sbl__resize:focus-visible::after,
  .sbl--dragging .sbl__resize::after {
    background: var(--color-accent, #c9a227);
  }
  .sbl__resize:focus-visible {
    outline: none;
  }

  /* On the list's border; at the page edge once the list is folded. */
  .sbl__handle {
    position: absolute;
    top: 0.75rem;
    left: var(--sbl-col);
    z-index: 5;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1rem;
    height: 2.25rem;
    padding: 0;
    transform: translateX(-50%);
    border: 1px solid var(--color-border, #333);
    border-radius: 3px;
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
    opacity: 0.8;
    transition: opacity 0.15s;
  }
  .sbl__handle :global(.icon) {
    transform: rotate(180deg);
  }
  .sbl--collapsed .sbl__handle {
    left: 0;
    transform: none;
    border-left: 0;
    border-radius: 0 3px 3px 0;
  }
  .sbl--collapsed .sbl__handle :global(.icon) {
    transform: none;
  }
  .sbl__handle:hover,
  .sbl__handle:focus-visible {
    opacity: 1;
    color: var(--color-fg, #e8e8e8);
  }

  /* One pane at a time: the list fills the screen or is out of the way. The handle is a pull-tab in the middle of the screen edge. */
  @media (max-width: 640px) {
    .sbl,
    .sbl--collapsed {
      grid-template-columns: minmax(0, 1fr);
    }
    .sbl:not(.sbl--collapsed) > :global(:nth-child(2)),
    .sbl--collapsed > :global(:first-child) {
      display: none;
    }
    .sbl__resize {
      display: none;
    }
    .sbl__handle {
      top: 50%;
      margin-top: -1.125rem;
      left: 100%;
      transform: translateX(-100%);
      border-left: 1px solid var(--color-border, #333);
      border-radius: 3px 0 0 3px;
    }
    .sbl--collapsed .sbl__handle {
      left: 0;
      transform: none;
      border-left: 0;
      border-right: 1px solid var(--color-border, #333);
      border-radius: 0 3px 3px 0;
    }
  }
</style>
