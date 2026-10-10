<script>
  import { listTags, createTag, updateTag, deleteTag } from '../js/api.ts'
  import Icon from '../ui/Icon.svelte'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import ConfirmModal from './ConfirmModal.svelte'
  import { untrack } from 'svelte'

  /** @type {{ open: boolean, onClose: () => void }} */
  let { open = false, onClose } = $props()

  let loading = $state(false)
  let error = $state('')
  let tags = $state(/** @type {{ id: number, slug: string, label: string, color?: string }[]} */ ([]))
  let saving = $state(false)

  let newSlug = $state('')
  let newLabel = $state('')
  let newColor = $state('')

  /** @type {number | null} */
  let editingId = $state(null)
  let editLabel = $state('')
  let editColor = $state('')

  let deleteOpen = $state(false)
  /** @type {{ id: number, slug: string, label: string } | null} */
  let deleteTarget = $state(null)

  function normalizeSlug(raw) {
    return String(raw || '')
      .trim()
      .toLowerCase()
      .replace(/ё/g, 'е')
      .replace(/[\s_]+/g, '-')
      .replace(/[^a-z0-9а-яё-]+/gi, '')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '')
  }

  async function refresh() {
    loading = true
    error = ''
    try {
      tags = await listTags()
    } catch (e) {
      error = e.message || String(e)
      tags = []
    } finally {
      loading = false
    }
  }

  $effect(() => {
    if (!open) return
    untrack(() => {
      error = ''
      newSlug = ''
      newLabel = ''
      newColor = ''
      editingId = null
      deleteOpen = false
      deleteTarget = null
      void refresh()
    })
  })

  async function onCreate(event) {
    event.preventDefault()
    if (saving) return
    const slug = normalizeSlug(newSlug)
    const label = newLabel.trim()
    if (!slug || !label) {
      error = 'Нужны slug и подпись'
      return
    }
    if (label.length > 6) {
      error = 'Подпись — не длиннее 6 символов'
      return
    }
    const color = newColor.trim() || undefined
    saving = true
    error = ''
    try {
      await createTag({ slug, label, color })
      newSlug = ''
      newLabel = ''
      newColor = ''
      await refresh()
    } catch (e) {
      error = e.message || String(e)
    } finally {
      saving = false
    }
  }

  function startEdit(tag) {
    editingId = tag.id
    editLabel = tag.label || ''
    editColor = tag.color || ''
    error = ''
  }

  function cancelEdit() {
    editingId = null
    editLabel = ''
    editColor = ''
  }

  async function saveEdit(id) {
    if (saving) return
    const label = editLabel.trim()
    if (!label) {
      error = 'Подпись не может быть пустой'
      return
    }
    if (label.length > 6) {
      error = 'Подпись — не длиннее 6 символов'
      return
    }
    const color = editColor.trim() || null
    saving = true
    error = ''
    try {
      await updateTag(id, { label, color })
      editingId = null
      await refresh()
    } catch (e) {
      error = e.message || String(e)
    } finally {
      saving = false
    }
  }

  function askDelete(tag) {
    deleteTarget = tag
    deleteOpen = true
  }

  async function confirmDelete() {
    if (!deleteTarget || saving) return
    saving = true
    error = ''
    try {
      await deleteTag(deleteTarget.id)
      if (editingId === deleteTarget.id) cancelEdit()
      deleteOpen = false
      deleteTarget = null
      await refresh()
    } catch (e) {
      error = e.message || String(e)
    } finally {
      saving = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="tags-modal-title" zIndex={40} maxWidth="32rem">
  <ModalHead id="tags-modal-title" title="Теги" icon="pin" {onClose} />

  <div class="modal__body">
    {#if loading}
      <p class="hint">Загрузка…</p>
    {:else}
      {#if error}
        <p class="modal__error">{error}</p>
      {/if}

      <ul class="tags-list">
        {#each tags as tag (tag.id)}
          <li class="tag-row">
            <span
              class="tag-row__swatch"
              class:tag-row__swatch--empty={!tag.color}
              style:--swatch={tag.color || null}
              aria-hidden="true"
            ></span>
            <code class="tag-row__slug">{tag.slug}</code>
            {#if editingId === tag.id}
              <input type="text" class="tag-row__label" maxlength="6" bind:value={editLabel} />
              <input
                type="text"
                class="tag-row__color"
                placeholder="#hex"
                bind:value={editColor}
                spellcheck="false"
              />
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                aria-label="Сохранить"
                onclick={() => saveEdit(tag.id)}
                disabled={saving}
              >
                <Icon name="checkmark" size={14} />
              </button>
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                aria-label="Отмена"
                onclick={cancelEdit}
                disabled={saving}
              >
                <Icon name="close" size={14} />
              </button>
            {:else}
              <span class="tag-row__label-text">{tag.label}</span>
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                aria-label="Изменить"
                onclick={() => startEdit(tag)}
                disabled={saving}
              >
                <Icon name="edit" size={14} />
              </button>
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                aria-label="Удалить"
                onclick={() => askDelete(tag)}
                disabled={saving}
              >
                <Icon name="delete" size={14} />
              </button>
            {/if}
          </li>
        {:else}
          <li class="hint">Тегов пока нет.</li>
        {/each}
      </ul>

      <form class="add-row" onsubmit={onCreate}>
        <input
          type="text"
          class="add-row__slug"
          placeholder="slug"
          bind:value={newSlug}
          spellcheck="false"
          autocomplete="off"
        />
        <input
          type="text"
          class="add-row__label"
          placeholder="подпись"
          maxlength="6"
          bind:value={newLabel}
          autocomplete="off"
        />
        <input
          type="text"
          class="add-row__color"
          placeholder="#цвет"
          bind:value={newColor}
          spellcheck="false"
          autocomplete="off"
        />
        <button type="submit" class="btn btn--accent" disabled={saving}>
          <Icon name="add" size={14} />
          <span class="btn__text">Добавить</span>
        </button>
      </form>
      <p class="hint">Подпись на бейдже — до 6 символов; slug не меняется после создания.</p>
    {/if}
  </div>
</ModalShell>

<ConfirmModal
  open={deleteOpen}
  title="Удалить тег?"
  message={deleteTarget ? `Удалить тег «${deleteTarget.label}» (${deleteTarget.slug})?` : ''}
  busy={saving}
  onConfirm={confirmDelete}
  onCancel={() => {
    deleteOpen = false
    deleteTarget = null
  }}
/>

<style>
  .tags-list {
    display: grid;
    gap: 0.4rem;
    margin: 0 0 1rem;
    padding: 0;
    list-style: none;
    min-width: 0;
  }

  .tag-row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    box-sizing: border-box;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    min-height: 2.5rem;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg-muted, #242424);
  }

  .tag-row__swatch {
    flex: none;
    width: 0.75rem;
    height: 0.75rem;
    border-radius: 50%;
    background: var(--swatch);
  }

  .tag-row__swatch--empty {
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--color-fg-subtle, #6e6e6e);
  }

  .tag-row__slug {
    flex: 0 1 4.5rem;
    min-width: 0;
    max-width: 5rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-subtle, #6e6e6e);
  }

  /* Slug is immutable — hide while editing so label gets the leftover width. */
  .tag-row:has(:global(input.tag-row__label)) .tag-row__slug {
    display: none;
  }

  .tag-row__label-text {
    flex: 1 1 0%;
    min-width: 0;
    font-size: var(--text-sm, 0.875rem);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /*
   * UA input min-width (~20ch) blew the modal on mobile. Label flexes into
   * leftover space (basis 0 + min-width 0); color stays fixed for #rrggbb.
   */
  .tag-row :global(input.tag-row__label) {
    flex: 1 1 0%;
    box-sizing: border-box;
    width: auto;
    min-width: 0;
    max-width: none;
    height: 1.9rem;
    padding: 0.2rem 0.4rem;
  }

  .tag-row :global(input.tag-row__color) {
    flex: 0 0 5.5rem;
    box-sizing: border-box;
    width: 5.5rem;
    min-width: 5.5rem;
    max-width: 5.5rem;
    height: 1.9rem;
    padding: 0.2rem 0.35rem;
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
  }

  .tag-row :global(.btn) {
    flex: none;
  }

  /* slug | label (flex) | color (fixed) | submit */
  .add-row {
    display: grid;
    grid-template-columns: minmax(0, 5.5rem) minmax(0, 1fr) 5.5rem auto;
    gap: 0.5rem;
    align-items: stretch;
    min-width: 0;
  }

  .add-row :global(input) {
    box-sizing: border-box;
    width: 100%;
    min-width: 0;
    height: 2.25rem;
    margin: 0;
  }

  .add-row__slug {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-sm, 0.875rem);
  }

  .add-row__color {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
  }

  .add-row :global(.btn) {
    box-sizing: border-box;
    height: 2.25rem;
    margin: 0;
    padding-block: 0;
    display: inline-flex;
    align-items: center;
    align-self: stretch;
  }

  @media (max-width: 480px) {
    .add-row {
      grid-template-columns: minmax(0, 1fr) 5.5rem;
    }

    .add-row__slug {
      grid-column: 1 / -1;
    }

    .add-row :global(.btn) {
      grid-column: 1 / -1;
      justify-self: stretch;
      width: 100%;
    }
  }
</style>
