<script lang="ts">
  import { dismissToast, holdToast, releaseToast, runToastAction, toasts } from '@/shared/lib/toasts.svelte.ts'
</script>

{#if toasts.length}
  <div class="toasts" aria-live="polite" aria-relevant="additions text">
    {#each toasts as t (t.id)}
      <div
        class="toast"
        data-kind={t.kind}
        role={t.kind === 'error' ? 'alert' : 'status'}
        onmouseenter={() => holdToast(t.id)}
        onmouseleave={() => releaseToast(t.id)}
        onfocusin={() => holdToast(t.id)}
        onfocusout={() => releaseToast(t.id)}
      >
        {#if t.kind === 'progress'}
          <span class="toast__pulse" aria-hidden="true"></span>
        {/if}
        <span class="toast__msg">{t.message}</span>
        {#if t.action}
          <button type="button" class="toast__act" onclick={() => runToastAction(t.id)}>{t.action.label}</button>
        {/if}
        <button
          type="button"
          class="toast__x"
          aria-label="Закрыть"
          onclick={() => dismissToast(t.id)}
        >
          ×
        </button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toasts {
    position: fixed;
    right: var(--space-4, 1rem);
    bottom: var(--space-4, 1rem);
    z-index: 90;
    display: flex;
    flex-direction: column-reverse;
    align-items: flex-end;
    gap: 0.4rem;
    max-width: min(22rem, calc(100vw - 2rem));
    pointer-events: none;
  }

  .toast {
    pointer-events: auto;
    display: flex;
    align-items: baseline;
    gap: 0.55rem;
    max-width: 100%;
    padding: 0.45rem 0.55rem 0.45rem 0.7rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-sm, 2px);
    background: color-mix(in srgb, var(--color-bg-raised, #1a1a1a) 92%, transparent);
    box-shadow: 0 8px 24px color-mix(in srgb, #000 35%, transparent);
    color: var(--color-fg, #e8e8e8);
    font-family: var(--font-ui, sans-serif);
    font-size: 0.78rem;
    letter-spacing: 0.01em;
    line-height: 1.35;
    animation: toast-in 160ms ease-out;
  }

  .toast[data-kind='success'] {
    border-color: color-mix(in srgb, var(--color-sig-uncommon, #8ec07c) 45%, var(--color-border, #333));
  }

  .toast[data-kind='error'] {
    border-color: color-mix(in srgb, var(--color-danger, #b54a3a) 50%, var(--color-border, #333));
    color: color-mix(in srgb, var(--color-danger, #b54a3a) 70%, var(--color-fg, #e8e8e8));
  }

  .toast[data-kind='progress'] {
    border-color: color-mix(in srgb, var(--color-accent, #c9a227) 40%, var(--color-border, #333));
  }

  .toast__msg {
    flex: 1 1 auto;
    min-width: 0;
  }

  /* The button of a toast ("Отменить"): text in the accent colour, no frame; it is the only thing in a toast to press. */
  .toast__act {
    flex: 0 0 auto;
    margin: 0;
    padding: 0 0.1rem;
    border: 0;
    background: transparent;
    color: var(--color-accent, #c9a227);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
    text-decoration: underline;
    text-underline-offset: 0.15em;
  }

  .toast__act:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .toast__act:focus-visible {
    outline: 1px solid var(--color-accent, #c9a227);
    outline-offset: 2px;
  }

  .toast__x {
    flex: 0 0 auto;
    border: 0;
    margin: 0;
    padding: 0 0.15rem;
    background: transparent;
    color: var(--color-fg-subtle, #6e6e6e);
    font: inherit;
    font-size: 1rem;
    line-height: 1;
    cursor: pointer;
  }

  .toast__x:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .toast__pulse {
    flex: 0 0 auto;
    width: 0.4rem;
    height: 0.4rem;
    border-radius: 50%;
    background: var(--color-accent, #c9a227);
    align-self: center;
    animation: toast-pulse 1.1s ease-in-out infinite;
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(0.35rem);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  @keyframes toast-pulse {
    0%,
    100% {
      opacity: 0.35;
    }
    50% {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .toast {
      animation: none;
    }
    .toast__pulse {
      animation: none;
      opacity: 0.85;
    }
  }
</style>
