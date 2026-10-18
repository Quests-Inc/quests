<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import {
    createHook,
    deleteHook,
    getHookEvents,
    getHooks,
    updateHook,
    type Hook,
    type HookEventsCatalog,
  } from '@/shared/api/client.ts'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import HookForm from './HookForm.svelte'
  import { displayEvents, newHookDraft, type HookDraft } from '../model/events.ts'

  /**
   * Hooks for a quest (or drafts while creating). Owners add/edit; members see the list.
   * `defaultEvents` seeds a new row (`complete` in the quest modal, `step` in the step modal).
   */
  let {
    questId = null,
    drafts = $bindable<HookDraft[]>([]),
    canEdit = true,
    defaultEvents = ['complete'],
    showScope = true,
    open = false,
  }: {
    questId?: number | null
    drafts?: HookDraft[]
    canEdit?: boolean
    defaultEvents?: string[]
    showScope?: boolean
    /** When the parent FormSection opens, refresh from the API. */
    open?: boolean
  } = $props()

  let catalog: HookEventsCatalog | null = $state(null)
  let hooks: Hook[] = $state([])
  let loaded = $state(false)
  let editingId: number | null = $state(null)
  let adding = $state(false)
  let busy = $state(false)

  let name = $state('')
  let enabled = $state(true)
  let events = $state<string[]>([])
  let scriptId = $state('')
  let runOn = $state('')
  let thisQuest = $state(true)

  const aliases = $derived(catalog?.aliases ?? {})
  const live = $derived(questId != null && questId > 0)
  const rows = $derived(live ? hooks : drafts.map(draftAsRow))

  function draftAsRow(d: HookDraft) {
    return {
      id: d.localId,
      name: d.name,
      enabled: d.enabled,
      events: d.events,
      events_raw: d.events,
      script_id: Number(d.script_id) || 0,
      script_title: '',
      run_on: d.run_on,
      quest_id: d.thisQuest ? -1 : null,
      timeout_sec: 30,
      _draft: true as const,
      thisQuest: d.thisQuest,
    }
  }

  async function ensureCatalog() {
    if (catalog) return
    try {
      catalog = await getHookEvents()
    } catch {
      /* toast */
    }
  }

  async function loadLive() {
    if (!live) return
    try {
      const all = (await getHooks()) ?? []
      hooks = all.filter((h) => h.quest_id === questId)
    } catch {
      hooks = []
    } finally {
      loaded = true
    }
  }

  $effect(() => {
    if (!open) return
    void ensureCatalog()
    if (live) void loadLive()
    else loaded = true
  })

  function resetForm(seedEvents: string[]) {
    name = ''
    enabled = true
    events = [...seedEvents]
    scriptId = ''
    runOn = ''
    thisQuest = true
  }

  function startAdd() {
    if (!canEdit) return
    editingId = null
    adding = true
    resetForm(defaultEvents)
    if (!showScope) thisQuest = true
  }

  function startEdit(row: ReturnType<typeof draftAsRow> | Hook) {
    if (!canEdit) return
    adding = false
    editingId = row.id
    name = row.name || ''
    enabled = row.enabled
    events = [...displayEvents(row)]
    scriptId = row.script_id ? String(row.script_id) : ''
    runOn = row.run_on || ''
    thisQuest = row.quest_id != null
  }

  function cancelForm() {
    adding = false
    editingId = null
  }

  async function saveForm() {
    const sid = Number(scriptId)
    if (!events.length || !(sid > 0) || busy || !canEdit) return
    busy = true
    try {
      if (!live) {
        if (editingId != null) {
          drafts = drafts.map((d) =>
            d.localId === editingId
              ? { ...d, name: name.trim(), enabled, events: [...events], script_id: scriptId, run_on: runOn.trim(), thisQuest }
              : d,
          )
        } else {
          drafts = [
            ...drafts,
            newHookDraft({
              name: name.trim(),
              enabled,
              events: [...events],
              script_id: scriptId,
              run_on: runOn.trim(),
              thisQuest,
            }),
          ]
        }
        cancelForm()
        return
      }
      const body = {
        name: name.trim(),
        enabled,
        events: [...events],
        script_id: sid,
        run_on: runOn.trim(),
        quest_id: thisQuest ? questId : null,
      }
      if (editingId != null) await updateHook(editingId, body)
      else await createHook(body)
      cancelForm()
      await loadLive()
      if (!thisQuest) toast('Хук на весь журнал — во вкладке «Хуки»')
      else toast(editingId != null ? 'Хук сохранён' : 'Хук добавлен')
    } catch (e) {
      toast(errMsg(e, 'Не удалось сохранить хук'), { kind: 'error' })
    } finally {
      busy = false
    }
  }

  async function remove(row: { id: number }) {
    if (!canEdit) return
    if (!confirm(`Удалить хук?`)) return
    try {
      if (!live) {
        drafts = drafts.filter((d) => d.localId !== row.id)
        if (editingId === row.id) cancelForm()
        return
      }
      await deleteHook(row.id)
      if (editingId === row.id) cancelForm()
      await loadLive()
    } catch (e) {
      toast(errMsg(e, 'Не удалось удалить'), { kind: 'error' })
    }
  }

  async function setEnabled(row: { id: number }, on: boolean) {
    if (!canEdit) return
    try {
      if (!live) {
        drafts = drafts.map((d) => (d.localId === row.id ? { ...d, enabled: on } : d))
        return
      }
      await updateHook(row.id, { enabled: on })
      await loadLive()
    } catch (e) {
      toast(errMsg(e, 'Не удалось изменить'), { kind: 'error' })
    }
  }

  function errMsg(e: unknown, fallback: string) {
    return e instanceof Error && e.message ? e.message : fallback
  }

</script>

{#if !loaded}
  <p class="hint">Загрузка…</p>
{:else}
  {#if rows.length === 0 && !adding}
    <p class="hint">
      {canEdit
        ? 'На событие квеста можно повесить скрипт из библиотеки — он уйдёт на раннер.'
        : 'Хуков на этот квест нет.'}
    </p>
  {:else}
    <ul class="hooksfield__list">
      {#each rows as row (row.id)}
        <li class="hooksfield__row" class:hooksfield__row--off={!row.enabled}>
          <button type="button" class="hooksfield__main" disabled={!canEdit} onclick={() => startEdit(row)}>
            <span class="hooksfield__title">{row.name || `хук #${Math.abs(row.id)}`}</span>
            <span class="hooksfield__meta">
              {displayEvents(row).join(', ')}
              · {row.script_title || (row.script_id ? `скрипт ${row.script_id}` : 'без скрипта')}
              {#if row.run_on}· {row.run_on}{/if}
              {#if live && row.quest_id == null}· весь журнал{/if}
            </span>
          </button>
          {#if canEdit}
            <label class="check hooksfield__en">
              <input
                type="checkbox"
                checked={row.enabled}
                onchange={(e) => void setEnabled(row, e.currentTarget.checked)}
              />
            </label>
            <button type="button" class="btn hooksfield__ico" title="Удалить" onclick={() => void remove(row)}>
              <Icon name="delete" size={14} />
            </button>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if canEdit && (adding || editingId != null)}
    <div class="hooksfield__form">
      <HookForm
        bind:name
        bind:enabled
        bind:events
        bind:scriptId
        bind:runOn
        bind:thisQuest
        {aliases}
        {showScope}
        disabled={busy}
      />
      <div class="hooksfield__actions">
        <button type="button" class="btn" disabled={busy} onclick={cancelForm}>Отмена</button>
        <button
          type="button"
          class="btn btn--primary"
          disabled={busy || !scriptId || !events.length}
          onclick={() => void saveForm()}
        >
          {editingId != null ? 'Сохранить' : 'Добавить'}
        </button>
      </div>
    </div>
  {:else if canEdit}
    <button type="button" class="btn" onclick={startAdd}>
      <Icon name="add" size={14} />
      Хук
    </button>
  {/if}
{/if}

<style>
  .hooksfield__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .hooksfield__row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border, #333) 70%, transparent);
  }

  .hooksfield__row--off {
    opacity: 0.55;
  }

  .hooksfield__main {
    flex: 1;
    min-width: 0;
    text-align: left;
    background: none;
    border: 0;
    padding: 0.2rem 0;
    cursor: pointer;
    color: inherit;
    font: inherit;
  }

  .hooksfield__main:disabled {
    cursor: default;
  }

  .hooksfield__title {
    display: block;
    font-size: var(--text-sm, 0.875rem);
    font-weight: 600;
  }

  .hooksfield__meta {
    display: block;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .hooksfield__en {
    margin: 0;
  }

  .hooksfield__ico {
    padding: 0.25rem 0.4rem;
  }

  .hooksfield__form {
    display: grid;
    gap: var(--space-3, 0.75rem);
    margin-top: 0.5rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--color-border, #333);
  }

  .hooksfield__actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
  }
</style>
