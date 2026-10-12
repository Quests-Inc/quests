<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'

  /** Заголовок группы вложений: стрелка сворачивает группу, название открывает владельца. */
  let {
    kind,
    title,
    count,
    folded,
    onToggle,
    onOpen,
  }: { kind: string; title: string; count: number; folded: boolean; onToggle: () => void; onOpen: () => void } = $props()
</script>

<div class="agh">
  <button
    type="button"
    class="agh__fold"
    aria-expanded={!folded}
    aria-label={folded ? 'Развернуть группу' : 'Свернуть группу'}
    title={folded ? 'Развернуть' : 'Свернуть'}
    onclick={onToggle}
  >
    <Icon name={folded ? 'chevron-right' : 'chevron-down'} size={12} />
  </button>
  <button type="button" class="agh__open" title="Открыть владельца" onclick={onOpen}>
    <span class="agh__kind">{kind}</span>
    <span class="agh__title">{title}</span>
    <span class="agh__n">{count}</span>
  </button>
</div>

<style>
  .agh {
    display: flex;
    align-items: baseline;
    padding-left: 0.6rem;
  }

  .agh__fold {
    flex: 0 0 auto;
    display: grid;
    place-items: center;
    align-self: center;
    width: 1.4rem;
    height: 1.4rem;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm, 3px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .agh__fold:hover {
    color: var(--color-fg, #e8e8e8);
    background: rgba(127, 127, 127, 0.2);
  }

  .agh__open {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    flex: 1 1 auto;
    min-width: 0;
    padding: 0.5rem 1rem 0.3rem 0.3rem;
    border: 0;
    background: transparent;
    text-align: left;
    cursor: pointer;
  }

  .agh__open:hover .agh__title {
    color: var(--color-accent, #c9a227);
  }

  .agh__kind {
    flex: 0 0 auto;
    font-size: 0.65rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .agh__title {
    min-width: 0;
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-body, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .agh__n {
    font-variant-numeric: tabular-nums;
    opacity: 0.7;
    font-size: var(--text-xs, 0.75rem);
  }
</style>
