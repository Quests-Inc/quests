<script>
  import { QUESTLINE_COLORS, QUESTLINE_ICONS } from '../js/api.ts'
  import Icon from './Icon.svelte'
  import QuestlineIcon from './QuestlineIcon.svelte'

  /**
   * Colour swatches + built-in icon grid + "upload your own" — shared by the
   * questline and note-icon modals. The upload itself happens on the
   * parent's save: it reads `pendingFile` (upload it) and `clearCustom`
   * (drop the current custom icon).
   * @type {{
   *   color?: string,
   *   icon?: string,
   *   iconUrl?: string | null,
   *   pendingFile?: File | null,
   *   clearCustom?: boolean,
   *   ownerLabel?: string,
   * }}
   */
  let {
    color = $bindable('#9a9a9a'),
    icon = $bindable('document'),
    iconUrl = null,
    pendingFile = $bindable(null),
    clearCustom = $bindable(false),
    ownerLabel = 'этого элемента',
  } = $props()

  let fileInput = $state(/** @type {HTMLInputElement | null} */ (null))
  let pendingPreview = $state(/** @type {string | null} */ (null))

  // Object URL for the picked file, revoked when it changes or goes away.
  $effect(() => {
    if (!pendingFile) {
      pendingPreview = null
      return
    }
    const url = URL.createObjectURL(pendingFile)
    pendingPreview = url
    return () => URL.revokeObjectURL(url)
  })

  let previewUrl = $derived(pendingPreview || (!clearCustom ? iconUrl : null))

  function pickBuiltin(name) {
    icon = name
    clearCustom = true
    pendingFile = null
  }

  function onFileChange(event) {
    const input = /** @type {HTMLInputElement} */ (event.currentTarget)
    const file = input.files?.[0] || null
    input.value = ''
    if (!file) return
    pendingFile = file
    clearCustom = false
  }

  function removeCustom() {
    pendingFile = null
    clearCustom = true
  }
</script>

<div class="field">
  <span class="label">Цвет</span>
  <div class="cip__row">
    <div class="cip__swatches" role="radiogroup" aria-label="Цвет">
      {#each QUESTLINE_COLORS as c}
        <button
          type="button"
          class="cip__swatch"
          class:cip__swatch--on={color === c}
          style="--swatch: {c}"
          role="radio"
          aria-checked={color === c}
          aria-label={c}
          onclick={() => (color = c)}
        ></button>
      {/each}
    </div>
    <input class="mono cip__hex" type="text" bind:value={color} maxlength="16" aria-label="Цвет, hex" />
  </div>
</div>

<div class="field">
  <span class="label">Иконка</span>
  <div class="cip__row">
    <div class="cip__icons" role="radiogroup" aria-label="Иконка">
      {#each QUESTLINE_ICONS as name}
        <button
          type="button"
          class="cip__icon"
          class:cip__icon--on={!previewUrl && icon === name}
          style="--line-color: {color}"
          role="radio"
          aria-checked={!previewUrl && icon === name}
          aria-label={name}
          title={name}
          onclick={() => pickBuiltin(name)}
        >
          <Icon {name} size={16} />
        </button>
      {/each}
      {#if previewUrl}
        <span class="cip__icon cip__icon--on cip__icon--custom" style="--line-color: {color}" title="Своя иконка">
          <QuestlineIcon iconUrl={previewUrl} size="md" />
        </span>
      {/if}
    </div>
    <div class="cip__custom">
      {#if previewUrl}
        <button type="button" class="btn btn--ghost" onclick={removeCustom}>Убрать свою</button>
      {/if}
      <button
        type="button"
        class="btn"
        title="Своя иконка только у {ownerLabel}, в общий набор не попадает"
        onclick={() => fileInput?.click()}
      >
        Загрузить…
      </button>
      <input
        bind:this={fileInput}
        class="cip__file"
        type="file"
        accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
        onchange={onFileChange}
      />
    </div>
  </div>
</div>

<style>
  .cip__row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem 0.75rem;
  }

  .cip__swatches,
  .cip__icons {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .cip__swatch {
    width: 1.4rem;
    height: 1.4rem;
    padding: 0;
    border: 2px solid transparent;
    border-radius: var(--radius-sm, 2px);
    background: var(--swatch);
    cursor: pointer;
  }

  .cip__swatch--on {
    border-color: var(--color-fg, #e8e8e8);
  }

  .cip__row .cip__hex {
    width: 7rem;
  }

  .cip__icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg-muted, #242424);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .cip__icon--on {
    border-color: var(--line-color, var(--color-accent, #c9a227));
    color: var(--line-color, var(--color-accent, #c9a227));
    background: color-mix(in srgb, var(--line-color, var(--color-accent, #c9a227)) 16%, transparent);
  }

  .cip__icon--custom {
    cursor: default;
  }

  .cip__custom {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .cip__file {
    position: absolute;
    width: 1px;
    height: 1px;
    opacity: 0;
    pointer-events: none;
  }
</style>
