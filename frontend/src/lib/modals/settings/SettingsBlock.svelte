<script>
  /** A titled group inside a settings section. `danger` frames it as the risky zone.
   * @type {{ title?: string, hint?: string, danger?: boolean, children?: import('svelte').Snippet }} */
  let { title = '', hint = '', danger = false, children } = $props()
</script>

<section class="block" class:block--danger={danger}>
  {#if title}<h4 class="block__title">{title}</h4>{/if}
  {#if hint}<p class="block__hint">{hint}</p>{/if}
  {@render children?.()}
</section>

<style>
  .block {
    display: grid;
    gap: var(--space-2, 0.5rem);
    padding: var(--space-3, 0.75rem) var(--space-4, 1rem);
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-lg, 12px);
    background: color-mix(in srgb, var(--color-bg-muted, #242424) 45%, transparent);
  }
  .block--danger {
    border-color: color-mix(in srgb, var(--color-danger, #b54a3a) 45%, var(--color-border, #333));
  }
  .block__title {
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    font-weight: 600;
    color: var(--color-fg);
  }
  .block--danger .block__title {
    color: var(--color-danger, #b54a3a);
  }
  .block__hint {
    margin: 0;
    font-size: 0.8rem;
    line-height: 1.4;
    color: var(--color-fg-muted, #9a9a9a);
  }
</style>
