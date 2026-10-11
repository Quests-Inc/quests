<script lang="ts">
  import { tick } from 'svelte'
  import Icon from './Icon.svelte'
  import { placePopover, watchPopover } from '../js/popover.ts'

  /**
   * Single choice from an open-ended list (questlines, sections) — a button
   * showing the current pick, opening a searchable list. Replaces the wall of
   * pills those lists used to be: it grew a row with every new questline.
   * `''` is the "none" choice and always comes first.
   *
   * Keyboard: ↓ opens; in the list ↑/↓ move, Enter picks, Escape closes.
   */
  let {
    options,
    value = $bindable(''),
    noneLabel = 'Нет',
    label = '',
    disabled = false,
    id = undefined,
    onChange,
  }: {
    options: { id: string, label: string, color?: string }[],
    value?: string,
    noneLabel?: string,
    label?: string,
    disabled?: boolean,
    id?: string,
    onChange?: (id: string) => void,
  } = $props()

  let open = $state(false)
  let query = $state('')
  let active = $state(0)
  let triggerEl = $state((null as HTMLButtonElement | null))
  let popEl = $state((null as HTMLDivElement | null))
  let searchEl = $state((null as HTMLInputElement | null))
  let listEl = $state((null as HTMLUListElement | null))

  let all = $derived([{ id: '', label: noneLabel, color: '' }, ...options])
  let current = $derived(all.find((o) => o.id === value) ?? all[0])
  let shown = $derived.by(() => {
    const q = query.trim().toLowerCase()
    return q ? all.filter((o) => o.id !== '' && o.label.toLowerCase().includes(q)) : all
  })

  function place() {
    if (open && triggerEl && popEl) placePopover(triggerEl, popEl, { matchWidth: true })
  }

  async function openList() {
    if (disabled) return
    query = ''
    open = true
    active = Math.max(0, all.findIndex((o) => o.id === value))
    await tick()
    place()
    searchEl?.focus()
    scrollActiveIntoView()
  }

  function close(refocus = true) {
    open = false
    if (refocus) triggerEl?.focus()
  }

  function pick(opt: { id: any; label?: string; color?: string }) {
    value = opt.id
    onChange?.(opt.id)
    close()
  }

  async function scrollActiveIntoView() {
    await tick()
    listEl?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' })
  }

  function onSearchKey(event: { key: string; preventDefault: () => void; stopPropagation: () => void }) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      active = Math.min(active + 1, shown.length - 1)
      scrollActiveIntoView()
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      active = Math.max(active - 1, 0)
      scrollActiveIntoView()
    } else if (event.key === 'Enter') {
      // Never let Enter here submit the surrounding form.
      event.preventDefault()
      if (shown[active]) pick(shown[active])
    } else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      close()
    } else if (event.key === 'Tab') {
      close(false)
    }
  }

  function onTriggerKey(event: { key: string; preventDefault: () => void }) {
    if (!open && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
      event.preventDefault()
      openList()
    }
  }

  $effect(() => {
    if (!open) return
    return watchPopover(() => [popEl, triggerEl], () => close(false), place)
  })
</script>

<button
  bind:this={triggerEl}
  type="button"
  class="picker"
  {id}
  {disabled}
  aria-haspopup="listbox"
  aria-expanded={open}
  aria-label={label ? `${label}: ${current.label}` : undefined}
  onclick={() => (open ? close() : openList())}
  onkeydown={onTriggerKey}
>
  <span
    class="picker__dot"
    class:picker__dot--none={!current.color}
    style:--dot={current.color || null}
    aria-hidden="true"
  ></span>
  <span class="picker__label">{current.label}</span>
  <span class="picker__chev" aria-hidden="true"><Icon name="chevron-down" size={12} /></span>
</button>

{#if open}
  <div bind:this={popEl} class="picker__pop" data-own-escape>
    <input
      bind:this={searchEl}
      class="picker__search"
      type="search"
      placeholder="Поиск…"
      aria-label={label ? `Поиск: ${label}` : 'Поиск'}
      bind:value={query}
      oninput={() => (active = 0)}
      onkeydown={onSearchKey}
    />
    <ul bind:this={listEl} class="picker__list" role="listbox" aria-label={label || undefined}>
      {#each shown as opt, i (opt.id)}
        <li role="none">
          <button
            type="button"
            class="picker__opt"
            class:picker__opt--on={opt.id === value}
            data-active={i === active}
            role="option"
            aria-selected={opt.id === value}
            tabindex="-1"
            onpointerdown={(e) => e.preventDefault()}
            onpointermove={() => (active = i)}
            onclick={() => pick(opt)}
          >
            <span
              class="picker__dot"
              class:picker__dot--none={!opt.color}
              style:--dot={opt.color || null}
              aria-hidden="true"
            ></span>
            <span class="picker__label">{opt.label}</span>
            {#if opt.id === value}<Icon name="check" size={12} />{/if}
          </button>
        </li>
      {:else}
        <li class="picker__empty">Ничего не найдено</li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .picker {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    box-sizing: border-box;
    width: 100%;
    min-width: 0;
    height: 2.1rem;
    padding: 0 0.55rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }

  .picker:focus-visible {
    outline: 1px solid var(--color-accent, #c9a227);
    outline-offset: 1px;
  }

  .picker:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .picker__dot {
    flex-shrink: 0;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--dot);
  }

  .picker__dot--none {
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--color-fg-subtle, #6e6e6e);
  }

  .picker__label {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .picker__chev {
    display: inline-flex;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .picker__pop {
    position: fixed;
    z-index: 200;
    display: grid;
    gap: 0.35rem;
    box-sizing: border-box;
    width: max-content;
    max-width: min(22rem, calc(100vw - 16px));
    padding: 0.35rem;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 10px 28px color-mix(in srgb, #000 45%, transparent);
  }

  .picker__search {
    box-sizing: border-box;
    width: 100%;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .picker__search:focus-visible {
    outline: 1px solid var(--color-accent, #c9a227);
  }

  .picker__list {
    max-height: 15rem;
    margin: 0;
    padding: 0;
    overflow-y: auto;
    list-style: none;
  }

  .picker__opt {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 0;
    border-radius: var(--radius-sm, 2px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }

  .picker__opt[data-active='true'] {
    background: var(--color-bg-hover, #2a2a2a);
    color: var(--color-fg, #e8e8e8);
  }

  .picker__opt--on {
    color: var(--color-accent, #c9a227);
  }

  .picker__empty {
    padding: 0.4rem 0.5rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-subtle, #6e6e6e);
  }
</style>
