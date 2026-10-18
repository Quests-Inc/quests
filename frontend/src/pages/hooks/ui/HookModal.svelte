<script lang="ts">
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import Picker from '@/shared/ui/Picker.svelte'
  import { questCatalog } from '@/entities/quest'
  import { HookForm } from '@/entities/hook'
  import { createHook, getHookEvents, updateHook, type Hook } from '@/shared/api/client.ts'

  /** One hook, created or edited: events, script, runner, the quest it is limited to, the time it may take. */
  let {
    open,
    hook,
    onClose,
    onSaved,
  }: { open: boolean; hook: Hook | null; onClose: () => void; onSaved: () => void } = $props()

  let aliases = $state<Record<string, string[]>>({})
  let name = $state('')
  let enabled = $state(true)
  let events = $state<string[]>([])
  let scriptId = $state('')
  let runOn = $state('')
  /** the quest the hook is limited to ('' = every quest of the journal) */
  let questId = $state('')
  let timeout = $state('30')
  let saving = $state(false)
  let formError = $state('')

  // «№12 · название»: the number is searchable too. A quest the hook is bound to that the catalog does not list stays pickable.
  const questOptions = $derived.by(() => {
    const list = questCatalog.list.map((q) => ({ id: String(q.id), label: `№${q.id} · ${q.title}` }))
    const bound = hook?.quest_id
    if (bound != null && !list.some((o) => o.id === String(bound))) list.unshift({ id: String(bound), label: `№${bound}` })
    return list
  })

  $effect(() => {
    if (!open) return
    name = hook?.name ?? ''
    enabled = hook?.enabled ?? true
    events = [...(hook?.events_raw?.length ? hook.events_raw : (hook?.events ?? ['complete']))]
    scriptId = hook ? String(hook.script_id) : ''
    runOn = hook?.run_on ?? ''
    questId = hook?.quest_id != null ? String(hook.quest_id) : ''
    timeout = String(hook?.timeout_sec ?? 30)
    formError = ''
    if (!Object.keys(aliases).length) void getHookEvents().then((c) => (aliases = c.aliases ?? {})).catch(() => {})
  })

  async function onSubmit(e: { preventDefault: () => void }): Promise<void> {
    e.preventDefault()
    const script = Number(scriptId)
    if (!events.length) return void (formError = 'Выберите хотя бы одно событие')
    if (!(script > 0)) return void (formError = 'Выберите скрипт')
    const body = {
      name: name.trim(),
      enabled,
      events,
      script_id: script,
      run_on: runOn.trim().toLowerCase(),
      timeout_sec: Math.min(600, Math.max(1, Number(timeout) || 30)),
    }
    saving = true
    formError = ''
    try {
      if (hook) await updateHook(hook.id, { ...body, quest_id: questId ? Number(questId) : null })
      else await createHook({ ...body, quest_id: questId ? Number(questId) : null })
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
    <HookForm bind:name bind:enabled bind:events bind:scriptId bind:runOn {aliases} disabled={saving} />
    <div class="field">
      <span class="label">Квест</span>
      <Picker options={questOptions} bind:value={questId} noneLabel="Не привязан: весь журнал" label="Квест, к которому привязан хук" disabled={saving} />
    </div>
    <label class="field">
      <span class="label">Время на выполнение, сек</span>
      <input type="number" min="1" max="600" bind:value={timeout} aria-label="Таймаут, сек" />
    </label>
    <ModalFoot onCancel={onClose} submitLabel={hook ? 'Сохранить' : 'Создать'} submitIcon="save" busy={saving} />
  </form>
</ModalShell>
