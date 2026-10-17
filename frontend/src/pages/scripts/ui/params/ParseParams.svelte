<script lang="ts">
  /** The schema a Parse node reads its input by: a row per field. A single value stands for the first field. */
  type Field = { name: string; type: string; required?: boolean; min?: number }
  let { params, onChange }: { params: { schema?: Field[]; whole?: boolean } | undefined; onChange: (p: unknown) => void } = $props()

  let schema = $state<Field[]>((params?.schema ?? []).map((f) => ({ ...f })))
  let whole = $state(Boolean(params?.whole))
  const TYPES = ['int', 'float', 'str', 'bool', 'doc']

  function commit() {
    const fields = schema.map((f) => {
      const out: Field = { name: f.name.trim(), type: f.type }
      if (f.required) out.required = true
      if ((f.type === 'int' || f.type === 'float') && f.min !== undefined && f.min !== null && String(f.min) !== '') out.min = Number(f.min)
      return out
    })
    onChange(whole ? { schema: fields, whole: true } : { schema: fields })
  }
  function add() {
    schema = [...schema, { name: `field${schema.length + 1}`, type: 'int' }]
    commit()
  }
  function drop(i: number) {
    schema = schema.filter((_, j) => j !== i)
    commit()
  }
</script>

<div class="rows">
  {#each schema as f, i (i)}
    <div class="row">
      <input class="name" bind:value={f.name} onchange={commit} aria-label="Имя поля" placeholder="имя" />
      <select bind:value={f.type} onchange={commit} aria-label="Тип поля">
        {#each TYPES as t (t)}<option value={t}>{t}</option>{/each}
      </select>
      {#if f.type === 'int' || f.type === 'float'}
        <input class="min" type="number" bind:value={f.min} onchange={commit} aria-label="Минимум" placeholder="мин" />
      {/if}
      <label class="check" title="Обязательное: без него ответ отвергается"><input type="checkbox" bind:checked={f.required} onchange={commit} />обяз.</label>
      <button type="button" class="btn btn--ghost" onclick={() => drop(i)} aria-label="Убрать поле">×</button>
    </div>
  {/each}
</div>
<button type="button" class="btn" onclick={add}>+ поле</button>
{#if schema.length === 1}
  <label class="check whole" title="Весь ответ — значение этого поля (так читается документ, который сам и есть ответ)">
    <input type="checkbox" bind:checked={whole} onchange={commit} />
    весь ответ — значение поля
  </label>
{/if}
<p class="note">Ответ — JSON: весь вывод или его последняя строка. Одиночное значение попадает в первое поле. Необязательное поле с неверным значением считается отсутствующим.</p>

<style>
  .rows {
    display: grid;
    gap: 0.3rem;
    margin-bottom: 0.4rem;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.3rem;
  }
  .name {
    flex: 1 1 6rem;
    min-width: 0;
  }
  .min {
    width: 4rem;
  }
  .whole {
    display: block;
    margin-top: 0.4rem;
  }
  .note {
    margin: 0.4rem 0 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
</style>
