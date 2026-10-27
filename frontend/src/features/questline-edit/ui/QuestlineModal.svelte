<script lang="ts">
  import {
    clearQuestlineIcon,
    createQuestline,
    deleteQuestline,
    listCategories,
    updateQuestline,
    uploadQuestlineIcon,
  } from '@/shared/api/client.ts'
  import Icon from '@/shared/ui/Icon.svelte'
  import { MentionTextarea } from '@/entities/template'
  import { ValueTable } from '@/entities/value'
  import Picker from '@/shared/ui/Picker.svelte'
  import { ColorIconPicker } from '@/entities/questline'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import { AttachmentsBlock } from '@/entities/attachment'
  import { untrack } from 'svelte'

  let {
    open = false,
    mode = 'create',
    line = null,
    quests = [],
    questlines = [],
    notes = [],
    attachments = [],
    onClose,
    onSaved,
    onDeleted,
  }: { open: boolean, mode: 'create' | 'edit', line?: any, quests?: any[], questlines?: any[], notes?: any[], attachments?: any[], onClose: () => void, onSaved: (line: any, meta?: { mode: string }) => void, onDeleted?: (id: number) => void } = $props()

  let title = $state('')
  let description = $state('')
  /** Empty string = no category. */
  let categoryId = $state('')
  let color = $state('#9a9a9a')
  let icon = $state('document')
  /** Current server custom icon URL (if any). */
  let iconUrl = $state((null as string | null))
  /** Pending local file for upload after save. */
  let pendingFile = $state((null as File | null))
  /** User chose to drop custom icon on save. */
  let clearCustom = $state(false)
  let categories: { id: number, slug: string, label: string, color?: string }[] = $state([])
  let saving = $state(false)
  let deleting = $state(false)
  let deleteConfirmOpen = $state(false)
  let formError = $state('')

  let categoryOptions = $derived(
    categories.map((c) => ({ id: String(c.id), label: c.label, color: c.color || '' })),
  )

  function resetFromLine(row: { title: string; description: string; category_id: null; color: string; icon: string; icon_url: null }) {
    pendingFile = null
    clearCustom = false
    title = row?.title ?? ''
    description = row?.description ?? ''
    categoryId = row?.category_id != null ? String(row.category_id) : ''
    color = row?.color || '#9a9a9a'
    icon = row?.icon || 'document'
    iconUrl = row?.icon_url || null
  }

  // Init when `open` becomes true — only track `open`.
  $effect(() => {
    if (!open) return
    untrack(() => {
      formError = ''
      saving = false
      deleting = false
      deleteConfirmOpen = false
      resetFromLine(mode === 'edit' ? line : null)
      listCategories()
        .then((rows) => {
          categories = Array.isArray(rows) ? rows : []
        })
        .catch(() => {
          categories = []
        })
    })
  })

  async function onSubmit(event: { preventDefault: () => void }) {
    event.preventDefault()
    if (!title.trim()) {
      formError = 'Нужен заголовок'
      return
    }
    saving = true
    formError = ''
    try {
      const payload = {
        title: title.trim(),
        description: description.trim(),
        category_id: categoryId === '' ? null : Number(categoryId),
        color: color || '#9a9a9a',
        icon: icon || 'document',
      }
      let saved =
        mode === 'create'
          ? await createQuestline(payload)
          : await updateQuestline(line.id, payload)

      if (pendingFile && saved?.id) {
        saved = await uploadQuestlineIcon(saved.id, pendingFile)
      } else if (clearCustom && mode === 'edit' && line?.id && iconUrl) {
        saved = await clearQuestlineIcon(line.id)
      }

      onSaved(saved, { mode })
      onClose()
    } catch (e) {
      formError = e.message || String(e)
    } finally {
      saving = false
    }
  }

  async function confirmDelete() {
    if (!line?.id || deleting) return
    deleting = true
    formError = ''
    try {
      await deleteQuestline(line.id)
      deleteConfirmOpen = false
      onDeleted?.(line.id)
      onClose()
    } catch (e) {
      deleteConfirmOpen = false
      formError = e.message || String(e)
    } finally {
      deleting = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="ql-modal-title" zIndex={50} maxWidth="32rem">
  <ModalHead
    id="ql-modal-title"
    title={mode === 'create' ? 'Новый квестлайн' : 'Редактировать квестлайн'}
    icon="flag"
    {onClose}
  />

  {#if formError}
    <p class="modal__error">{formError}</p>
  {/if}

  {#if mode === 'edit' && line?.id}
    <div class="modal__body">
      <ValueTable variant="inline" owner={{ kind: 'questline', id: line.id }} />
    </div>
  {/if}

  <form class="modal__form" onsubmit={onSubmit}>
    <label class="field">
      <span class="label">Заголовок</span>
      <input type="text" bind:value={title} required />
    </label>

    <div class="field">
      <span class="label">Описание</span>
      <MentionTextarea
        bind:value={description}
        {quests}
        {questlines}
        {notes}
        {attachments}
        rows={2}
        placeholder="@название — квест, заметка, файл, шаг, квестлайн"
      />
    </div>

    <div class="field">
      <span class="label">Раздел</span>
      <Picker options={categoryOptions} bind:value={categoryId} label="Раздел" />
    </div>

    <ColorIconPicker bind:color bind:icon {iconUrl} bind:pendingFile bind:clearCustom ownerLabel="этого квестлайна" />

    {#if mode === 'edit' && line?.id}
      <div class="field">
        <AttachmentsBlock ownerType="questline" ownerId={line.id} labelClass="label" />
      </div>
    {/if}

    <ModalFoot onCancel={onClose} submitLabel={mode === 'create' ? 'Создать' : 'Сохранить'} submitIcon="save" busy={saving} disabled={deleting}>
      {#snippet left()}
        {#if mode === 'edit'}
          <button
            type="button"
            class="btn btn--danger"
            onclick={() => (deleteConfirmOpen = true)}
            disabled={saving || deleting}
            title="Удалить квестлайн"
          >
            <Icon name="delete" size={14} />
            <span class="btn__text">Удалить</span>
          </button>
        {/if}
      {/snippet}
    </ModalFoot>
  </form>
</ModalShell>

<ConfirmModal
  open={deleteConfirmOpen}
  title="Удалить квестлайн?"
  message="Квесты останутся, но отвяжутся от линии."
  confirmLabel="Удалить"
  busy={deleting}
  onCancel={() => (deleteConfirmOpen = false)}
  onConfirm={confirmDelete}
/>
