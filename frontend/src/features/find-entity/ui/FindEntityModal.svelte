<script lang="ts">
  import { untrack } from 'svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import FlagHints from '@/shared/ui/FlagHints.svelte'
  import { ENTITY_SPEC, findEntities } from '@/entities/quest'

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
      onkeydown={(e) => {
        if (e.key === 'Enter' && matches.length) pick(matches[0])
      }}
    />
    <FlagHints value={query} spec={ENTITY_SPEC} parsed={parsed} onPick={(v) => (query = v)} />
    <ul class="find__list">
      {#each matches as m (m.type + m.id)}
        <li>
          <button type="button" class="find__row" onclick={() => pick(m)}>
            <span class="find__type">{LABEL[m.type]}</span>
            <span>{m.title}</span>
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

  .find__row:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .find__type {
    flex: none;
    width: 6rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .find__empty {
    padding: 0.4rem 0.5rem;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-sm, 0.875rem);
  }
</style>
