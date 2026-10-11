<script lang="ts">
  import Icon from '../ui/Icon.svelte'
  import { getQuestLog } from '../js/api.ts'
  import { KINDS, detailOf } from '../js/historyKinds.ts'
  import { formatLocal } from '../js/time.ts'

  /** Collapsed timeline of one entity's change-log entries (plus what hangs off it).
   * `version` is any value that changes when the entity does, to refetch while open.
   * @type {{ entityType: 'quest' | 'note' | 'attachment', entityId: number, version?: string | number | null, label?: string, labelClass?: string }} */
  let { entityType, entityId, version = null, label = 'История', labelClass = 'block__label' }: { entityType: 'quest' | 'note' | 'attachment', entityId: number, version?: string | number | null, label?: string, labelClass?: string } = $props()

  const LIMIT = 50

  let open = $state(false)
  let rows = $state(([] as any[]))
  let loaded = $state(false)
  let error = $state('')
  let seq = 0

  function params() {
    if (entityType === 'quest') return { quest_id: entityId, entity_type: 'all' }
    if (entityType === 'note') return { owner_type: 'note', owner_id: entityId, entity_type: 'all' }
    return { entity_type: 'attachment', entity_id: entityId }
  }

  $effect(() => {
    void [entityType, entityId, version]
    if (!open) {
      loaded = false
      return
    }
    const mine = ++seq
    getQuestLog({ ...params(), limit: LIMIT })
      .then((r) => {
        if (mine !== seq) return
        rows = Array.isArray(r) ? r : []
        error = ''
        loaded = true
      })
      .catch((e) => {
        if (mine === seq) error = e?.message || String(e)
      })
  })
</script>

<div class="ehist">
  <button
    type="button"
    class="{labelClass} ehist__toggle"
    onclick={() => (open = !open)}
    aria-expanded={open}
  >
    {label}
    <Icon name={open ? 'chevron-down' : 'chevron-right'} size={12} />
  </button>

  {#if open}
    {#if error}
      <p class="ehist__empty">{error}</p>
    {:else if loaded && rows.length === 0}
      <p class="ehist__empty">Событий пока нет</p>
    {:else}
      <ol class="ehist__list">
        {#each rows as r (r.id)}
          {@const info = KINDS[r.kind] ?? [r.kind.replaceAll('_', ' '), 'scroll', '']}
          <li class="ehist__item ehist__item--{info[2] || 'plain'}">
            <span class="ehist__when">{formatLocal(r.at, { year: undefined })}</span>
            <span class="ehist__icon"><Icon name={info[1]} size={13} /></span>
            <span class="ehist__what">
              {info[0]}{#if detailOf(r)} · {detailOf(r)}{/if}
              {#if entityType !== 'attachment' && r.entity_type === 'attachment'}
                <span class="ehist__sub"> · {r.title}</span>
              {/if}
            </span>
          </li>
        {/each}
      </ol>
    {/if}
  {/if}
</div>

<style>
  .ehist__toggle {
    display: flex;
    width: fit-content;
    align-items: center;
    gap: 0.3rem;
    border: 0;
    background: transparent;
    margin: 0;
    padding: 0;
    line-height: 1;
    cursor: pointer;
    font: inherit;
    font-family: var(--font-ui, sans-serif);
    font-size: 0.68rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    font-variant: small-caps;
    font-weight: 500;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .ehist__toggle:hover {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .ehist__list {
    list-style: none;
    margin: 0.5rem 0 0;
    padding: 0;
    display: grid;
    gap: 0.25rem;
  }

  .ehist__item {
    display: grid;
    grid-template-columns: 8.5rem 1.2rem minmax(0, 1fr);
    gap: 0.4rem;
    align-items: baseline;
    font-size: var(--text-xs, 0.75rem);
  }

  .ehist__when {
    font-family: var(--font-mono, monospace);
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .ehist__icon {
    display: inline-flex;
    align-self: center;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .ehist__item--ok .ehist__icon {
    color: var(--color-success, #4fa56b);
  }
  .ehist__item--bad .ehist__icon {
    color: var(--color-danger, #b54a3a);
  }
  .ehist__item--warn .ehist__icon {
    color: var(--color-warning, #c8a03c);
  }
  .ehist__item--new .ehist__icon {
    color: var(--color-accent, #c9a227);
  }

  .ehist__what {
    color: var(--color-fg-muted, #9a9a9a);
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .ehist__sub {
    font-family: var(--font-mono, monospace);
  }

  .ehist__empty {
    margin: 0.5rem 0 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-subtle, #6e6e6e);
  }

  @media (max-width: 40rem) {
    .ehist__item {
      grid-template-columns: 1.2rem minmax(0, 1fr);
    }
    .ehist__when {
      grid-column: 1 / -1;
    }
  }
</style>
