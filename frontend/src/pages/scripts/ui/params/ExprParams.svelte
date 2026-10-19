<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'

  /**
   * An Expr node: arithmetic over named numeric inputs, with operators the owner defines. An operator is a symbol, how many operands
   * it takes, its place in the order of operations and a definition: an expression over a (and b) made of the safe core.
   * Without a text here the expression arrives on the `expr` input.
   */
  type Op = { symbol: string; arity: number; prec: number; right?: boolean; def: string }
  type P = { expr?: string; vars?: string[]; operators?: Op[] }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  let expr = $state(params?.expr ?? '')
  let vars = $state((params?.vars ?? []).join(', '))
  let ops = $state<Op[]>((params?.operators ?? []).map((o) => ({ ...o })))

  function commit() {
    const out: P = {
      expr: expr.trim(),
      vars: vars
        .split(/[\s,]+/)
        .map((v) => v.trim())
        .filter(Boolean),
    }
    if (ops.length) out.operators = ops.map((o) => ({ symbol: o.symbol.trim(), arity: Number(o.arity), prec: Number(o.prec), ...(o.right && Number(o.arity) === 2 ? { right: true } : {}), def: o.def }))
    onChange(out)
  }
  function add() {
    ops = [...ops, { symbol: '', arity: 2, prec: 5, def: 'a + b' }]
  }
  function drop(i: number) {
    ops = ops.filter((_, j) => j !== i)
    commit()
  }
  function dice() {
    ops = [
      { symbol: 'к', arity: 2, prec: 5, def: 'sum(a, rand(1, b))' },
      { symbol: 'd', arity: 2, prec: 5, def: 'sum(a, rand(1, b))' },
      { symbol: 'к', arity: 1, prec: 5, def: 'rand(1, a)' },
      { symbol: 'd', arity: 1, prec: 5, def: 'rand(1, a)' },
    ]
    commit()
  }
</script>

<label class="field">
  <span class="label">Выражение</span>
  <input type="text" bind:value={expr} onchange={commit} placeholder="пусто — придёт на вход «expr»" aria-label="Выражение" />
</label>
<label class="field">
  <span class="label">Переменные (числовые входы)</span>
  <input type="text" bind:value={vars} onchange={commit} placeholder="a, b" aria-label="Переменные" />
</label>

<span class="label">Свои операторы</span>
<div class="rows">
  {#each ops as o, i (i)}
    <div class="op">
      <div class="op__top">
        <input class="sym" type="text" maxlength="3" bind:value={o.symbol} onchange={commit} aria-label="Символ оператора" placeholder="к" />
        <select bind:value={o.arity} onchange={commit} aria-label="Операндов">
          <option value={2}>между (a к b)</option>
          <option value={1}>перед (к a)</option>
        </select>
        <button type="button" class="btn btn--icon btn--ghost" onclick={() => drop(i)} title="Убрать оператор" aria-label="Убрать оператор"><Icon name="close" /></button>
      </div>
      <div class="op__opts">
        <label class="check" title="Чем больше, тем раньше выполняется: + и − это 1, * / % это 2">
          приоритет
          <input class="prec" type="number" min="1" max="9" bind:value={o.prec} onchange={commit} aria-label="Приоритет" />
        </label>
        {#if Number(o.arity) === 2}
          <label class="check" title="a ⊕ b ⊕ c считается как a ⊕ (b ⊕ c)"><input type="checkbox" bind:checked={o.right} onchange={commit} />вправо</label>
        {/if}
      </div>
      <input type="text" bind:value={o.def} onchange={commit} aria-label="Определение" placeholder={o.arity === 2 ? 'sum(a, rand(1, b))' : 'rand(1, a)'} spellcheck="false" />
    </div>
  {/each}
</div>
<div class="btns">
  <button type="button" class="btn btn--ghost" onclick={add}><Icon name="add" /> оператор</button>
  <button type="button" class="btn btn--ghost" onclick={dice} title="к и d: NкM — сумма N бросков от 1 до M">кости</button>
</div>

<p class="explain">
  + − * / %, скобки, min max floor ceil round abs rand(от, до) randf(от, до) и повторы sum(n, e), rmax(n, e), rmin(n, e): e считается заново на каждом
  повторе. В определении доступны a (и b); своих операторов внутри определения нет. Выходы: <code>value</code> (число) и <code>text</code> (строка: 24, а не 24.0).
</p>

<style>
  .rows { display: grid; gap: 0.5rem; }
  .op { display: grid; gap: 0.4rem; padding: 0.5rem; border-radius: var(--pill-radius, 12px); background: var(--pill-bg, #0f0f0f); box-shadow: var(--pill-shadow, none); }
  .op__top, .op__opts { display: flex; align-items: center; gap: 0.4rem; min-width: 0; }
  .op__top .sym { flex: 0 0 3.5rem; width: 3.5rem; text-align: center; }
  .op__top select { flex: 1 1 auto; width: auto; }
  .op__opts .prec { width: 3.5rem; flex: none; }
  .btns { display: flex; gap: 0.4rem; }
</style>
