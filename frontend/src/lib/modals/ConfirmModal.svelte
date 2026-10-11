<script lang="ts">
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import ModalFoot from './ModalFoot.svelte'

  let {
    open = false,
    title = 'Подтверждение',
    message = '',
    confirmLabel = 'Удалить',
    cancelLabel = 'Отмена',
    busy = false,
    onConfirm = () => {},
    onCancel = () => {},
  }: { open?: boolean, title?: string, message?: string, confirmLabel?: string, cancelLabel?: string, busy?: boolean, onConfirm?: () => void, onCancel?: () => void } = $props()
</script>

<ModalShell {open} onClose={onCancel} labelledby="confirm-modal-title" zIndex={60} maxWidth="24rem" closeDisabled={busy}>
  <ModalHead id="confirm-modal-title" {title} onClose={onCancel} closeDisabled={busy} />
  <div class="modal__body">
    <p class="modal__text">{message}</p>
    <ModalFoot
      onCancel={onCancel}
      {cancelLabel}
      submitLabel={confirmLabel}
      submitIcon="delete"
      submitVariant="danger-solid"
      onSubmit={onConfirm}
      {busy}
    />
  </div>
</ModalShell>
