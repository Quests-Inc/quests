<script lang="ts">
  import { EnvName } from '../model/env.ts'
  import type { ScriptEditor } from '../model/editor.svelte.ts'
  import ChoiceParams from './params/ChoiceParams.svelte'
  import CodeParams from './params/CodeParams.svelte'
  import CompareParams from './params/CompareParams.svelte'
  import ParseParams from './params/ParseParams.svelte'

  /** What the selected node or link is made of, and its parameters. */
  let { ed }: { ed: ScriptEditor } = $props()

  const node = $derived(ed.selectedNode != null ? ed.graph.nodes.find((n) => n.id === ed.selectedNode) : undefined)
  const edge = $derived(ed.selectedEdge != null ? ed.graph.edges[ed.selectedEdge] : undefined)
  const intoCode = $derived(
    edge ? ed.graph.nodes.find((n) => n.id === edge.to.node)?.type === 'code' && edge.to.port === 'in' : false
  )

  const RESULTS = [
    { id: 'done', label: 'выполнен: шаг закрыт' },
    { id: 'not_started', label: 'не запущен: ждёт устройство' },
    { id: 'stopped', label: 'остановлен: квест не проваливается' },
    { id: 'failed', label: 'провален: квест проваливается' },
  ]
  const KINDS = [
    { id: 'error', label: 'ошибка' },
    { id: 'timeout', label: 'таймаут' },
    { id: 'bad_json', label: 'битый JSON' },
  ]
  const VALUE_TYPES = ['int', 'float', 'str', 'bool', 'doc'].map((t) => ({ id: t, label: t }))

  const set = (p: unknown): void => {
    if (node) void ed.params(node.id, p)
  }
</script>

<aside class="insp" aria-label="Свойства">
  {#if node}
    {@const meta = ed.meta(node.type)}
    <h2>{meta.title} <small>№{node.id}</small></h2>
    <p class="insp__type">{node.type}{meta.trusted ? '' : ' · исполняется только на раннере'}</p>
    {#if ed.portsOf(node.id)?.error}<p class="insp__err">{ed.portsOf(node.id)?.error}</p>{/if}
    {#key node.id}
      {#if node.type === 'code'}
        <CodeParams params={node.params as never} onChange={set} />
      {:else if node.type === 'parse'}
        <ParseParams params={node.params as never} onChange={set} />
      {:else if node.type === 'compare'}
        <CompareParams params={node.params as never} onChange={set} />
      {:else if node.type === 'any_of'}
        <ChoiceParams params={node.params as never} field="type" options={VALUE_TYPES} label="Тип значений" onChange={set} />
      {:else if node.type === 'sink.step_result'}
        <ChoiceParams params={node.params as never} field="result" options={RESULTS} label="Итог" onChange={set} />
      {:else if node.type === 'sink.template_error'}
        <ChoiceParams params={node.params as never} field="kind" options={KINDS} label="Вид ошибки" onChange={set} />
      {:else}
        <p class="insp__none">Параметров нет.</p>
      {/if}
    {/key}
    {#each ed.problemNodes.get(node.id) ?? [] as m, i (i)}<p class="insp__err">{m}</p>{/each}
    <button type="button" class="btn btn--danger-solid" onclick={() => void ed.remove()}>Удалить ноду</button>
  {:else if edge && ed.selectedEdge != null}
    <h2>Связь</h2>
    <p class="insp__type">№{edge.from.node}.{edge.from.port} → №{edge.to.node}.{edge.to.port}</p>
    {#if intoCode}
      {@const index = ed.selectedEdge}
      <label class="field">
        <span class="label">Псевдоним переменной</span>
        <input
          value={edge.alias ?? ''}
          placeholder="например NAME"
          aria-label="Псевдоним"
          onchange={(e) => void ed.edge(index, { alias: e.currentTarget.value.trim().toUpperCase() })}
        />
      </label>
      <label class="check">
        <input type="checkbox" checked={edge.optional ?? false} onchange={(e) => void ed.edge(index, { optional: e.currentTarget.checked })} />
        необязательное значение (код запустится и без него)
      </label>
      <p class="insp__type">В окружении: <code>{EnvName(edge)}</code></p>
    {/if}
    <button type="button" class="btn btn--danger-solid" onclick={() => void ed.remove()}>Удалить связь</button>
  {:else}
    <h2>Скрипт</h2>
    <p class="insp__none">
      Выберите ноду или связь. Тяните от выхода (справа) ко входу (слева), чтобы связать. Колесо — масштаб, фон тянется,
      <kbd>V</kbd> — выделение рамкой, <kbd>Del</kbd> — удалить, <kbd>Ctrl+Z</kbd> — отменить.
    </p>
    {#if ed.error}<p class="insp__err">{ed.error}</p>{/if}
  {/if}
</aside>

<style>
  .insp {
    flex: 0 0 17rem;
    overflow: auto;
    display: grid;
    align-content: start;
    gap: 0.5rem;
    padding-left: 0.3rem;
  }
  h2 {
    margin: 0;
    font-size: var(--text-md, 1rem);
  }
  small {
    color: var(--color-fg-muted, #9a9a9a);
    font-weight: 400;
  }
  .insp__type,
  .insp__none {
    margin: 0;
    font-size: var(--text-xs, 0.78rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
  .insp__err {
    margin: 0;
    font-size: var(--text-xs, 0.78rem);
    color: var(--color-danger, #b54a3a);
  }
  @media (max-width: 960px) {
    .insp {
      flex-basis: auto;
    }
  }
</style>
