<script lang="ts">
  import type { Snippet } from 'svelte'
  let { title = '', description = '', toolbar, children }: { title?: string; description?: string; toolbar?: Snippet; children: Snippet } = $props()
</script>
<section class="page-layout">
  {#if title || toolbar}
    <header class="page-layout__head">
      {#if title}<h1>{title}</h1>{/if}
      {#if description}<p class="hint">{description}</p>{/if}
      {@render toolbar?.()}
    </header>
  {/if}
  <div class="page-layout__content">{@render children()}</div>
</section>
<style>
  .page-layout { display:flex; flex-direction:column; flex:1 1 auto; width:100%; min-width:0; min-height:0; height:100%; font-family:var(--font-ui); }
  .page-layout__head { flex:none; padding:0.75rem 1rem; border-bottom:1px solid var(--color-border); display:grid; gap:0.55rem; }
  h1 { margin:0; color:var(--color-fg); font-family:var(--font-display); font-size:var(--text-lg); font-weight:600; }
  p { margin:0; }
  .page-layout__content { display:flex; flex-direction:column; flex:1 1 auto; min-width:0; min-height:0; overflow:auto; container-type:inline-size; }
  @media(max-width:40rem) { .page-layout__head { padding:0.75rem; } }
</style>
