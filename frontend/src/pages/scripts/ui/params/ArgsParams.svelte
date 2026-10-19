<script lang="ts">
  import Icon from '@/shared/ui/Icon.svelte'
  /** The named arguments an embedded call passes in (`script=N{…}`): a row per argument. */
  type Arg = { name: string; type: string }
  let { params, onChange }: { params: { params?: Arg[] } | undefined; onChange: (p: unknown) => void } = $props()

  let rows = $state<Arg[]>((params?.params ?? []).map((a) => ({ ...a })))
  const TYPES = ['str', 'int', 'float', 'bool']
  const commit = () => onChange({ params: rows.map((a) => ({ name: a.name.trim(), type: a.type })) })
  function add() {
    rows = [...rows, { name: `arg${rows.length + 1}`, type: 'str' }]
    commit()
  }
  function drop(i: number) {
    rows = rows.filter((_, j) => j !== i)
    commit()
  }
</script>

<div class="rows">
  {#each rows as a, i (i)}
    <div class="row">
      <input type="text" bind:value={a.name} onchange={commit} aria-label="Имя аргумента" placeholder="имя" />
      <select bind:value={a.type} onchange={commit} aria-label="Тип аргумента">
        {#each TYPES as t (t)}<option value={t}>{t}</option>{/each}
      </select>
      <button type="button" class="btn btn--icon btn--ghost" onclick={() => drop(i)} title="Убрать аргумент" aria-label="Убрать аргумент"><Icon name="close" /></button>
    </div>
  {/each}
</div>
<button type="button" class="btn btn--ghost add" onclick={add}><Icon name="add" /> аргумент</button>
<p class="explain">Текст из <code>script=N&#123;…&#125;</code> попадает в первый аргумент; каждый аргумент — выход ноды.</p>

<style>
  .rows { display: grid; gap: 0.4rem; }
  .row { display: flex; gap: 0.4rem; align-items: center; min-width: 0; }
  .row input { flex: 1 1 auto; min-width: 0; }
  .row select { flex: 0 0 5rem; width: 5rem; }
  .add { justify-self: start; }
</style>
