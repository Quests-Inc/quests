<script lang="ts">
  /** `json.get`: one value out of a JSON document by a dotted path (`current.temperature_2m`, `items.0.name`). */
  type P = { path?: string; type?: string }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  const TYPES = [
    { id: 'str', label: 'строка' },
    { id: 'int', label: 'целое' },
    { id: 'float', label: 'дробное' },
    { id: 'bool', label: 'да/нет' },
    { id: 'doc', label: 'документ' },
  ]
  let path = $state(params?.path ?? '')
  let type = $state(params?.type ?? 'str')
  const bad = $derived(path !== '' && !/^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$/.test(path.trim()))

  function commit() {
    if (!path.trim() || bad) return
    onChange({ path: path.trim(), type })
  }
</script>

<label class="field">
  <span class="label">Путь</span>
  <input type="text" bind:value={path} onchange={commit} placeholder="current.temperature_2m" aria-label="Путь" />
</label>
{#if bad}<p class="insp__err">Части пути из букв, цифр, «_» и «-», через точку; номер элемента массива — числом.</p>{/if}
<label class="field">
  <span class="label">Тип</span>
  <select bind:value={type} onchange={commit} aria-label="Тип значения">
    {#each TYPES as o (o.id)}<option value={o.id}>{o.label}</option>{/each}
  </select>
</label>
<p class="explain">Нет такого пути или тип другой: значение не приходит, причина идёт на выход «bad».</p>
