<script lang="ts">
  import { KIND_COLORS, KIND_ORDER, KIND_TITLES } from '@/entities/script'
  import type { ScriptEditor } from '../model/editor.svelte.ts'
  import type { Canvas } from '../model/canvas.svelte.ts'

  /** The node types, grouped by what they do. A click puts a node in the middle of the field. */
  let { ed, cv }: { ed: ScriptEditor; cv: Canvas } = $props()

  const groups = $derived(
    KIND_ORDER.map((kind) => ({ kind, items: ed.infos.filter((i) => i.kind === kind) })).filter((g) => g.items.length)
  )
</script>

<aside class="pal" aria-label="Ноды">
  {#each groups as g (g.kind)}
    <h3 style:--c={KIND_COLORS[g.kind]}>{KIND_TITLES[g.kind] ?? g.kind}</h3>
    <ul>
      {#each g.items as i (i.name)}
        <li>
          <button type="button" class="pal__item" style:--c={KIND_COLORS[g.kind]} title={i.name} onclick={() => cv.addAtCenter(i.name)}>
            {i.title}
            {#if !i.trusted}<small>раннер</small>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/each}
</aside>

<style>
  .pal {
    flex: 0 0 12.5rem;
    overflow: auto;
    padding-right: 0.3rem;
  }
  h3 {
    margin: 0.7rem 0 0.25rem;
    padding-left: 0.4rem;
    border-left: 3px solid var(--c);
    font-size: var(--text-xs, 0.75rem);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-fg-muted, #9a9a9a);
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.2rem;
  }
  .pal__item {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-left: 3px solid var(--c);
    border-radius: 8px;
    background: var(--pill-bg, #151515);
    color: inherit;
    font: inherit;
    font-size: var(--text-xs, 0.78rem);
    text-align: left;
    cursor: pointer;
  }
  .pal__item:hover {
    border-color: var(--c);
  }
  small {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.65rem;
  }
</style>
