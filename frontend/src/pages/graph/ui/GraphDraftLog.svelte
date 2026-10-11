<script lang="ts">
  import { describeOp } from '@/entities/graph'
  import Icon from '@/shared/ui/Icon.svelte'

  /** Журнал черновика карты: список отложенных операций, «Сохранить» и «Отменить всё»; сворачивается в полоску. */
  let {
    ops,
    open = $bindable(true),
    committing,
    ownerlessCount,
    problems,
    flashOp,
    titleOf,
    onSave,
    onCancelAll,
    onDrop,
  }: {
    ops: { id: number; kind: string; payload: unknown }[]
    open?: boolean
    committing: boolean
    ownerlessCount: number
    problems: Record<number, string>
    flashOp: number | null
    titleOf: (key: string) => string
    onSave: () => void
    onCancelAll: () => void
    onDrop(op: { id: number }): void
  } = $props()
</script>

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="gdraft"
      class:gdraft--strip={!open}
      onpointerdown={(e) => e.stopPropagation()}
      onwheel={(e) => e.stopPropagation()}
      oncontextmenu={(e) => {
        e.preventDefault()
        e.stopPropagation()
      }}
    >
      <div class="gdraft__head">
        <button
          type="button"
          class="gdraft__toggle"
          aria-expanded={open}
          title={open ? 'Свернуть в полоску' : 'Развернуть журнал'}
          onclick={() => (open = !open)}
        >
          <Icon name={open ? 'chevron-down' : 'chevron-right'} size={12} />
          Журнал ({ops.length})
        </button>
        <span class="gdraft__btns">
          <button
            type="button"
            class="graph__btn graph__btn--primary"
            disabled={committing || ownerlessCount > 0}
            title={ownerlessCount ? 'Есть невалидные элементы (красная рамка)' : 'Применить все изменения (Ctrl+S)'}
            onclick={onSave}
          >
            {committing ? 'Сохраняю…' : 'Сохранить'}
          </button>
          <button type="button" class="graph__btn" onclick={onCancelAll}>Отменить всё</button>
        </span>
      </div>
      {#if open}
        <p class="gdraft__hint">
          {#if ownerlessCount}
            Нужно исправить: {ownerlessCount} (красная рамка) — шаг или файл без владельца, квест без шагов. Пока не исправлено, сохранить нельзя.
          {:else}
            В базе пока ничего нет — сохраните или отмените.
          {/if}
        </p>
        <ol class="gdraft__list">
          {#each ops as op (op.id)}
            <li class:gdraft__row--bad={problems[op.id]} class:gdraft__row--flash={flashOp === op.id} data-op={op.id}>
              <span class="gdraft__text">{describeOp(op, titleOf)}</span>
              {#if problems[op.id]}<span class="gdraft__msg">{problems[op.id]}</span>{/if}
              <button type="button" class="gdraft__x" title="Убрать из черновика" aria-label="Убрать из черновика" onclick={() => onDrop(op)}>
                <Icon name="close" size={12} />
              </button>
            </li>
          {/each}
        </ol>
      {/if}
    </div>

<style>


  /* The folded log is a capsule; its buttons are capsules too. */
  .gdraft--strip .graph__btn {
    border-radius: 999px;
    padding-inline: 0.8rem;
  }

  /* The change log floats over the top-right of the field; folded it is a strip. */
  .gdraft {
    position: absolute;
    z-index: 6;
    top: 0.6rem;
    right: 0.6rem;
    width: min(24rem, calc(100% - 1.2rem));
    max-height: calc(100% - 1.2rem);
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.5rem 0.6rem;
    border: 1px solid color-mix(in srgb, var(--color-accent, #c9a227) 60%, transparent);
    border-radius: 12px;
    background: color-mix(in srgb, var(--color-accent, #c9a227) 9%, var(--color-bg-raised, #1a1a1a));
    box-shadow: 0 4px 18px color-mix(in srgb, #000 45%, transparent);
    font-size: var(--text-sm, 0.875rem);
    cursor: default;
  }

  .gdraft--strip {
    width: auto;
    max-width: calc(100% - 1.2rem);
    padding: 0.3rem 0.5rem;
    border-radius: 999px;
  }

  .gdraft__head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .gdraft__btns {
    display: inline-flex;
    gap: 0.4rem;
  }

  .gdraft__toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    border: 0;
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }

  .gdraft__hint {
    margin: 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gdraft__list {
    list-style: none;
    margin: 0;
    padding: 0.2rem 0.4rem;
    min-height: 0;
    overflow: auto;
    border: 1px solid var(--color-border, #333);
    border-radius: 10px;
    background: color-mix(in srgb, #000 18%, transparent);
    font-size: var(--text-xs, 0.75rem);
  }

  .gdraft__list li {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.2rem 0.3rem;
  }

  .gdraft__text {
    flex: 1 1 auto;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .gdraft__msg {
    color: #e0443e;
  }

  .gdraft__row--bad {
    background: color-mix(in srgb, #e0443e 12%, transparent);
    border-radius: 6px;
  }

  .gdraft__x {
    flex: none;
    display: inline-grid;
    place-items: center;
    width: 1.3rem;
    height: 1.3rem;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .gdraft__x:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .gdraft__row--flash {
    background: color-mix(in srgb, #f2c230 28%, transparent);
    border-radius: 6px;
    outline: 1px solid #f2c230;
  }</style>
