<script lang="ts">
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import Picker from '@/shared/ui/Picker.svelte'
  import { questCatalog } from '@/entities/quest'
  import { noteCatalog } from '@/entities/note'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import { HookForm } from '@/entities/hook'
  import { createHook, updateHook, type Hook } from '@/shared/api/client.ts'

  /** One hook, created or edited: events, script, runner, its area, the limits against storms, the time it may take. */
  let {
    open,
    hook,
    onClose,
    onSaved,
  }: { open: boolean; hook: Hook | null; onClose: () => void; onSaved: () => void } = $props()

  let name = $state('')
  let enabled = $state(true)
  let events = $state<string[]>([])
  let scriptId = $state('')
  let runOn = $state('')
  /** the area: the whole workspace, one quest / step / note, or a note with everything under it */
  let scopeKind = $state('workspace')
  /** the id of the quest, step or note of the area */
  let scopeId = $state('')
  let cooldown = $state('5')
  let perHour = $state('120')
  let includePrivate = $state(false)
  let timeout = $state('30')
  let saving = $state(false)
  let formError = $state('')

  // «№12 · название»: the number is searchable too. An entity the hook is bound to that the catalog does not list stays pickable.
  function options(list: { id: number; title: string }[]) {
    const out = list.map((e) => ({ id: String(e.id), label: `№${e.id} · ${e.title}` }))
    const bound = hook?.scope_id
    if (bound != null && !out.some((o) => o.id === String(bound))) out.unshift({ id: String(bound), label: `№${bound}` })
    return out
  }
  const questOptions = $derived(options(questCatalog.list))
  const noteOptions = $derived(options(noteCatalog.list))
  const AREAS = [
    { id: 'workspace', label: 'Всё пространство' },
    { id: 'quest', label: 'Квест' },
    { id: 'step', label: 'Шаг' },
    { id: 'note', label: 'Заметка' },
    { id: 'note_tree', label: 'Заметка и вложенные' },
  ]

  $effect(() => {
    if (!open) return
    name = hook?.name ?? ''
    enabled = hook?.enabled ?? true
    events = [...(hook?.events?.length ? hook.events : ['quest_completed'])]
    scriptId = hook ? String(hook.script_id) : ''
    runOn = hook?.run_on ?? ''
    scopeKind = hook?.scope_kind || 'workspace'
    scopeId = hook?.scope_id != null ? String(hook.scope_id) : ''
    cooldown = String(hook?.cooldown_sec ?? 5)
    perHour = String(hook?.max_per_hour ?? 120)
    includePrivate = hook?.include_private ?? false
    timeout = String(hook?.timeout_sec ?? 30)
    formError = ''
  })

  async function onSubmit(e: { preventDefault: () => void }): Promise<void> {
    e.preventDefault()
    const script = Number(scriptId)
    if (!events.length) return void (formError = 'Выберите хотя бы одно событие')
    if (!(script > 0)) return void (formError = 'Выберите скрипт')
    if (scopeKind !== 'workspace' && !(Number(scopeId) > 0)) return void (formError = 'Выберите сущность области хука')
    const body = {
      name: name.trim(),
      enabled,
      events,
      script_id: script,
      run_on: runOn.trim().toLowerCase(),
      timeout_sec: Math.min(600, Math.max(1, Number(timeout) || 30)),
      scope_kind: scopeKind,
      scope_id: scopeKind === 'workspace' ? null : Number(scopeId),
      cooldown_sec: Math.min(86400, Math.max(0, Math.trunc(Number(cooldown) || 0))),
      max_per_hour: Math.min(10000, Math.max(1, Math.trunc(Number(perHour) || 120))),
      include_private: includePrivate,
    }
    saving = true
    formError = ''
    try {
      if (hook) await updateHook(hook.id, body)
      else await createHook(body)
      onSaved()
      onClose()
    } catch (err) {
      formError = err instanceof Error ? err.message : String(err)
    } finally {
      saving = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="hook-modal-title" maxWidth="34rem">
  <ModalHead id="hook-modal-title" title={hook ? 'Редактировать хук' : 'Новый хук'} icon="flag" {onClose} />
  {#if formError}<p class="modal__error">{formError}</p>{/if}
  <form class="modal__form" onsubmit={onSubmit}>
    <HookForm bind:name bind:enabled bind:events bind:scriptId bind:runOn disabled={saving}>
      {#snippet area()}
        <div class="field">
          <span class="label">Где слушать</span>
          <OptionPills options={AREAS} value={scopeKind} label="Область хука" disabled={saving} onToggle={(id) => ((scopeKind = id), (scopeId = ''))} />
        </div>
        {#if scopeKind === 'quest'}
          <div class="field">
            <Picker options={questOptions} bind:value={scopeId} noneLabel="Выберите квест" label="Квест области" disabled={saving} />
          </div>
        {:else if scopeKind === 'note' || scopeKind === 'note_tree'}
          <div class="field">
            <Picker options={noteOptions} bind:value={scopeId} noneLabel="Выберите заметку" label="Заметка области" disabled={saving} />
          </div>
        {:else if scopeKind === 'step'}
          <label class="field">
            <span class="label">Номер шага</span>
            <input type="number" min="1" bind:value={scopeId} aria-label="Номер шага" />
          </label>
        {/if}
      {/snippet}
    </HookForm>
    <div class="hookmodal__limits">
      <label class="field">
        <span class="label">Пауза между запусками, с</span>
        <input type="number" min="0" max="86400" bind:value={cooldown} aria-label="Пауза между запусками, сек" />
      </label>
      <label class="field">
        <span class="label">Запусков в час, не больше</span>
        <input type="number" min="1" max="10000" bind:value={perHour} aria-label="Запусков в час" />
      </label>
    </div>
    <p class="explain">Хук, который превысит число запусков в час, выключается сам: причина видна в его карточке. Заметка не запускает хук чаще раза в 30 секунд.</p>
    {#if scopeKind === 'workspace' || scopeKind === 'note' || scopeKind === 'note_tree'}
      <label class="check">
        <input type="checkbox" bind:checked={includePrivate} disabled={saving} />
        события приватных заметок тоже
      </label>
    {/if}
    <label class="field">
      <span class="label">Время на выполнение, сек</span>
      <input type="number" min="1" max="600" bind:value={timeout} aria-label="Таймаут, сек" />
    </label>
    <ModalFoot onCancel={onClose} submitLabel={hook ? 'Сохранить' : 'Создать'} submitIcon="save" busy={saving} />
  </form>
</ModalShell>

<style>
  .hookmodal__limits {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.6rem;
  }
</style>
