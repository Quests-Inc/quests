<script lang="ts">
  import { clearNoteIcon, updateNote, uploadNoteIcon } from '../js/api.ts'
  import ColorIconPicker from '../ui/ColorIconPicker.svelte'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import ModalFoot from './ModalFoot.svelte'
  import { untrack } from 'svelte'

  /** @type {{ open: boolean, note?: any | null, onClose: () => void, onSaved: (row: any) => void }} */
  let { open = false, note = null, onClose, onSaved }: { open: boolean, note?: any | null, onClose: () => void, onSaved: (row: any) => void } = $props()

  let color = $state('#9a9a9a')
  let icon = $state('document')
  let iconUrl = $state((null as string | null))
  let pendingFile = $state((null as File | null))
  let clearCustom = $state(false)
  let saving = $state(false)
  let formError = $state('')

  let title = $derived(note?.title?.trim() ? note.title : `note=${note?.id ?? ''}`)

  $effect(() => {
    if (!open) return
    untrack(() => {
      formError = ''
      saving = false
      pendingFile = null
      clearCustom = false
      color = note?.color || '#9a9a9a'
      icon = note?.icon || 'document'
      iconUrl = note?.icon_url || null
    })
  })

  async function onSubmit(event: { preventDefault: () => void }) {
    event.preventDefault()
    if (!note?.id || saving) return
    saving = true
    formError = ''
    try {
      let saved = await updateNote(note.id, {
        color: color || '#9a9a9a',
        icon: icon || 'document',
      })
      if (pendingFile) {
        saved = await uploadNoteIcon(note.id, pendingFile)
      } else if (clearCustom && iconUrl) {
        saved = await clearNoteIcon(note.id)
      }
      onSaved(saved)
      onClose()
    } catch (e) {
      formError = e.message || String(e)
    } finally {
      saving = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="note-icon-title" zIndex={50} maxWidth="32rem">
  <ModalHead id="note-icon-title" title="Иконка — {title}" icon="document" {onClose} />
  {#if formError}
    <p class="modal__error">{formError}</p>
  {/if}
  <form class="modal__form" onsubmit={onSubmit}>
    <ColorIconPicker bind:color bind:icon {iconUrl} bind:pendingFile bind:clearCustom ownerLabel="этой заметки" />
    <ModalFoot onCancel={onClose} submitLabel="Сохранить" submitIcon="save" busy={saving} />
  </form>
</ModalShell>
