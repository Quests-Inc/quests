<script lang="ts">
  import Icon from '../ui/Icon.svelte'

  /** Dialog title bar: icon + title on the left, close (and optional extra
   * `actions`) on the right. `id` is what ModalShell's `labelledby` points at.
   * @type {{
   *   id: string,
   *   title: string,
   *   icon?: string,
   *   onClose: () => void,
   *   closeDisabled?: boolean,
   *   actions?: import('svelte').Snippet,
   * }} */
  let { id, title, icon = '', onClose, closeDisabled = false, actions }: {
    id: string,
    title: string,
    icon?: string,
    onClose: () => void,
    closeDisabled?: boolean,
    actions?: import('svelte').Snippet,
  } = $props()
</script>

<header class="modal__head">
  <h2 {id} class="modal__title">
    {#if icon}<Icon name={icon} size={18} />{/if}
    <span>{title}</span>
  </h2>
  <div class="modal__head-actions">
    {@render actions?.()}
    <button
      type="button"
      class="btn btn--ghost btn--icon"
      onclick={onClose}
      disabled={closeDisabled}
      aria-label="Закрыть"
      title="Закрыть"
    >
      <Icon name="close" size={14} />
    </button>
  </div>
</header>
