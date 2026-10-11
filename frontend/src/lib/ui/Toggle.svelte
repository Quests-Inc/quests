<script lang="ts">
  /** On/off switch with a label (a real checkbox underneath, so keyboard and screen readers work).
   * @type {{ checked?: boolean, label: string, disabled?: boolean, hint?: string }} */
  let { checked = $bindable(false), label, disabled = false, hint = '' }: { checked?: boolean, label: string, disabled?: boolean, hint?: string } = $props()
</script>

<label class="toggle" class:toggle--disabled={disabled}>
  <input type="checkbox" role="switch" bind:checked {disabled} />
  <span class="toggle__track" aria-hidden="true"><span class="toggle__thumb"></span></span>
  <span class="toggle__text">
    {label}
    {#if hint}<small>{hint}</small>{/if}
  </span>
</label>

<style>
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    cursor: pointer;
    font-family: var(--font-ui);
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg);
    user-select: none;
  }
  .toggle--disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  input {
    position: absolute;
    opacity: 0;
    width: 1px;
    height: 1px;
  }
  .toggle__track {
    position: relative;
    flex: none;
    width: 2.4rem;
    height: 1.35rem;
    border-radius: 999px;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    background: var(--color-bg-muted, #242424);
    transition: background 0.15s, border-color 0.15s;
  }
  .toggle__thumb {
    position: absolute;
    top: 50%;
    left: 0.15rem;
    width: 0.95rem;
    height: 0.95rem;
    border-radius: 50%;
    background: var(--color-fg-muted, #9a9a9a);
    transform: translateY(-50%);
    transition: left 0.15s, background 0.15s;
  }
  input:checked + .toggle__track {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 30%, var(--color-bg-muted, #242424));
    border-color: var(--color-accent, #c9a227);
  }
  input:checked + .toggle__track .toggle__thumb {
    left: calc(100% - 0.95rem - 0.15rem);
    background: var(--color-accent, #c9a227);
  }
  input:focus-visible + .toggle__track {
    outline: 2px solid var(--color-accent, #c9a227);
    outline-offset: 2px;
  }
  .toggle__text small {
    display: block;
    font-size: 0.75rem;
    color: var(--color-fg-muted, #9a9a9a);
  }
</style>
