<script lang="ts">
  /**
   * Shared backdrop + dialog + Escape-close wrapper used by every modal.
   * The dialog's own look, and the chrome/form primitives inside it, live in
   * styles/modal.css (scoped under .modal) rather than being copied into
   * each modal.
   */
  let {
    open = false,
    onClose,
    labelledby,
    zIndex = 50,
    maxWidth = '32rem',
    closeDisabled = false,
    dialogClass = '',
    children,
  }: {
    open: boolean,
    onClose: () => void,
    labelledby: string,
    zIndex?: number,
    maxWidth?: string,
    closeDisabled?: boolean,
    dialogClass?: string,
    children?: import('svelte').Snippet,
  } = $props()

  $effect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      // A popover inside the dialog (picker list, help tip) closes itself on
      // Escape; this capture-phase listener runs first, so it has to step
      // aside or one keypress would take the whole dialog down with it.
      if (event.target instanceof Element && event.target.closest('[data-own-escape]')) return
      event.preventDefault()
      event.stopPropagation()
      if (!closeDisabled) onClose()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  })

  function onBackdrop(event: { target: any; currentTarget: any }) {
    if (closeDisabled) return
    if (event.target === event.currentTarget) onClose()
  }
</script>

{#if open}
  <div class="backdrop" role="presentation" onclick={onBackdrop} style:z-index={zIndex}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="modal {dialogClass}"
      role="dialog"
      aria-modal="true"
      aria-labelledby={labelledby}
      tabindex="-1"
      style:max-width={maxWidth}
      onclick={(e) => e.stopPropagation()}
    >
      {@render children?.()}
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    padding: var(--space-4, 1rem);
    background: color-mix(in srgb, var(--color-bg, #121212) 55%, transparent);
    backdrop-filter: blur(2px);
  }
</style>
