<script lang="ts">
  import type { Snippet } from 'svelte'
  import { ScriptPicker, scriptsList } from '@/entities/script/@x/hook'
  import RunnerPicker from '@/shared/ui/RunnerPicker.svelte'
  import { loadRunners, runnersList } from '@/entities/runner/@x/hook'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import { expandEvents, FAMILY_WORDS, hookCatalog, loadHookCatalog } from '../model/catalog.svelte.ts'

  /**
   * One hook: its name, where it listens (the `area` the caller draws, or the quest/workspace switch), which events, which
   * script, where the script's code runs, and whether it is on. Events are chosen from the catalog by their titles, a whole
   * family at once from its head; the codes are in the hints.
   */
  let {
    name = $bindable(''),
    enabled = $bindable(true),
    events = $bindable<string[]>([]),
    scriptId = $bindable(''),
    runOn = $bindable(''),
    thisQuest = $bindable(true),
    showScope = false,
    disabled = false,
    area,
  }: {
    name?: string
    enabled?: boolean
    events?: string[]
    scriptId?: string
    runOn?: string
    thisQuest?: boolean
    showScope?: boolean
    disabled?: boolean
    /** The area of the hook, drawn by the caller right after the name. */
    area?: Snippet
  } = $props()

  const groups = $derived(
    Object.keys(FAMILY_WORDS)
      .map((family) => ({ family, label: FAMILY_WORDS[family], items: hookCatalog.events.filter((e) => e.family === family) }))
      .filter((g) => g.items.length)
  )

  $effect(() => {
    void loadHookCatalog()
    void loadRunners()
  })
  // Aliases saved by older forms (`complete`, `note`…) become the kinds they stand for once the catalog is here.
  $effect(() => {
    if (!hookCatalog.loaded) return
    const kinds = expandEvents(events, hookCatalog.aliases)
    if (kinds.length !== events.length || kinds.some((k, i) => k !== events[i])) events = kinds
  })

  /** The script's code goes to a runner: only then is there a runner to choose. */
  const needsRunner = $derived(Boolean(scriptsList.items.find((x) => String(x.id) === String(scriptId))?.needs_runner) || (!scriptId && Boolean(runOn)))

  function toggleEvent(id: string) {
    if (disabled) return
    if (events.includes(id)) events = events.filter((e) => e !== id)
    else events = [...events, id]
  }
  function toggleFamily(kinds: string[]) {
    if (disabled) return
    const all = kinds.every((k) => events.includes(k))
    events = all ? events.filter((e) => !kinds.includes(e)) : [...new Set([...events, ...kinds])]
  }
</script>

<label class="field">
  <span class="label">Название</span>
  <input type="text" bind:value={name} placeholder="необязательно" {disabled} />
</label>

{#if area}
  {@render area()}
{:else if showScope}
  <div class="field">
    <span class="label">Где слушать</span>
    <OptionPills
      options={[
        { id: 'quest', label: 'Этот квест' },
        { id: 'all', label: 'Всё пространство' },
      ]}
      value={thisQuest ? 'quest' : 'all'}
      label="Область хука"
      {disabled}
      onToggle={(id) => (thisQuest = id === 'quest')}
    />
  </div>
{/if}

<div class="field">
  <span class="label">События</span>
  {#if !hookCatalog.loaded}
    <p class="explain">Загрузка событий…</p>
  {/if}
  {#each groups as g (g.family)}
    {@const kinds = g.items.map((e) => e.kind)}
    {@const all = kinds.every((k) => events.includes(k))}
    <div class="hookform__group">
      <label class="hookform__fam">
        <input type="checkbox" checked={all} indeterminate={!all && kinds.some((k) => events.includes(k))} {disabled} onchange={() => toggleFamily(kinds)} />
        {g.label}: все
      </label>
      <div class="hookform__events" role="group" aria-label={g.label}>
        {#each g.items as e (e.kind)}
          <button
            type="button"
            class="hookform__ev"
            class:hookform__ev--on={events.includes(e.kind)}
            title={`${e.kind}${e.fields?.length ? ` · поля: ${e.fields.join(', ')}` : ''}`}
            {disabled}
            onclick={() => toggleEvent(e.kind)}
          >
            {e.title}
          </button>
        {/each}
      </div>
    </div>
  {/each}
</div>

<div class="field">
  <span class="label">Скрипт</span>
  <ScriptPicker bind:value={scriptId} kind="hook" label="Скрипт хука" />
</div>

{#if needsRunner}
  <div class="field">
    <span class="label">Где выполнять код</span>
    <RunnerPicker bind:value={runOn} runners={runnersList.items} />
  </div>
{:else if scriptId}
  <p class="explain">Скрипт без кода: выполняется на сервере, раннер не нужен.</p>
{/if}

<label class="check">
  <input type="checkbox" bind:checked={enabled} {disabled} />
  Включён
</label>

<style>
  .hookform__group {
    display: grid;
    gap: 0.25rem;
    margin-top: 0.3rem;
  }

  .hookform__fam {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
    cursor: pointer;
  }

  .hookform__events {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .hookform__ev {
    padding: 0.28rem 0.55rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font: inherit;
    font-size: var(--text-xs, 0.75rem);
    cursor: pointer;
  }

  .hookform__ev:hover:not(:disabled) {
    color: var(--color-fg, #e8e8e8);
  }

  .hookform__ev--on {
    color: var(--color-fg, #e8e8e8);
    border-color: var(--color-accent, #c9a227);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 18%, transparent);
  }

  .hookform__ev:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
