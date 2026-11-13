<script lang="ts">
  import {untrack} from 'svelte'
  import {imageLevel} from '../model/imageQuality.ts'
  let {id,pixels}: {id:number|string;pixels:number}=$props()
  let quality=$state(untrack(()=>imageLevel(pixels)))
  let failed=$state(false)
  const wanted=$derived(imageLevel(pixels))
  $effect(()=>{const next=wanted;const timer=setTimeout(()=>{if(next===0||quality!==0&&next>quality){quality=next;failed=false}},160);return()=>clearTimeout(timer)})
</script>
{#if failed}
 <button type="button" class="image-fallback" onpointerdown={e=>e.stopPropagation()} onclick={()=>{quality=0;failed=false}}>Загрузить оригинал</button>
{:else}
 <img src="/api/attachments/{id}/image?preview=1{quality?`&thumb=${quality}`:''}" alt="" loading="lazy" decoding="async" draggable="false" onerror={()=>failed=true}/>
{/if}
<style>img{display:block;width:100%;height:100%;min-height:0;object-fit:contain;pointer-events:none}.image-fallback{font-size:12px;background:transparent;color:inherit;border:0;padding:4px;cursor:pointer}</style>
