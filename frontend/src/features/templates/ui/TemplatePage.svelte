<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  let { title, onBack, busy = false, children }: { title: string; onBack?: () => void; busy?: boolean; children: Snippet } = $props()
</script>

<section class="templates-page formscope" class:templates-page--editing={Boolean(onBack)} aria-labelledby="templates-modal-title">
  <header class="templates-page__head">
    {#if onBack}
      <button type="button" class="btn btn--ghost" onclick={onBack} disabled={busy}><span aria-hidden="true">←</span> Назад к шаблонам</button>
    {/if}
    <h2 id="templates-modal-title"><Icon name="repeat" size={20} />{title}</h2>
    {#if !onBack}<p class="hint">Повторяющиеся квесты: расписание, состояние и выпуск.</p>{/if}
  </header>
  {@render children()}
</section>

<style>
  .templates-page { container-type: inline-size; width: 100%; max-width: 76rem; min-width: 0; margin: 0 auto; padding: var(--space-4, 1rem); font-family: var(--font-ui); }
  .templates-page--editing { max-width: 56rem; }
  .templates-page__head { display: grid; gap: 0.5rem; padding: 0 0 1rem; border-bottom: 1px solid var(--color-border); }
  .templates-page__head > button { justify-self: start; }
  .templates-page__head h2 { display: flex; align-items: center; gap: 0.5rem; margin: 0; overflow-wrap: anywhere; font-family: var(--font-display); color: var(--color-accent); font-size: var(--text-lg); }
  .templates-page__head p { margin: 0; }
  .templates-page :global(.modal__body), .templates-page :global(.modal__form) { padding-inline: 0; min-width: 0; grid-template-columns: minmax(0, 1fr); }
  .templates-page :global(.modal__foot) { margin-inline: 0; padding-inline: 0; }
  @container (max-width: 40rem) {
    .templates-page :global(.field-row) { grid-template-columns: minmax(0, 1fr); }
    .templates-page :global(.modal__foot) { flex-wrap: wrap; }
    .templates-page :global(.modal__foot-right) { margin-left: auto; }
  }
  @media (max-width: 40rem) { .templates-page { padding: 0.75rem; } }
</style>
