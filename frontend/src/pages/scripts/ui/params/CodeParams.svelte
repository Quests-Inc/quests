<script lang="ts">
  /** The code of a Code node: where it runs is the runner; the language only picks an interpreter line when the text has none. */
  import Icon from '@/shared/ui/Icon.svelte'

  let {
    params,
    onChange,
  }: { params: { language?: string; text?: string; vars?: Record<string, string> } | undefined; onChange: (p: unknown) => void } = $props()

  let language = $state(params?.language ?? '')
  let text = $state(params?.text ?? '')
  // Fixed variables of the job: a row each (kept as rows so a half-typed name does not collapse into the others).
  let vars = $state<{ name: string; value: string }[]>(Object.entries(params?.vars ?? {}).map(([name, value]) => ({ name, value })))
  const LANGS = [
    { id: '', label: 'sh' },
    { id: 'bash', label: 'bash' },
    { id: 'python', label: 'python' },
    { id: 'node', label: 'node' },
  ]
  function commit() {
    const fixed: Record<string, string> = {}
    for (const v of vars) if (v.name.trim()) fixed[v.name.trim().toUpperCase()] = v.value
    onChange(Object.keys(fixed).length ? { language, text, vars: fixed } : { language, text })
  }
  function addVar() {
    vars = [...vars, { name: '', value: '' }]
  }
  function dropVar(i: number) {
    vars = vars.filter((_, j) => j !== i)
    commit()
  }
</script>

<label class="field">
  <span class="label">Язык</span>
  <select bind:value={language} onchange={commit} aria-label="Язык">
    {#each LANGS as l (l.id)}<option value={l.id}>{l.label}</option>{/each}
  </select>
</label>
<label class="field">
  <span class="label">Текст</span>
  <textarea class="mono code" rows="12" spellcheck="false" bind:value={text} onchange={commit} aria-label="Текст кода"></textarea>
</label>
<div class="field">
  <span class="label">Переменные</span>
  {#each vars as v, i (i)}
    <div class="var">
      <input type="text" class="var__name" bind:value={v.name} onchange={commit} placeholder="ИМЯ" aria-label="Имя переменной" />
      <input type="text" bind:value={v.value} onchange={commit} placeholder="значение" aria-label="Значение переменной" />
      <button type="button" class="btn btn--icon btn--ghost" onclick={() => dropVar(i)} title="Убрать переменную" aria-label="Убрать переменную">
        <Icon name="close" />
      </button>
    </div>
  {/each}
  <button type="button" class="btn btn--ghost" onclick={addVar}><Icon name="add" size={14} /> Переменная</button>
</div>
<p class="explain">
  Значения со связей приходят в окружение: <code>QUESTS_IN_N&lt;номер&gt;_&lt;ПОРТ&gt;</code> или псевдоним связи. Выход
  <code>stdout</code> — вывод при коде 0 (JSON). Выход <code>failed</code> — любой другой исход документом:
  <code>kind</code> (<code>failed</code>, <code>timeout</code>, <code>unreachable</code>, <code>abandoned</code>,
  <code>canceled</code>), <code>message</code>, а у ошибки ещё <code>exit_code</code> и <code>stderr</code>. Разберите его нодой
  «Разбор» и ветвите по <code>kind</code>. Исполняется только на раннере.
</p>

<style>
  .var {
    display: flex;
    gap: 0.3rem;
    margin-bottom: 0.3rem;
  }
  .var input {
    flex: 1 1 auto;
    min-width: 0;
  }
  .var .var__name {
    flex: 0 0 8rem;
    text-transform: uppercase;
  }
  .code {
    width: 100%;
    font-size: var(--text-xs, 0.75rem);
    line-height: 1.4;
    resize: vertical;
  }
</style>
