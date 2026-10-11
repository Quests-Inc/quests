<script lang="ts">
  import Icon from './Icon.svelte'

  /**
   * The one dialog footer: secondary/destructive actions on the left
   * (`left` snippet — Удалить, Копировать…), Cancel + the primary action on
   * the right, both always labelled. Without `onSubmit` the primary button is
   * type=submit for the surrounding <form>.
   */
  let {
    onCancel,
    cancelLabel = 'Отмена',
    submitLabel,
    submitIcon = '',
    submitVariant = 'accent',
    onSubmit = undefined,
    busy = false,
    busyLabel = '…',
    disabled = false,
    left,
  }: {
    onCancel: () => void,
    cancelLabel?: string,
    submitLabel: string,
    submitIcon?: string,
    submitVariant?: 'accent' | 'danger-solid',
    onSubmit?: () => void,
    busy?: boolean,
    busyLabel?: string,
    disabled?: boolean,
    left?: import('svelte').Snippet,
  } = $props()
</script>

<footer class="modal__foot">
  <div class="modal__foot-left">
    {@render left?.()}
  </div>
  <div class="modal__foot-right">
    <button type="button" class="btn btn--ghost" onclick={onCancel} disabled={busy}>
      {cancelLabel}
    </button>
    <button
      type={onSubmit ? 'button' : 'submit'}
      class="btn btn--{submitVariant}"
      onclick={onSubmit}
      disabled={busy || disabled}
    >
      {#if submitIcon && !busy}<Icon name={submitIcon} size={14} />{/if}
      <span>{busy ? busyLabel : submitLabel}</span>
    </button>
  </div>
</footer>
