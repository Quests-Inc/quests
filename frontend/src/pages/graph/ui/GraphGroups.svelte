<script lang="ts">
  import { TYPES, type GraphGroupView } from '@/entities/graph'
  import Icon from '@/shared/ui/Icon.svelte'

  /** Шары групп на карте: рамка вокруг вложений одного владельца; раскрытый кластер можно свернуть обратно. */
  let {
    groups,
    collapseKey,
    onGroupDown,
    onGroupCtx,
    onCollapse,
  }: {
    groups: GraphGroupView[]
    collapseKey: (g: GraphGroupView) => string | null
    onGroupDown: (e: PointerEvent, g: GraphGroupView) => void
    onGroupCtx: (e: MouseEvent, g: GraphGroupView) => void
    onCollapse: (key: string) => void
  } = $props()
</script>

    {#each groups as g (g.key)}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="ggroup"
        style:left="{g.x - g.w / 2}px"
        style:top="{g.y - g.h / 2}px"
        style:width="{g.w}px"
        style:height="{g.h}px"
        style:--c={TYPES[g.type].color}
        onpointerdown={(e) => onGroupDown(e, g)}
        oncontextmenu={(e) => onGroupCtx(e, g)}
      >
        {#if collapseKey(g)}
          <button
            type="button"
            class="ggroup__label ggroup__label--btn"
            title="Свернуть обратно"
            onpointerdown={(e) => e.stopPropagation()}
            onclick={() => onCollapse(collapseKey(g) ?? '')}
          >
            <span class="ggroup__fold" aria-hidden="true"><Icon name="chevron-down" size={12} /></span>
            {g.title} · {g.count} · свернуть
          </button>
        {:else}
          <span class="ggroup__label">{g.title} · {g.count}</span>
        {/if}
      </div>
    {/each}

<style>


  .ggroup {
    position: absolute;
    box-sizing: border-box;
    border: 1.5px solid color-mix(in srgb, var(--c, #888) 45%, transparent);
    border-radius: 18px;
    background: color-mix(in srgb, var(--c, #888) 7%, transparent);
    cursor: grab;
  }

  .ggroup__label {
    position: absolute;
    left: 0.9rem;
    top: 0.55rem;
    font-size: 0.72rem;
    color: var(--c, #888);
    white-space: nowrap;
  }

  .ggroup__label--btn {
    padding: 0.1rem 0.5rem;
    border: 1px solid color-mix(in srgb, var(--c, #888) 55%, transparent);
    border-radius: 999px;
    background: var(--color-bg-raised, #1a1a1a);
    font: inherit;
    font-size: 0.68rem;
    cursor: pointer;
    pointer-events: auto;
  }

  .ggroup__label--btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .ggroup__fold {
    display: inline-flex;
    transform: rotate(180deg);
  }

  .ggroup__label--btn:hover {
    border-color: var(--c, #888);
  }</style>
