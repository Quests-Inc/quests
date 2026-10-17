<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
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
    <div class="pfield">
      <div class="pfield__top">
        <input type="text" class="name" bind:value={f.name} onchange={commit} aria-label="Имя поля" placeholder="имя" />
        <button type="button" class="btn btn--icon btn--ghost" onclick={() => drop(i)} title="Убрать поле" aria-label="Убрать поле">
          <Icon name="close" />
        </button>
      </div>
      <div class="pfield__opts">
        <select bind:value={f.type} onchange={commit} aria-label="Тип поля">
          {#each TYPES as t (t)}<option value={t}>{t}</option>{/each}
        </select>
        {#if f.type === 'int' || f.type === 'float'}
          <input class="min" type="number" bind:value={f.min} onchange={commit} aria-label="Минимум" placeholder="мин" />
        {/if}
        <label class="check" title="Обязательное: без него ответ отвергается">
          <input type="checkbox" bind:checked={f.required} onchange={commit} />обязательное
        </label>
      </div>
    </div>
  {/each}
</div>
<button type="button" class="btn btn--ghost add" onclick={add}><Icon name="add" /> поле</button>
{#if schema.length === 1}
  <label class="check" title="Весь ответ — значение этого поля (так читается документ, который сам и есть ответ)">
    <input type="checkbox" bind:checked={whole} onchange={commit} />
    весь ответ — значение поля
  </label>
{/if}
<p class="explain">Ответ — JSON: весь вывод или его последняя строка. Одиночное значение попадает в первое поле. Необязательное поле с неверным значением считается отсутствующим.</p>

<style>
  .rows {
    display: grid;
    gap: 0.5rem;
  }
  .pfield {
    display: grid;
    gap: 0.4rem;
    padding: 0.5rem;
    border-radius: var(--pill-radius, 12px);
    background: var(--pill-bg, #0f0f0f);
    box-shadow: var(--pill-shadow, none);
  }
  .pfield__top,
  .pfield__opts {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
  }
  .pfield__top .name {
    flex: 1 1 auto;
  }
  .pfield__opts select {
    flex: 1 1 5rem;
    width: auto;
  }
  .pfield__opts .min {
    flex: 0 0 4.5rem;
    width: 4.5rem;
  }
  .pfield__opts .check {
    flex: none;
  }
  .add {
    justify-self: start;
  }
</style>
