<script lang="ts">
  import { COMPARE_OPS } from '@/entities/script'

  /** A Compare node: the value goes to `yes` or to `no`, depending on how it compares with the operand. */
  type P = { type?: string; op?: string; operand?: unknown }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  let type = $state(params?.type ?? 'int')
  let op = $state(params?.op ?? 'eq')
  let operand = $state<string | boolean>(
    typeof params?.operand === 'boolean' ? params.operand : params?.operand === undefined ? '' : String(params.operand)
  )
  const TYPES = ['int', 'float', 'str', 'bool']
  const ops = $derived(COMPARE_OPS[type] ?? [])

  function typed(): unknown {
    if (type === 'bool') return Boolean(operand)
    if (type === 'str') return String(operand)
    return operand === '' ? 0 : Number(operand)
  }
  function onType() {
    if (!ops.some((o) => o.id === op)) op = ops[0]?.id ?? 'eq'
    operand = type === 'bool' ? true : type === 'str' ? '' : '0'
    commit()
  }
  const commit = () => onChange({ type, op, operand: typed() })
</script>

<label class="field">
  <span class="label">Тип входа</span>
  <select bind:value={type} onchange={onType} aria-label="Тип входа">
    {#each TYPES as t (t)}<option value={t}>{t}</option>{/each}
  </select>
</label>
<label class="field">
  <span class="label">Операция</span>
  <select bind:value={op} onchange={commit} aria-label="Операция">
    {#each ops as o (o.id)}<option value={o.id}>{o.label}</option>{/each}
  </select>
</label>
<label class="field">
  <span class="label">Операнд</span>
  {#if type === 'bool'}
    <input type="checkbox" checked={Boolean(operand)} onchange={(e) => { operand = e.currentTarget.checked; commit() }} aria-label="Операнд" />
  {:else if type === 'str'}
    <input type="text" bind:value={operand} onchange={commit} aria-label="Операнд" placeholder={op === 'matches' ? 'RE2: ^Ш.г$' : 'текст'} />
  {:else}
    <input type="number" step={type === 'int' ? 1 : 'any'} bind:value={operand} onchange={commit} aria-label="Операнд" />
  {/if}
</label>
<p class="explain">Значение уходит на выход «yes», если условие верно, иначе на «no»; другой выход получает сигнал выхода, и его ветка не выполняется.</p>
