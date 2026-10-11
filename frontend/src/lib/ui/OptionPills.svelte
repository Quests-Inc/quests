<script lang="ts">
  /**
   * A row of pill buttons for a short, fixed set of choices (status,
   * significance, frequency…). Single-choice by default (`bind:value`); pass
   * `selected` + `onToggle` for multi-choice (TOC filters, weekdays).
   * Layout lives in styles/modal.css, per-option colour in theme-accents.css.
   *
   * @typedef {{
   *   id: string,
   *   label: string,
   *   kind?: 'status' | 'sig' | 'cat',
   *   color?: string,
   *   none?: boolean,
   * }} PillOption
   */
  type PillOption = {
    id: string,
    label: string,
    kind?: 'status' | 'sig' | 'cat',
    color?: string,
    none?: boolean,
  }


  /**
   * @type {{
   *   options: PillOption[],
   *   value?: string,
   *   selected?: Set<string> | null,
   *   onToggle?: (id: string) => void,
   *   label?: string,
   *   wrap?: boolean,
   *   compact?: boolean,
   *   disabled?: boolean,
   * }}
   */
  let {
    options,
    value = $bindable(''),
    selected = null,
    onToggle,
    label = '',
    wrap = false,
    compact = false,
    disabled = false,
  }: {
    options: PillOption[],
    value?: string,
    selected?: Set<string> | null,
    onToggle?: (id: string) => void,
    label?: string,
    wrap?: boolean,
    compact?: boolean,
    disabled?: boolean,
  } = $props()

  let multi = $derived(selected != null)

  function choose(id: string) {
    if (!multi) value = id
    onToggle?.(id)
  }
</script>

<div
  class="opt-slider"
  class:opt-slider--wrap={wrap}
  class:opt-slider--compact={compact}
  class:opt-slider--locked={disabled}
  role={multi ? 'group' : 'radiogroup'}
  aria-label={label || undefined}
  aria-disabled={disabled || undefined}
>
  {#each options as opt (opt.id)}
    {@const on = multi ? selected.has(opt.id) : value === opt.id}
    <button
      type="button"
      class="opt-slider__opt"
      class:opt-slider__opt--on={on}
      class:opt-slider__opt--sig={opt.kind === 'sig'}
      class:opt-slider__opt--cat={opt.kind === 'cat'}
      class:opt-slider__opt--status={opt.kind === 'status'}
      data-sig={opt.kind === 'sig' ? opt.id : undefined}
      data-status={opt.kind === 'status' ? opt.id : undefined}
      data-cat={opt.none ? 'none' : undefined}
      style={opt.color ? `--opt-color: ${opt.color}` : undefined}
      role={multi ? undefined : 'radio'}
      aria-checked={multi ? undefined : on}
      aria-pressed={multi ? on : undefined}
      {disabled}
      onclick={() => choose(opt.id)}
    >
      {opt.label}
    </button>
  {/each}
</div>
