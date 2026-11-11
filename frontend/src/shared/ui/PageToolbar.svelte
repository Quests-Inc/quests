<script lang="ts">
  import type { Snippet } from 'svelte'
  let { children }: { children: Snippet } = $props()
</script>
<div class="page-toolbar">{@render children()}</div>
<style>
  .page-toolbar { display:flex; flex-wrap:wrap; align-items:center; gap:0.55rem; min-width:0; }
  .page-toolbar :global(input), .page-toolbar :global(select) { min-height:2.25rem; }
  .page-toolbar :global(.search) { flex:1 1 14rem; min-width:0; }
  .page-toolbar :global(.picker) { flex:0 1 18rem; width:auto; max-width:100%; }
  .page-toolbar :global(.btn) { min-height:2.25rem; }
  .page-toolbar :global(.opt-slider) { min-height:2.25rem; align-items:stretch; flex:0 1 auto; width:max-content; max-width:100%; }
</style>
