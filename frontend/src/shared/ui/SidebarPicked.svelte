<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'

  /** «Выбрано: N» — floats over the foot of the list, so selecting never moves the rows under the pointer.
   * The list keeps a spare strip at its end (see `--sbl-foot`) so the last row can scroll clear of it. */
  let { count, onClear, compact = false }: { count: number, onClear: () => void, compact?: boolean } = $props()
</script>

{#if count > 0}
  <div class="sbp" class:sbp--compact={compact} role="status">
    <span>Выбрано: {count}</span>
    <button type="button" class="sbp__clear" title="Снять выделение (Esc)" aria-label="Снять выделение" onclick={onClear}>
      <Icon name="close" size={14} />
    </button>
  </div>
{/if}

<style>
  .sbp {
    position: absolute;
    left: 0.5rem;
    right: 0.5rem;
    bottom: 0.5rem;
    z-index: 3;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.4rem 2.25rem;
    border-radius: 999px;
    background: color-mix(in srgb, var(--color-accent, #c9a227) 28%, var(--color-bg-raised, #1a1a1a));
    color: var(--color-fg, #e8e8e8);
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.4);
    font-size: var(--text-sm, 0.875rem);
    font-weight: 600;
  }
  /* A wide page: a centred pill instead of a strip across the whole width. */
  .sbp--compact {
    left: 50%;
    right: auto;
    width: min(18rem, calc(100% - 1rem));
    transform: translateX(-50%);
  }
  .sbp__clear {
    position: absolute;
    right: 0.45rem;
    top: 50%;
    transform: translateY(-50%);
    display: inline-flex;
    padding: 0.2rem;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: inherit;
    cursor: pointer;
    opacity: 0.7;
  }
  .sbp__clear:hover,
  .sbp__clear:focus-visible {
    opacity: 1;
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 12%, transparent);
  }
</style>
