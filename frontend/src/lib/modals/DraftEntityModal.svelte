<script>
  import { untrack } from 'svelte'
  import MentionTextarea from '../ui/MentionTextarea.svelte'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import ModalFoot from './ModalFoot.svelte'

  /** Title + description for a note or step created on the map; goes into the
   * draft, nothing is saved yet.
   * @type {{
   *   open: boolean,
   *   kind: 'note' | 'step' | 'questline',
   *   initial?: { title: string, description: string } | null,
   *   heading?: string,
   *   quests?: any[],
   *   notes?: any[],
   *   onClose: () => void,
   *   onSubmit: (fields: { title: string, description: string }) => void,
   * }} */
  let { open = false, kind = 'note', initial = null, heading = '', quests = [], notes = [], onClose, onSubmit } = $props()

  let title = $state('')
  let description = $state('')
  let error = $state('')

  $effect(() => {
    if (!open) return
    untrack(() => {
      title = initial?.title ?? ''
      description = initial?.description ?? ''
      error = ''
    })
  })

  function submit(event) {
    event.preventDefault()
    if (!title.trim()) {
      error = kind === 'step' ? 'Нужно название шага' : 'Нужен заголовок'
      return
    }
    onSubmit({ title: title.trim(), description: description.trim() })
    onClose()
  }
</script>

<ModalShell {open} {onClose} labelledby="draft-entity-title" zIndex={46} maxWidth="32rem">
  <ModalHead
    id="draft-entity-title"
    title={heading || (kind === 'note' ? 'Новая заметка' : kind === 'questline' ? 'Новый квестлайн' : 'Новый шаг')}
    icon="add"
    {onClose}
  />
  {#if error}
    <p class="modal__error">{error}</p>
  {/if}
  <form class="modal__form" onsubmit={submit}>
    <label class="field">
      <span class="label">{kind === 'step' ? 'Название' : 'Заголовок'}</span>
      <input type="text" bind:value={title} />
    </label>
    <div class="field">
      <span class="label">Описание</span>
      <MentionTextarea
        bind:value={description}
        {quests}
        {notes}
        questlines={[]}
        attachments={[]}
        rows={kind === 'note' ? 6 : 3}
        placeholder="@название — квест, заметка, файл, шаг, квестлайн"
      />
    </div>
    <p class="hint">Попадёт в черновик карты: пока не нажато «Сохранить», в базе ничего не появится.</p>
    <ModalFoot onCancel={onClose} submitLabel="В черновик" submitIcon="add" />
  </form>
</ModalShell>
