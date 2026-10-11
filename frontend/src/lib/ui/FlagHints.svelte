<script lang="ts">
  import { suggestions, completeToken } from '../js/searchFlags.ts'

  /** Chips under a search box while a `key:…` token is being typed, and a warning for an unknown value.
   * @type {{ value: string, spec: Record<string, any>, parsed: { unknown: string[] }, onPick: (next: string) => void }} */
  let { value, spec, parsed, onPick }: { value: string, spec: Record<string, any>, parsed: { unknown: string[] }, onPick: (next: string) => void } = $props()
  let hints = $derived(suggestions(value, spec))
</script>

{#if hints || parsed.unknown.length}
  <div class="fh" role="listbox" aria-label="Фильтры">
    {#if hints}
      {#each hints.items as f (f.id)}
        <button type="button" class="fh__chip" onclick={() => onPick(completeToken(value, hints.key, f.id, hints.neg))}>
          {hints.neg ? '-' : ''}{hints.key}:{f.id}<span>{f.label}</span>
        </button>
      {/each}
    {:else}
      <span class="fh__warn">Не понимаю: {parsed.unknown.join(', ')}</span>
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
