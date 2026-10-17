<script lang="ts">
  /** A node with one parameter chosen from a short list (the result of a step run, the kind of a template error, the type of an Any of). */
  let {
    params,
    field,
    options,
    label,
    onChange,
  }: {
    params: Record<string, string> | undefined
    field: string
    options: { id: string; label: string }[]
    label: string
    onChange: (p: unknown) => void
  } = $props()

  let value = $state(params?.[field] ?? options[0]?.id ?? '')
</script>

<label class="field">
  <span class="label">{label}</span>
  <select bind:value onchange={() => onChange({ [field]: value })} aria-label={label}>
    {#each options as o (o.id)}<option value={o.id}>{o.label}</option>{/each}
  </select>
</label>
