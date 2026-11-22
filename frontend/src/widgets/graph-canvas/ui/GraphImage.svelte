<script lang="ts">
  import { untrack } from 'svelte'
  import { enqueueMedia } from '@/shared/lib/mediaQueue.ts'
  import { imageLevel } from '../model/imageQuality.ts'

  let { id, pixels, hold = false, paced = false }: { id: number | string; pixels: number; hold?: boolean; paced?: boolean } =
    $props()
  let quality = $state(untrack(() => imageLevel(pixels)))
  let failed = $state(false)
  let src = $state('')
  let releaseSlot = () => {}
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
    if (!paced) {
      src = next
      return
    }
    src = ''
    let open: (() => void) | null = null
    const cancel = enqueueMedia((done) => {
      open = done
      releaseSlot = () => {
        open?.()
        open = null
        releaseSlot = () => {}
      }
      src = next
    })
    return () => {
      cancel()
      releaseSlot()
    }
  })
</script>

{#if failed}
  <button type="button" class="image-fallback" onpointerdown={(e) => e.stopPropagation()} onclick={() => { quality = 0; failed = false }}>Загрузить оригинал</button>
{:else if src}
  <img src={src} alt="" decoding="async" draggable="false" onload={releaseSlot} onerror={() => { failed = true; releaseSlot() }} />
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
