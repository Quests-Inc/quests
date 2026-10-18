<script lang="ts">
  /** A source node: the id of the entity it reads. Empty = the entity the script is bound to (what it was before the id). */
  type P = { id?: number }
  let { params, onChange }: { params: P | undefined; onChange: (p: unknown) => void } = $props()

  let id = $state(params?.id ? String(params.id) : '')

  function commit() {
    const n = Math.trunc(Number(id))
    onChange(n > 0 ? { id: n } : undefined)
  }
</script>

<label class="field">
  <span class="label">Номер сущности</span>
  <input type="number" min="1" step="1" bind:value={id} onchange={commit} placeholder="та, к которой привязан скрипт" aria-label="Номер сущности" />
</label>
<p class="explain">Свой номер у каждого источника: можно читать несколько сущностей в одном скрипте. Секреты берутся у этой же сущности.</p>
