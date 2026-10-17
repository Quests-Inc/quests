<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'

  /**
   * Кнопки поля: режим выделения, вписать, во весь экран; у карты ещё распутать и очистить, у редактора скриптов — отмена и повтор.
   * Всё, чего у поля нет, просто не передаётся, и кнопки нет.
   */
  let {
    selectMode,
    pickedCount = 0,
    full,
    canClear = false,
    onHidePicked,
    onClearPicked,
    onToggleSelect,
    onFit,
    onRelayout,
    onToggleFull,
    onClear,
    onUndo,
    onRedo,
    canUndo = false,
    canRedo = false,
  }: {
    selectMode: boolean
    pickedCount?: number
    full: boolean
    canClear?: boolean
    onHidePicked?: () => void
    onClearPicked?: () => void
    onToggleSelect: () => void
    onFit: () => void
    onRelayout?: () => void
    onToggleFull: () => void
    onClear?: () => void
    onUndo?: () => void
    onRedo?: () => void
    canUndo?: boolean
    canRedo?: boolean
  } = $props()
</script>

  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="gtools"
    onpointerdown={(e) => e.stopPropagation()}
    onwheel={(e) => e.stopPropagation()}
    oncontextmenu={(e) => {
      e.preventDefault()
      e.stopPropagation()
    }}
  >
    {#if selectMode && pickedCount && onHidePicked && onClearPicked}
      <span class="gtools__count">Выделено: {pickedCount}</span>
      <button type="button" class="gtool" title="Убрать выделенное с карты" aria-label="Убрать выделенное с карты" onclick={onHidePicked}>
        <Icon name="eye-off" size={16} />
      </button>
      <button type="button" class="gtool" title="Снять выделение" aria-label="Снять выделение" onclick={onClearPicked}>
        <Icon name="close" size={16} />
      </button>
      <span class="gtools__sep" aria-hidden="true"></span>
    {/if}
    <button
      type="button"
      class="gtool"
      class:gtool--on={selectMode}
      aria-pressed={selectMode}
      title="Режим выделения (V): рамкой по полю или кликами по карточкам. Выделенные двигаются вместе. Esc — выйти"
      aria-label="Режим выделения"
      onclick={onToggleSelect}
    >
      <Icon name="select" size={16} />
    </button>
    <button type="button" class="gtool" title="Вписать карту в окно (F)" aria-label="Вписать карту в окно" onclick={onFit}>
      <Icon name="fit" size={16} />
    </button>
    {#if onRelayout}
      <button type="button" class="gtool" title="Распутать: пересчитать раскладку (R)" aria-label="Распутать" onclick={onRelayout}>
        <Icon name="arrange" size={16} />
      </button>
    {/if}
    <button
      type="button"
      class="gtool"
      class:gtool--on={full}
      aria-pressed={full}
      title={full ? 'Выйти из полноэкранного режима (Esc)' : 'Полноэкранный режим'}
      aria-label="Полноэкранный режим"
      onclick={onToggleFull}
    >
      <Icon name={full ? 'fullscreen-exit' : 'fullscreen'} size={16} />
    </button>
    {#if onClear}
    <button
      type="button"
      class="gtool"
      disabled={!canClear}
      title="Очистить карту (Shift+Delete): центр, добавленные центры и скрытое. Журнал изменений остаётся"
      aria-label="Очистить карту"
      onclick={onClear}
    >
      <Icon name="clear" size={16} />
    </button>
    {/if}
    {#if onUndo && onRedo}
      <span class="gtools__sep" aria-hidden="true"></span>
      <button type="button" class="gtool" disabled={!canUndo} title="Отменить (Ctrl+Z)" aria-label="Отменить" onclick={onUndo}>
        <Icon name="undo" size={16} />
      </button>
      <button type="button" class="gtool" disabled={!canRedo} title="Повторить (Ctrl+Shift+Z)" aria-label="Повторить" onclick={onRedo}>
        <Icon name="redo" size={16} />
      </button>
    {/if}
  </div>

<style>


  .gtools {
    position: absolute;
    z-index: 6;
    right: 0.6rem;
    bottom: 0.6rem;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    cursor: default;
  }

  .gtool {
    display: grid;
    place-items: center;
    width: 2rem;
    height: 2rem;
    padding: 0;
    border: 1px solid var(--color-border, #333);
    border-radius: 8px;
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .gtool:hover:not(:disabled) {
    color: var(--color-fg, #e8e8e8);
    border-color: var(--color-fg-subtle, #6e6e6e);
  }

  .gtool:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .gtool--on {
    color: #5aa9ff;
    border-color: #5aa9ff;
  }

  .gtools__count {
    margin-right: 0.15rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .gtools__sep {
    width: 1px;
    height: 1.2rem;
    margin: 0 0.1rem;
    background: var(--color-border, #333);
  }</style>
