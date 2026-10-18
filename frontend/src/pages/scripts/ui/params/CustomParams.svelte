<script lang="ts">
  import { fieldValue } from '@/entities/script'
  import type { CustomField } from '@/shared/api/scriptgraph.gen.ts'

  /** The fields of a custom node: what its author chose to leave editable. The node's own values, else the baked ones. */
  let {
    fields,
    params,
    onChange,
  }: { fields: CustomField[]; params: Record<string, unknown> | undefined; onChange: (p: unknown) => void } = $props()

  let values = $state<Record<string, unknown>>({ ...(params ?? {}) })

  function set(f: CustomField, v: unknown) {
    values = { ...values, [f.name]: v }
    onChange(values)
  }
  const num = (s: string) => (s.trim() === '' ? 0 : Math.trunc(Number(s)))
</script>

{#each fields as f (f.name)}
  <label class={f.type === 'bool' ? 'check' : 'field'}>
    {#if f.type === 'bool'}
      <input type="checkbox" checked={Boolean(fieldValue(f, values))} onchange={(e) => set(f, e.currentTarget.checked)} />
      {f.label || f.name}
    {:else}
      <span class="label">{f.label || f.name}</span>
      {#if f.type === 'int'}
        <input type="number" step="1" value={String(fieldValue(f, values))} onchange={(e) => set(f, num(e.currentTarget.value))} />
      {:else}
        <input type="text" value={String(fieldValue(f, values))} onchange={(e) => set(f, e.currentTarget.value)} />
      {/if}
    {/if}
  </label>
{:else}
  <p class="explain">У этой ноды нет редактируемых полей.</p>
{/each}
