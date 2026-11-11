<script lang="ts">
  import Icon from './Icon.svelte'
  let { label, onClear }: { label: string; onClear: () => void } = $props()
</script>
<span class="filter-chip"><span>{label}</span><button type="button" onclick={onClear} aria-label={`Снять фильтр: ${label}`} title="Снять фильтр"><Icon name="close" size={12}/></button></span>
<style>
  .filter-chip { display:inline-flex; align-items:center; gap:0.35rem; min-width:0; max-width:100%; padding:0.2rem 0.35rem 0.2rem 0.55rem; border:1px solid var(--color-border); border-radius:var(--radius-md); font-size:var(--text-xs); }
  .filter-chip > span { overflow-wrap:anywhere; }
  button { flex:none; display:grid; place-items:center; width:2rem; height:2rem; padding:0; border:0; border-radius:var(--radius-md); background:transparent; color:var(--color-fg-muted); cursor:pointer; }
  button:hover { background:var(--color-bg-hover); color:var(--color-fg); }
  button:focus-visible { outline:2px solid var(--color-accent); }
</style>
