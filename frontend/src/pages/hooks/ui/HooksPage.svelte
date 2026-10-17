<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import RunnerPicker from '@/shared/ui/RunnerPicker.svelte'
  import { ScriptPicker } from '@/entities/script'
  import { loadRunners, runnersList } from '@/entities/runner'
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

  /** Event hooks of the workspace: an event plus a library script, run on a device. */
  let { active = true }: { active?: boolean } = $props()

  let hooks: Hook[] = $state([])
  let catalog: HookEventsCatalog | null = $state(null)
  let loaded = $state(false)
  let editing: Hook | null = $state(null)
  let formOpen = $state(false)

  let name = $state('')
  let enabled = $state(true)
  let pickedEvents = $state<string[]>(['complete'])
  let scriptId = $state('')
  let questId = $state('')
  let runOn = $state('')
  let busy = $state(false)

  const aliasEntries = $derived(
    catalog ? Object.entries(catalog.aliases).sort(([a], [b]) => a.localeCompare(b)) : []
  )

  async function load() {
    try {
      const [h, ev] = await Promise.all([getHooks(), catalog ? Promise.resolve(catalog) : getHookEvents()])
      hooks = h ?? []
      if (!catalog) catalog = ev
    } catch {
      /* toast from request */
    } finally {
      loaded = true
    }
  }

  $effect(() => {
    if (!active) return
    void load()
    void loadRunners()
  })

  function openNew() {
    editing = null
    name = ''
    enabled = true
    pickedEvents = ['complete']
    scriptId = ''
    questId = ''
    runOn = ''
    formOpen = true
  }

  function openEdit(h: Hook) {
    editing = h
    name = h.name
    enabled = h.enabled
    pickedEvents = h.events_raw?.length ? [...h.events_raw] : [...h.events]
    scriptId = String(h.script_id)
    questId = h.quest_id != null ? String(h.quest_id) : ''
    runOn = h.run_on || ''
    formOpen = true
  }

  function toggleEvent(ev: string) {
    if (pickedEvents.includes(ev)) pickedEvents = pickedEvents.filter((e) => e !== ev)
    else pickedEvents = [...pickedEvents, ev]
  }

  async function save() {
    const sid = Number(scriptId)
    if (!pickedEvents.length || !(sid > 0) || busy) return
    busy = true
    try {
      const q = Number(questId)
      const body = {
        name: name.trim(),
        enabled,
        events: pickedEvents,
        script_id: sid,
        run_on: runOn.trim(),
        quest_id: q > 0 ? q : null,
      }
      if (editing) await updateHook(editing.id, body)
      else await createHook(body)
      formOpen = false
      await load()
      toast(editing ? 'Хук сохранён' : 'Хук создан')
    } catch (e: any) {
      toast(e?.message || 'Не удалось сохранить хук', { kind: 'error' })
    } finally {
      busy = false
    }
  }

  async function remove(h: Hook) {
    if (!confirm(`Удалить хук «${h.name || h.id}»?`)) return
    try {
      await deleteHook(h.id)
      if (editing?.id === h.id) formOpen = false
      await load()
    } catch (e: any) {
      toast(e?.message || 'Не удалось удалить', { kind: 'error' })
    }
  }

  async function setEnabled(h: Hook, on: boolean) {
    try {
      await updateHook(h.id, { enabled: on })
      await load()
    } catch (e: any) {
      toast(e?.message || 'Не удалось изменить', { kind: 'error' })
    }
  }
</script>

<div class="hooks">
  <section class="hooks__block">
    <header class="hooks__head">
      <h2>Хуки</h2>
      <button type="button" class="btn" onclick={openNew}>
        <Icon name="add" size={14} />
        Новый
      </button>
    </header>
    <p class="hooks__lead">На событие квеста запускается скрипт из библиотеки — на раннере, не на сервере.</p>

    {#if !loaded}
      <p class="hooks__muted">Загрузка…</p>
    {:else if !hooks.length}
      <p class="hooks__muted">Хуков пока нет.</p>
    {:else}
      <ul class="hooks__list">
        {#each hooks as h (h.id)}
          <li class="hooks__row" class:hooks__row--off={!h.enabled}>
            <button type="button" class="hooks__main" onclick={() => openEdit(h)}>
              <span class="hooks__title">{h.name || `хук #${h.id}`}</span>
              <span class="hooks__meta">
                {(h.events_raw?.length ? h.events_raw : h.events).join(', ')}
                · {h.script_title || `скрипт ${h.script_id}`}
                {#if h.quest_id}· квест {h.quest_id}{/if}
                {#if h.run_on}· {h.run_on}{/if}
              </span>
            </button>
            <label class="hooks__en">
              <input type="checkbox" checked={h.enabled} onchange={(e) => void setEnabled(h, e.currentTarget.checked)} />
              вкл
            </label>
            <button type="button" class="btn hooks__del" title="Удалить" onclick={() => void remove(h)}>
              <Icon name="delete" size={14} />
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if formOpen}
    <section class="hooks__block formscope">
      <header class="hooks__head">
        <h2>{editing ? 'Правка хука' : 'Новый хук'}</h2>
        <button type="button" class="btn" onclick={() => (formOpen = false)}>Закрыть</button>
      </header>

      <label class="hooks__field">
        <span>Название</span>
        <input type="text" bind:value={name} placeholder="необязательно" />
      </label>

      <fieldset class="hooks__field">
        <legend>События</legend>
        <div class="hooks__events">
          {#each aliasEntries as [alias, kinds]}
            <label class="hooks__chip" title={kinds.join(', ')}>
              <input type="checkbox" checked={pickedEvents.includes(alias)} onchange={() => toggleEvent(alias)} />
              {alias}
            </label>
          {/each}
        </div>
      </fieldset>

      <div class="hooks__field">
        <ScriptPicker bind:value={scriptId} kind="quest" label="Скрипт" />
      </div>

      <label class="hooks__field">
        <span>Квест (необязательно)</span>
        <input type="text" bind:value={questId} placeholder="id — только этот квест" inputmode="numeric" />
      </label>

      <div class="hooks__field hooks__run">
        <span>Раннер</span>
        <RunnerPicker bind:value={runOn} runners={runnersList.items} />
      </div>

      <label class="hooks__en hooks__field">
        <input type="checkbox" bind:checked={enabled} />
        Включён
      </label>

      <div class="hooks__actions">
        <button type="button" class="btn btn--primary" disabled={busy || !scriptId || !pickedEvents.length} onclick={() => void save()}>
          {editing ? 'Сохранить' : 'Создать'}
        </button>
      </div>
    </section>
  {/if}
</div>

<style>
  .hooks {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
    padding: 1rem 1.25rem 2rem;
    max-width: 40rem;
  }
  .hooks__block h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
  }
  .hooks__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
  }
  .hooks__lead,
  .hooks__muted {
    margin: 0 0 1rem;
    color: var(--muted, #666);
    font-size: 0.9rem;
  }
  .hooks__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .hooks__row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    border-bottom: 1px solid color-mix(in srgb, currentColor 12%, transparent);
    padding: 0.35rem 0;
  }
  .hooks__row--off {
    opacity: 0.55;
  }
  .hooks__main {
    flex: 1;
    text-align: left;
    background: none;
    border: 0;
    padding: 0.25rem 0;
    cursor: pointer;
    color: inherit;
    font: inherit;
  }
  .hooks__title {
    display: block;
    font-weight: 560;
  }
  .hooks__meta {
    display: block;
    font-size: 0.8rem;
    color: var(--muted, #666);
  }
  .hooks__en {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.85rem;
    white-space: nowrap;
  }
  .hooks__del {
    padding: 0.25rem 0.4rem;
  }
  .hooks__field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    margin: 0 0 0.9rem;
    border: 0;
    padding: 0;
    font-size: 0.9rem;
  }
  .hooks__field input[type='text'] {
    font: inherit;
    padding: 0.4rem 0.55rem;
  }
  .hooks__events {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem 0.75rem;
  }
  .hooks__chip {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.85rem;
  }
  .hooks__run {
    flex-direction: row;
    align-items: center;
    gap: 0.75rem;
  }
  .hooks__actions {
    margin-top: 0.5rem;
  }
</style>
