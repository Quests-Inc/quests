<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import {
    deleteHook,
    getHooks,
    updateHook,
    type Hook,
  } from '@/shared/api/client.ts'
  import { displayEvents } from '@/entities/hook'
  import HookModal from './HookModal.svelte'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { toastTrashed } from '@/shared/lib/trashUndo.ts'

  /** Workspace hooks: the list, and a form to add or change one (a quest's or a step's own card has the same form). */
  let {
    active = true,
    onOpenQuest,
  }: {
    active?: boolean
    onOpenQuest?: (id: number) => void
  } = $props()

  let hooks: Hook[] = $state([])
  let loaded = $state(false)
  let search = $state('')
  let scope: 'all' | 'quest' | 'workspace' = $state('all')
  let stateFilter: 'all' | 'on' | 'off' = $state('all')
  let pending: Hook | null = $state(null)
  let editing: Hook | null = $state(null)
  let formOpen = $state(false)
  let deleteBusy = $state(false)

  async function load() {
    try {
      hooks = (await getHooks()) ?? []
    } catch {
      hooks = []
    } finally {
      loaded = true
    }
  }

  $effect(() => {
    if (!active) return
    void load()
  })

  let filtered = $derived.by(() => {
    const q = search.trim().toLowerCase()
    return hooks.filter((h) => {
      if (scope === 'quest' && h.scope_kind === 'workspace') return false
      if (scope === 'workspace' && h.scope_kind !== 'workspace') return false
      if (stateFilter === 'on' && !h.enabled) return false
      if (stateFilter === 'off' && h.enabled) return false
      if (!q) return true
      const hay = [
        h.name,
        String(h.id),
        h.script_title,
        String(h.script_id),
        h.run_on,
        h.scope_kind === 'workspace' ? 'журнал пространство' : `${h.scope_kind} ${h.scope_id}`,
        ...displayEvents(h),
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return hay.includes(q)
    })
  })

  async function setEnabled(h: Hook, on: boolean) {
    try {
      await updateHook(h.id, { enabled: on })
      await load()
    } catch (e) {
      toast(e instanceof Error && e.message ? e.message : 'Не удалось изменить', { kind: 'error' })
    }
  }

  async function confirmDelete() {
    if (!pending) return
    deleteBusy = true
    try {
      const id = pending.id
      await deleteHook(id)
      pending = null
      await load()
      toastTrashed('hook', [id], 'Хук в корзине', load)
    } catch (e) {
      toast(e instanceof Error && e.message ? e.message : 'Не удалось удалить', { kind: 'error' })
    } finally {
      deleteBusy = false
    }
  }

  const AREA_WORD: Record<string, string> = { step: 'шаг', note: 'заметка', note_tree: 'заметка и вложенные' }
  function areaLabel(h: Hook) {
    return `${AREA_WORD[h.scope_kind] ?? h.scope_kind} ${h.scope_id ?? ''}`.trim()
  }

  function openForm(h: Hook | null) {
    editing = h
    formOpen = true
  }
</script>

<div class="hookpage">
  <div class="hookpage__tools">
    <div class="hookpage__top">
      <input
        class="search hookpage__search"
        type="search"
        placeholder="Поиск по названию, событию, скрипту…"
        bind:value={search}
        aria-label="Поиск хуков"
      />
      <button type="button" class="btn btn--accent btn--icon-only" title="Новый хук" aria-label="Новый хук" onclick={() => openForm(null)}>
        <Icon name="add" size={16} />
      </button>
    </div>
    <div class="hookpage__row">
      <div class="hookpage__seg" role="group" aria-label="Область">
        {#each [
          { id: 'all', label: 'Все' },
          { id: 'quest', label: 'С областью' },
          { id: 'workspace', label: 'Всё пространство' },
        ] as s}
          <button
            type="button"
            class="hookpage__pill"
            class:hookpage__pill--on={scope === s.id}
            onclick={() => (scope = s.id as typeof scope)}>{s.label}</button
          >
        {/each}
      </div>
      <div class="hookpage__seg" role="group" aria-label="Состояние">
        {#each [
          { id: 'all', label: 'Любые' },
          { id: 'on', label: 'Вкл' },
          { id: 'off', label: 'Выкл' },
        ] as s}
          <button
            type="button"
            class="hookpage__pill"
            class:hookpage__pill--on={stateFilter === s.id}
            onclick={() => (stateFilter = s.id as typeof stateFilter)}
          >
            {s.label}
            {#if s.id === 'all'}
              <span class="hookpage__n">{hooks.length}</span>
            {/if}
          </button>
        {/each}
      </div>
    </div>
  </div>

  <div class="hookpage__list">
    {#if !loaded}
      <p class="empty">Загрузка…</p>
    {:else if hooks.length === 0}
      <p class="empty">Хуков пока нет. Добавьте первый кнопкой «Хук» или в карточке квеста или шага.</p>
    {:else if filtered.length === 0}
      <p class="empty">Ничего не найдено</p>
    {:else}
      {#each filtered as h (h.id)}
        <article
          class="hookpage__item"
          class:hookpage__item--off={!h.enabled}
          role="listitem"
          onclick={() => openForm(h)}
        >
          <div class="hookpage__icon" aria-hidden="true">
            <Icon name="flag" size={18} />
          </div>
          <div class="hookpage__main">
            <div class="hookpage__name">
              <span class="hookpage__file">{h.name || `хук #${h.id}`}</span>
              {#if !h.enabled}
                <span class="hookpage__chip">выкл</span>
              {/if}
              {#if h.scope_kind === 'workspace'}
                <span class="hookpage__chip">всё пространство</span>
              {:else if h.scope_kind === 'quest'}
                <button
                  type="button"
                  class="hookpage__chip hookpage__chip--link"
                  title="Открыть квест"
                  onclick={(e) => {
                    e.stopPropagation()
                    onOpenQuest?.(h.scope_id as number)
                  }}>квест {h.scope_id}</button
                >
              {:else}
                <span class="hookpage__chip">{areaLabel(h)}</span>
              {/if}
              {#if h.disabled_reason}
                <span class="hookpage__chip hookpage__chip--warn" title={h.disabled_reason}>выключен сам</span>
              {/if}
            </div>
            <div class="hookpage__meta">
              <span>{displayEvents(h).join(', ')}</span>
              <span>{h.script_title || `скрипт ${h.script_id}`}</span>
              {#if h.run_on}<span>{h.run_on}</span>{/if}
            </div>
          </div>
          <div class="hookpage__acts">
            <button
              type="button"
              class="hookpage__ico"
              title="Редактировать"
              aria-label="Редактировать"
              onclick={(e) => {
                e.stopPropagation()
                openForm(h)
              }}
            >
              <Icon name="edit" size={16} />
            </button>
            <button
              type="button"
              class="hookpage__ico hookpage__ico--danger"
              title="Удалить"
              aria-label="Удалить"
              onclick={(e) => {
                e.stopPropagation()
                pending = h
              }}
            >
              <Icon name="delete" size={16} />
            </button>
            <button
              type="button"
              class="switch"
              class:switch--on={h.enabled}
              role="switch"
              aria-checked={h.enabled}
              title={h.enabled ? 'Выключить' : 'Включить'}
              aria-label={h.enabled ? 'Выключить хук' : 'Включить хук'}
              onclick={(e) => {
                e.stopPropagation()
                void setEnabled(h, !h.enabled)
              }}
            >
              <span class="switch__knob"></span>
            </button>
          </div>
        </article>
      {/each}
    {/if}
  </div>
</div>

<HookModal open={formOpen} hook={editing} onClose={() => (formOpen = false)} onSaved={() => void load()} />

<ConfirmModal
  open={pending != null}
  title="Удалить хук?"
  message={pending ? `«${pending.name || `хук ${pending.id}`}» уйдёт в корзину на 30 дней и перестанет срабатывать.` : ''}
  busy={deleteBusy}
  onCancel={() => {
    if (!deleteBusy) pending = null
  }}
  onConfirm={confirmDelete}
/>

<style>
  .hookpage {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  .hookpage__tools {
    display: grid;
    gap: 0.55rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--color-border, #333);
  }

  .hookpage__top {
    display: flex;
    gap: 0.5rem;
  }

  .hookpage__search {
    flex: 1 1 auto;
    min-width: 0;
  }

  .hookpage__row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }

  .hookpage__seg {
    display: inline-flex;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    overflow: hidden;
  }

  .hookpage__pill {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.28rem 0.65rem;
    border: 0;
    border-right: 1px solid var(--color-border, #333);
    background: transparent;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .hookpage__pill:last-child {
    border-right: 0;
  }

  .hookpage__pill:hover {
    color: var(--color-fg, #e8e8e8);
  }

  .hookpage__pill--on {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 18%, transparent);
  }

  .hookpage__n {
    font-variant-numeric: tabular-nums;
    opacity: 0.7;
  }

  .hookpage__list {
    overflow: auto;
    padding: 0.35rem 0 1rem;
    min-height: 0;
  }

  .empty {
    margin: 1.5rem 1rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .hookpage__item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 1rem;
    border-left: 2px solid transparent;
    cursor: pointer;
  }

  .hookpage__item:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 4%, transparent);
  }

  .hookpage__item--off .hookpage__main,
  .hookpage__item--off .hookpage__icon {
    opacity: 0.5;
  }

  .hookpage__icon {
    flex: 0 0 2.75rem;
    width: 2.75rem;
    height: 2.75rem;
    display: grid;
    place-items: center;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    color: var(--color-fg-muted, #9a9a9a);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 5%, transparent);
  }

  .hookpage__main {
    flex: 1 1 auto;
    min-width: 0;
    display: grid;
    gap: 0.15rem;
  }

  .hookpage__name {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
  }

  .hookpage__file {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm, 0.875rem);
    font-weight: 600;
  }

  .hookpage__chip {
    flex: 0 0 auto;
    padding: 0 0.3rem;
    border: 1px solid var(--color-border, #3a3a3a);
    border-radius: var(--radius-sm, 3px);
    font-family: var(--font-mono, monospace);
    font-size: 0.65rem;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .hookpage__meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem 0.85rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .hookpage__acts {
    display: flex;
    align-items: center;
    gap: 0.15rem;
    flex-shrink: 0;
  }

  .hookpage__chip--warn {
    color: var(--color-danger, #b54a3a);
    border-color: currentColor;
  }
  .hookpage__chip--link {
    background: transparent;
    cursor: pointer;
  }

  .hookpage__chip--link:hover {
    color: var(--color-accent, #c9a227);
    border-color: var(--color-accent, #c9a227);
  }

  .switch {
    position: relative;
    flex-shrink: 0;
    width: 2.25rem;
    height: 1.25rem;
    margin: 0 0 0 0.35rem;
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
  }

  .hookpage__ico {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }

  .hookpage__ico:hover {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .hookpage__ico--danger:hover {
    color: var(--color-danger, #c44);
  }
</style>
