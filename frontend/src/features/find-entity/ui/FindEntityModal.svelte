<script lang="ts">
  import { untrack } from 'svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import FlagHints from '@/shared/ui/FlagHints.svelte'
  import { ENTITY_SPEC, findEntities } from '@/entities/quest'
  import { TYPES } from '@/entities/graph'

  /** Search box over quests and notes; picks one existing entity. */
  let {
    open = false,
    title = 'Добавить на карту',
    types = ['quest', 'note'],
    quests = [],
    notes = [],
    questlines = [],
    onClose,
    onPick,
  }: {
    open: boolean,
    title?: string,
    types?: ('quest' | 'note' | 'questline')[],
    quests?: any[],
    notes?: any[],
    questlines?: any[],
    onClose: () => void,
    onPick: (type: string, id: number) => void,
  } = $props()

  const LABEL: Record<string, string> = { quest: 'Квест', note: 'Заметка', questline: 'Квестлайн' }
  let query = $state('')
  let input = $state((null as HTMLInputElement | null))
  let listEl = $state(null as HTMLUListElement | null)
  /** The row the arrow keys are on; Enter picks it. */
  let active = $state(0)

  $effect(() => {
    if (!open) return
    untrack(() => {
      query = ''
    })
    queueMicrotask(() => input?.focus())
  })

  let found = $derived(findEntities(query, { questlines, quests, notes, types }))
  let matches = $derived(found.items)
  let parsed = $derived(found.parsed)

  // A new search starts at its first hit.
  $effect(() => {
    void query
    active = 0
  })

  /** Keeps the active row inside the visible part of the list (the list scrolls, the dialog does not). */
  function reveal() {
    const ul = listEl
    const row = ul?.children[active] as HTMLElement | undefined
    if (!ul || !row) return
    if (row.offsetTop < ul.scrollTop) ul.scrollTop = row.offsetTop
    else if (row.offsetTop + row.offsetHeight > ul.scrollTop + ul.clientHeight) ul.scrollTop = row.offsetTop + row.offsetHeight - ul.clientHeight
  }

  function onKey(e: KeyboardEvent) {
    if (!matches.length) return
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      active = (active + (e.key === 'ArrowDown' ? 1 : -1) + matches.length) % matches.length
      queueMicrotask(reveal)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      pick(matches[Math.min(active, matches.length - 1)])
    }
  }

  function pick(m: { type: string; id: number }) {
    onPick(m.type, m.id)
    onClose()
  }
</script>

<ModalShell {open} {onClose} labelledby="find-entity-title" zIndex={46} maxWidth="30rem">
  <ModalHead id="find-entity-title" {title} icon="add" {onClose} />
  <div class="modal__body">
    <input
      class="search"
      type="search"
      placeholder="Название квеста, заметки или квестлайна, либо quest=N / note=N"
      bind:value={query}
      bind:this={input}
      onkeydown={onKey}
    />
    <FlagHints value={query} spec={ENTITY_SPEC} parsed={parsed} onPick={(v) => (query = v)} />
    <ul class="find__list" role="listbox" aria-label="Найденное" bind:this={listEl}>
      {#each matches as m, i (m.type + m.id)}
        <li role="presentation">
          <button type="button" class="find__row" class:find__row--on={i === active} role="option" aria-selected={i === active} onmouseenter={() => (active = i)} onclick={() => pick(m)}>
            <span class="find__type" style:--c={TYPES[m.type]?.color}>
              {LABEL[m.type]}
            </span>
            <span class="find__title">{m.title}</span>
          </button>
        </li>
      {:else}
        {#if query.trim() || parsed.active}
          <li class="find__empty">Ничего не найдено</li>
        {/if}
      {/each}
    </ul>
  </div>
</ModalShell>

<style>
  .find__list {
    position: relative;
    list-style: none;
    margin: 0.6rem 0 0;
    padding: 0;
    display: grid;
    gap: 0.15rem;
    max-height: 18rem;
    overflow: auto;
  }

  .find__row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    padding: 0.35rem 0.5rem;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .find__row:hover,
  .find__row--on {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  /* The kind: just the word, in the entity's own colour (the same as its card on the map), centred on the title's line. */
  .find__type {
    flex: none;
    width: 6rem;
    color: var(--c, var(--color-fg-muted, #9a9a9a));
    font-size: var(--text-xs, 0.75rem);
  }

  .find__title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .find__empty {
    padding: 0.4rem 0.5rem;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.875rem);
  }
</style>
