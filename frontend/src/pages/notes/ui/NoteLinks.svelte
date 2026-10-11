<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'

  type Ref = { kind: string; id: number; title?: string }

  /** Блоки «Ссылки» и «Ссылаются» под заметкой: исходящие и входящие упоминания, сворачиваются. */
  let {
    refs = [],
    backlinks = [],
    refsOpen = $bindable(true),
    backlinksOpen = $bindable(true),
    onRef,
  }: {
    refs?: Ref[]
    backlinks?: Ref[]
    refsOpen?: boolean
    backlinksOpen?: boolean
    onRef?: (kind: string, id: number) => void
  } = $props()
</script>

    {#if refs.length}
      <div class="block notes__links notes__links--first">
        <button
          type="button"
          class="block__label block__label--toggle"
          onclick={() => (refsOpen = !refsOpen)}
          aria-expanded={refsOpen}
        >
          Ссылки ({refs.length})
          <Icon name={refsOpen ? 'chevron-down' : 'chevron-right'} size={12} />
        </button>
        {#if refsOpen}
          <ul>
            {#each refs as ref (`${ref.kind}-${ref.id}`)}
              <li>
                <button type="button" class="notes__link" onclick={() => onRef?.(ref.kind, ref.id)}>
                  {ref.title || `${ref.kind}=${ref.id}`}
                  <span class="notes__kind">{ref.kind}={ref.id}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
    {#if backlinks.length}
      <div class="block notes__links" class:notes__links--first={!refs.length}>
        <button
          type="button"
          class="block__label block__label--toggle"
          onclick={() => (backlinksOpen = !backlinksOpen)}
          aria-expanded={backlinksOpen}
        >
          Ссылаются ({backlinks.length})
          <Icon name={backlinksOpen ? 'chevron-down' : 'chevron-right'} size={12} />
        </button>
        {#if backlinksOpen}
          <ul>
            {#each backlinks as ref (`b-${ref.kind}-${ref.id}`)}
              <li>
                <button type="button" class="notes__link" onclick={() => onRef?.(ref.kind, ref.id)}>
                  {ref.title || `${ref.kind}=${ref.id}`}
                  <span class="notes__kind">{ref.kind}={ref.id}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}

<style>


  .notes__links {
    margin: 0;
    padding: 0;
  }

  .notes__links--first {
    /* Sit the divider a flex-gap below the body (no extra margin), then pad
       the label the same flex-gap below the divider. Both the body above and
       the links group below then clear each divider by the same amount. */
    margin-top: 0;
    padding-top: var(--space-4, 1rem);
    border-top: 1px solid var(--color-border, #333);
  }

  .notes__links ul {
    list-style: none;
    margin: var(--space-2, 0.5rem) 0 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .notes__link {
    border: 0;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border, #333) 80%, transparent);
    background: transparent;
    color: inherit;
    padding: 0.15rem 0;
    cursor: pointer;
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .notes__link:hover {
    border-bottom-color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__kind {
    margin-left: 0.35rem;
    color: var(--color-fg-subtle, #6e6e6e);
    font-family: var(--font-mono, monospace);
    font-size: 0.7rem;
  }</style>
