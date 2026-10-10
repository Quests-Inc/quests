<script>
  import { fileKind } from '../js/fileKind.ts'

  /** @type {{ att: any, size?: number }} */
  let { att, size = 18 } = $props()
  let k = $derived(fileKind(att))
</script>

<span class="fki" style:--fki-size="{size}px" style:color={k.color} title="{k.label}{k.ext ? ` · ${k.ext}` : ''}" role="img" aria-label={k.label}>
  <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    {#if k.kind === 'word'}
      <path d="M9.5 1.75h-5a1 1 0 0 0-1 1v10.5a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1v-8.5z"/><path d="M9.5 1.75v3h3M5.5 7.5l1 4 1.5-3 1.5 3 1-4"/>
    {:else if k.kind === 'doc'}
      <path d="M9.5 1.75h-5a1 1 0 0 0-1 1v10.5a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1v-8.5z"/><path d="M9.5 1.75v3h3M6 8h4M6 10.5h4"/>
    {:else if k.kind === 'sheet'}
      <rect x="2.25" y="2.25" width="11.5" height="11.5" rx="1"/><path d="M2.25 6.25h11.5M2.25 9.75h11.5M7 6.25v7.5"/>
    {:else if k.kind === 'image'}
      <rect x="2" y="2.75" width="12" height="10.5" rx="1"/><circle cx="5.75" cy="6.25" r="1"/><path d="M2.5 12l3.75-3.5 2.5 2.25 2-1.75 3.25 3"/>
    {:else if k.kind === 'code'}
      <path d="M5.5 4.5L2 8l3.5 3.5M10.5 4.5L14 8l-3.5 3.5M9 3.5l-2 9"/>
    {:else if k.kind === 'archive'}
      <rect x="2.25" y="2.25" width="11.5" height="11.5" rx="1"/><path d="M8 2.25v3M8 7v1.5M8 10v1.5M6.5 5.5h3"/>
    {:else if k.kind === 'db'}
      <ellipse cx="8" cy="4" rx="5" ry="2"/><path d="M3 4v8c0 1.1 2.24 2 5 2s5-.9 5-2V4M3 8c0 1.1 2.24 2 5 2s5-.9 5-2"/>
    {:else if k.kind === 'media'}
      <rect x="2" y="2.75" width="12" height="10.5" rx="1"/><path d="M6.75 5.75v4.5l3.75-2.25z"/>
    {:else}
      <circle cx="8" cy="8" r="6"/><path d="M6.25 6.25a1.75 1.75 0 1 1 2.5 1.6c-.5.25-.75.6-.75 1.15M8 11v.01"/>
    {/if}
  </svg>
</span>

<style>
  .fki {
    display: inline-flex;
    flex-shrink: 0;
    width: var(--fki-size, 18px);
    height: var(--fki-size, 18px);
  }
  .fki svg {
    width: 100%;
    height: 100%;
    display: block;
  }
</style>
