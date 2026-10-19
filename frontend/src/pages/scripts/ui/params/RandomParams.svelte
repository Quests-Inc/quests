<script lang="ts">
  /** A Random node: a number in a range, or a pick from a list; repeated `count` times and folded by `aggregate`. */
  type P = { kind?: string; lo?: number; hi?: number; count?: number; aggregate?: string }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  let kind = $state(params?.kind ?? 'int')
  let lo = $state(params?.lo === undefined ? '' : String(params.lo))
  let hi = $state(params?.hi === undefined ? '' : String(params.hi))
  let count = $state(params?.count === undefined ? '' : String(params.count))
  let aggregate = $state(params?.aggregate ?? (params?.kind === 'pick' ? 'first' : 'sum'))

  const KINDS = [
    { id: 'int', label: 'целое в диапазоне' },
    { id: 'float', label: 'дробное в диапазоне' },
    { id: 'pick', label: 'выбор из списка' },
  ]
  const aggregates = $derived(
    kind === 'pick'
      ? [{ id: 'first', label: 'один элемент' }, { id: 'list', label: 'список' }]
      : [{ id: 'sum', label: 'сумма' }, { id: 'min', label: 'минимум' }, { id: 'max', label: 'максимум' }, { id: 'list', label: 'список' }]
  )
  function onKind() {
    if (!aggregates.some((a) => a.id === aggregate)) aggregate = aggregates[0].id
    commit()
  }
  function commit() {
    const out: P = { kind, aggregate }
    if (kind !== 'pick') {
      if (lo !== '') out.lo = Number(lo)
      if (hi !== '') out.hi = Number(hi)
    }
    if (count !== '') out.count = Number(count)
    onChange(out)
  }
</script>

<label class="field">
  <span class="label">Что бросать</span>
  <select bind:value={kind} onchange={onKind} aria-label="Вид">
    {#each KINDS as k (k.id)}<option value={k.id}>{k.label}</option>{/each}
  </select>
</label>
{#if kind !== 'pick'}
  <label class="field">
    <span class="label">От</span>
    <input type="number" step="any" bind:value={lo} onchange={commit} placeholder="пусто — со входа «lo»" aria-label="От" />
  </label>
  <label class="field">
    <span class="label">До (включительно)</span>
    <input type="number" step="any" bind:value={hi} onchange={commit} placeholder="пусто — со входа «hi»" aria-label="До" />
  </label>
{/if}
<label class="field">
  <span class="label">Сколько раз</span>
  <input type="number" min="1" max="1000" bind:value={count} onchange={commit} placeholder="1, или со входа «count»" aria-label="Сколько раз" />
</label>
<label class="field">
  <span class="label">Результат повторов</span>
  <select bind:value={aggregate} onchange={commit} aria-label="Результат повторов">
    {#each aggregates as a (a.id)}<option value={a.id}>{a.label}</option>{/each}
  </select>
</label>
<p class="explain">Не больше 1000 повторов в ноде. {kind === 'pick' ? 'Список приходит на вход «list» (JSON-массив).' : 'Границы — параметры здесь или входы ноды.'}</p>
