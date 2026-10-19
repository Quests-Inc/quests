<script lang="ts">
  import { AttachmentsBlock } from '@/entities/attachment'
  import { EntityHistory } from '@/entities/history'
  import { NoteCheck, NoteFind } from '@/entities/note'
  import { MentionTextarea } from '@/entities/template'
  import type { NoteEditor } from '../model/noteEditor.svelte.ts'
  import Icon from '@/shared/ui/Icon.svelte'
  import MarkdownBody from '@/shared/ui/MarkdownBody.svelte'
  import NoteHead from './NoteHead.svelte'
  import NoteLinks from './NoteLinks.svelte'

  type Ref = { kind: string; id: number; title?: string }
  type Crumb = { id: number | null; title: string }

  /** Тело открытой заметки: шапка, редактор, предпросмотр, ссылки и вложения. Раскладка страницы остаётся у NotesPage. */
  let {
    ed,
    selectedId,
    titleEl = $bindable(null),
    editEl = $bindable(null),
    previewEl = $bindable(null),
    findOpen = $bindable(false),
    refsOpen = $bindable(false),
    backlinksOpen = $bindable(false),
    titleReadonly,
    titleEditing = false,
    titleDraft = $bindable(''),
    noteIsOutdated,
    exportBusy,
    crumbs,
    labels = {},
    onRef,
    viewMode,
    viewModes,
    quests = [],
    questlines = [],
    notes = [],
    attachments = [],
    onOpenNoteMenu,
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
    onEditKeydown,
    onStartBodyEdit,
    onChanged,
    onOpenSecrets,
  }: {
    ed: NoteEditor
    selectedId: number | null
    titleEl?: HTMLInputElement | null
    editEl?: HTMLTextAreaElement | null
    previewEl?: HTMLDivElement | null
    findOpen?: boolean
    refsOpen?: boolean
    backlinksOpen?: boolean
    titleReadonly: boolean
    titleEditing?: boolean
    titleDraft?: string
    noteIsOutdated: boolean
    exportBusy: boolean
    crumbs: Crumb[]
    labels?: Record<string, string>
    onRef?: (kind: string, id: number) => void
    viewMode: string
    viewModes: readonly { id: string; label: string }[]
    quests?: { id: number; title?: string }[]
    questlines?: { id: number; title?: string }[]
    notes?: { id: number; title?: string }[]
    attachments?: { id: number; title?: string }[]
    onOpenNoteMenu: (e: MouseEvent, note: { id: number }) => void
    onTitleDblClick: () => void
    onTitleBlur: () => void
    onTitleKeydown: (event: KeyboardEvent) => void
    onTogglePin: () => void
    onToggleReadme: () => void
    onTogglePrivate: () => void
    onToggleCategory: () => void
    onToggleOutdated: () => void
    onRevert: () => void
    onSave: () => void
    onExport: (event: MouseEvent) => void
    onDelete: () => void
    onCrumb: (id: number | null) => void
    onParentPicker: (event: MouseEvent) => void
    onViewMode: (mode: string) => void
    onEditKeydown: (event: KeyboardEvent) => void
    onStartBodyEdit: (event?: MouseEvent) => void
    onChanged: () => void
    onOpenSecrets?: (id: number) => void
  } = $props()
</script>

{#if ed.error}
  <p class="notes__error">{ed.error}</p>
{/if}
{#if selectedId == null}
  <p class="notes__empty">Выберите заметку или создайте новую</p>
{:else}
  {#if ed.external}
    <div class="notes__external" role="status">
      <span>Заметку изменили снаружи, а у вас есть несохранённые правки.</span>
      <button type="button" class="notes__external-btn" onclick={() => ed.adoptExternal(selectedId)}>
        Загрузить новую версию
      </button>
      <button type="button" class="notes__external-btn" onclick={() => (ed.external = null)}>
        Оставить мою
      </button>
    </div>
  {/if}
  <NoteHead
    bind:title={() => (titleEditing ? titleDraft : ed.title), (v) => (titleDraft = v)}
    {titleEditing}
    bind:titleEl
    {titleReadonly}
    dirty={ed.dirty}
    pinned={ed.pinned}
    isReadme={ed.isReadme}
    isPrivate={ed.isPrivate}
    isCategory={ed.isCategory}
    outdated={noteIsOutdated}
    outdatedReason={ed.detail?.outdated_reason ?? ''}
    saving={ed.saving}
    {exportBusy}
    {crumbs}
    {labels}
    {onRef}
    bind:findOpen
    {viewMode}
    {viewModes}
    onContextMenu={(e) => {
      const n = notes.find((item) => item.id === selectedId)
      if (n) onOpenNoteMenu(e, n)
    }}
    onTitleDblClick={onTitleDblClick}
    onTitleBlur={onTitleBlur}
    {onTitleKeydown}
    {onTogglePin}
    {onToggleReadme}
    {onTogglePrivate}
    {onToggleCategory}
    {onToggleOutdated}
    {onRevert}
    {onSave}
    {onExport}
    {onDelete}
    {onCrumb}
    {onParentPicker}
    {onViewMode}
  />
  {#if ed.isPrivate && !ed.privateRevealed}
    <div class="notes__private-gate">
      <Icon name="lock" size={20} />
      <p>Приватная заметка — содержимое скрыто от случайного взгляда.</p>
      <button type="button" class="btn" onclick={() => (ed.privateRevealed = true)}>
        Показать
      </button>
    </div>
  {:else}
    {#if findOpen}
      <NoteFind noteId={selectedId} container={viewMode === 'raw' ? null : previewEl} dep={ed.description} onClose={() => (findOpen = false)} />
    {/if}
    <div
      class="notes__split"
      class:notes__split--raw={viewMode === 'raw'}
      class:notes__split--combined={viewMode === 'combined'}
      class:notes__split--formatted={viewMode === 'formatted'}
      class:notes__split--flash={ed.flashing}
    >
      {#if viewMode !== 'formatted'}
        <MentionTextarea
          class="notes__edit"
          placement="inside"
          bind:value={ed.description}
          bind:el={editEl}
          {quests}
          {questlines}
          {notes}
          {attachments}
          rows={16}
          placeholder="Markdown. @название — ссылка. Код и конфиг — в блоках ``` … ```"
          onkeydown={onEditKeydown}
        />
      {/if}
      {#if viewMode !== 'raw'}
        <div
          class="notes__preview block--prose"
          class:notes__preview--doc={viewMode === 'formatted'}
          bind:this={previewEl}
          ondblclick={onStartBodyEdit}
        >
          <MarkdownBody source={ed.description} {labels} {onRef} />
        </div>
      {/if}
    </div>
  {/if}
  <NoteLinks
    refs={(ed.detail?.refs ?? []) as Ref[]}
    backlinks={(ed.detail?.backlinks ?? []) as Ref[]}
    bind:refsOpen
    bind:backlinksOpen
    {onRef}
  />
  {#if selectedId}
    <div class="block notes__attach">
      <NoteCheck noteId={selectedId} {onChanged} onOpenSecrets={() => onOpenSecrets?.(selectedId)} />
    </div>
    <div class="block notes__attach">
      <AttachmentsBlock ownerType="note" ownerId={selectedId} onOpenOwner={onRef} />
    </div>
    <div class="block notes__attach">
      <EntityHistory entityType="note" entityId={selectedId} version={ed.detail?.updated_at} />
    </div>
  {/if}
{/if}

<style>
  .notes__private-gate {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2, 0.5rem);
    padding: var(--space-5, 2rem) var(--space-3, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
    text-align: center;
  }

  .notes__private-gate p {
    margin: 0;
    font-size: 0.9rem;
  }

  .notes__empty {
    color: var(--color-fg-muted, #9a9a9a);
    padding: 1rem;
  }

  .notes__error {
    margin: 0;
    color: var(--color-danger, #b54a3a);
  }

  .notes__external {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem 0.75rem;
    margin: 0;
    padding: 0.4rem 0.6rem;
    border-left: 3px solid var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 12%, transparent);
    font-size: 0.85rem;
  }

  .notes__external-btn {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent, #c9a227);
    font: inherit;
    text-decoration: underline;
    cursor: pointer;
  }

  .notes__split--flash {
    animation: notes-flash 1.5s ease-out;
  }

  @keyframes notes-flash {
    from {
      background: color-mix(in srgb, var(--color-accent, #c9a227) 22%, transparent);
    }
    to {
      background: transparent;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .notes__split--flash {
      animation: none;
    }
  }

  .notes__split {
    display: grid;
    gap: 0;
    min-height: 0;
    flex: 0 0 auto;
  }

  .notes__split--combined {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 0 1.1rem;
    background: linear-gradient(
      to right,
      transparent calc(50% - 0.5px),
      var(--color-border, #333) calc(50% - 0.5px),
      var(--color-border, #333) calc(50% + 0.5px),
      transparent calc(50% + 0.5px)
    );
  }

  .notes__split--combined :global(textarea.notes__edit) {
    border-right: 0;
    padding-right: 0.25rem;
  }

  .notes__split--combined .notes__preview {
    padding-left: 0.25rem;
  }

  .notes__split--raw,
  .notes__split--formatted {
    grid-template-columns: minmax(0, 1fr);
  }

  .notes__split--formatted .notes__preview {
    min-height: 0;
    overflow: visible;
  }

  .notes__split :global(textarea.notes__edit) {
    width: 100%;
    min-height: 18rem;
    resize: none;
    overflow: hidden;
    padding: 0.15rem 0.1rem;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: inherit;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .notes__preview {
    margin: 0;
    padding: 0.15rem 0.25rem 0.15rem 0.85rem;
    overflow: auto;
    min-height: 18rem;
    min-width: 0;
    overflow-wrap: break-word;
  }

  .notes__split--formatted .notes__preview {
    padding-left: 0.1rem;
  }

  .notes__preview--doc {
    cursor: text;
  }

  .notes__split--raw :global(textarea.notes__edit) {
    padding-left: 0.1rem;
  }

  .notes__attach {
    margin-top: 0;
    margin-bottom: 0;
    padding-top: var(--space-4, 1rem);
    border-top: 1px solid var(--color-border, #333);
  }

  @media (max-width: 720px) {
    .notes__split--combined {
      grid-template-columns: 1fr;
      gap: 0.85rem;
      background: none;
    }
    .notes__split--combined :global(textarea.notes__edit) {
      border-bottom: 1px solid var(--color-border, #333);
      padding-bottom: 0.85rem;
      padding-right: 0.1rem;
    }
    .notes__split--combined .notes__preview {
      padding-left: 0.1rem;
    }
  }
</style>
