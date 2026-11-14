<script lang="ts">
  import { untrack } from 'svelte'
  let { params, constant = false, onChange }: { params?: { type?: string; value?: unknown; from?: string; to?: string; mode?: string; round?: string }; constant?: boolean; onChange: (p: unknown) => void } = $props()
  const types = ['str', 'int', 'float', 'bool', 'doc']
  let type = $state(untrack(() => params?.type ?? 'str'))
  let value = $state(untrack(() => JSON.stringify(params?.value ?? '')))
  let from = $state(untrack(() => params?.from ?? 'str'))
  let to = $state(untrack(() => params?.to ?? 'int'))
  let mode = $state(untrack(() => params?.mode ?? 'value'))
  let round = $state(untrack(() => params?.round ?? 'exact'))
  let error = $state('')
  function commit() {
    try {
      if (constant) onChange({ type, value: JSON.parse(value) })
      else onChange({ from, to, mode, round })
      error = ''
    } catch { error = 'Введите корректное JSON-значение' }
  }
</script>
{#if constant}
  <label class="field"><span class="label">Тип</span><select bind:value={type} onchange={commit}>{#each types as t}<option>{t}</option>{/each}</select></label>
  <label class="field"><span class="label">Значение JSON</span><textarea bind:value onchange={commit} aria-label="Значение константы"></textarea></label>
{:else}
  <label class="field"><span class="label">Из типа</span><select bind:value={from} onchange={commit}>{#each types as t}<option>{t}</option>{/each}</select></label>
  <label class="field"><span class="label">В тип</span><select bind:value={to} onchange={commit}>{#each types as t}<option>{t}</option>{/each}</select></label>
  <label class="field"><span class="label">Режим</span><select bind:value={mode} onchange={commit}><option value="value">Значение</option><option value="parse_json">Разобрать JSON (str → doc)</option><option value="stringify">Сериализовать JSON (doc → str)</option></select></label>
  <label class="field"><span class="label">Округление float → int</span><select bind:value={round} onchange={commit}><option value="exact">Точное (дробь — ошибка)</option><option value="floor">Вниз</option><option value="ceil">Вверх</option><option value="trunc">К нулю</option><option value="nearest">Ближайшее, половины от нуля</option></select></label>
{/if}
{#if error}<p class="insp__err">{error}</p>{/if}
