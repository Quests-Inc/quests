<script>
  import { flagSuggestions, completeFlag } from '../js/searchFlags.js'

  /** Chips under a search box while a `flag:…` token is being typed, and a warning for an unknown flag.
   * @type {{ value: string, flags: Record<string, any>, parsed: { unknown: string[] }, onPick: (next: string) => void }} */
  let { value, flags, parsed, onPick } = $props()
  let hints = $derived(flagSuggestions(value, flags))
</script>

{#if hints || parsed.unknown.length}
  <div class="fh" role="listbox" aria-label="Флаги">
    {#if hints}
      {#each hints.items as f (f.id)}
        <button type="button" class="fh__chip" onclick={() => onPick(completeFlag(value, f.id, hints.neg))}>
          {hints.neg ? '-' : ''}flag:{f.id}<span>{f.label}</span>
        </button>
      {/each}
    {:else}
      <span class="fh__warn">Неизвестный флаг: {parsed.unknown.join(', ')}</span>
    {/if}
  </div>
{/if}

<style>
  .fh {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    padding: 0.4rem 0.6rem;
  }
  .fh__chip {
    display: inline-flex;
    gap: 0.4rem;
    align-items: baseline;
    padding: 0.1rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 999px;
    background: transparent;
    color: inherit;
    font-family: var(--font-mono, monospace);
    font-size: 0.72rem;
    cursor: pointer;
  }
  .fh__chip span {
    font-family: var(--font-ui, sans-serif);
    font-size: 0.68rem;
    color: var(--color-fg-subtle, #6e6e6e);
  }
  .fh__chip:hover {
    border-color: var(--color-fg-muted, #9a9a9a);
  }
  .fh__warn {
    font-size: 0.72rem;
    color: var(--color-danger, #b54a3a);
  }
</style>
