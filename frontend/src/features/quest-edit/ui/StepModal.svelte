<script lang="ts">
  import { bindScript, unbindScript, updateQuestStep } from '@/shared/api/client.ts'
  import { checkPayload, questStepDraft, StepAutoCheck } from '@/entities/quest'
  import { MentionTextarea } from '@/entities/template'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import { untrack } from 'svelte'

  let {
    open = false,
    questId = null,
    step = null,
    quests = [],
    questlines = [],
    notes = [],
    attachments = [],
    onClose,
    onSaved,
    onDraft = null,
  }: {
    open: boolean,
    questId?: number | null,
    step?: any | null,
    quests?: any[],
    questlines?: any[],
    notes?: any[],
    attachments?: any[],
    onClose: () => void,
    onSaved: (q: any) => void,
    onDraft?: ((payload: any) => void) | null,
  } = $props()

  let draft = $state(questStepDraft())
  let saving = $state(false)
  let formError = $state('')

  $effect(() => {
    if (!open) return
    untrack(() => {
      formError = ''
      saving = false
      draft = questStepDraft(step)
    })
  })

  async function onSubmit(event: { preventDefault: () => void }) {
    event.preventDefault()
    if (!step?.id || questId == null || saving) return
    const trimmed = draft.title.trim()
    if (!trimmed) {
      formError = 'Нужно название шага'
      return
    }
    saving = true
    formError = ''
    try {
      const payload = {
        title: trimmed,
        description: draft.description.trim(),
        progress_current: Math.max(0, Number(draft.progress_current) || 0),
        progress_total: Math.max(1, Number(draft.progress_total) || 1),
        ...checkPayload(draft),
      }
      if (onDraft) {
        onDraft(payload)
        onClose()
        return
      }
      // The script is bound before the rest is saved, so the step keeps its device and schedule; it is detached first for the
      // same reason in reverse (a command typed over a script replaces it on its own).
      const was = step.script_id != null ? String(step.script_id) : ''
      const now = String(draft.script_id || '')
      if (now && now !== was) await bindScript('step', step.id, Number(now))
      else if (!now && was && !String(draft.check_command || '').trim()) await unbindScript('step', step.id)
      const { script_id: _script, ...body } = payload
      const saved = await updateQuestStep(questId, step.id, body)
      onSaved(saved)
      onClose()
    } catch (e) {
      formError = e.message || String(e)
    } finally {
      saving = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="step-modal-title" zIndex={45} maxWidth="32rem">
  <ModalHead id="step-modal-title" title="Редактировать шаг" icon="edit" {onClose} />

  {#if formError}
    <p class="modal__error">{formError}</p>
  {/if}

  <form class="modal__form" onsubmit={onSubmit}>
    <label class="field">
      <span class="label">Название</span>
      <input type="text" bind:value={draft.title} required />
    </label>

    <div class="field">
      <span class="label">Описание</span>
      <MentionTextarea
        bind:value={draft.description}
        {quests}
        {questlines}
        {notes}
        {attachments}
        rows={3}
        placeholder="@название — квест, заметка, файл, шаг, квестлайн"
      />
    </div>

    <div class="field">
      <span class="label">Прогресс</span>
      <span class="inline-line">
        <input type="number" min="0" title="Сделано" aria-label="Сделано" bind:value={draft.progress_current} />
        <span aria-hidden="true">/</span>
        <input type="number" min="1" title="Всего" aria-label="Всего" bind:value={draft.progress_total} />
      </span>
    </div>

    <div class="field">
      <span class="label">Автопроверка</span>
      <StepAutoCheck
        text={`${draft.title}\n${draft.description || ''}`}
        bind:command={draft.check_command}
        bind:interval={draft.check_interval_seconds}
        bind:waitPrevious={draft.wait_previous}
        bind:runMode={draft.run_mode}
        bind:runOn={draft.run_on}
        bind:scriptId={draft.script_id}
      />
    </div>

    <ModalFoot onCancel={onClose} submitLabel={onDraft ? "В черновик" : "Сохранить"} submitIcon="save" busy={saving} />
  </form>
</ModalShell>
