<script lang="ts">
  import { untrack } from 'svelte'
  import Icon from './Icon.svelte'
  import { nearestVote, pickSuggestion } from '../js/suggest.ts'

  /**
   * "похоже на: ● Quests — MVP TODO" under a questline/section picker.
   * Never picks anything by itself: one click accepts, ✕ hides it for the
   * rest of this form. Waits for 3+ title characters and a 300 ms pause in
   * typing; once shown it holds on through small wobbles in confidence.
   * @type {{
   *   index: ReturnType<typeof import('../js/suggest.ts').buildSuggestIndex> | null,
   *   title: string,
   *   description?: string,
   *   target: 'questline' | 'category',
   *   options: { id: string, label: string, color?: string }[],
   *   enabled?: boolean,
   *   onAccept: (id: string) => void,
   * }}
   */
  let { index, title, description = '', target, options, enabled = true, onAccept }: {
    index: ReturnType<typeof import('../js/suggest.ts').buildSuggestIndex> | null,
    title: string,
    description?: string,
    target: 'questline' | 'category',
    options: { id: string, label: string, color?: string }[],
    enabled?: boolean,
    onAccept: (id: string) => void,
  } = $props()

  const DEBOUNCE_MS = 300
  const MIN_TITLE = 3

  let query = $state({ title: '', description: '' })
  let shownId = $state((null as number | null))
  let dismissed = $state(false)

  $effect(() => {
    const next = { title, description }
    const t = setTimeout(() => (query = next), DEBOUNCE_MS)
    return () => clearTimeout(t)
  })

  $effect(() => {
    if (!enabled || dismissed || !index || query.title.trim().length < MIN_TITLE) {
      shownId = null
      return
    }
    const prev = untrack(() => shownId)
    shownId = pickSuggestion(nearestVote(index, query, target), prev)
  })

  let option = $derived(shownId == null ? null : options.find((o) => o.id === String(shownId)) ?? null)
</script>

{#if option && enabled && !dismissed}
  <div class="suggest">
    <span class="suggest__lead">похоже на</span>
    <button
      type="button"
      class="suggest__pick"
      title="Выбрать «{option.label}»"
      onclick={() => onAccept(option.id)}
    >
      <span class="suggest__dot" style:--dot={option.color || null} class:suggest__dot--none={!option.color}></span>
      {option.label}
    </button>
    <button
      type="button"
      class="suggest__dismiss"
      aria-label="Скрыть подсказку"
      title="Скрыть подсказку"
      onclick={() => (dismissed = true)}
    >
      <Icon name="close" size={10} />
    </button>
  </div>
{/if}

<style>
  .suggest {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem;
    font-family: var(--font-ui, sans-serif);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-subtle, #6e6e6e);
    animation: suggest-in 0.15s ease-out;
  }

  .suggest__pick {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    max-width: 100%;
    padding: 0.15rem 0.5rem;
    border: 1px dashed var(--color-border-strong, #4a4a4a);
    border-radius: 999px;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font: inherit;
    cursor: pointer;
  }

  .suggest__pick:hover,
  .suggest__pick:focus-visible {
    border-style: solid;
    border-color: var(--color-accent, #c9a227);
    color: var(--color-fg, #e8e8e8);
  }

  .suggest__dot {
    flex-shrink: 0;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--dot);
  }

  .suggest__dot--none {
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--color-fg-subtle, #6e6e6e);
  }

  .suggest__dismiss {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: var(--color-fg-subtle, #6e6e6e);
    cursor: pointer;
  }

  .suggest__dismiss:hover {
    background: var(--color-bg-hover, #2a2a2a);
    color: var(--color-fg, #e8e8e8);
  }

  @keyframes suggest-in {
    from {
      opacity: 0;
      transform: translateY(-2px);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .suggest {
      animation: none;
    }
  }
</style>
