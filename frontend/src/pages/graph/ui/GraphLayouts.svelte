<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import { ago } from '@/shared/lib/time.ts'
  import { sidebars, SIDEBAR_ONE_PANE } from '@/shared/lib/sidebar.svelte.ts'
  import type { createGraphLayouts } from '../model/layouts.svelte.ts'

  /**
   * Боковая панель карты: личные расклады. Сверху — сохранить текущую карту под именем; в списке клик открывает расклад,
   * у строки — записать в него текущую карту, переименовать, удалить.
   */
  let { layouts }: { layouts: ReturnType<typeof createGraphLayouts> } = $props()

  let title = $state('')
  let renaming = $state<number | null>(null)
  let renameText = $state('')
  let removing = $state<{ id: number; title: string } | null>(null)

  $effect(() => {
    if (!layouts.loaded) void layouts.load()
  })

  async function saveNew(e: SubmitEvent) {
    e.preventDefault()
    const t = title.trim()
    if (!t) return
    await layouts.saveNew(t)
    title = ''
  }

  async function open(id: number) {
    await layouts.open(id)
    // Телефон: список во весь экран закрывает карту, после выбора показываем её.
    try {
      if (window.matchMedia(SIDEBAR_ONE_PANE).matches) sidebars.setCollapsed('graph', true)
    } catch {
      /* без matchMedia панель остаётся как есть */
    }
  }

  function startRename(id: number, current: string) {
    renaming = id
    renameText = current
  }

  async function finishRename() {
    const id = renaming
    const t = renameText.trim()
    renaming = null
    if (id != null && t) await layouts.rename(id, t)
  }
</script>

<aside class="glay" aria-label="Расклады карты">
  <form class="glay__new" onsubmit={saveNew}>
    <input
      type="text"
      maxlength="120"
      placeholder="Название расклада"
      aria-label="Название нового расклада"
      title="Сохранить текущую карту — центр, фильтры, раскрытое и позиции карточек — под этим именем"
      bind:value={title}
    />
    <button type="submit" class="btn btn--icon-only" disabled={!title.trim() || layouts.busy} title="Сохранить текущую карту" aria-label="Сохранить текущую карту">
      <Icon name="save" size={16} />
    </button>
  </form>

  {#if !layouts.loaded}
    <p class="glay__empty">Загрузка…</p>
  {:else if !layouts.items.length}
    <p class="glay__empty">Раскладов пока нет. Настройте карту и сохраните её под именем: центр, фильтры, раскрытое и позиции карточек вернутся одним кликом.</p>
  {:else}
    <ul class="glay__list">
      {#each layouts.items as l (l.id)}
        <li class="glay__item" class:glay__item--on={l.id === layouts.current}>
          {#if renaming === l.id}
            <!-- svelte-ignore a11y_autofocus -->
            <input
              class="glay__rename"
              type="text"
              maxlength="120"
              aria-label="Новое название расклада"
              bind:value={renameText}
              autofocus
              onblur={() => void finishRename()}
              onkeydown={(e) => {
                if (e.key === 'Enter') void finishRename()
                if (e.key === 'Escape') renaming = null
              }}
            />
          {:else}
            <button type="button" class="glay__open" disabled={layouts.busy} onclick={() => void open(l.id)} title="Открыть расклад">
              <span class="glay__title">{l.title}</span>
              <small>изменён {ago(l.updated_at)}</small>
            </button>
            <span class="glay__acts">
              <button type="button" class="btn btn--ghost btn--icon" disabled={layouts.busy} title="Записать сюда текущую карту" aria-label="Записать сюда текущую карту" onclick={() => void layouts.overwrite(l.id)}>
                <Icon name="save" size={14} />
              </button>
              <button type="button" class="btn btn--ghost btn--icon" title="Переименовать" aria-label="Переименовать" onclick={() => startRename(l.id, l.title)}>
                <Icon name="edit" size={14} />
              </button>
              <button type="button" class="btn btn--ghost btn--icon" title="Удалить" aria-label="Удалить" onclick={() => (removing = { id: l.id, title: l.title })}>
                <Icon name="delete" size={14} />
              </button>
            </span>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</aside>

<ConfirmModal
  open={removing != null}
  title="Удалить расклад?"
  message={`«${removing?.title ?? ''}» исчезнет из списка. Сама карта и её данные не меняются.`}
  confirmLabel="Удалить"
  onCancel={() => (removing = null)}
  onConfirm={() => {
    const id = removing?.id
    removing = null
    if (id != null) void layouts.remove(id)
  }}
/>

<style>
  .glay {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }
  .glay__new {
    display: flex;
    gap: 0.3rem;
    padding: 0.5rem;
    border-bottom: 1px solid var(--color-border, #333);
  }
  .glay__new input {
    flex: 1 1 auto;
    min-width: 0;
  }
  .glay__empty {
    margin: 0.75rem;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
  .glay__list {
    flex: 1 1 auto;
    overflow: auto;
    margin: 0;
    padding: 0.25rem;
    list-style: none;
  }
  .glay__item {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    border-radius: var(--radius-md, 4px);
  }
  .glay__item:hover {
    background: var(--color-bg-muted, #1e1e1e);
  }
  .glay__item--on {
    box-shadow: inset 2px 0 0 var(--color-accent, #c9a227);
  }
  .glay__open {
    flex: 1 1 auto;
    min-width: 0;
    display: grid;
    gap: 0.1rem;
    padding: 0.4rem 0.5rem;
    border: 0;
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .glay__title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .glay__item--on .glay__title {
    color: var(--color-accent, #c9a227);
  }
  .glay__open small {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .glay__acts {
    display: flex;
    opacity: 0;
  }
  .glay__item:hover .glay__acts,
  .glay__item:focus-within .glay__acts,
  .glay__item--on .glay__acts {
    opacity: 1;
  }
  .glay__rename {
    flex: 1 1 auto;
    margin: 0.25rem;
  }
  @media (hover: none) {
    .glay__acts {
      opacity: 1;
    }
  }
</style>
