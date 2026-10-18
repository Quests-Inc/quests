<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import { EnvName } from '../model/env.ts'
  import type { ScriptEditor } from '../model/editor.svelte.ts'
  import ChoiceParams from './params/ChoiceParams.svelte'
  import CodeParams from './params/CodeParams.svelte'
  import CompareParams from './params/CompareParams.svelte'
  import CustomParams from './params/CustomParams.svelte'
  import ParseParams from './params/ParseParams.svelte'
  import SourceParams from './params/SourceParams.svelte'

  /** What the selected node or link is made of, and its parameters. */
  let { ed, onEditCustom }: { ed: ScriptEditor; onEditCustom?: (type: string) => void } = $props()

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

<aside class="insp formscope" aria-label="Свойства">
  {#if node}
    {@const meta = ed.meta(node.type)}
    <div class="insp__head">
      <h2>{meta.title} <small>№{node.id}</small></h2>
      <div class="detail__actions">
        <button type="button" class="btn btn--icon btn--danger" title="Удалить ноду" aria-label="Удалить ноду" onclick={() => void ed.remove()}><Icon name="delete" /></button>
      </div>
    </div>
    <p class="explain">{node.type}{meta.trusted ? '' : ' · исполняется только на раннере'}</p>
    {#if ed.portsOf(node.id)?.error}<p class="insp__err">{ed.portsOf(node.id)?.error}</p>{/if}
    {#key node.id}
      {#if node.type.startsWith('custom.')}
        <CustomParams fields={ed.infos.find((i) => i.name === node.type)?.fields ?? []} params={node.params as never} onChange={set} />
        {#if ed.infos.find((i) => i.name === node.type)?.description}<p class="explain">{ed.infos.find((i) => i.name === node.type)?.description}</p>{/if}
        {#if onEditCustom}<button type="button" class="btn btn--ghost" onclick={() => onEditCustom(node.type)}>Править ноду…</button>{/if}
      {:else if node.type === 'code'}
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
      {:else if node.type.startsWith('source.') && node.type !== 'source.workspace'}
        <SourceParams params={node.params as never} onChange={set} />
      {:else}
        <p class="explain">Параметров нет.</p>
      {/if}
    {/key}
    {#each ed.problemNodes.get(node.id) ?? [] as m, i (i)}<p class="insp__err">{m}</p>{/each}
  {:else if edge && ed.selectedEdge != null}
    <div class="insp__head">
      <h2>Связь</h2>
      <div class="detail__actions">
        <button type="button" class="btn btn--icon btn--danger" title="Удалить связь" aria-label="Удалить связь" onclick={() => void ed.remove()}><Icon name="delete" /></button>
      </div>
    </div>
    <p class="explain">№{edge.from.node}.{edge.from.port} → №{edge.to.node}.{edge.to.port}</p>
    {#if intoCode}
      {@const index = ed.selectedEdge}
      <label class="field">
        <span class="label">Псевдоним переменной</span>
        <input
          type="text"
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
      <p class="explain">В окружении: <code>{EnvName(edge)}</code></p>
    {/if}
  {:else}
    <div class="insp__head"><h2>Скрипт</h2></div>
    <p class="explain">
      Выберите ноду или связь. Тяните от выхода (справа) ко входу (слева), чтобы связать. Колесо — масштаб, фон тянется,
      <kbd>V</kbd> — выделение рамкой, <kbd>Del</kbd> — удалить, <kbd>Ctrl+Z</kbd> — отменить.
    </p>
    {#if ed.error}<p class="insp__err">{ed.error}</p>{/if}
  {/if}
</aside>

<style>
  .insp {
    flex: 0 0 18rem;
    overflow: auto;
    display: grid;
    align-content: start;
    gap: 0.6rem;
    padding: 0 0.3rem 0.3rem;
  }
  .insp__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }
  h2 {
    margin: 0;
    font-size: var(--text-md, 1rem);
  }
  small {
    color: var(--color-fg-muted, #9a9a9a);
    font-weight: 400;
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
