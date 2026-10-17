<script lang="ts">
  import type { ComponentProps } from 'svelte'
  import type { NoteListItem } from '@/shared/api/knowledge_http.gen'
  import { NOTE_SPEC } from '@/entities/note'
  import { QuestlineIcon } from '@/entities/questline'
  import { helpLine } from '@/shared/lib/searchFlags.ts'
  import { snippetHtml, WHERE_LABEL } from '@/shared/lib/searchSnippet.ts'
  import FlagHints from '@/shared/ui/FlagHints.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import RefText from '@/shared/ui/RefText.svelte'

  type SearchHit = { id: number; title?: string; outdated?: boolean; outdated_reason?: string; where?: string; snippet?: string }

  /** Боковое дерево заметок: поиск, результаты поиска, вложенные строки с переименованием и перетаскиванием. */
  let {
    search = $bindable(''),
    parsedSearch,
    searchActive,
    searchHits,
    notes,
    roots,
    selectedId,
    dragOverKey,
    renamingId,
    renamingValue = $bindable(''),
    renameInputEl = $bindable(null),
    readmeBubbleIds,
    labels,
    onRef,
    onSelect,
    pickedIds = new Set<number>(),
    onRowClick,
    onClearPicked,
    onAdd,
    onMenu,
    hasChildren,
    showChildren,
    childrenOf,
    onToggleTree,
    rowTitle,
    rowDirty,
    onRenameKeydown,
    onCommitRename,
    onListDragOver,
    onListDragLeave,
    onListDrop,
    onRowDragStart,
    onRowDragEnd,
    onRowDragOver,
    onRowDragLeave,
    onRowDrop,
  }: {
    search?: string
    parsedSearch: ComponentProps<typeof FlagHints>['parsed']
    searchActive: boolean
    searchHits: SearchHit[] | null
    notes: NoteListItem[]
    roots: NoteListItem[]
    selectedId: number | null
    dragOverKey: string | null
    renamingId: number | null
    renamingValue?: string
    renameInputEl?: HTMLInputElement | null
    readmeBubbleIds: Set<unknown>
    labels: ComponentProps<typeof RefText>['labels']
    onRef: ComponentProps<typeof RefText>['onRef']
    onSelect: (id: number) => void
    /** Notes picked with Ctrl/Shift-click. */
    pickedIds?: ReadonlySet<number>
    /** Ctrl/Shift-click goes here first: true = taken into the selection, the row is not opened. */
    onRowClick?: (event: MouseEvent, id: number, order: number[]) => boolean
    onClearPicked?: () => void
    onAdd: () => void
    onMenu(e: MouseEvent, n: { id: number }): void
    hasChildren(id: number): boolean
    showChildren(id: number): boolean
    childrenOf(id: number): NoteListItem[]
    onToggleTree(id: number, e: MouseEvent): void
    rowTitle(n: NoteListItem): string
    rowDirty(n: NoteListItem): boolean
    onRenameKeydown(e: KeyboardEvent): void
    onCommitRename: () => void
    onListDragOver(e: DragEvent): void
    onListDragLeave(): void
    onListDrop(e: DragEvent): void
    onRowDragStart(e: DragEvent, id: number): void
    onRowDragEnd: () => void
    onRowDragOver(e: DragEvent, id: number): void
    onRowDragLeave(e: DragEvent, id: number): void
    onRowDrop(e: DragEvent, id: number): void
  } = $props()

  let treeEl: HTMLElement | null = $state(null)
  /** Note ids in the order the rows are on screen now (folded branches are not there). */
  function visibleOrder() {
    const els: HTMLElement[] = treeEl ? [...treeEl.querySelectorAll<HTMLElement>('[data-nid]')] : []
    return [...new Set(els.map((el) => Number(el.dataset.nid)))]
  }
  function rowClick(e: MouseEvent, id: number) {
    if (!onRowClick?.(e, id, visibleOrder())) onSelect(id)
  }
</script>

<aside class="notes__tree" bind:this={treeEl}>
  <div class="notes__tools">
    <input
      class="search"
      type="search"
      placeholder="Поиск…"
      title={`Фильтры: ${helpLine(NOTE_SPEC)}; -flag:… — без флага`}
      bind:value={search}
    />
    <button
      type="button"
      class="btn btn--accent notes__add"
      aria-label="Новая заметка"
      onclick={onAdd}
    >
      +
    </button>
  </div>
  {#if pickedIds.size > 0}
    <div class="notes__picked" role="status">
      <span>Выбрано: {pickedIds.size}</span>
      <button type="button" class="notes__picked-clear" title="Снять выделение (Esc)" aria-label="Снять выделение" onclick={() => onClearPicked?.()}><Icon name="close" size={14} /></button>
    </div>
  {/if}
  <div class="notes__flaghints"><FlagHints value={search} spec={NOTE_SPEC} parsed={parsedSearch} onPick={(v) => (search = v)} /></div>
  <div
    class="notes__list"
    data-nav-root
    class:notes__list--dragover={dragOverKey === 'root'}
    ondragover={onListDragOver}
    ondragleave={onListDragLeave}
    ondrop={onListDrop}
  >
    {#if searchActive && searchHits}
      {#if searchHits.length === 0}
        <p class="empty">Ничего не найдено</p>
      {:else}
        <ul class="hits">
          {#each searchHits as h (h.id)}
            <li>
              <button type="button" data-nav data-nid={h.id} class="hit" class:hit--on={h.id === selectedId} class:hit--picked={pickedIds.has(h.id)} onclick={(e) => rowClick(e, h.id)} oncontextmenu={(e) => onMenu(e, h)}>
                <span class="hit__title">{h.title || 'Без названия'}{#if h.outdated}<span class="notes__status-dot notes__status-dot--outdated notes__hit-flag" title={h.outdated_reason || 'Устарела'}><Icon name="clock-alert" size={12} /></span>{/if}</span>
                <span class="hit__where">{WHERE_LABEL[h.where] ?? ''}</span>
                {#if h.snippet}<span class="hit__snippet">{@html snippetHtml(h.snippet)}</span>{/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {:else if notes.length === 0}
      <p class="empty">Пока пусто — это хранилище знания, не список дел.</p>
    {:else if roots.length === 0 && search}
      <p class="empty">Ничего не найдено</p>
    {:else}
      {#snippet tree(nodes: any[], depth: number)}
        {#each nodes as n (n.id)}
          <div class="notes__node" data-nav-node style:--notes-depth="{depth}">
            {#if hasChildren(n.id)}
              <button
                type="button"
                class="notes__fold"
                aria-expanded={showChildren(n.id)}
                aria-label={showChildren(n.id) ? 'Свернуть' : 'Развернуть'}
                onclick={(e) => onToggleTree(n.id, e)}
              >
                <Icon
                  name={showChildren(n.id) ? 'chevron-down' : 'chevron-right'}
                  size={12}
                />
              </button>
            {:else}
              <span class="notes__fold notes__fold--spacer" aria-hidden="true"></span>
            {/if}
            <button
              type="button"
              data-nav
              class="notes__row"
              data-nid={n.id}
              class:notes__row--on={n.id === selectedId}
              class:notes__row--picked={pickedIds.has(n.id)}
              class:notes__row--pin={n.pinned}
              class:notes__row--dragover={dragOverKey === String(n.id)}
              draggable={n.id !== renamingId}
              onclick={(e) => rowClick(e, n.id)}
              oncontextmenu={(e) => onMenu(e, n)}
              ondragstart={(e) => onRowDragStart(e, n.id)}
              ondragend={onRowDragEnd}
              ondragover={(e) => onRowDragOver(e, n.id)}
              ondragleave={(e) => onRowDragLeave(e, n.id)}
              ondrop={(e) => onRowDrop(e, n.id)}
            >
              <span class="notes__row-icon" style="--line-color: {n.color || '#9a9a9a'}">
                <QuestlineIcon
                  icon={n.icon || 'document'}
                  iconUrl={n.icon_url}
                  size="sm"
                />
              </span>
              <span class="notes__row-label">
                {#if n.id === renamingId}
                  <input
                    bind:this={renameInputEl}
                    class="notes__row-rename"
                    type="text"
                    bind:value={renamingValue}
                    onclick={(e) => e.stopPropagation()}
                    onkeydown={onRenameKeydown}
                    onblur={onCommitRename}
                  />
                {:else}
                  <RefText class="notes__row-title" source={rowTitle(n)} {labels} {onRef} />{#if rowDirty(n)}<span
                      class="notes__unsaved"
                      title="Несохранено">*</span>{/if}
                {/if}
              </span>
              {#if n.is_readme || n.is_private || n.is_category || n.outdated || readmeBubbleIds.has(n.id)}
                <span class="notes__row-status" aria-hidden="true">
                  {#if n.is_readme}<span class="notes__status-dot notes__status-dot--readme"><Icon name="alert" size={12} /></span>{:else if readmeBubbleIds.has(n.id)}<span class="notes__status-dot notes__status-dot--readme-bubble" title="Ниже есть README"><Icon name="alert" size={12} /></span>{/if}
                  {#if n.is_private}<span class="notes__status-dot notes__status-dot--private"><Icon name="lock" size={12} /></span>{/if}
                  {#if n.is_category}<span class="notes__status-dot notes__status-dot--category"><Icon name="folder" size={12} /></span>{/if}
                  {#if n.outdated}<span class="notes__status-dot notes__status-dot--outdated" title={n.outdated_reason || 'Устарела'}><Icon name="clock-alert" size={12} /></span>{/if}
                </span>
              {/if}
            </button>
          </div>
          {#if showChildren(n.id)}
            {@render tree(childrenOf(n.id), depth + 1)}
          {/if}
        {/each}
      {/snippet}
      {@render tree(roots, 0)}
    {/if}
  </div>
</aside>

<style>


  .notes__tree {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    border-right: 1px solid var(--color-border, #333);
    background: var(--color-bg-raised, #1a1a1a);
  }

  .notes__tools {
    display: flex;
    gap: 0.4rem;
    padding: 0.55rem 0.6rem;
    border-bottom: 1px solid var(--color-border, #333);
  }

  .notes__flaghints:empty {
    display: none;
  }

  .hits {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 2px;
  }
  .hit {
    display: grid;
    gap: 0.15rem;
    width: 100%;
    padding: 0.45rem 0.6rem;
    text-align: left;
    color: inherit;
    background: transparent;
    border: 0;
    border-radius: var(--radius-sm, 6px);
    cursor: pointer;
  }
  .hit:hover,
  .hit--on {
    background: var(--color-bg-hover, #2a2a2a);
  }
  .hit__title {
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg);
  }
  .hit__where {
    font-size: 0.68rem;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-fg-subtle, #6e6e6e);
  }
  .hit__snippet {
    font-size: 0.78rem;
    line-height: 1.35;
    color: var(--color-fg-muted, #9a9a9a);
    overflow-wrap: anywhere;
  }
  .hit__snippet :global(mark) {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 35%, transparent);
    color: var(--color-fg);
    border-radius: 2px;
  }

  .notes__tools .search {
    flex: 1 1 auto;
    min-width: 0;
  }

  .notes__add {
    flex: 0 0 auto;
    min-width: 2rem;
    padding-inline: 0.55rem;
    font-size: 1.1rem;
    line-height: 1;
  }

  .notes__list {
    flex: 1 1 auto;
    overflow: auto;
    padding: 0.25rem 0;
  }

  .notes__node {
    display: flex;
    align-items: stretch;
    padding-left: calc(0.35rem + var(--notes-depth, 0) * 0.85rem);
  }

  .notes__fold {
    flex: 0 0 1.35rem;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    padding: 0;
    margin: 0;
    background: transparent;
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 55%, transparent);
    cursor: pointer;
    border-radius: 2px;
  }

  .notes__fold:hover {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 6%, transparent);
  }

  .notes__row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1 1 auto;
    min-width: 0;
    border: 0;
    border-left: 2px solid transparent;
    background: transparent;
    color: inherit;
    text-align: left;
    padding: 0.28rem 0.45rem 0.28rem 0.35rem;
    cursor: pointer;
    font-family: var(--font-body, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
  }

  .notes__row:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 4%, transparent);
  }

  .notes__hit-flag {
    margin-left: 0.35rem;
    vertical-align: middle;
  }

  .empty {
    color: var(--color-fg-muted, #9a9a9a);
    padding: 1rem;
  }

  .notes__fold--spacer {
    pointer-events: none;
  }

  .notes__picked {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.3rem 0.75rem;
    border-bottom: 1px solid var(--color-border, #333);
    font-size: var(--text-sm, 0.875rem);
  }

  .notes__picked-clear {
    display: inline-flex;
    padding: 0.15rem;
    border: 0;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .notes__picked-clear:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .notes__row--picked,
  .hit--picked {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 14%, var(--color-bg-raised, #1a1a1a));
    box-shadow: inset 3px 0 0 var(--color-accent, #c9a227);
  }

  /* Hover must not wipe the pick (the plain hover rule wins on specificity): picked + hover is stronger. */
  .notes__row--picked:hover,
  .notes__row--picked:focus-visible,
  .hit--picked:hover,
  .hit--picked:focus-visible {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 24%, var(--color-bg-raised, #1a1a1a));
    box-shadow:
      inset 3px 0 0 var(--color-accent, #c9a227),
      inset 0 0 0 1px color-mix(in srgb, var(--color-accent, #c9a227) 45%, transparent);
  }

  .notes__row--on {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 6%, var(--color-bg-raised, #1a1a1a));
    border-left-color: var(--color-fg, #e8e8e8);
  }

  .notes__row--dragover {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 16%, transparent);
    border-left-color: var(--color-accent, #c9a227);
  }

  .notes__list--dragover {
    box-shadow: inset 0 0 0 1px var(--color-accent, #c9a227);
  }

  .notes__row-icon {
    flex-shrink: 0;
    display: inline-flex;
    color: var(--line-color, #9a9a9a);
  }

  .notes__row-label {
    display: inline-flex;
    align-items: baseline;
    flex: 1 1 auto;
    min-width: 0;
    max-width: 100%;
  }

  :global(.notes__row-title) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notes__row-rename {
    width: 100%;
    min-width: 0;
    padding: 0.1rem 0.3rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg, #121212);
    color: var(--color-fg, #e8e8e8);
    font: inherit;
  }

  .notes__unsaved {
    flex-shrink: 0;
  }

  .notes__row-status {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    gap: 0.15rem;
  }

  .notes__status-dot {
    display: inline-flex;
  }
</style>
