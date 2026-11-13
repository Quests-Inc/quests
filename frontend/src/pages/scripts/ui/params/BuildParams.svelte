<script lang="ts">
  import { untrack } from 'svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  type Input = { name: string; type: string }
  type P = { inputs?: Input[]; template?: unknown }
  let { params, text = false, onChange }: { params: P | undefined; text?: boolean; onChange: (p: unknown) => void } = $props()
  let inputs = $state<Input[]>(untrack(() => (params?.inputs ?? []).map(i => ({ ...i }))))
  let template = $state(untrack(() => text ? String(params?.template ?? '') : JSON.stringify(params?.template ?? {}, null, 2)))
  let error = $state('')
  function commit() {
    try {
      const value = text ? template : JSON.parse(template)
      onChange({ inputs: inputs.map(i => ({ name: i.name.trim(), type: i.type })), template: value })
      error = ''
    } catch { error = 'Шаблон должен быть корректным JSON' }
  }
  function add() { inputs = [...inputs, { name: `value${inputs.length + 1}`, type: 'str' }]; commit() }
</script>
{#each inputs as input, i (i)}
  <div class="build-input">
    <input aria-label="Имя входа" bind:value={input.name} onchange={commit} />
    <select aria-label="Тип входа" bind:value={input.type} onchange={commit}>
      {#each ['str', 'int', 'float', 'bool', 'doc'] as type (type)}<option value={type}>{type}</option>{/each}
    </select>
    <button type="button" class="btn btn--icon btn--ghost" aria-label="Убрать вход" onclick={() => { inputs = inputs.filter((_, j) => i !== j); commit() }}><Icon name="close" /></button>
  </div>
{/each}
<button type="button" class="btn btn--ghost" onclick={add}>Добавить вход</button>
<label class="field"><span class="label">{text ? 'Текстовый шаблон' : 'JSON-шаблон'}</span><textarea aria-label="Шаблон" rows="8" bind:value={template} onchange={commit} spellcheck="false"></textarea></label>
{#if error}<p role="alert">{error}</p>{/if}
{#if text}<p class="explain">Подстановка: <code>{'{{имя}}'}</code>. Значения вставляются как текст.</p>
{:else}<p class="explain">Подстановка целого значения: <code>{'{"$input":"имя"}'}</code>. Остальные поля — константы. Типы и экранирование сохраняются.</p>{/if}
<style>.build-input { display: flex; gap: 0.35rem; } .build-input input { min-width: 0; flex: 1; } .build-input select { width: 5rem; }</style>
