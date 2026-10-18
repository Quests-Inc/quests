<script lang="ts">
  import { KIND_COLORS, KIND_ORDER, KIND_TITLES } from '@/entities/script'
  import type { ScriptEditor } from '../model/editor.svelte.ts'
  import type { Canvas } from '../model/canvas.svelte.ts'

  /** The node types, grouped by what they do. A click puts a node in the middle of the field. */
  let { ed, cv, onNewCode }: { ed: ScriptEditor; cv: Canvas; onNewCode?: () => void } = $props()

  const groups = $derived(
    KIND_ORDER.map((kind) => ({ kind, items: ed.infos.filter((i) => i.kind === kind) })).filter((g) => g.items.length)
  )
</script>

<aside class="pal" aria-label="Ноды">
  {#if onNewCode}
    <button type="button" class="pal__new" onclick={onNewCode}>+ Нода из кода</button>
  {/if}
  {#each groups as g (g.kind)}
    <section class="pal__group" style:--c={KIND_COLORS[g.kind]}>
    <h3>{KIND_TITLES[g.kind] ?? g.kind}</h3>
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
    </section>
  {/each}
</aside>

<style>
  .pal {
    flex: 0 0 11.5rem;
    overflow: auto;
    display: grid;
    align-content: start;
    gap: 0.6rem;
    padding: 0 0.3rem 0.3rem 0;
  }
  /* A group shares one background in its colour; the cards on it are lighter and stand out. */
  .pal__group {
    padding: 0.5rem;
    border-radius: var(--pill-radius, 12px);
    background: color-mix(in srgb, var(--c) 11%, var(--pill-bg, #0f0f0f));
  }
  h3 {
    margin: 0 0 0.4rem;
    padding-left: 0.1rem;
    font-size: var(--text-xs, 0.75rem);
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: color-mix(in srgb, var(--c) 60%, var(--color-fg-muted, #9a9a9a));
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.35rem;
  }
  .pal__item {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
    padding: 0.4rem 0.6rem;
    border: 0;
    border-radius: 6px;
    background: color-mix(in srgb, var(--c) 20%, var(--color-bg-raised, #1a1a1a));
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.85rem);
    text-align: left;
    cursor: pointer;
  }
  .pal__item:hover {
    background: color-mix(in srgb, var(--c) 28%, var(--color-bg-raised, #1a1a1a));
  }
  .pal__item:active {
    box-shadow: none;
  }
  .pal__new {
    padding: 0.4rem 0.6rem;
    border: 1px dashed var(--color-border-strong, #4a4a4a);
    border-radius: 6px;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.85rem);
    cursor: pointer;
  }
  .pal__new:hover {
    background: var(--color-bg-hover, #2a2a2a);
  }
  small {
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 65%, transparent);
    font-size: 0.65rem;
  }
</style>
