<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import type { Problem } from '@/shared/api/scriptgraph.gen.ts'

  /** What the validator found, above the field: red, with the number of the node and what is wrong. A problem tied to a node selects that node. */
  let {
    problems,
    label,
    onPick,
  }: { problems: Problem[]; label: (node: number) => string; onPick: (node: number) => void } = $props()
</script>

{#if problems.length}
  <div class="problems" role="alert">
    <span class="problems__icon"><Icon name="alert" size={18} /></span>
    <ul>
      {#each problems as p, i (i)}
        <li>
          {#if p.node}
            <button type="button" class="problems__node" onclick={() => onPick(p.node as number)}>{label(p.node)}</button>
          {/if}
          {p.message}
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .problems {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    max-height: 7rem;
    overflow: auto;
    padding: 0.55rem 0.85rem;
    border: 1px solid color-mix(in srgb, var(--color-danger, #b54a3a) 55%, var(--color-border, #333));
    border-radius: var(--pill-radius, 12px);
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 14%, var(--pill-bg, #0f0f0f));
    color: var(--color-danger, #b54a3a);
    font-size: var(--text-sm, 0.875rem);
  }
  .problems__icon {
    flex: none;
    display: grid;
    place-items: center;
    margin-top: 0.05rem;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.2rem;
    min-width: 0;
  }
  li {
    overflow-wrap: anywhere;
  }
  .problems__node {
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    font-weight: 600;
    text-decoration: underline;
    text-underline-offset: 0.2em;
    cursor: pointer;
  }
</style>
