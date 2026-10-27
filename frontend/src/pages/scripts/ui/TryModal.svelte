<script lang="ts">
  import { onDestroy } from 'svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import { getHookEvents, getRun, tryScript, type RunDetail } from '@/shared/api/client.ts'
  import type { HookEventInfo } from '@/shared/api/hooks.ts'
  import type { Graph } from '@/shared/api/scriptgraph.gen.ts'
  import { loadRunners, runnersList } from '@/entities/runner'
  import { openTarget } from '@/shared/lib/openTarget.ts'

  /**
   * «Запустить на…» (quest=489): the script runs once over an entity the person picks, or a made-up event, or arguments — the
   * way a binding, a hook or a text would run it. By default the sinks write nothing and only show what they would; code
   * still goes to a runner for real. The result is a run in the run log.
   */
  let {
    open,
    scriptId,
    graph,
    dirty,
    subjects,
    needsRunner,
    onClose,
  }: {
    open: boolean
    scriptId: number | null
    graph: Graph
    /** The editor holds changes: the test runs them, not the saved version. */
    dirty: boolean
    subjects: string[]
    needsRunner: boolean
    onClose: () => void
  } = $props()

  const KIND_WORDS: Record<string, string> = { step: 'Шаг', note: 'Заметка', quest: 'Квест', template: 'Шаблон' }
  const argNode = $derived(graph.nodes.find((n) => n.type === 'args'))
  const argNames = $derived(
    ((argNode?.params as { params?: { name: string }[] } | undefined)?.params ?? []).map((p) => p.name).filter(Boolean)
  )
  const eventNode = $derived(graph.nodes.find((n) => n.type === 'source.event'))
  const eventFamily = $derived(((eventNode?.params as { family?: string } | undefined)?.family ?? 'any') as string)

  let kind = $state('note')
  let entityId = $state<number | null>(null)
  let event = $state('')
  let args = $state<Record<string, string>>({})
  let live = $state(false)
  let on = $state('')
  let events = $state<HookEventInfo[]>([])
  let busy = $state(false)
  let error = $state('')
  let run = $state<RunDetail | null>(null)
  let timer: ReturnType<typeof setTimeout> | undefined

  const kinds = $derived(subjects.filter((s) => s in KIND_WORDS).length ? subjects.filter((s) => s in KIND_WORDS) : Object.keys(KIND_WORDS))
  const eventChoices = $derived(eventFamily === 'any' ? events : events.filter((e) => e.family === eventFamily))

  $effect(() => {
    if (!open) return
    error = ''
    run = null
    live = false
    if (!kinds.includes(kind)) kind = kinds[0]
    if (eventNode && !events.length) void getHookEvents().then((c) => (events = c.events ?? [])).catch(() => {})
    if (needsRunner) void loadRunners()
  })
  $effect(() => {
    if (eventNode && !event && eventChoices.length) event = eventChoices[0].kind
  })
  onDestroy(() => clearTimeout(timer))

  async function follow(id: number) {
    try {
      run = await getRun(id)
      if (run.state === 'waiting' && open) timer = setTimeout((): void => void follow(id), 1200)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function start() {
    if (scriptId == null) return
    busy = true
    error = ''
    run = null
    clearTimeout(timer)
    try {
      const body: Parameters<typeof tryScript>[1] = { live }
      if (dirty) body.graph = graph
      if (argNode) body.args = args
      else if (entityId != null && entityId > 0) body.subject = { type: kind, id: entityId }
      if (eventNode && event) body.event = event
      if (needsRunner && on) body.on = on
      const { run: id } = await tryScript(scriptId, body)
      await follow(id)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  const ranSinks = $derived(run ? run.nodes.filter((n) => n.type.startsWith('sink.') && n.state === 'ran').length : 0)
  const failed = $derived(run ? run.nodes.filter((n) => n.state === 'failed') : [])
  const STATE: Record<string, string> = { done: 'готово', failed: 'ошибка', waiting: 'ждёт раннер…' }
</script>

<ModalShell {open} {onClose} labelledby="try-title" maxWidth="34rem">
  <ModalHead id="try-title" title="Запустить на…" icon="target" {onClose} />
  <form
    class="modal__form"
    onsubmit={(e) => {
      e.preventDefault()
      void start()
    }}
  >
    {#if argNode}
      {#each argNames.length ? argNames : ['_'] as name (name)}
        <label class="field">
          <span class="label">{name === '_' ? 'Текст в скобках' : name}</span>
          <input type="text" bind:value={args[name]} placeholder="как в script=N{'{'}…{'}'}" />
        </label>
      {/each}
    {:else}
      <div class="field">
        <span class="label">{eventNode ? 'О чём событие' : 'Что читают источники'}</span>
        <div class="row">
          <select bind:value={kind} aria-label="Вид сущности">
            {#each kinds as k (k)}<option value={k}>{KIND_WORDS[k]}</option>{/each}
          </select>
          <input type="number" min="1" bind:value={entityId} placeholder="номер, например 12" aria-label="Номер сущности" />
        </div>
      </div>
      {#if eventNode}
        <label class="field">
          <span class="label">Событие</span>
          <select bind:value={event}>
            {#each eventChoices as e (e.kind)}<option value={e.kind} title={e.kind}>{e.title}</option>{/each}
          </select>
        </label>
      {/if}
    {/if}
    {#if needsRunner}
      <label class="field">
        <span class="label">Раннер для кода</span>
        <select bind:value={on}>
          <option value="">по умолчанию для пространства</option>
          {#each runnersList.items as r (r.label)}<option value={r.label}>{r.label}{r.online ? '' : ' (не в сети)'}</option>{/each}
        </select>
      </label>
    {/if}
    <label class="check">
      <input type="checkbox" bind:checked={live} />
      По-настоящему: приёмники запишут результат
    </label>
    <p class="explain">
      {live ? 'Запуск изменит данные, как настоящий.' : 'Приёмники ничего не запишут, только покажут, что записали бы.'}
      {needsRunner ? ' Код уходит на раннер и выполняется по-настоящему.' : ''}
      {dirty ? ' Проверяется несохранённый граф.' : ''}
    </p>
    {#if error}<p class="modal__error">{error}</p>{/if}
    {#if run}
      <div class="result result--{run.state}">
        <strong>{STATE[run.state] ?? run.state}</strong>
        {#if run.error}<span>{run.error}</span>{/if}
        {#if run.result}<span>Результат: <code>{run.result}</code></span>{/if}
        {#each failed as n (n.id)}<span>Нода {n.id}: {n.failure}</span>{/each}
        {#if ranSinks}<span>{live ? `Записали приёмники: ${ranSinks}` : `Приёмников сработало бы: ${ranSinks}`}</span>{/if}
        <button type="button" class="btn btn--ghost" onclick={() => { onClose(); openTarget({ kind: 'runs', run: run!.id }) }}>Подробнее в «Запусках»</button>
      </div>
    {/if}
    <ModalFoot onCancel={onClose} cancelLabel="Закрыть" submitLabel={run ? 'Ещё раз' : 'Запустить'} submitIcon="target" {busy} disabled={scriptId == null} />
  </form>
</ModalShell>

<style>
  .row {
    display: flex;
    gap: 0.3rem;
  }
  .row select {
    flex: 0 0 9rem;
    width: 9rem;
  }
  .row input {
    flex: 1 1 0;
    width: auto;
    min-width: 0;
  }
  .check {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .result {
    display: grid;
    gap: 0.25rem;
    margin: 0.25rem 0 0.5rem;
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--color-border, #333);
    border-left-width: 3px;
    border-radius: var(--radius-md, 4px);
    font-size: var(--text-sm, 0.85rem);
  }
  .result--done {
    border-left-color: var(--color-success, #7a9e3a);
  }
  .result--failed {
    border-left-color: var(--color-danger, #b54a3a);
  }
  .result--waiting {
    border-left-color: var(--color-accent, #c9a227);
  }
  .result .btn {
    justify-self: start;
  }
</style>
