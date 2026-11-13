<script lang="ts">
  import { untrack } from 'svelte'
  type P = { table?: string; column?: number; omit?: string[] }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()
  let table = $state(untrack(() => params?.table ?? ''))
  let column = $state(untrack(() => params?.column ?? 0))
  let omit = $state(untrack(() => (params?.omit ?? []).join('\n')))
  const commit = () => onChange({ table: table.trim(), column, omit: omit.split('\n').filter(Boolean) })
</script>
<label class="field"><span class="label">ID таблицы</span><input aria-label="ID таблицы" bind:value={table} onchange={commit} /></label>
<label class="field"><span class="label">Номер колонки (0 — строки целиком)</span><input type="number" min="0" max="100" step="1" aria-label="Номер колонки" bind:value={column} onchange={commit} /></label>
<label class="field"><span class="label">Исключить значения (по одному на строку)</span><textarea aria-label="Исключённые значения" bind:value={omit} onchange={commit}></textarea></label>
<p class="explain">Читает исходный Markdown без запуска вставок. Нужны уникальные table-id и row-id. Выход rows — строки с id и cells; items — строки либо варианты выбранной колонки с id и value. Колонки считаются с 1. Вход column переопределяет номер из параметров. Исключения применяются к выбранной колонке.</p>
