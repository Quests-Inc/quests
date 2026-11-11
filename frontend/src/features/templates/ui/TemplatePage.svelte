<script lang="ts">
  import type { Snippet } from 'svelte'
  import PageLayout from '@/shared/ui/PageLayout.svelte'
  let { title, onBack, busy = false, children }: { title: string; onBack?: () => void; busy?: boolean; children: Snippet } = $props()
</script>
<PageLayout {title} description={onBack ? '' : 'Повторяющиеся квесты: расписание, состояние и выпуск.'}>
  {#snippet toolbar()}
    {#if onBack}<button type="button" class="btn btn--ghost" onclick={onBack} disabled={busy}>← Назад к шаблонам</button>{/if}
  {/snippet}
  <div class="templates-page formscope" class:templates-page--editing={Boolean(onBack)}>{@render children()}</div>
</PageLayout>
<style>
  .templates-page { padding:1rem; min-width:0; }
  .templates-page--editing { max-width:56rem; }
  .templates-page :global(.modal__body), .templates-page :global(.modal__form) { padding-inline:0; min-width:0; grid-template-columns:minmax(0,1fr); }
  .templates-page :global(.modal__foot) { margin-inline:0; padding-inline:0; }
  @container(max-width:40rem) { .templates-page :global(.field-row) { grid-template-columns:minmax(0,1fr); } .templates-page :global(.modal__foot) { flex-wrap:wrap; } }
  @media(max-width:40rem) { .templates-page { padding:0.75rem; } }
</style>
