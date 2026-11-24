<script lang="ts">
  import type { PDFDocumentProxy, PDFDocumentLoadingTask, RenderTask } from 'pdfjs-dist'
  // Legacy build: it polyfills Map#getOrInsertComputed. The modern worker throws
  // "getOrInsertComputed is not a function" in browsers that do not have it yet.
  // oxlint-disable-next-line import/default -- Vite returns an asset URL for the worker.
  import workerUrl from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url'
  let { blob }: { blob: Blob } = $props()
  let pdf = $state.raw<PDFDocumentProxy | null>(null)
  let canvas = $state<HTMLCanvasElement | null>(null)
  let container = $state<HTMLDivElement | null>(null)
  let page = $state(1)
  let width = $state(600)
  let loading = $state(false)
  let error = $state('')
  $effect(()=>{
    const node=container;if(!node)return
    const observer=new ResizeObserver(entries=>{width=Math.max(150,Math.floor(entries[0].contentRect.width)-24)})
    observer.observe(node);return()=>observer.disconnect()
  })
  $effect(()=>{
    const source=blob
    let disposed=false
    let task:PDFDocumentLoadingTask|undefined
    loading=true;error='';pdf=null;page=1
    void (async()=>{
      try{
        const lib = (await import('pdfjs-dist/legacy/build/pdf.mjs')) as typeof import('pdfjs-dist')
        lib.GlobalWorkerOptions.workerSrc=workerUrl
        const data=new Uint8Array(await source.arrayBuffer())
        if(disposed)return
        task=lib.getDocument({data,useWasm:false})
        const document=await task.promise
        if(disposed){await document.cleanup();return}
        pdf=document
      }catch(e){if(!disposed)error=e instanceof Error?e.message:String(e)}
      finally{if(!disposed)loading=false}
    })()
    return()=>{disposed=true;void task?.destroy()}
  })
  $effect(()=>{
    const document=pdf,node=canvas,index=page,available=width
    if(!document||!node)return
    let disposed=false
    let task:RenderTask|undefined
    void (async()=>{
      try{
        const p=await document.getPage(index)
        if(disposed)return
        const initial=p.getViewport({scale:1})
        const scale=Math.min(2,window.devicePixelRatio||1)
        const viewport=p.getViewport({scale:available/initial.width*scale})
        node.width=Math.floor(viewport.width);node.height=Math.floor(viewport.height)
        node.style.width=`${Math.floor(viewport.width/scale)}px`;node.style.height=`${Math.floor(viewport.height/scale)}px`
        task=p.render({canvas:node,viewport})
        await task.promise
      }catch(e){if(!disposed)error=e instanceof Error?e.message:String(e)}
    })()
    return()=>{disposed=true;task?.cancel()}
  })
</script>
<div class="preview-pages" bind:this={container}>
  {#if pdf}
    <nav aria-label="Страницы предпросмотра">
      <button type="button" class="btn btn--ghost" onclick={()=>page--} disabled={page<=1}>Назад</button>
      <span>Страница {page} / {pdf.numPages}</span>
      <button type="button" class="btn btn--ghost" onclick={()=>page++} disabled={page>=pdf.numPages}>Далее</button>
    </nav>
  {/if}
  {#if loading}<p role="status">Открываем PDF…</p>{/if}
  {#if error}<p role="alert">Не удалось показать PDF: {error}</p>{/if}
  <div class="page-scroll"><canvas bind:this={canvas} aria-label={`Предпросмотр страницы ${page}`}></canvas></div>
</div>
<style>
 .preview-pages{flex:1;min-height:0;min-width:0;display:flex;flex-direction:column;border:1px solid var(--color-border);border-radius:var(--radius-md);overflow:hidden;background:var(--color-bg)}
 nav{display:flex;justify-content:center;align-items:center;gap:.5rem;padding:.5rem;flex-wrap:wrap;font-size:.85rem}
 .page-scroll{flex:1;overflow:auto;min-height:10rem;text-align:center;padding:.5rem}
 canvas{display:block;margin:auto;max-width:100%}
 p{padding:.75rem;margin:0}
</style>
