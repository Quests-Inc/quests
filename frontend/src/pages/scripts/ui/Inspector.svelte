<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  import type { ScriptUse } from '@/shared/api/execution.gen.ts'
  import { openTarget } from '@/shared/lib/openTarget.ts'
  import { EnvName } from '../model/env.ts'
  import type { ScriptEditor } from '../model/editor.svelte.ts'
  import ChoiceParams from './params/ChoiceParams.svelte'
  import CodeParams from './params/CodeParams.svelte'
  import CompareParams from './params/CompareParams.svelte'
  import ArgsParams from './params/ArgsParams.svelte'
  import ExprParams from './params/ExprParams.svelte'
  import RandomParams from './params/RandomParams.svelte'
  import RegexParams from './params/RegexParams.svelte'
  import CustomParams from './params/CustomParams.svelte'
  import ParseParams from './params/ParseParams.svelte'
  import SourceParams from './params/SourceParams.svelte'
  import ValueSinkParams from './params/ValueSinkParams.svelte'
  import HttpGetParams from './params/HttpGetParams.svelte'
  import HttpRunnerParams from './params/HttpRunnerParams.svelte'
  import JsonGetParams from './params/JsonGetParams.svelte'

  /** What the selected node or link is made of, and its parameters; with nothing selected, where the script is used. */
  let { ed, onEditCustom, uses = [] }: { ed: ScriptEditor; onEditCustom?: (type: string) => void; uses?: ScriptUse[] } = $props()

  const USE_WORDS: Record<string, string> = {
    step: 'Шаг',
    note: 'Заметка',
    template: 'Шаблон',
    template_step: 'Шаг шаблона',
    hook: 'Хук',
    quest: 'Квест',
    questline: 'Квестлайн',
  }
  const bound = $derived(uses.filter((u) => !u.via))
  const texts = $derived(uses.filter((u) => u.via === 'text'))
  function openUse(u: ScriptUse) {
    if (u.subject_type === 'hook') openTarget({ kind: 'hook', id: u.subject_id })
    else openTarget({ kind: 'ref', ref: u.subject_type === 'template_step' ? 'template' : u.subject_type, id: u.subject_id })
  }

  let unfoldError = $state('')
  async function unfold(id: number) {
    unfoldError = ''
    try {
      await ed.unfold(id)
    } catch (e) {
      unfoldError = e instanceof Error ? e.message : String(e)
    }
  }

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
        <button type="button" class="btn btn--ghost" title="Заменить ноду её частями, чтобы изменить их и запечь снова" onclick={() => void unfold(node.id)}>Развернуть в ноды</button>
        {#if unfoldError}<p class="insp__err">{unfoldError}</p>{/if}
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
      {:else if node.type === 'http.get'}
        <HttpGetParams params={node.params as never} onChange={set} />
      {:else if node.type === 'http.runner'}
        <HttpRunnerParams params={node.params as never} onChange={set} />
      {:else if node.type === 'json.get'}
        <JsonGetParams params={node.params as never} onChange={set} />
      {:else if node.type === 'sink.values'}
        <p class="explain">Объект из входа «values» записывается в переменные сущности из «ref»: один ключ — одна переменная. При ошибке не записывается ничего. Формулы не заменяются.</p>
      {:else if node.type === 'sink.value'}
        <ValueSinkParams params={node.params as never} onChange={set} />
      {:else if node.type === 'args'}
        <ArgsParams params={node.params as never} onChange={set} />
      {:else if node.type === 'expr'}
        <ExprParams params={node.params as never} onChange={set} />
      {:else if node.type === 'random'}
        <RandomParams params={node.params as never} onChange={set} />
      {:else if node.type === 'regex'}
        <RegexParams params={node.params as never} onChange={set} />
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
    <div class="insp__uses">
      <h3>Используется</h3>
      {#if !uses.length}
        <p class="explain">Нигде: скрипт не подключён к шагу, заметке, шаблону или хуку и не вставлен в тексты.</p>
      {/if}
      {#each bound as u (u.subject_type + u.subject_id)}
        <button type="button" class="insp__use" onclick={() => openUse(u)}>
          <span class="insp__use-kind">{USE_WORDS[u.subject_type] ?? u.subject_type}</span>
          <span class="insp__use-title">{u.title || u.subject_id}</span>
        </button>
      {/each}
      {#if texts.length}
        <h4>Вставлен в тексты</h4>
        {#each texts as u (u.subject_type + u.subject_id)}
          <button type="button" class="insp__use" onclick={() => openUse(u)}>
            <span class="insp__use-kind">{USE_WORDS[u.subject_type] ?? u.subject_type}</span>
            <span class="insp__use-title">{u.title || u.subject_id}</span>
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</aside>

<style>
  .insp__uses {
    display: grid;
    gap: 0.2rem;
  }
  .insp__uses h3,
  .insp__uses h4 {
    margin: 0.4rem 0 0.1rem;
    font-size: var(--text-sm, 0.85rem);
  }
  .insp__uses h4 {
    color: var(--color-fg-muted, #9a9a9a);
    font-weight: normal;
  }
  .insp__use {
    display: flex;
    gap: 0.4rem;
    padding: 0.2rem 0.3rem;
    border: 0;
    border-radius: var(--radius-sm, 3px);
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .insp__use:hover {
    background: var(--color-bg-muted, #1e1e1e);
  }
  .insp__use-kind {
    flex: 0 0 auto;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: var(--text-xs, 0.75rem);
  }
  .insp__use-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
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
