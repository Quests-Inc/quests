<script lang="ts">
  import { untrack } from 'svelte'
  type P = { table?: string; row?: string; heads?: string }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()
  let table = $state(untrack(() => params?.table ?? ''))
  let row = $state(untrack(() => params?.row ?? ''))
  let heads = $state(untrack(() => params?.heads ?? ''))
  const commit = () => onChange({ table: table.trim(), row: row.trim(), heads })
</script>

<label class="field"><span class="label">ID таблицы</span><input aria-label="ID таблицы" bind:value={table} onchange={commit} /></label>
<label class="field"><span class="label">ID строки</span><input aria-label="ID строки" bind:value={row} onchange={commit} /></label>
<label class="field">
  <span class="label">Головные заметки</span>
  <textarea aria-label="Головные заметки" bind:value={heads} onchange={commit}></textarea>
</label>
<p class="explain">
  Список через запятую или с новой строки. note=12 — только эта заметка. note=12+ и note=12+1 — она и прямые дочки,
  note=12+2 — ещё одно поколение, note=12+! — все поколения ниже. В строке ищется ссылка note= на текущую заметку.
</p>
