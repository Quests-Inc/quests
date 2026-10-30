<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import SidebarTools from '@/shared/ui/SidebarTools.svelte'
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

  async function saveNew(e?: SubmitEvent) {
    e?.preventDefault()
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
  <form onsubmit={saveNew}>
    <SidebarTools
      bind:value={title}
      placeholder="Название расклада"
      label="Название нового расклада"
      hint="Сохранить текущую карту — центр, фильтры, раскрытое и позиции карточек — под этим именем"
      createLabel="Сохранить текущую карту"
      createBusy={!title.trim() || !layouts.canSave}
      onCreate={() => void saveNew()}
    />
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
              <button type="button" class="btn btn--ghost btn--icon" disabled={!layouts.canSave} title="Записать сюда текущую карту" aria-label="Записать сюда текущую карту" onclick={() => void layouts.overwrite(l.id)}>
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

<ModalShell open={layouts.overwriteAsk != null} onClose={layouts.cancelOverwrite} labelledby="overwrite-layout-title" maxWidth="38rem" closeDisabled={layouts.busy}>
  <ModalHead id="overwrite-layout-title" title="Перезаписать расклад?" onClose={layouts.cancelOverwrite} closeDisabled={layouts.busy} />
  <div class="modal__body">
    <p>В «{layouts.overwriteAsk?.title}» будет записана текущая карта: настройки деревьев, позиции, фильтры и зоны.</p>
    {#if layouts.overwriteAsk?.legacy}<p class="glay__empty">Для старого расклада состав восстановлен по сохранённым позициям и корням. Названия некоторых карточек могут быть недоступны.</p>{/if}
    <div class="glay__diff">
      {#each [['added', 'Добавлено'], ['removed', 'Убрано']] as [key, label] (key)}
        {@const rows = key === 'added' ? layouts.overwriteAsk?.diff.added ?? [] : layouts.overwriteAsk?.diff.removed ?? []}
        <section><h3>{label}: {rows.length}</h3>
          {#if rows.length}<ul>{#each rows as entity (entity.key)}<li>{entity.title}{#if entity.title !== entity.key} <small>{entity.key}</small>{/if}</li>{/each}</ul>{:else}<p>Нет</p>{/if}
        </section>
      {/each}
    </div>
    <p>Сущности в журнале не удаляются. Будет заменён только сохранённый вид карты.</p>
    <ModalFoot onCancel={layouts.cancelOverwrite} submitLabel="Перезаписать" submitIcon="save" onSubmit={() => void layouts.confirmOverwrite()} busy={layouts.busy} />
  </div>
</ModalShell>

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
  /* The look of the other lists (scripts, runs): a raised panel, rows with a straight left rail that turns accent on the open one. */
  .glay__diff { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
  .glay__diff ul { max-height: 16rem; overflow: auto; padding-left: 1.2rem; }
  .glay__diff small { color: var(--color-fg-muted); }
  .glay {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
    background: var(--color-bg-raised, #1a1a1a);
  }
  .glay__empty {
    margin: 0.75rem 0.6rem;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
  .glay__list {
    flex: 1 1 auto;
    overflow: auto;
    margin: 0;
    padding: 0 0 0.5rem;
    list-style: none;
  }
  .glay__item {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    padding-right: 0.3rem;
    border-left: 2px solid transparent;
  }
  .glay__item:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 4%, transparent);
  }
  .glay__item--on,
  .glay__item--on:hover {
    border-left-color: var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 14%, var(--color-bg-raised, #1a1a1a));
  }
  .glay__open {
    flex: 1 1 auto;
    min-width: 0;
    display: grid;
    gap: 0.1rem;
    padding: 0.4rem 0.6rem 0.4rem 0.55rem;
    border: 0;
    background: transparent;
    color: inherit;
    font-family: var(--font-body, Georgia, serif);
    font-size: var(--text-sm, 0.875rem);
    text-align: left;
    cursor: pointer;
  }
  .glay__title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .glay__open small {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .glay__acts {
    display: flex;
    opacity: 0;
  }
  /* Flat icons: the row is the thing, its actions are a quiet extra (a boxed button per action weighs more than the title). */
  .glay__acts :global(.btn) {
    width: 1.9rem;
    min-width: 0;
    height: 1.9rem;
    padding: 0;
    border-color: transparent;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .glay__acts :global(.btn:hover),
  .glay__acts :global(.btn:focus-visible) {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
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
