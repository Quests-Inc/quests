<script lang="ts">
  import { tick } from 'svelte'
  import { placePopover, watchPopover } from '../js/popover.ts'

  /**
   * A small "?" that opens an explanation on click — for the paragraphs of
   * how-it-works text that used to sit permanently under form fields.
   */
  let { label = 'Подробнее', children }: { label?: string, children?: import('svelte').Snippet } = $props()

  let open = $state(false)
  let btnEl = $state((null as HTMLButtonElement | null))
  let popEl = $state((null as HTMLDivElement | null))

  function place() {
    if (open && btnEl && popEl) placePopover(btnEl, popEl)
  }

  async function toggle() {
    open = !open
    if (!open) return
    await tick()
    place()
  }

  function onKey(event: { key: string; preventDefault: () => void; stopPropagation: () => void }) {
    if (open && event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      open = false
    }
  }

  $effect(() => {
    if (!open) return
    return watchPopover(() => [popEl, btnEl], () => (open = false), place)
  })
</script>

<button
  bind:this={btnEl}
  type="button"
  class="helptip"
  class:helptip--open={open}
  aria-label={label}
  aria-expanded={open}
  title={label}
  data-own-escape={open || undefined}
  onclick={toggle}
  onkeydown={onKey}
>?</button>

{#if open}
  <div bind:this={popEl} class="helptip__pop" role="note">
    {@render children?.()}
  </div>
{/if}

<style>
  .helptip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    box-sizing: border-box;
    width: 16px;
    height: 16px;
    padding: 0;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: 50%;
    background: transparent;
    color: var(--color-fg-subtle, #6e6e6e);
    font-family: var(--font-ui, sans-serif);
    font-size: 10px;
    font-weight: 700;
    line-height: 1;
    text-transform: none;
    letter-spacing: 0;
    cursor: pointer;
  }

  .helptip:hover,
  .helptip--open {
    border-color: var(--color-accent, #c9a227);
    color: var(--color-accent, #c9a227);
  }

  .helptip__pop {
    position: fixed;
    z-index: 200;
    box-sizing: border-box;
    width: max-content;
    max-width: min(22rem, calc(100vw - 16px));
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: var(--radius-md, 4px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 10px 28px color-mix(in srgb, #000 45%, transparent);
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-xs, 0.75rem);
    font-weight: 400;
    line-height: 1.45;
    letter-spacing: 0;
    text-transform: none;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .helptip__pop :global(p) {
    margin: 0 0 0.4rem;
  }

  .helptip__pop :global(p:last-child) {
    margin-bottom: 0;
  }

  .helptip__pop :global(code) {
    font-family: var(--font-mono, monospace);
    color: var(--color-fg, #e8e8e8);
  }
</style>
