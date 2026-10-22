<script lang="ts">
  import type { ComponentProps } from 'svelte'
  import RefText from '@/shared/ui/RefText.svelte'
  import Icon from '@/shared/ui/Icon.svelte'

  /** Шапка заметки: заголовок, флаги и действия, цепочка родителей, режим просмотра. */
  let {
    title = $bindable(''),
    titleEl = $bindable(null),
    titleReadonly,
    titleEditing = false,
    dirty,
    pinned,
    isReadme,
    isPrivate,
    isCategory,
    outdated,
    outdatedReason = '',
    saving,
    exportBusy,
    crumbs,
    labels,
    onRef,
    findOpen = $bindable(false),
    viewMode,
    viewModes,
    onContextMenu,
    onTitleDblClick,
    onTitleBlur,
    onTitleKeydown,
    onTogglePin,
    onToggleReadme,
    onTogglePrivate,
    onToggleCategory,
    onToggleOutdated,
    onRevert,
    onSave,
    onExport,
    onDelete,
    onCrumb,
    onParentPicker,
    onViewMode,
    tools,
  }: {
    title?: string
    titleEl?: HTMLInputElement | null
    titleReadonly: boolean
    titleEditing?: boolean
    dirty: boolean
    pinned: boolean
    isReadme: boolean
    isPrivate: boolean
    isCategory: boolean
    outdated: boolean
    outdatedReason?: string
    saving: boolean
    exportBusy: boolean
    crumbs: { id: number | null; title: string }[]
    labels: ComponentProps<typeof RefText>['labels']
    onRef: ComponentProps<typeof RefText>['onRef']
    findOpen?: boolean
    viewMode: string
    viewModes: readonly { id: string; label: string }[]
    onContextMenu: (e: MouseEvent) => void
    onTitleDblClick: () => void
    onTitleBlur: () => void
    onTitleKeydown: (e: KeyboardEvent) => void
    onTogglePin: () => void
    onToggleReadme: () => void
    onTogglePrivate: () => void
    onToggleCategory: () => void
    onToggleOutdated: () => void
    onRevert: () => void
    onSave: () => void
    onExport: (e: MouseEvent) => void
    onDelete: () => void
    onCrumb: (id: number | null) => void
    onParentPicker: (e: MouseEvent) => void
    onViewMode: (id: string) => void
    tools?: import('svelte').Snippet
  } = $props()
</script>

    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <header
      class="notes__head"
      oncontextmenu={onContextMenu}
    >
      <div class="notes__head-row">
        <div class="notes__title-cluster" class:notes__title-cluster--editing={titleEditing}>
          <span class="notes__title-grow">
            <span class="notes__title-sizer" aria-hidden="true">{title || '\u00a0'}</span>
            <input
              class="notes__title"
              class:notes__title--locked={titleReadonly}
              size="1"
              bind:this={titleEl}
              bind:value={title}
              readonly={titleReadonly}
              title={titleReadonly ? 'Двойной клик — переименовать' : 'Enter — сохранить, Esc — отмена'}
              ondblclick={onTitleDblClick}
              onblur={onTitleBlur}
              onkeydown={onTitleKeydown}
            />
          </span>
          {#if dirty && !titleEditing}<span class="notes__unsaved" title="Несохранено">*</span>{/if}
        </div>
        {#if !titleEditing}
        <div class="detail__actions notes__actions">
          <button
            type="button"
            class="btn btn--icon"
            class:notes__pin--on={pinned}
            onclick={onTogglePin}
            title={pinned ? 'Открепить' : 'Закрепить'}
            aria-label={pinned ? 'Открепить' : 'Закрепить'}
            aria-pressed={pinned}
          >
            <Icon name={pinned ? 'pin-filled' : 'pin'} />
          </button>
          <button
            type="button"
            class="btn btn--icon notes__status-btn notes__status-btn--readme"
            class:notes__status-btn--on={isReadme}
            onclick={onToggleReadme}
            title={isReadme ? 'Снять README (прочитано)' : 'Пометить README (прочитать)'}
            aria-label={isReadme ? 'Снять README' : 'Пометить README'}
            aria-pressed={isReadme}
          >
            <Icon name="alert" />
          </button>
          <button
            type="button"
            class="btn btn--icon notes__status-btn notes__status-btn--private"
            class:notes__status-btn--on={isPrivate}
            onclick={onTogglePrivate}
            title={isPrivate ? 'Снять PRIVATE' : 'Пометить PRIVATE (скрыть от MCP)'}
            aria-label={isPrivate ? 'Снять PRIVATE' : 'Пометить PRIVATE'}
            aria-pressed={isPrivate}
          >
            <Icon name="lock" />
          </button>
          <button
            type="button"
            class="btn btn--icon notes__status-btn notes__status-btn--category"
            class:notes__status-btn--on={isCategory}
            onclick={onToggleCategory}
            title={isCategory ? 'Снять CATEGORY' : 'Пометить CATEGORY (папка-агрегатор)'}
            aria-label={isCategory ? 'Снять CATEGORY' : 'Пометить CATEGORY'}
            aria-pressed={isCategory}
          >
            <Icon name="folder" />
          </button>
          <button
            type="button"
            class="btn btn--icon notes__status-btn notes__status-btn--outdated"
            class:notes__status-btn--on={outdated}
            onclick={onToggleOutdated}
            title={outdated ? `Снять «устарела»${outdatedReason ? ': ' + outdatedReason : ''}` : 'Пометить «устарела» (содержимое расходится с действительностью)'}
            aria-label={outdated ? 'Снять «устарела»' : 'Пометить «устарела»'}
            aria-pressed={outdated}
          >
            <Icon name="clock-alert" />
          </button>
          <button
            type="button"
            class="btn btn--icon"
            class:notes__status-btn--on={findOpen}
            onclick={() => (findOpen = !findOpen)}
            title="Поиск в заметке (Ctrl+F)"
            aria-label="Поиск в заметке"
            aria-pressed={findOpen}
          >
            <Icon name="search" />
          </button>
          {@render tools?.()}
          {#if dirty}
            <button
              type="button"
              class="btn btn--icon"
              onclick={onRevert}
              title="Отменить правки"
              aria-label="Отменить правки"
            >
              <Icon name="undo" />
            </button>
          {/if}
          <button
            type="button"
            class="btn btn--icon"
            disabled={saving || !dirty}
            onclick={onSave}
            title={saving ? 'Сохранение…' : 'Сохранить (Ctrl+S)'}
            aria-label={saving ? 'Сохранение…' : 'Сохранить'}
          >
            {#if saving}…{:else}<Icon name="save" />{/if}
          </button>
          <button
            type="button"
            class="btn btn--icon"
            disabled={exportBusy}
            onclick={onExport}
            title={exportBusy ? 'Выгрузка…' : 'Скачать'}
            aria-label={exportBusy ? 'Выгрузка…' : 'Скачать заметку'}
            aria-haspopup="menu"
          >
            {#if exportBusy}…{:else}<Icon name="file-download" />{/if}
          </button>
          <button
            type="button"
            class="btn btn--icon btn--danger"
            onclick={onDelete}
            title="Удалить"
            aria-label="Удалить"
          >
            <Icon name="delete" />
          </button>
        </div>
        {/if}
      </div>
      <div class="notes__colophon">
        <nav class="notes__crumbs" aria-label="Родитель">
          {#each crumbs as c, i (c.id ?? 'root')}
            {#if i > 0}<span class="notes__crumb-sep" aria-hidden="true">/</span>{/if}
            <button
              type="button"
              class="notes__crumb"
              class:notes__crumb--here={i === crumbs.length - 1}
              onclick={(e) => {
                if (i === crumbs.length - 1) onParentPicker(e)
                else onCrumb(c.id)
              }}
              title={i === crumbs.length - 1 ? 'Сменить родителя' : `Вложить в «${c.title}»`}
            >
              <RefText source={c.title} {labels} {onRef} />
            </button>
          {/each}
        </nav>
        <div class="notes__modes" role="radiogroup" aria-label="Режим просмотра">
          {#each viewModes as mode (mode.id)}
            <button
              type="button"
              class="notes__mode"
              class:notes__mode--on={viewMode === mode.id}
              data-mode={mode.id}
              role="radio"
              aria-checked={viewMode === mode.id}
              onclick={() => onViewMode(mode.id)}
            >
              {mode.label}
            </button>
          {/each}
        </div>
      </div>
    </header>

<style>


  /* Status toggles in the open note's header, and their matching sidebar badges. */
  .notes__status-btn {
    opacity: 0.35;
  }

  .notes__status-btn--on {
    opacity: 1;
  }

  .notes__head {
    /* No bottom margin: the page's flex gap alone sets the distance from the
       header divider to the note body, matching the gap below the body so the
       body sits evenly between its two dividers. */
    margin: 0;
    padding: 0 0 var(--space-3, 0.75rem);
    border-bottom: 1px solid var(--color-border, #333);
  }

  .notes__head-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3, 0.75rem);
  }

  .notes__actions {
    flex-shrink: 0;
  }

  .notes__pin--on {
    color: var(--color-accent, #c9a227);
  }

  .notes__title-cluster {
    display: inline-flex;
    align-items: baseline;
    flex: 1 1 auto;
    min-width: 0;
    max-width: 100%;
    font-family: var(--font-display, Georgia, serif);
    font-size: clamp(1.35rem, 2.4vw, 1.85rem);
    font-weight: 700;
    line-height: 1.15;
  }

  .notes__title-grow {
    position: relative;
    display: inline-grid;
    /* Flex items default to min-width:auto (= their content's min-content),
       and the sizer's white-space:pre makes that the full title width —
       without an explicit 0 here a long title refuses to shrink and
       overlaps the pin/save buttons instead of clipping. */
    min-width: 0;
    max-width: 100%;
    overflow: hidden;
  }

  .notes__title-sizer,
  .notes__title {
    grid-area: 1 / 1;
    /* Same min-width:auto trap applies to grid items sizing their track —
       without this the track still grows to the sizer's full-text
       min-content width even though the container above is now capped. */
    min-width: 0;
    padding: 0.15rem 0;
    font: inherit;
  }

  .notes__title-sizer {
    visibility: hidden;
    white-space: pre;
    pointer-events: none;
  }

  .notes__title {
    width: 100%;
    min-width: 0;
    border: 0;
    background: transparent;
    color: inherit;
    box-sizing: border-box;
  }

  .notes__title--locked {
    cursor: default;
    caret-color: transparent;
  }

  .notes__title--locked:focus {
    outline: none;
  }

  /* Editing the name: the whole cluster becomes an obvious field. */
  .notes__title-cluster--editing {
    padding: 0.1rem 0.6rem;
    border: 1px solid var(--color-accent, #c9a227);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-accent, #c9a227) 22%, transparent);
  }

  .notes__title-cluster--editing .notes__title:focus {
    outline: none;
  }

  .notes__title-cluster :global(.notes__unsaved) {
    flex-shrink: 0;
    padding: 0.15rem 0;
  }

  .notes__colophon {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.35rem 0.85rem;
    margin: 0.45rem 0 0;
    font-family: var(--font-ui, sans-serif);
    font-size: 0.72rem;
    letter-spacing: 0.02em;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__crumbs {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.25rem 0.35rem;
    min-width: 0;
  }

  .notes__crumb {
    border: 0;
    padding: 0;
    margin: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
    max-width: 12rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notes__crumb:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .notes__crumb--here {
    color: var(--color-fg, #e8e8e8);
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  .notes__crumb-sep {
    opacity: 0.45;
    user-select: none;
  }

  .notes__modes {
    margin-left: auto;
    display: flex;
    flex-wrap: wrap;
    gap: 0.65rem;
    align-items: baseline;
  }

  .notes__mode {
    border: 0;
    padding: 0;
    margin: 0;
    background: transparent;
    color: var(--color-fg-subtle, #6e6e6e);
    font: inherit;
    letter-spacing: 0.02em;
    cursor: pointer;
  }

  .notes__mode:hover {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .notes__mode--on {
    color: var(--color-fg, #e8e8e8);
    box-shadow: 0 1px 0 currentColor;
  }@media (max-width: 480px) {

    .notes__head-row {
      flex-wrap: wrap;
      row-gap: var(--space-2, 0.5rem);
    }

    .notes__title-cluster {
      flex-basis: 100%;
    }

    .notes__actions {
      margin-left: auto;
    }
}
@media (max-width: 720px) {

    .notes__modes {
      margin-left: 0;
      width: 100%;
    }
}
@media (max-width: 480px) {


    /* No room for a side-by-side split; the $effect above steers away from
       it too, so this is just for anyone still mid-toggle on resize. */
    .notes__mode[data-mode="combined"] {
      display: none;
    }
}
</style>
