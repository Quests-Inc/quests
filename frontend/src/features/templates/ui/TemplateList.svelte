<script lang="ts">
  import type { TemplateRead } from '@/shared/api/automation.gen'
  import Icon from '@/shared/ui/Icon.svelte'

  /** Список шаблонов в окне: строка с названием и сводкой, быстрые действия и переключатель «включён». */
  let {
    error = '',
    loading,
    templates,
    rowMeta,
    onCreate,
    onOpen,
    onMenu,
    onEmit,
    onCopy,
    onToggleEnabled,
  }: {
    error?: string
    loading: boolean
    templates: TemplateRead[]
    rowMeta(t: TemplateRead): string
    onCreate: () => void
    onOpen(t: TemplateRead): void
    onMenu(e: MouseEvent, t: TemplateRead): void
    onEmit(t: TemplateRead, e: Event): void
    onCopy(t: TemplateRead, e: Event): void
    onToggleEnabled(t: TemplateRead): void
  } = $props()
</script>

  <div class="modal__body">
    {#if error}
      <p class="modal__error">{error}</p>
    {/if}
    <div class="toolbar">
      <button type="button" class="btn btn--accent" onclick={onCreate}>
        <Icon name="add" size={14} />
        <span>Новый шаблон</span>
      </button>
    </div>
    {#if loading}
      <p class="hint">Загрузка…</p>
    {:else if templates.length === 0}
      <p class="hint">Пока нет шаблонов — создай дейлик или еженедельный.</p>
    {:else}
      <ul class="tpl-list">
        {#each templates as t (t.id)}
          <li class="tpl-row" class:tpl-row--off={!t.enabled} oncontextmenu={(e) => onMenu(e, t)}>
            <button type="button" class="tpl-row__main" onclick={() => onOpen(t)}>
              <span class="tpl-row__title">
                {t.title}
                {#if t.emit_pool_last_outcome === 'error'}
                  <span class="tpl-row__err" title="Команда пула стабильно падает — все попытки за период исчерпаны">
                    ⚠ пул падает
                  </span>
                {/if}
              </span>
              <span class="tpl-row__meta">{rowMeta(t)}</span>
            </button>
            <div class="tpl-row__actions">
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                onclick={(e) => onEmit(t, e)}
                title="Эмитировать сейчас"
                aria-label="Эмитировать сейчас"
              >
                <Icon name="renew" size={14} />
              </button>
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                onclick={(e) => onCopy(t, e)}
                title="Копировать (копия будет выключена)"
                aria-label="Копировать шаблон"
              >
                <Icon name="copy" size={14} />
              </button>
              <button
                type="button"
                class="switch"
                class:switch--on={t.enabled}
                role="switch"
                aria-checked={t.enabled}
                onclick={() => onToggleEnabled(t)}
                title={t.enabled ? 'Выключить' : 'Включить'}
                aria-label={t.enabled ? 'Выключить шаблон' : 'Включить шаблон'}
              >
                <span class="switch__knob"></span>
              </button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

<style>

  .toolbar {
    display: flex;
    justify-content: flex-end;
  }

  .tpl-list {
    display: grid;
    gap: 0.35rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .tpl-row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding-right: 0.55rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-bg, #121212) 70%, transparent);
  }

  .tpl-row--off {
    opacity: 0.55;
  }

  .tpl-row__main {
    display: grid;
    flex: 1;
    gap: 0.15rem;
    min-width: 0;
    padding: 0.55rem 0.75rem;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .tpl-row__title {
    font-weight: 600;
  }

  .tpl-row__err {
    margin-left: 0.4em;
    padding: 0.05em 0.4em;
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 12%, transparent);
    color: var(--color-danger, #b54a3a);
    font-size: var(--text-xs, 0.75rem);
    font-weight: 500;
  }

  .tpl-row__meta {
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .tpl-row__actions {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    flex-shrink: 0;
  }

  .switch {
    position: relative;
    flex-shrink: 0;
    width: 2.25rem;
    height: 1.25rem;
    margin: 0;
    padding: 0;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: 999px;
    background: var(--color-bg-muted, #242424);
    cursor: pointer;
    transition:
      background 0.15s ease,
      border-color 0.15s ease;
  }

  .switch__knob {
    position: absolute;
    top: 1px;
    left: 1px;
    width: calc(1.25rem - 4px);
    height: calc(1.25rem - 4px);
    border-radius: 999px;
    background: var(--color-fg-muted, #9a9a9a);
    transition:
      transform 0.15s ease,
      background 0.15s ease;
  }

  .switch--on {
    border-color: color-mix(in srgb, var(--color-accent, #c9a227) 55%, var(--color-border, #333));
    background: color-mix(in srgb, var(--color-accent, #c9a227) 28%, var(--color-bg-muted, #242424));
  }

  .switch--on .switch__knob {
    transform: translateX(1rem);
    background: var(--color-accent, #c9a227);
  }</style>
