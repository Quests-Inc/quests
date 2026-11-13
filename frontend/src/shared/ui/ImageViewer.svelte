<script lang="ts">
 import {untrack} from 'svelte'
 import { imageViewer as viewer } from '@/shared/lib/imageViewer.svelte.ts'
 let dialog=$state<HTMLDivElement>()
 let area=$state<HTMLDivElement>()
 let scale=$state(1), x=$state(0), y=$state(0)
 let natural=$state({w:0,h:0}), size=$state({w:1,h:1})
 let loading=$state(true), failed=$state(false), retry=$state(0)
 const image=$derived(viewer.images[viewer.index])
 const fit=$derived(Math.min(size.w/(natural.w||1),size.h/(natural.h||1),1))
 const w=$derived(natural.w*fit),h=$derived(natural.h*fit)
 const points=new Map<number,{x:number;y:number}>()
 let gesture:{x:number;y:number;distance:number;scale:number;panX:number;panY:number}|null=null
 let start:{x:number;y:number}|null=null, moved=false, pinched=false, backgroundDown=false
 function loaded(event:Event){const img=event.currentTarget as HTMLImageElement;natural={w:img.naturalWidth,h:img.naturalHeight};loading=false}
 function close(){viewer.open=false}
 function reset(){scale=1;x=0;y=0}
 function step(delta:number){viewer.index=Math.max(0,Math.min(viewer.images.length-1,viewer.index+delta))}
 function clamp(){x=Math.max(-Math.max(0,(w*scale-size.w)/2),Math.min(Math.max(0,(w*scale-size.w)/2),x));y=Math.max(-Math.max(0,(h*scale-size.h)/2),Math.min(Math.max(0,(h*scale-size.h)/2),y))}
 function zoom(next:number,px=0,py=0){next=Math.min(8,Math.max(1,next));const ratio=next/scale;x=px-(px-x)*ratio;y=py-(py-y)*ratio;scale=next;clamp()}
 function pointerDown(e:PointerEvent){
  if(e.button!==0 || e.target instanceof Element && e.target.closest('button'))return
  area?.setPointerCapture?.(e.pointerId);points.set(e.pointerId,{x:e.clientX,y:e.clientY})
  if(points.size===1){start={x:e.clientX,y:e.clientY};moved=false;pinched=false;backgroundDown=e.target===area}
  const p=[...points.values()];const center=p.length===2?{x:(p[0].x+p[1].x)/2,y:(p[0].y+p[1].y)/2}:p[0]
  gesture={...center,distance:p.length===2?Math.hypot(p[0].x-p[1].x,p[0].y-p[1].y):0,scale,panX:x,panY:y}
  if(p.length===2)pinched=true
 }
 function pointerMove(e:PointerEvent){
  if(!points.has(e.pointerId)||!gesture)return
  points.set(e.pointerId,{x:e.clientX,y:e.clientY});const p=[...points.values()]
  if(start&&Math.hypot(e.clientX-start.x,e.clientY-start.y)>5)moved=true
  if(p.length===2&&gesture.distance){zoom(gesture.scale*Math.hypot(p[0].x-p[1].x,p[0].y-p[1].y)/gesture.distance)}
  else if(p.length===1&&scale>1){x=gesture.panX+e.clientX-gesture.x;y=gesture.panY+e.clientY-gesture.y;clamp()}
 }
 function pointerUp(e:PointerEvent){
  points.delete(e.pointerId)
  if(!points.size){
   if(!pinched&&scale===1&&start&&Math.abs(e.clientX-start.x)>60&&Math.abs(e.clientY-start.y)<60)step(e.clientX<start.x?1:-1)
   else if(!moved&&!pinched&&backgroundDown)close()
   start=null;gesture=null
  } else {const p=[...points.values()][0];gesture={...p,distance:0,scale,panX:x,panY:y}}
 }
 function key(e:KeyboardEvent){
  if(!viewer.open)return
  if(['Escape','ArrowLeft','ArrowRight','+','=','-','0','Home','Tab'].includes(e.key)){e.preventDefault();e.stopPropagation()}
  if(e.key==='Escape')close()
  if(e.key==='ArrowLeft')step(-1)
  if(e.key==='ArrowRight')step(1)
  if(e.key==='+'||e.key==='=')zoom(scale*1.4)
  if(e.key==='-')zoom(scale/1.4)
  if(e.key==='0'||e.key==='Home')reset()
  if(e.key==='Tab'){
   const buttons=[...dialog?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')||[]]
   const i=buttons.indexOf(document.activeElement as HTMLButtonElement)
   buttons[(i+(e.shiftKey?-1:1)+buttons.length)%buttons.length]?.focus()
  }
 }
 $effect(()=>{if(!viewer.open)return;window.addEventListener('keydown',key,true);const previous=document.activeElement;const overflow=document.body.style.overflow;document.body.style.overflow='hidden';dialog?.focus();return()=>{window.removeEventListener('keydown',key,true);document.body.style.overflow=overflow;if(previous instanceof HTMLElement&&previous.isConnected)previous.focus()}})
 $effect(()=>{void image?.src;void retry;reset();natural={w:0,h:0};loading=true;failed=false;points.clear()})
 $effect(()=>{if(!area)return;const observer=new ResizeObserver(entries=>{const rect=entries[0].contentRect;untrack(()=>{size={w:rect.width,h:rect.height};clamp()})});observer.observe(area);return()=>observer.disconnect()})
</script>
{#if viewer.open && image}
 <div class="image-viewer" role="dialog" aria-modal="true" aria-label="Просмотр изображения" tabindex="-1" bind:this={dialog} data-own-escape>
  <header><span class="image-title">{image.title}</span><span>{viewer.index+1} / {viewer.images.length}</span>
   <button type="button" onclick={()=>zoom(scale/1.4)} aria-label="Уменьшить" disabled={scale<=1}>−</button>
   <button type="button" onclick={reset} aria-label="Вписать изображение">{Math.round(scale*100)}%</button>
   <button type="button" onclick={()=>zoom(scale*1.4)} aria-label="Увеличить" disabled={scale>=8}>+</button>
   <button type="button" onclick={close} aria-label="Закрыть просмотр">✕</button></header>
  <div class="image-area" bind:this={area} onpointerdown={pointerDown} onpointermove={pointerMove} onpointerup={pointerUp} onpointercancel={()=>{points.clear();gesture=null;start=null}} onwheel={e=>{e.preventDefault();const rect=area!.getBoundingClientRect();zoom(scale*Math.exp(-e.deltaY*.002),e.clientX-rect.left-size.w/2,e.clientY-rect.top-size.h/2)}} role="presentation" ondblclick={()=>scale>1?reset():zoom(2)}>
   {#key image.src+retry}
    <img src={image.src} alt={image.title} draggable="false" style:width={`${w}px`} style:height={`${h}px`} style:transform={`translate(-50%,-50%) translate(${x}px,${y}px) scale(${scale})`} style:visibility={loading||failed?'hidden':'visible'} onload={loaded} onerror={()=>{failed=true;loading=false}}/>
   {/key}
   {#if loading}<span class="image-status" role="status">Загрузка…</span>{/if}
   {#if failed}<div class="image-status" role="alert">Не удалось загрузить изображение <button type="button" onclick={()=>retry++}>Повторить</button></div>{/if}
  </div>
  <button type="button" class="image-nav image-nav--prev" onclick={()=>step(-1)} disabled={viewer.index===0} aria-label="Предыдущее изображение">‹</button>
  <button type="button" class="image-nav image-nav--next" onclick={()=>step(1)} disabled={viewer.index===viewer.images.length-1} aria-label="Следующее изображение">›</button>
  <footer>Колесо / + − — масштаб · перетаскивание — перемещение · ← → — изображения · Esc — закрыть</footer>
 </div>
{/if}
<style>
 .image-viewer{position:fixed;inset:0;z-index:2000000;background:color-mix(in srgb,var(--color-bg) 96%,transparent);color:var(--color-fg);display:grid;grid-template-rows:auto minmax(0,1fr) auto;outline:none}
 header{display:flex;align-items:center;gap:.5rem;padding:.6rem 1rem;background:var(--color-bg-raised);z-index:2}.image-title{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 button{background:transparent;color:inherit;border:0;min-width:2.5rem;min-height:2.5rem;border-radius:.4rem;cursor:pointer}button:hover{background:var(--color-bg-muted)}button:focus-visible{outline:2px solid var(--color-accent)}button:disabled{opacity:.25;cursor:default}
 .image-area{position:relative;min-height:0;overflow:hidden;touch-action:none;user-select:none}.image-area img{position:absolute;left:50%;top:50%;max-width:none;max-height:none;object-fit:contain;cursor:grab}.image-status{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;gap:1rem;pointer-events:none}.image-status button{pointer-events:auto}
 .image-nav{position:absolute;top:50%;font-size:2.5rem;background:color-mix(in srgb,var(--color-bg) 65%,transparent);z-index:2}.image-nav--prev{left:.5rem}.image-nav--next{right:.5rem}footer{text-align:center;padding:.4rem;color:var(--color-fg-muted);font-size:.75rem}
 @media(max-width:40rem){header{gap:.1rem;padding:.3rem}.image-title{font-size:.8rem}footer{font-size:.65rem}.image-nav{min-width:2rem}}
</style>
