<script lang="ts">
  import { scheduleImage } from '@/shared/lib/imageStart.ts'
  import { untrack } from 'svelte'
  import { imageLevel } from '../model/imageQuality.ts'

  let { id, pixels, hold = false, cardKey = '' }: { id: number | string; pixels: number; hold?: boolean; cardKey?: string } = $props()
  let quality = $state(untrack(() => imageLevel(pixels)))
  let failed = $state(false)
  let src = $state('')
  const wanted = $derived(imageLevel(pixels))
  const url = $derived(`/api/attachments/${id}/image?preview=1${quality ? `&thumb=${quality}` : ''}`)

  $effect(() => {
    const next = wanted
    const timer = setTimeout(() => {
      if (next === 0 || (quality !== 0 && next > quality)) {
        quality = next
        failed = false
      }
    }, 160)
    return () => clearTimeout(timer)
  })

  $effect(() => {
    if (hold) {
      src = ''
      failed = false
      return
    }
    const next = url
    return scheduleImage(() => {
      src = next
    }, cardKey)
  })
</script>

{#if failed}
  <button type="button" class="image-fallback" onpointerdown={(e) => e.stopPropagation()} onclick={() => { quality = 0; failed = false }}>Загрузить оригинал</button>
{:else if src}
  <img src={src} alt="" decoding="async" draggable="false" onerror={() => (failed = true)} />
{/if}

<style>
  img {
    display: block;
    width: 100%;
    height: 100%;
    min-height: 0;
    object-fit: contain;
    pointer-events: none;
  }
  .image-fallback {
    font-size: 12px;
    background: transparent;
    color: inherit;
    border: 0;
    padding: 4px;
    cursor: pointer;
  }
</style>
