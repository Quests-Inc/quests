<script lang="ts">
  /** Which named value a script writes to, and its type: the type fixes what the `value` input accepts. */
  type P = { name?: string; type?: string }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  const TYPES = [
    { id: 'int', label: 'целое' },
    { id: 'float', label: 'дробное' },
    { id: 'str', label: 'строка' },
    { id: 'bool', label: 'да/нет' },
  ]
  const NAME = /^[a-z][a-z0-9_]{0,23}$/

  let name = $state(params?.name ?? '')
  let type = $state(params?.type ?? 'str')
  const bad = $derived(name !== '' && (!NAME.test(name) || name === 'expr' || name === 'true' || name === 'false'))

  function commit() {
    if (!NAME.test(name)) return
    onChange({ name, type })
  }
</script>

<label class="field">
  <span class="label">Имя значения</span>
  <input type="text" bind:value={name} onchange={commit} maxlength="24" placeholder="hp" aria-label="Имя значения" />
</label>
{#if bad}<p class="insp__err">Строчные латинские, цифры и _, до 24 символов, не expr, true, false.</p>{/if}
<label class="field">
  <span class="label">Тип</span>
  <select bind:value={type} onchange={commit} aria-label="Тип значения">
    {#each TYPES as o (o.id)}<option value={o.id}>{o.label}</option>{/each}
  </select>
</label>
<p class="explain">Пишет в значения сущности из входа «ref». Значение-формулу не затирает: нода сообщит ошибку.</p>
