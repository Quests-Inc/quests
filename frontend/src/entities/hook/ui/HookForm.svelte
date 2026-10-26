<script lang="ts">
  import { ScriptPicker } from '@/entities/script/@x/hook'
  import RunnerPicker from '@/shared/ui/RunnerPicker.svelte'
  import { loadRunners, runnersList } from '@/entities/runner/@x/hook'
  import OptionPills from '@/shared/ui/OptionPills.svelte'

  /** Compact editor for one hook: events, script, runner. */
  let {
    name = $bindable(''),
    enabled = $bindable(true),
    events = $bindable<string[]>([]),
    scriptId = $bindable(''),
    runOn = $bindable(''),
    thisQuest = $bindable(true),
    aliases = {} as Record<string, string[]>,
    catalog = [] as { kind: string; family: string; title: string }[],
    showScope = false,
    disabled = false,
  }: {
    name?: string
    enabled?: boolean
    events?: string[]
    scriptId?: string
    runOn?: string
    thisQuest?: boolean
    aliases?: Record<string, string[]>
    catalog?: { kind: string; family: string; title: string }[]
    showScope?: boolean
    disabled?: boolean
  } = $props()

  // The short names first; the plural and `on_` spellings mean the same and only crowd the list.
  const eventOptions = $derived(
    Object.keys(aliases)
      .filter((id) => !id.startsWith('on_') && id !== 'notes' && id !== 'variables' && id !== 'variable')
      .sort((a, b) => a.localeCompare(b))
      .map((id) => ({ id, label: id, title: (aliases[id] || []).join(', ') })),
  )
  const FAMILIES: Record<string, string> = { quest: 'Квест', step: 'Шаг', note: 'Заметка', value: 'Переменная', attachment: 'Вложение' }
  const kindGroups = $derived(
    Object.keys(FAMILIES)
      .map((family) => ({ family, label: FAMILIES[family], items: catalog.filter((e) => e.family === family) }))
      .filter((g) => g.items.length),
  )

  $effect(() => {
    void loadRunners()
  })

  function toggleEvent(id: string) {
    if (disabled) return
    if (events.includes(id)) events = events.filter((e) => e !== id)
    else events = [...events, id]
  }
</script>

<label class="field">
  <span class="label">Название</span>
  <input type="text" bind:value={name} placeholder="необязательно" {disabled} />
</label>

<div class="field">
  <span class="label">События</span>
  <div class="hookform__events" role="group" aria-label="События">
    {#each eventOptions as opt (opt.id)}
      <button
        type="button"
        class="hookform__ev"
        class:hookform__ev--on={events.includes(opt.id)}
        title={opt.title}
        {disabled}
        onclick={() => toggleEvent(opt.id)}
      >
        {opt.label}
      </button>
    {/each}
  </div>
</div>

{#if kindGroups.length}
  <div class="field">
    <span class="label">Или точнее</span>
    {#each kindGroups as g (g.family)}
      <div class="hookform__group">
        <span class="hookform__fam">{g.label}</span>
        <div class="hookform__events" role="group" aria-label={g.label}>
          {#each g.items as e (e.kind)}
            <button
              type="button"
              class="hookform__ev"
              class:hookform__ev--on={events.includes(e.kind)}
              title={e.kind}
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
{/if}

<div class="field">
  <ScriptPicker bind:value={scriptId} kind="quest" label="Скрипт" />
</div>

<div class="field">
  <span class="label">Раннер</span>
  <RunnerPicker bind:value={runOn} runners={runnersList.items} />
</div>

{#if showScope}
  <div class="field">
    <span class="label">Область</span>
    <OptionPills
      options={[
        { id: 'quest', label: 'Этот квест' },
        { id: 'all', label: 'Весь журнал' },
      ]}
      value={thisQuest ? 'quest' : 'all'}
      label="Область хука"
      {disabled}
      onToggle={(id) => (thisQuest = id === 'quest')}
    />
  </div>
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
    color: var(--color-fg-subtle, #6e6e6e);
    font-size: var(--text-xs, 0.75rem);
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
