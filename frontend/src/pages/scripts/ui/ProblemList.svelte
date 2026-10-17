<script lang="ts">
  import type { Problem } from '@/shared/api/scriptgraph.gen.ts'

  /** What the validator found. A problem tied to a node selects that node. */
  let { problems, checking, onPick }: { problems: Problem[]; checking: boolean; onPick: (node: number) => void } = $props()
</script>

<div class="problems" class:problems--ok={!problems.length}>
  {#if !problems.length}
    <span>{checking ? 'Проверка…' : 'Граф корректен'}</span>
  {:else}
    <ul>
      {#each problems as p, i (i)}
        <li>
          {#if p.node}
            <button type="button" class="link" onclick={() => onPick(p.node as number)}>нода {p.node}</button>:
          {/if}
          {p.message}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .problems {
    max-height: 7rem;
    overflow: auto;
    padding: 0.4rem 0.7rem;
    border: 1px solid var(--color-danger, #b54a3a);
    border-radius: 10px;
    font-size: var(--text-xs, 0.78rem);
  }
  .problems--ok {
    border-color: var(--color-border, #333);
    color: var(--color-fg-muted, #9a9a9a);
  }
  ul {
    margin: 0;
    padding-left: 1rem;
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent, #c9a227);
    cursor: pointer;
    font: inherit;
  }
</style>
