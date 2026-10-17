<script lang="ts">
  import { tick } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { MentionTextarea } from '@/entities/template/@x/quest'
  import StepAutoCheck from './StepAutoCheck.svelte'
  import { mirrorQuestId, questStepDraft, templateStepDraft } from '../model/steps.ts'

  /**
   * The one step-list editor (quest and template modals). Each step is a
   * single row — title · progress · ⚙ · delete — and ⚙ unfolds its details:
   * description (quests only; template steps have none) and auto-check.
   * The ⚙ lights up when a folded step has something set in there.
   *
   * Steps are drafts from js/steps.js; untitled ones are dropped on save.
   */
  let {
    steps = $bindable([]),
    variant = 'quest',
    quests = [],
    questlines = [],
    notes = [],
    attachments = [],
  }: {
    steps: any[],
    variant?: 'quest' | 'template',
    quests?: any[],
    questlines?: any[],
    notes?: any[],
    attachments?: any[],
  } = $props()

  let isQuest = $derived(variant === 'quest')

  function blank() {
    return isQuest ? questStepDraft() : templateStepDraft()
  }

  let rootEl: HTMLElement | undefined = $state()

  async function add() {
    steps = [...steps, blank()]
    await tick()
    // The new step's title field takes the cursor at once.
    const titles = rootEl?.querySelectorAll<HTMLInputElement>('.steps-ed__title')
    titles?.[titles.length - 1]?.focus()
  }

  function remove(key: any) {
    steps = steps.length <= 1 ? [blank()] : steps.filter((s) => s.key !== key)
  }

  /** quest=N in a folded step's title/description with no check yet: offer it
   * right on the row — the check field itself sits inside the ⚙ details. */
  function foldedMirrorId(s: { open: any; check_command: any; script_id?: any; title: string; description: string }) {
    if (s.open || String(s.check_command || '').trim() || String(s.script_id || '')) return null
    return mirrorQuestId(s.title, s.description)
  }

  function useMirrorCheck(s: { check_command: string; run_mode: string; open: boolean }, questId: any) {
    s.check_command = `quests progress ${questId}`
    s.run_mode = 'watch'
    s.open = true
  }

  function hasDetails(s: { check_command: any; description: any; script_id?: any }) {
    return Boolean(String(s.check_command || '').trim() || String(s.description || '').trim() || String(s.script_id || ''))
  }
</script>

<div class="steps-ed" bind:this={rootEl}>
  {#each steps as s, i (s.key)}
    <div class="steps-ed__item">
      <div class="steps-ed__row">
        <input
          type="text"
          class="steps-ed__title"
          placeholder="Шаг {i + 1}"
          aria-label="Название шага {i + 1}"
          bind:value={s.title}
        />
        {#if isQuest}
          <span class="steps-ed__progress" role="group" aria-label="Прогресс шага {i + 1}">
            <input type="number" min="0" title="Сделано" aria-label="Сделано" bind:value={s.progress_current} />
            <span class="steps-ed__slash" aria-hidden="true">/</span>
            <input type="number" min="1" title="Всего" aria-label="Всего" bind:value={s.progress_total} />
          </span>
        {:else}
          <input
            type="text"
            inputmode="numeric"
            class="steps-ed__range"
            placeholder="1"
            title="Количество: 5 или диапазон 5..10 — случайное при появлении"
            aria-label="Количество для шага {i + 1}"
            bind:value={s.progress_range}
          />
        {/if}
        <button
          type="button"
          class="btn btn--ghost btn--icon"
          class:steps-ed__more--set={!s.open && hasDetails(s)}
          aria-expanded={s.open}
          aria-label={isQuest ? 'Описание и автопроверка' : 'Автопроверка'}
          title={isQuest ? 'Описание и автопроверка' : 'Автопроверка'}
          onclick={() => (s.open = !s.open)}
        >
          <Icon name="settings" size={14} />
        </button>
        <button
          type="button"
          class="btn btn--ghost btn--icon steps-ed__remove"
          aria-label="Удалить шаг"
          title="Удалить шаг"
          onclick={() => remove(s.key)}
        >
          <Icon name="delete" size={14} />
        </button>
      </div>
      {#if foldedMirrorId(s) != null}
        <button
          type="button"
          class="btn btn--ghost steps-ed__mirror"
          onclick={() => useMirrorCheck(s, foldedMirrorId(s))}
        >
          Следить за quest={foldedMirrorId(s)}: <code>quests progress {foldedMirrorId(s)}</code>
        </button>
      {/if}
      {#if s.open}
        <div class="steps-ed__details">
          {#if isQuest}
            <MentionTextarea
              bind:value={s.description}
              {quests}
              {questlines}
              {notes}
              {attachments}
              rows={2}
              placeholder="Описание шага — markdown, @упоминания"
            />
          {/if}
          <StepAutoCheck
            text={`${s.title}\n${s.description || ''}`}
            bind:command={s.check_command}
            bind:interval={s.check_interval_seconds}
            bind:waitPrevious={s.wait_previous}
            bind:runMode={s.run_mode}
            bind:runOn={s.run_on}
            bind:scriptId={s.script_id}
          />
        </div>
      {/if}
    </div>
  {/each}
  <button type="button" class="btn btn--ghost steps-ed__add" onclick={add}>
    <Icon name="add" size={14} />
    <span>Шаг</span>
  </button>
</div>

<style>
  .steps-ed {
    display: grid;
    gap: 0.4rem;
  }

  .steps-ed__item {
    display: grid;
    gap: 0.4rem;
  }

  .steps-ed__row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .steps-ed__row .steps-ed__title {
    flex: 1 1 auto;
  }

  .steps-ed__progress {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    flex-shrink: 0;
  }

  .steps-ed__progress input[type='number'] {
    width: 3.4rem;
    text-align: center;
  }

  .steps-ed__row .steps-ed__range {
    flex: 0 0 5rem;
    width: 5rem;
    text-align: center;
  }

  .steps-ed__slash {
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .steps-ed__more--set {
    color: var(--color-accent, #c9a227);
  }

  .steps-ed__remove:hover:not(:disabled) {
    color: var(--color-danger, #b54a3a);
  }

  .steps-ed__mirror {
    justify-self: start;
    margin-top: 0.2rem;
    font-size: var(--text-xs, 0.75rem);
  }

  .steps-ed__details {
    display: grid;
    gap: 0.4rem;
    margin-left: 0.75rem;
    padding-left: 0.75rem;
    border-left: 2px solid var(--color-border, #333);
  }

  .steps-ed__add {
    justify-self: start;
  }
</style>
