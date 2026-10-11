<script lang="ts">
  import { tick } from 'svelte'
  import Icon from './Icon.svelte'
  import { placePopover, watchPopover } from '@/shared/lib/popover.ts'

  /**
   * One filter axis: the name stays on the closed control, the choices
   * sit in a multi-select menu.
   */
  let { label, options = [], selected, onChange }: {
    label: string,
    options: { id: string, label: string, color?: string }[],
    selected: Set<string>,
    onChange: (next: Set<string>) => void,
  } = $props()

  let open = $state(false)
  let active = $state(0)
  let triggerEl = $state((null as HTMLButtonElement | null))
  let popEl = $state((null as HTMLDivElement | null))
  let listEl = $state((null as HTMLUListElement | null))
  let popPlacement = $state(('below' as 'below' | 'above'))

  let allOn = $derived(options.length > 0 && options.every((o) => selected.has(o.id)))

  let summary = $derived.by(() => {
    const picked = options.filter((o) => selected.has(o.id))
    if (picked.length === 0) return 'ничего'
    if (picked.length === options.length) return 'все'
    if (picked.length <= 2) return picked.map((o) => o.label).join(', ')
    return `${picked.length} из ${options.length}`
  })

  function place() {
    if (open && triggerEl && popEl) {
      popPlacement = placePopover(triggerEl, popEl, { matchWidth: true, gap: -1 })
    }
  }

  async function openList() {
    open = true
    active = 0
    await tick()
    place()
    listEl?.focus()
  }

  function close(refocus = true) {
    open = false
    if (refocus) triggerEl?.focus()
  }

  function toggle(id: string) {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onChange(next)
  }

  function selectAll() {
    onChange(new Set(options.map((o) => o.id)))
  }

  function onTriggerKey(event: KeyboardEvent) {
    if (!open && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
      event.preventDefault()
      openList()
    }
  }

  function onListKey(event: KeyboardEvent) {
    const last = options.length
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      active = Math.min(active + 1, last)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      active = Math.max(active - 1, 0)
    } else if (event.key === 'Home') {
      event.preventDefault()
      active = 0
    } else if (event.key === 'End') {
      event.preventDefault()
      active = last
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      if (active === 0) selectAll()
      else if (options[active - 1]) toggle(options[active - 1].id)
    } else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      close()
    } else if (event.key === 'Tab') {
      close(false)
    }
  }

  $effect(() => {
    if (!open) return
    return watchPopover(() => [popEl, triggerEl], () => close(false), place)
  })

  $effect(() => {
    if (!open || !listEl) return
    void active
    listEl.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' })
  })
</script>

<div class="fsel">
  <span class="fsel__axis">{label}</span>
  <button
    bind:this={triggerEl}
    type="button"
    class="fsel__btn"
    class:fsel__btn--open={open}
    class:fsel__btn--below={open && popPlacement === 'below'}
    class:fsel__btn--above={open && popPlacement === 'above'}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label="{label}: {summary}"
    onclick={() => (open ? close() : openList())}
    onkeydown={onTriggerKey}
  >
    <span class="fsel__value">{summary}</span>
    <span class="fsel__chev" aria-hidden="true"><Icon name="chevron-down" size={12} /></span>
  </button>
</div>

{#if open}
  <div
    bind:this={popEl}
    class="fsel__pop"
    class:fsel__pop--below={popPlacement === 'below'}
    class:fsel__pop--above={popPlacement === 'above'}
    data-own-escape
  >
    <ul
      bind:this={listEl}
      class="fsel__list"
      role="listbox"
      aria-multiselectable="true"
      aria-label={label}
      tabindex="-1"
      onkeydown={onListKey}
    >
      <li role="none">
        <button
          type="button"
          class="fsel__opt"
          class:fsel__opt--on={allOn}
          data-active={active === 0}
          role="option"
          aria-selected={allOn}
          tabindex="-1"
          onpointerdown={(e) => e.preventDefault()}
          onpointermove={() => (active = 0)}
          onclick={selectAll}
        >
          <span class="fsel__box" aria-hidden="true">
            {#if allOn}<Icon name="check" size={12} />{/if}
          </span>
          <span class="fsel__name">Все</span>
        </button>
      </li>
      {#each options as opt, i (opt.id)}
        <li role="none">
          <button
            type="button"
            class="fsel__opt"
            class:fsel__opt--on={selected.has(opt.id)}
            data-active={active === i + 1}
            role="option"
            aria-selected={selected.has(opt.id)}
            tabindex="-1"
            onpointerdown={(e) => e.preventDefault()}
            onpointermove={() => (active = i + 1)}
            onclick={() => toggle(opt.id)}
          >
            <span class="fsel__box" aria-hidden="true">
              {#if selected.has(opt.id)}<Icon name="check" size={12} />{/if}
            </span>
            {#if opt.color || opt.id === 'none'}
              <span
                class="fsel__dot"
                class:fsel__dot--none={!opt.color}
                style:--dot={opt.color || null}
                aria-hidden="true"
              ></span>
            {/if}
            <span class="fsel__name">{opt.label}</span>
          </button>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .fsel {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 0;
  }

  .fsel__axis {
    padding-left: 0.15rem;
    font-family: var(--font-ui, sans-serif);
    font-size: 0.65rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .fsel__btn {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    box-sizing: border-box;
    width: 100%;
    min-width: 0;
    min-height: 2.5rem;
    padding: 0 0.65rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }

  .fsel__btn:focus-visible {
    outline: 1px solid var(--color-border-strong, #4a4a4a);
    outline-offset: 1px;
  }

  .fsel__btn--open {
    border-color: var(--color-border-strong, #4a4a4a);
    z-index: 81;
  }

  .fsel__btn--open.fsel__btn--below {
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
  }

  .fsel__btn--open.fsel__btn--above {
    border-top-left-radius: 0;
    border-top-right-radius: 0;
  }

  .fsel__list:focus {
    outline: none;
  }

  .fsel__value {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .fsel__chev {
    display: inline-flex;
    flex: none;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .fsel__pop {
    position: fixed;
    z-index: 80;
    box-sizing: border-box;
    width: max-content;
    max-width: calc(100vw - 16px);
    max-height: min(18rem, calc(100dvh - 16px));
    padding: 0.25rem;
    overflow: auto;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 10px 28px color-mix(in srgb, #000 40%, transparent);
    outline: none;
  }

  .fsel__pop--below {
    border-top-left-radius: 0;
    border-top-right-radius: 0;
  }

  .fsel__pop--above {
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
  }

  .fsel__list {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .fsel__opt {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    min-height: 2.25rem;
    padding: 0.35rem 0.5rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }

  .fsel__opt[data-active='true'] {
    background: var(--color-bg-hover, #2a2a2a);
  }

  .fsel__box {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 1rem;
    height: 1rem;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: 3px;
    color: var(--color-accent, #c9a227);
  }

  .fsel__opt--on .fsel__box {
    border-color: var(--color-accent, #c9a227);
  }

  .fsel__dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--dot);
  }

  .fsel__dot--none {
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--color-fg-subtle, #6e6e6e);
  }

  .fsel__name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (max-width: 600px) {
    .fsel {
      flex-direction: row;
      align-items: center;
      gap: 0.65rem;
    }

    .fsel__axis {
      flex: 0 0 6.25rem;
      padding-left: 0;
    }

    .fsel__btn {
      flex: 1 1 auto;
      width: auto;
    }
  }
</style>
