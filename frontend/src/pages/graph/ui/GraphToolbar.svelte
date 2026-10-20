<script lang="ts">
  import type { ComponentProps } from 'svelte'
  import { ENTITY_LABELS, ENTITY_TYPES, EDGES, LAYOUTS, TYPES } from '@/entities/graph'
  import { ENTITY_SPEC } from '@/entities/quest'
  import { QUEST_STATUSES, QUEST_STATUS_LABELS } from '@/shared/api/client.ts'
  import { helpLine } from '@/shared/lib/searchFlags.ts'
  import FlagHints from '@/shared/ui/FlagHints.svelte'

  /** Панель управления картой: выбор центра, раскладка, фильтры, глубина, виды рёбер. Кнопки справа приходят в `children`. */
  let {
    rootQuery = $bindable(''),
    pickOpen = $bindable(false),
    layoutMode = $bindable('tree'),
    depth = $bindable(2),
    showEdge = $bindable({}),
    matches,
    rootParsed,
    statusLabel,
    filteredOut,
    questStatuses,
    typesLabel,
    shownTypes,
    onChooseRoot,
    onToggleStatus,
    onToggleType,
    onLayoutChange,
    children,
  }: {
    rootQuery?: string
    pickOpen?: boolean
    layoutMode?: string
    depth?: number
    showEdge?: Record<string, boolean>
    matches: { type: string; id: number; title: string }[]
    rootParsed: ComponentProps<typeof FlagHints>['parsed']
    statusLabel: string
    filteredOut: number
    questStatuses: Set<string>
    typesLabel: string
    shownTypes: Set<string>
    onChooseRoot: (m: any) => void
    onToggleStatus: (st: string) => void
    onToggleType: (t: string) => void
    onLayoutChange: () => void
    children?: import('svelte').Snippet
  } = $props()
</script>

<div class="graph__tools">
  <div class="graph__pick">
    <input
      class="search graph__search"
      type="search"
      placeholder="Центр: найти квест или заметку…"
      bind:value={rootQuery}
      onfocus={() => (pickOpen = true)}
      onblur={() => setTimeout(() => (pickOpen = false), 150)}
      aria-label="Выбрать центр карты"
      title={`Фильтры: ${helpLine(ENTITY_SPEC)}`}
    />
    <FlagHints value={rootQuery} spec={ENTITY_SPEC} parsed={rootParsed} onPick={(v) => (rootQuery = v)} />
    {#if pickOpen && matches.length}
      <ul class="graph__matches" role="listbox">
        {#each matches as m (m.type + m.id)}
          <li>
            <button type="button" onmousedown={(e) => e.preventDefault()} onclick={() => onChooseRoot(m)}>
              <span style:color={TYPES[m.type].color}>{TYPES[m.type].glyph}</span>
              {m.title}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
  <label class="graph__ctl">
    Раскладка
    <select
      bind:value={layoutMode}
      onchange={onLayoutChange}
      aria-label="Раскладка"
    >
      {#each Object.entries(LAYOUTS) as [k, label] (k)}
        <option value={k}>{label}</option>
      {/each}
    </select>
  </label>
  <details class="graph__filter">
    <summary title="Какие квесты показывать на карте и предлагать в «Добавить на карту»">
      Квесты: {statusLabel}{#if filteredOut} · скрыто {filteredOut}{/if}
    </summary>
    <div class="graph__filter-pop">
      {#each QUEST_STATUSES as st (st)}
        <label>
          <input type="checkbox" checked={questStatuses.has(st)} onchange={() => onToggleStatus(st)} />
          {QUEST_STATUS_LABELS[st]}
        </label>
      {/each}
    </div>
  </details>
  <details class="graph__filter">
    <summary title="Какие сущности показывать на карте">Сущности: {typesLabel}</summary>
    <div class="graph__filter-pop">
      {#each ENTITY_TYPES as t (t)}
        <label>
          <input type="checkbox" checked={shownTypes.has(t)} onchange={() => onToggleType(t)} />
          {ENTITY_LABELS[t]}
        </label>
      {/each}
    </div>
  </details>
  <label class="graph__ctl">
    Глубина
    <select bind:value={depth} aria-label="Глубина">
      <option value={1}>1</option>
      <option value={2}>2</option>
      <option value={3}>3</option>
    </select>
  </label>
  {#each Object.entries(EDGES) as [k, e] (k)}
    <label class="graph__ctl graph__edge">
      <input type="checkbox" bind:checked={showEdge[k]} />
      <svg width="26" height="8" aria-hidden="true">
        <line x1="0" y1="4" x2="26" y2="4" stroke={e.color} stroke-width="2" />
      </svg>
      {e.label}
    </label>
  {/each}
  {@render children?.()}
</div>

<style>


  .graph__tools {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.9rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__ctl {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .graph__ctl select {
    padding: 0.2rem 0.4rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 4px;
    background: transparent;
    color: inherit;
    font: inherit;
  }

  .graph__pick {
    position: relative;
    flex: 1 1 18rem;
    min-width: 11rem;
  }

  .graph__search {
    width: 100%;
    box-sizing: border-box;
  }

  .graph__matches {
    position: absolute;
    z-index: 5;
    top: calc(100% + 0.25rem);
    left: 0;
    right: 0;
    margin: 0;
    padding: 0.25rem;
    list-style: none;
    border: 1px solid var(--color-border, #333);
    border-radius: 8px;
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: var(--pill-shadow, none);
  }

  .graph__matches button {
    display: block;
    width: 100%;
    padding: 0.3rem 0.5rem;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }

  .graph__matches button:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .graph__filter {
    position: relative;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .graph__filter summary {
    cursor: pointer;
    padding: 0.25rem 0.6rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 4px;
    color: var(--color-fg, #e8e8e8);
    list-style: none;
  }

  .graph__filter summary::-webkit-details-marker {
    display: none;
  }

  .graph__filter-pop {
    position: absolute;
    z-index: 8;
    top: calc(100% + 0.3rem);
    left: 0;
    display: grid;
    gap: 0.25rem;
    min-width: 11rem;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 8px;
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: var(--pill-shadow, none);
  }

  .graph__filter-pop label {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--color-fg, #e8e8e8);
    font-size: var(--text-sm, 0.875rem);
  }@media (max-width: 720px) {


    .graph__tools {
      display: grid;
      grid-template-columns: repeat(6, 1fr);
      gap: 0.5rem 0.6rem;
      align-items: stretch;
      flex: 0 0 auto;
    }

    .graph__pick {
      grid-column: 1 / -1;
    }

    .graph__tools > .graph__ctl,
    .graph__tools > .graph__filter {
      grid-column: span 3;
      min-width: 0;
    }

    .graph__tools > .graph__edge {
      grid-column: span 2;
      justify-content: center;
      gap: 0.25rem;
      white-space: nowrap;
    }

    .graph__edge svg {
      width: 14px;
    }

    .graph__tools > :global(.graph__btn) {
      grid-column: span 3;
    }

    .graph__tools > .graph__ctl {
      justify-content: space-between;
    }

    .graph__tools > .graph__ctl select {
      flex: 1 1 auto;
      min-width: 0;
    }

    .graph__tools > .graph__filter summary {
      text-align: center;
      white-space: nowrap;
    }
}
</style>
