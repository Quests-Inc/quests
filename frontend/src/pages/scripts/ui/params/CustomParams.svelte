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
  /** Code and other long text: a block that can be stretched, not a one-line input. */
  const block = (f: CustomField) => f.type === 'str' && (f.param === 'text' || String(fieldValue(f, values)).length > 60 || String(fieldValue(f, values)).includes('\n'))
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
      {:else if block(f)}
        <textarea class="mono block" rows="10" spellcheck="false" value={String(fieldValue(f, values))} onchange={(e) => set(f, e.currentTarget.value)}></textarea>
      {:else}
        <input type="text" value={String(fieldValue(f, values))} onchange={(e) => set(f, e.currentTarget.value)} />
      {/if}
    {/if}
  </label>
{:else}
  <p class="explain">У этой ноды нет редактируемых полей.</p>
{/each}

<style>
  .block {
    width: 100%;
    font-size: var(--text-xs, 0.75rem);
    line-height: 1.4;
    resize: vertical;
    white-space: pre;
    overflow: auto;
  }
</style>
