<script lang="ts">
  import type { GraphZone } from '@/entities/graph'
  import Icon from '@/shared/ui/Icon.svelte'

  let { zones, onJump }: { zones: GraphZone[]; onJump: (zone: GraphZone) => void } = $props()
  let open = $state(false)
  const rows = $derived(
    [...zones].sort((a, b) => (a.title || 'Без названия').localeCompare(b.title || 'Без названия', 'ru')),
  )
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<nav
  class="zmenu"
  class:zmenu--open={open}
  aria-label="Зоны карты"
  onpointerdown={(e) => e.stopPropagation()}
  onwheel={(e) => e.stopPropagation()}
>
  <div class="zmenu__panel">
    {#if open}
      <ul id="zone-menu">
        {#each rows as zone (zone.id)}
          <li>
            <button
              type="button"
              class="zmenu__go"
              style:border-left-color={zone.color}
              onclick={() => onJump(zone)}
            >
              {zone.title || 'Без названия'}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
    <button
      type="button"
      class="zmenu__fold"
      class:zmenu__fold--up={!open}
      aria-expanded={open}
      aria-controls="zone-menu"
      title={open ? 'Свернуть список зон' : 'Зоны на этой карте'}
      onclick={() => (open = !open)}
    >
      <span>Зоны <em>{zones.length}</em></span>
      <Icon name="chevron-down" size={12} />
    </button>
  </div>
</nav>

<style>
  .zmenu {
    position: relative;
    width: min(15rem, 46vw);
    height: 1.7rem;
    pointer-events: auto;
  }

  /* The slot stays one line tall, so opening the list does not lift the tool row. The panel grows upward over the map. */
  .zmenu__panel {
    position: absolute;
    left: 0;
    bottom: 0;
    width: 100%;
    box-sizing: border-box;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 10px);
    background: color-mix(in srgb, var(--color-bg-raised, #1a1a1a) 72%, transparent);
    backdrop-filter: blur(6px);
  }

  .zmenu--open .zmenu__panel {
    max-height: min(50vh, 20rem);
    overflow: auto;
    padding: 0.35rem 0.2rem 0.15rem;
  }

  .zmenu__fold {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
    width: 100%;
    box-sizing: border-box;
    padding: 0.28rem 0.6rem;
    border: 0;
    background: transparent;
    color: var(--color-accent, #c9a227);
    font: inherit;
    font-size: 0.74rem;
    cursor: pointer;
  }

  .zmenu--open .zmenu__fold {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .zmenu__fold:hover {
    color: var(--color-fg, #ddd);
  }

  .zmenu__fold--up :global(svg) {
    transform: rotate(180deg);
  }

  .zmenu__fold span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .zmenu__fold em {
    font-style: normal;
    color: var(--color-fg-muted, #9a9a9a);
  }

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .zmenu__go {
    display: block;
    width: 100%;
    box-sizing: border-box;
    padding: 0.22rem 0.5rem;
    border: 0;
    border-left: 2px solid transparent;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font: inherit;
    font-size: 0.78rem;
    line-height: 1.3;
    text-align: left;
    cursor: pointer;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .zmenu__go:hover {
    color: var(--color-fg, #ddd);
  }
</style>
