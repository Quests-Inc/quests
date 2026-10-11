<script lang="ts">
  import Icon from './Icon.svelte'

  /**
   * Collapsible group of form fields. Collapsed it still says what's set
   * (`summary`, e.g. "активен · обычное"), so a form can stay short without
   * hiding its state. `aside` renders at the right of the header (a "?" or a
   * small action), outside the toggle button.
   */
  let { title, summary = '', open = $bindable(false), aside, children }: {
    title: string,
    summary?: string,
    open?: boolean,
    aside?: import('svelte').Snippet,
    children?: import('svelte').Snippet,
  } = $props()
</script>

<section class="fsec" class:fsec--open={open}>
  <div class="fsec__head">
    <button type="button" class="fsec__toggle" aria-expanded={open} onclick={() => (open = !open)}>
      <span class="fsec__chev" aria-hidden="true"><Icon name="chevron-right" size={12} /></span>
      <span class="fsec__title">{title}</span>
      {#if summary && !open}
        <span class="fsec__summary">{summary}</span>
      {/if}
    </button>
    {#if aside}
      <div class="fsec__aside">{@render aside()}</div>
    {/if}
  </div>
  {#if open}
    <div class="fsec__body">
      {@render children?.()}
    </div>
  {/if}
</section>

<style>
  .fsec {
    border-top: 1px solid var(--color-border, #333);
  }

  .fsec__head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 2.25rem;
  }

  .fsec__toggle {
    display: flex;
    flex: 1 1 auto;
    align-items: baseline;
    gap: 0.5rem;
    min-width: 0;
    padding: 0.5rem 0;
    border: 0;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-ui, sans-serif);
    text-align: left;
    cursor: pointer;
  }

  .fsec__toggle:focus-visible {
    outline: 1px solid var(--color-accent, #c9a227);
    outline-offset: 2px;
  }

  .fsec__chev {
    display: inline-flex;
    align-self: center;
    color: var(--color-fg-subtle, #6e6e6e);
    transition: transform 0.15s ease;
  }

  .fsec--open .fsec__chev {
    transform: rotate(90deg);
  }

  .fsec__title {
    flex-shrink: 0;
    font-size: var(--text-xs, 0.75rem);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .fsec__summary {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .fsec__toggle:hover .fsec__summary {
    color: var(--color-fg, #e8e8e8);
  }

  .fsec__aside {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    flex-shrink: 0;
  }

  .fsec__body {
    display: grid;
    gap: var(--space-3, 0.75rem);
    padding: 0.15rem 0 var(--space-3, 0.75rem);
  }
</style>
