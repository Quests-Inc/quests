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
      placeholder="Добавить дерево: найти квест или заметку…"
      bind:value={rootQuery}
      onfocus={() => (pickOpen = true)}
      onblur={() => setTimeout(() => (pickOpen = false), 150)}
      aria-label="Добавить дерево на карту"
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
  <details class="graph__filter graph__defaults">
    <summary title="Эти параметры применяются только при добавлении дерева">Новое дерево: {LAYOUTS[layoutMode]} · {depth}</summary>
    <div class="graph__filter-pop">
      <label class="graph__ctl">Раскладка
        <select bind:value={layoutMode} onchange={onLayoutChange} aria-label="Раскладка нового дерева">
          {#each Object.entries(LAYOUTS) as [k, label] (k)}<option value={k}>{label}</option>{/each}
        </select>
      </label>
      <label class="graph__ctl">Глубина
        <select bind:value={depth} aria-label="Глубина нового дерева">
          {#each [1, 2, 3] as d (d)}<option value={d}>{d}</option>{/each}
        </select>
      </label>
      <small>Для деревьев на карте — ПКМ по карточке или выделению.</small>
    </div>
  </details>
  <div class="graph__visibility" role="group" aria-label="Видимость на карте">
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
  <details class="graph__filter">
    <summary>Связи</summary>
    <div class="graph__filter-pop">
  {#each Object.entries(EDGES) as [k, e] (k)}
    <label class="graph__ctl graph__edge">
      <input type="checkbox" bind:checked={showEdge[k]} aria-label={e.label} />
      <svg width="26" height="8" aria-hidden="true">
        <line x1="0" y1="4" x2="26" y2="4" stroke={e.color} stroke-width="2" />
      </svg>
      <span>{e.label}</span>
    </label>
  {/each}
    </div>
  </details>
  </div>
  <div class="graph__actions">{@render children?.()}</div>
</div>

<style>
  .graph__tools { display: flex; flex-wrap: wrap; align-items: center; gap: 0.5rem; font-size: var(--text-xs, 0.75rem); color: var(--color-fg-muted, #9a9a9a); }
  .graph__pick { position: relative; flex: 1 1 20rem; min-width: 12rem; }
  .graph__search { width: 100%; box-sizing: border-box; }
  .graph__visibility, .graph__actions { display: flex; align-items: center; flex-wrap: wrap; gap: 0.4rem; }
  .graph__visibility { padding-left: 0.5rem; border-left: 1px solid var(--color-border, #333); }
  .graph__ctl { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; }
  .graph__ctl select { padding: 0.25rem; border: 1px solid var(--color-border, #333); border-radius: 4px; background: var(--color-bg-raised, #1a1a1a); color: inherit; font: inherit; }
  .graph__filter { position: relative; }
  .graph__filter summary { cursor: pointer; padding: 0.35rem 0.6rem; border: 1px solid var(--color-border, #333); border-radius: 6px; color: var(--color-fg, #e8e8e8); list-style: none; white-space: nowrap; }
  .graph__filter summary::-webkit-details-marker { display: none; }
  .graph__filter-pop { position: absolute; z-index: 18; top: calc(100% + 0.3rem); right: 0; display: grid; gap: 0.5rem; min-width: 14rem; max-width: min(22rem, 85vw); padding: 0.7rem; border: 1px solid var(--color-border, #333); border-radius: 8px; background: var(--color-bg-raised, #1a1a1a); box-shadow: var(--pill-shadow, none); }
  .graph__filter-pop label { display: flex; align-items: center; gap: 0.4rem; color: var(--color-fg, #e8e8e8); }
  .graph__filter-pop .graph__edge { display: grid; grid-template-columns: 1.1rem 26px 1fr; justify-content: start; gap: 0.6rem; }
  .graph__edge input { margin: 0; }
  .graph__edge svg { display: block; }
  .graph__filter-pop small { line-height: 1.5; }
  .graph__matches { position: absolute; z-index: 19; top: calc(100% + 0.25rem); left: 0; right: 0; margin: 0; padding: 0.25rem; list-style: none; border: 1px solid var(--color-border, #333); border-radius: 8px; background: var(--color-bg-raised, #1a1a1a); box-shadow: var(--pill-shadow, none); }
  .graph__matches button { display: block; width: 100%; padding: 0.4rem 0.5rem; border: 0; border-radius: 4px; background: transparent; color: var(--color-fg, #e8e8e8); font: inherit; font-size: var(--text-sm, 0.875rem); text-align: left; cursor: pointer; }
  .graph__matches button:hover { background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent); }
  @media (max-width: 720px) {
    .graph__pick { flex-basis: 100%; }
    .graph__visibility { border: 0; padding: 0; }
    .graph__defaults .graph__filter-pop { left: 0; right: auto; }
  }
</style>
