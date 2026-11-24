<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import Icon from './Icon.svelte'
 import PdfPreview from './PdfPreview.svelte'
  import ModalFoot from './ModalFoot.svelte'
  import { renderPdf } from '@/shared/api/exportPdf.ts'
  import { listAllAttachments, uploadAttachment } from '@/shared/api/attachments.ts'
  import { closePdfSettings, writePdfPreferences, type PdfExportSession } from '@/shared/lib/pdfSettings.svelte.ts'
  import { builtinPdfPresets, pdfOptionsError, savePdfBlob, type PdfOptions, type PdfPreset } from '@/shared/lib/pdfOptions.ts'
  import type { PdfDocument } from '@/shared/lib/pdfExport.ts'

  let { session }: { session: PdfExportSession } = $props()
  let draft = $state<PdfOptions>(untrack(()=>({...session.preferences.quick.options})))
  let custom = $state<PdfPreset[]>(untrack(()=>session.preferences.custom.map(p=>({...p,options:{...p.options}}))))
  let presetId=$state('quick')
  let presetName=$state(untrack(()=>session.preferences.quick.name))
  let useForQuick=$state(false)
  let linkMargins=$state(false)
  let book=$state(untrack(()=>!!session.document.book))
  let busy=$state(false)
  let error=$state('')
  let notice=$state('')
  let previewBlob=$state<Blob|null>(null)
  let previewSignature=$state('')
  let uploadInput=$state<HTMLInputElement|null>(null)
  let images=$state<{id:number;filename:string}[]>([])
  const presets=$derived([{...session.preferences.quick,id:'quick',name:session.preferences.quick.name+' (быстрый экспорт)'},...builtinPdfPresets,...custom])
  const validation=$derived(pdfOptionsError(draft))
  const signature=$derived(JSON.stringify({draft,book}))
  const fresh=$derived(!!previewBlob&&signature===previewSignature)
  const format=$derived(draft.width===210&&draft.height===297?'a4':draft.width===148&&draft.height===210?'a5':draft.width===215.9&&draft.height===279.4?'letter':'custom')
  let cachedBook:PdfDocument|null=null
  const close=()=>{if(!busy)closePdfSettings()}
  onMount(async()=>{
    try {
      const groups=await listAllAttachments()
      const seen=new Set<number>()
      const loaded=Object.values(groups).flatMap(owners=>Object.values(owners).flat()).filter(a=>{
        if(seen.has(a.id)||a.hidden||!/^image\/(png|jpeg|gif|webp)$/.test(a.content_type_detected||''))return false
        seen.add(a.id);return true
      }).map(a=>({id:a.id,filename:a.filename}))
      images=[...new Map([...loaded,...images].map(image=>[image.id,image])).values()]
    }catch{notice='Список фоновых изображений недоступен. Остальные настройки работают.'}
  })
  function chooseBackground(event:Event){
    const select=event.currentTarget as HTMLSelectElement
    draft.background_image=Number(select.value)
  }
  async function uploadBackground(event:Event){
    const input=event.currentTarget as HTMLInputElement
    const file=input.files?.[0];input.value=''
    const owner=session.attachmentOwner
    if(!file||!owner||busy)return
    busy=true;error='';notice=''
    try{
      const image=await uploadAttachment(owner.kind,owner.id,file,'Фон экспорта PDF')
      if(!/^image\/(png|jpeg|gif|webp)$/.test(image.content_type_detected||''))throw new Error('Файл сохранён во вложениях, но не поддерживается как фон. Выберите PNG, JPEG, GIF или WebP.')
      images=[...images.filter(item=>item.id!==image.id),{id:image.id,filename:image.filename}]
      draft.background_image=image.id
      notice=owner.kind==='note'?'Изображение загружено вложением к заметке и выбрано фоном.':'Изображение загружено вложением к квесту и выбрано фоном.'
    }catch(e){error=e instanceof Error?e.message:String(e)}finally{busy=false}
  }
  function selectPreset(id:string){
    const p=presets.find(p=>p.id===id);if(!p)return
    draft={...p.options};presetId=p.id;presetName=p.name;error='';notice=''
  }
  function setFormat(value:string){
    const dimensions:Record<string,[number,number]>={a4:[210,297],a5:[148,210],letter:[215.9,279.4]}
    if(dimensions[value]) [draft.width,draft.height]=dimensions[value]
  }
  function allMargins(value:string){if(linkMargins){const n=value===''?NaN:Number(value);draft.top=n;draft.right=n;draft.bottom=n;draft.left=n}}
  function savePreset(){
    if(validation){error=validation;return}
    const name=presetName.trim();if(!name){error='Введите название пресета.';return}
    const existing=custom.find(p=>p.id===presetId)
    if(!existing&&custom.length>=20){error='Можно сохранить до 20 своих пресетов.';return}
    const p:PdfPreset={id:existing?.id??crypto.randomUUID(),name:name.slice(0,64),options:{...draft}}
    const next=existing?custom.map(v=>v.id===p.id?p:v):[...custom,p]
    try{
      writePdfPreferences(session.storageKey,{quick:session.preferences.quick,custom:next})
      custom=next;presetId=p.id;notice='Пресет сохранён в этом браузере.';error=''
    }catch{error='Не удалось сохранить пресет в браузере.'}
  }
  function deletePreset(){
    const next=custom.filter(p=>p.id!==presetId)
    try{
      writePdfPreferences(session.storageKey,{quick:session.preferences.quick,custom:next})
      custom=next;selectPreset('standard');notice='Пресет удалён. Настройки быстрого экспорта сохранены.'
    }catch{error='Не удалось удалить пресет.'}
  }
  async function generate(){
    if(validation)throw new Error(validation)
    if(book&&!session.document.book&&!cachedBook) {
      if(!session.buildBook)throw new Error('Сборка книги недоступна.')
      cachedBook=await session.buildBook()
    }
    const source=book?(session.document.book?session.document:cachedBook!):session.document
    const captured=signature
    const blob=await renderPdf({...source,options:{...draft}},session.filename)
    previewBlob=blob;previewSignature=captured
  }
  async function preview(){
    if(busy)return;busy=true;error=''
    try{await generate()}catch(e){error=e instanceof Error?e.message:String(e)}finally{busy=false}
  }
  async function download(){
    if(busy)return;busy=true;error=''
    try{
      if(!fresh)await generate()
      if(useForQuick){
        const selected=presets.find(p=>p.id===presetId)
        const name=selected&&JSON.stringify(selected.options)===JSON.stringify(draft)?(presetId==='quick'?session.preferences.quick.name:selected.name):'Свои настройки'
        const quick:PdfPreset={id:presetId,name,options:{...draft}}
        writePdfPreferences(session.storageKey,{quick,custom})
      }
      savePdfBlob(previewBlob!,book?session.filename.replace(/^note-/,'book-'):session.filename)
      closePdfSettings(true)
    }catch(e){error=e instanceof Error?e.message:String(e)}finally{busy=false}
  }
</script>

<ModalShell open onClose={close} labelledby="pdf-settings-title" maxWidth="76rem" dialogClass="pdf-settings-modal" closeDisabled={busy} zIndex={100}>
  <ModalHead id="pdf-settings-title" title="Настроить экспорт PDF" onClose={close} closeDisabled={busy} />
  <div class="pdf-layout modal__body">
    <div class="pdf-controls">
      <p class="pdf-document">{session.document.title}</p>
      <fieldset disabled={busy}>
        {#if session.buildBook}
          <label class="field"><span class="label">Состав</span><select bind:value={book}><option value={false}>Только эта заметка</option><option value={true}>Сшить вложенные заметки в книгу</option></select></label>
        {/if}
        <label class="field"><span class="label">Пресет</span><select value={presetId} onchange={e=>selectPreset(e.currentTarget.value)}>{#each presets as p (p.id)}<option value={p.id}>{p.name}</option>{/each}</select></label>
        <div class="preset-save">
          <label class="field"><span class="label">Название своего пресета</span><input bind:value={presetName} maxlength="64" /></label>
          <button type="button" class="btn" onclick={savePreset} disabled={!!validation}>Сохранить</button>
          {#if custom.some(p=>p.id===presetId)}<button type="button" class="btn btn--ghost" onclick={deletePreset}>Удалить</button>{/if}
        </div>
        <p class="hint">Пресеты сохраняются в этом браузере для текущего пользователя и пространства.</p>
        <details open>
          <summary>Страница</summary>
          <div class="settings-grid">
            <label class="field"><span class="label">Формат</span><select value={format} onchange={e=>setFormat(e.currentTarget.value)}><option value="a4">A4</option><option value="a5">A5</option><option value="letter">Letter</option><option value="custom">Свой размер</option></select></label>
            <label class="field"><span class="label">Ориентация</span><select bind:value={draft.landscape}><option value={false}>Книжная</option><option value={true}>Альбомная</option></select></label>
            <label class="field"><span class="label">Ширина листа, мм</span><input type="number" min="80" max="600" step="0.1" bind:value={draft.width} /></label>
            <label class="field"><span class="label">Высота листа, мм</span><input type="number" min="80" max="600" step="0.1" bind:value={draft.height} /></label>
          </div>
          <p class="hint">Размеры задают стороны листа; ориентация определяет их расположение.</p>
        </details>
        <details open>
          <summary>Поля</summary>
          <label class="check"><input type="checkbox" bind:checked={linkMargins} /> Менять все поля вместе</label>
          <div class="settings-grid">
            {#each ([{key:'top',label:'Верхнее'},{key:'right',label:'Правое'},{key:'bottom',label:'Нижнее'},{key:'left',label:'Левое'}] as const) as field (field.key)}
              <label class="field"><span class="label">{field.label}, мм</span><input type="number" min="0" max="100" step="1" bind:value={draft[field.key]} oninput={e=>allMargins(e.currentTarget.value)} /></label>
            {/each}
          </div>
        </details>
        <details open>
          <summary>Текст</summary>
          <div class="settings-grid">
            <label class="field"><span class="label">Шрифт</span><select bind:value={draft.font}><option value="serif">DejaVu Serif — с засечками</option><option value="sans">DejaVu Sans — без засечек</option><option value="mono">DejaVu Mono — моноширинный</option><option value="gothic">CyrillicGothSHA — готический</option></select></label>
            <label class="field"><span class="label">Размер, пт</span><input type="number" min="6" max="24" step="0.5" bind:value={draft.font_size} /></label>
            <label class="field"><span class="label">Межстрочный интервал</span><input type="number" min="1" max="2.5" step="0.05" bind:value={draft.line_spacing} /></label>
            <label class="field"><span class="label">Цвет текста</span><span class="color-input"><input type="color" bind:value={draft.text_color} /></span></label>
          </div>
          <p class="font-sample" style:font-family={draft.font==='gothic'?'PDF Gothic':draft.font==='serif'?'serif':draft.font==='mono'?'monospace':'sans-serif'}>Съешь ещё этих мягких французских булок. 0123456789</p>
        </details>
        <details>
          <summary>Нумерация</summary>
          <div class="settings-grid">
            <label class="field"><span class="label">Формат номера</span><select bind:value={draft.number_format}><option value="none">Выключена</option><option value="page">n</option><option value="total">n/m</option></select></label>
            <label class="field"><span class="label">Положение</span><select bind:value={draft.number_position}><option value="bottom">Снизу</option><option value="top">Сверху</option></select></label>
            <label class="field"><span class="label">Выравнивание</span><select bind:value={draft.number_align}><option value="left">Слева</option><option value="center">По центру</option><option value="right">Справа</option></select></label>
          </div>
        </details>
        <details open>
          <summary>Фон</summary>
          <div class="settings-grid">
            <label class="field"><span class="label">Цвет фона</span><span class="color-input"><input type="color" bind:value={draft.background_color} /></span></label>
            <div class="field">
              <label class="label" for="pdf-background-image">Фоновое изображение</label>
              <div class="background-picker">
                <select id="pdf-background-image" value={draft.background_image} onchange={chooseBackground}><option value={0}>Без изображения</option>{#each images as image (image.id)}<option value={image.id}>{image.filename}</option>{/each}</select>
                {#if session.attachmentOwner}<button type="button" class="btn btn--ghost btn--icon background-upload" onclick={()=>uploadInput?.click()} disabled={busy} aria-label="Загрузить изображение" title="Загрузить изображение вложением"><Icon name="add" size={16} /></button>{/if}
              </div>
            </div>
            <label class="field"><span class="label">Непрозрачность, %</span><input type="number" min="0" max="100" step="5" value={Math.round(draft.background_opacity*100)} oninput={e=>draft.background_opacity=Number(e.currentTarget.value)/100} /></label>
            <label class="field"><span class="label">Размещение</span><select bind:value={draft.background_fit}><option value="contain">Вписать целиком</option><option value="cover">Заполнить с обрезкой</option></select></label>
          </div>
          {#if session.attachmentOwner}
            <input type="file" accept="image/png,image/jpeg,image/gif,image/webp" bind:this={uploadInput} onchange={uploadBackground} hidden aria-label="Загрузить фоновое изображение" />
            <p class="hint">{session.attachmentOwner.kind==='note'?'Изображение будет загружено вложением к этой заметке.':'Изображение будет загружено вложением к этому квесту.'} После загрузки оно автоматически выберется фоном.</p>
          {/if}
          <p class="hint">Фон повторяется на каждой странице. PNG, JPEG, GIF или WebP.</p>
        </details>
        {#if book}
          <details open><summary>Книга</summary>
            <label class="check"><input type="checkbox" bind:checked={draft.chapter_new_page} /> Главы первого уровня с новой страницы</label>
            <label class="check"><input type="checkbox" bind:checked={draft.mirror_margins} /> Зеркальные поля</label>
            <label class="field"><span class="label">Отступ под переплёт, мм</span><input type="number" min="0" max="100" bind:value={draft.binding} /></label>
            <p class="hint">Переплёт добавляется к левому полю; при зеркальных полях на чётных страницах — к правому.</p>
          </details>
        {/if}
        <label class="check"><input type="checkbox" bind:checked={useForQuick} /> Использовать для быстрого экспорта</label>
      </fieldset>
      {#if validation}<p role="alert" class="error">{validation}</p>{/if}
      {#if error}<p role="alert" class="error">{error}</p>{/if}
      {#if notice}<p role="status" class="hint">{notice}</p>{/if}
    </div>
    <section class="pdf-preview" aria-label="Предпросмотр PDF">
      <div class="preview-toolbar"><button type="button" class="btn" onclick={preview} disabled={busy||!!validation}>{busy?'Подготовка…':'Обновить предпросмотр'}</button><span class="hint">{fresh?'Предпросмотр актуален':previewBlob?'Настройки изменились':'Предпросмотр ещё не создан'}</span></div>
      {#if previewBlob}<PdfPreview blob={previewBlob} />{:else}<p class="preview-empty">Настройте оформление и нажмите «Обновить предпросмотр». Он покажет реальные переносы и страницы.</p>{/if}
    </section>
  </div>
  <ModalFoot onCancel={close} submitLabel="Скачать PDF" onSubmit={download} {busy} busyLabel="Подготовка PDF…" disabled={!!validation} />
</ModalShell>

<style>
 @font-face{font-family:"PDF Gothic";src:url("../../assets/fonts/CyrillicGothSHA.ttf") format("truetype");font-display:swap}
  .pdf-layout{display:grid;grid-template-columns:minmax(19rem,27rem) minmax(0,1fr);gap:1.25rem;min-height:0;flex:1;box-sizing:border-box;width:100%;overflow:hidden}
  .pdf-controls{overflow-y:auto;padding-right:.5rem;min-width:0}
  .pdf-document{font-weight:600;overflow-wrap:anywhere;margin:0 0 1rem}
  fieldset{border:0;padding:0;margin:0;display:grid;gap:.9rem;min-width:0}
  details{border-top:1px solid var(--color-border);padding-top:.75rem}
  summary{cursor:pointer;font-weight:600;margin-bottom:.75rem}
  .settings-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:.75rem}
  .pdf-controls{--field-h:calc(var(--text-sm, 0.875rem) * 1.45 + 0.9rem + 2px)}
  .field{min-width:0}.field input,.field select{width:100%;box-sizing:border-box;min-width:0}
  .field input:not([type='color']),.field select{height:var(--field-h)}
  .background-picker{display:flex;align-items:center;gap:.35rem;min-width:0}
  .background-picker select{flex:1;min-width:0}
  .background-upload{flex:none;width:var(--field-h);height:var(--field-h);padding:0;box-sizing:border-box}
  .color-input{display:flex;align-items:center;box-sizing:border-box;width:100%;height:var(--field-h);padding:0 .55rem;border:1px solid var(--color-border, #333);border-radius:var(--radius-md, 4px);background:var(--color-bg, #121212)}
  .color-input input{appearance:none;-webkit-appearance:none;width:100%;height:1.05rem;margin:0;padding:0;border:0;background:transparent;cursor:pointer}
  .color-input input::-webkit-color-swatch-wrapper{padding:0}
  .color-input input::-webkit-color-swatch,.color-input input::-moz-color-swatch{border:0;border-radius:2px}
  .color-input:focus-within{outline:1px solid var(--color-accent, #c9a227);outline-offset:1px}
  .color-input input:focus-visible{outline:none}
  .check{display:flex;align-items:center;gap:.5rem;margin:.5rem 0;font-size:.9rem}
  .check input{flex:none}.hint{font-size:.8rem;color:var(--color-fg-muted);line-height:1.4}
  .preset-save{display:flex;align-items:end;gap:.5rem}.preset-save .field{flex:1}
  .preset-save>.btn{height:var(--field-h);padding-block:0;box-sizing:border-box}
  .font-sample{font-size:1rem;padding:.5rem;border:1px solid var(--color-border);border-radius:var(--radius-md);margin-bottom:0}
  .pdf-preview{min-width:0;display:flex;flex-direction:column;gap:.75rem;min-height:22rem}
  .preview-toolbar{display:flex;align-items:center;gap:.75rem;flex-wrap:wrap}
  :global(.pdf-settings-modal){display:flex;flex-direction:column;box-sizing:border-box;overflow:hidden;min-width:0;height:min(90vh,54rem)}
 :global(.pdf-settings-modal > .modal__foot){margin:0;flex-shrink:0}
 :global(.pdf-settings-modal > .modal__head){flex-shrink:0}
  .preview-empty{display:grid;place-content:center;text-align:center;flex:1;border:1px dashed var(--color-border);padding:2rem;color:var(--color-fg-muted)}
  .error{color:var(--color-danger)}
  @media(max-width:800px){.pdf-layout{grid-template-columns:1fr;overflow-y:auto;max-height:70vh}.pdf-controls{overflow:visible}.pdf-preview{min-height:28rem}.preset-save{flex-wrap:wrap}}
</style>
