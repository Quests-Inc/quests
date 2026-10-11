<script lang="ts">
  import { TYPES } from '@/entities/graph'

  /** Подвал карты: что выбрано (с кнопками «Открыть» / «Сделать центром») или легенда видов узлов. */
  let {
    selected,
    target,
    canRecenter,
    onOpen,
    onRecenter,
  }: {
    selected: { type: string; title: string; links: number } | null
    target: [string, number] | null
    canRecenter: boolean
    onOpen: (kind: string, id: number) => void
    onRecenter: () => void
  } = $props()
</script>

<div class="graph__foot">
  {#if selected}
          <span class="graph__sel">
      <b>{TYPES[selected.type]?.label}</b>
      {selected.title}
      <span class="graph__meta">· ссылок: {selected.links}</span>
    </span>
    {#if target}
      <button type="button" class="graph__btn" onclick={() => onOpen(target[0], target[1])}>Открыть</button>
    {/if}
    {#if canRecenter}
      <button type="button" class="graph__btn" onclick={() => onRecenter()}>Сделать центром</button>
    {/if}
  {:else}
    <span class="graph__meta">
      Клик по карточке — подробности, ПКМ — действия. Пунктирная рамка у заметки или вложения — на неё никто не ссылается.
      <span class="graph__keys">
        {#each ['quest', 'step', 'note', 'attachment', 'questline', 'template'] as k (k)}
          <span class="graph__key" style:color={TYPES[k].color}>{TYPES[k].glyph} {TYPES[k].label}</span>
        {/each}
      </span>
    </span>
  {/if}
</div>

<style>


  .graph__foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem;
    min-height: 1.8rem;
    font-size: var(--text-sm, 0.875rem);
  }

  .graph__sel {
    flex: 1 1 auto;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .graph__meta {
    min-width: 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  /* Wide: in the line after the hint. Narrow: two rows of three. */
  .graph__keys {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 0 0.6rem;
    margin-left: 0.6rem;
  }

  .graph__key {
    white-space: nowrap;
  }@media (max-width: 720px) {

    .graph__keys {
      display: grid;
      grid-template-columns: repeat(3, auto);
      justify-content: start;
      gap: 0.15rem 0.9rem;
      margin: 0.3rem 0 0;
    }
}
</style>
