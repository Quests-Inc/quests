<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'

  /** Head of a left-hand list: the search field, the «create» action next to it, page-specific filters below (`children`). */
  let {
    value = $bindable(''),
    placeholder = 'Поиск…',
    label = 'Поиск',
    hint = undefined,
    createLabel = '',
    createBusy = false,
    onCreate = null,
    children,
  }: {
    value?: string,
    placeholder?: string,
    /** aria-label of the field. */
    label?: string,
    hint?: string,
    /** «Новый …»: title and aria-label of the + button; the button shows only with `onCreate`. */
    createLabel?: string,
    createBusy?: boolean,
    onCreate?: (() => void) | null,
    children?: Snippet,
  } = $props()
</script>

<div class="sbt">
  <div class="sbt__row">
    <input class="search" type="search" {placeholder} aria-label={label} title={hint} bind:value />
    {#if onCreate}
      <button type="button" class="btn btn--accent sbt__add" title={createLabel} aria-label={createLabel} disabled={createBusy} onclick={onCreate}>
        <Icon name="add" size={16} />
      </button>
    {/if}
  </div>
  {@render children?.()}
</div>

<style>
  .sbt {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    flex: none;
    padding: 0.55rem 0.6rem;
    border-bottom: 1px solid var(--color-border, #333);
  }
  .sbt__row {
    display: flex;
    gap: 0.4rem;
  }
  .sbt__row :global(.search) {
    flex: 1 1 auto;
    min-width: 0;
  }
  .sbt__add {
    flex: none;
    display: grid;
    place-items: center;
    min-width: 2.25rem;
    padding: 0;
  }
</style>
