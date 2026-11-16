<script lang="ts">
  import { attachmentImage, showImages } from '@/shared/lib/imageViewer.svelte.ts'
  import { isAudioFile } from '@/shared/lib/audioFile.ts'
  import { fetchId3Tags } from '@/shared/api/audioTags.ts'
  import { audioSrc, trackOf } from '@/shared/lib/player.svelte.ts'
  import PlayFile from '@/shared/ui/PlayFile.svelte'
  import { onMount } from 'svelte'
  import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'
  import { getAttachmentPreview } from '@/shared/api/attachments.ts'
  import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
  import { fileKind } from '@/shared/lib/fileKind.ts'
  import { fileCodeLanguage, highlightCode } from '@/shared/lib/codeHighlight.ts'
  import { extOf, isRaster, isTextPreview, thumbSrc } from '../model/list.ts'

  let { a, gallery = [] }: { a: AttachmentView; gallery?: AttachmentView[] } = $props()
  let host = $state<HTMLDivElement | null>(null)
  let visible = $state(false)
  let excerpt = $state('')
  let kind = $state('')
  let requestedKey = $state('')
  let status = $state<'idle' | 'loading' | 'done'>('idle')
  let coverUrl = $state('')
  let fileKey = $derived(JSON.stringify([a.id, a.revision || 1, a.uploaded_at]))
  let manual = $derived(!isRaster(a) && !isTextPreview(a) && !isAudioFile(a))
  let canPreview = $derived(a.available !== false && a.scan_status === 'clean')
  let imageFailed = $state(false)
  let language = $derived(kind === 'text' ? fileCodeLanguage(a.filename) : '')
  let highlighted = $derived(language ? highlightCode(excerpt, language) : '')
  const LABELS: Record<string, string> = {
    pdf: 'Первая страница PDF', text: 'Начало файла', office: 'Текст документа', zip: 'Файлы в архиве',
  }

  onMount(() => {
    if (!host) return
    if (typeof IntersectionObserver === 'undefined') { visible = true; return }
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) {
        visible = true
        observer.disconnect()
      }
    }, { rootMargin: '150px' })
    observer.observe(host)
    return () => observer.disconnect()
  })

  $effect(() => {
    void fileKey
    requestedKey = ''
  })

  $effect(() => {
    const id = a.id, revision = a.revision || 1
    const requested = requestedKey === fileKey
    const needsClick = manual
    const allowed = canPreview
    imageFailed = false
    excerpt = ''
    kind = ''
    status = 'idle'
    if (isRaster(a) || isAudioFile(a) || !allowed || (needsClick ? !requested : !visible)) return
    status = 'loading'
    const abort = new AbortController()
    void getAttachmentPreview(id, revision, abort.signal).then((result) => {
      if (!abort.signal.aborted) { excerpt = result.text; kind = result.kind }
    }).catch(() => { /* Keep the icon on failure. */ }).finally(() => {
      if (!abort.signal.aborted) status = 'done'
    })
    return () => abort.abort()
  })

  $effect(() => {
    void fileKey
    coverUrl = ''
    if (!isAudioFile(a) || !canPreview || !visible) return
    const ac = new AbortController()
    let objectUrl = ''
    const src = audioSrc({
      id: Number(a.id),
      filename: a.filename || '',
      hidden: !!a.hidden,
      revision: Number(a.revision) || 0,
    })
    void fetchId3Tags(src, ac.signal)
      .then((tags) => {
        if (ac.signal.aborted || !tags.cover) return
        objectUrl = URL.createObjectURL(tags.cover)
        coverUrl = objectUrl
      })
      .catch(() => {})
    return () => {
      ac.abort()
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  })
</script>

<div class="file-preview" class:is-playing-file={trackOf(a.id) === 'playing'} class:is-current-file={trackOf(a.id) === 'paused'} style:--file-color={fileKind(a).color} bind:this={host}>
  {#if isRaster(a) && !imageFailed}
    <button type="button" class="file-preview__image" aria-label={`Открыть изображение ${a.filename}`} onclick={event=>{event.stopPropagation();showImages((gallery.some(item=>item.id===a.id)?gallery:[a,...gallery]).filter(isRaster).map(attachmentImage),attachmentImage(a).src)}}><img src={thumbSrc(a)} alt={a.filename} loading="lazy" onerror={() => (imageFailed = true)} /></button>
  {:else if excerpt}
    <div class="file-preview__text" title={LABELS[kind] || 'Превью'}>
      <pre class="file-code">{#if language}{@html highlighted}{:else}{excerpt}{/if}</pre>
    </div>
  {:else if isAudioFile(a)}
    <div class="file-preview__audio" class:file-preview__audio--cover={!!coverUrl}>
      {#if coverUrl}
        <img class="file-preview__cover" src={coverUrl} alt="" />
        <PlayFile file={a} />
      {:else}
        <FileKindIcon att={a} size={36} />
        <PlayFile file={a} />
        <span>{extOf(a) || 'FILE'}</span>
      {/if}
    </div>
  {:else}
    <div class="file-preview__icon">
      {#if manual && canPreview}
        <button type="button" class="file-preview__load" disabled={status !== 'idle'}
          onclick={(event) => { event.stopPropagation(); requestedKey = fileKey }}>
          <FileKindIcon att={a} size={36} />
          <span>{status === 'loading' ? 'Загрузка…' : status === 'done' ? 'Превью недоступно' : 'Показать превью'}</span>
        </button>
      {:else}
      <FileKindIcon att={a} size={36} />
      <span>{extOf(a) || 'FILE'}</span>
      {/if}
    </div>
  {/if}
</div>

<style>
  .file-preview { width: 100%; height: 100%; min-width: 0; min-height: 0; overflow: hidden; background: var(--attachment-preview-bg, color-mix(in srgb, var(--file-color) 8%, var(--color-bg-raised, #1a1a1a))); }
  .file-preview__image {display:block;width:100%;height:100%;padding:0;border:0;background:transparent;cursor:zoom-in}
  .file-preview img { width: 100%; height: 100%; object-fit: contain; }
  .file-preview__icon { display: grid; place-content: center; justify-items: center; gap: 0.4rem; height: 100%; }
  .file-preview__icon span { font: var(--text-xs, 0.75rem) var(--font-mono, monospace); color: var(--color-fg-muted, #9a9a9a); }
  .file-preview { position: relative; }
  .file-preview__audio {
    position: relative;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: 0.4rem;
    height: 100%;
  }
  .file-preview__audio span { font: var(--text-xs, 0.75rem) var(--font-mono, monospace); color: var(--color-fg-muted, #9a9a9a); }
  .file-preview__audio--cover {
    place-content: stretch;
  }
  .file-preview__cover {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .file-preview__audio :global(.play-file) {
    position: relative;
    z-index: 1;
  }
  .file-preview__audio--cover :global(.play-file) {
    position: absolute;
    inset: 0;
    margin: auto;
    width: 2rem;
    height: 2rem;
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 90%, transparent);
    background: color-mix(in srgb, var(--color-bg, #121212) 40%, transparent);
    border-radius: 50%;
  }
  .file-preview__load { display: grid; place-content: center; justify-items: center; gap: 0.6rem; position: absolute; inset: 0; width: 100%; border: 0; background: transparent; color: inherit; cursor: pointer; }
  .file-preview__load:hover { background: color-mix(in srgb, var(--file-color) 8%, transparent); }
  .file-preview__load:focus-visible { outline: 2px solid var(--file-color); outline-offset: -3px; }
  .file-preview__load:disabled { cursor: default; }
  .file-preview__text { display: flex; flex-direction: column; width: 100%; height: 100%; padding: 0.6rem; text-align: left; }
  pre { flex: 1; min-height: 0; overflow: hidden; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: 0.7rem/1.5 var(--font-mono, monospace); color: var(--color-fg, #e8e8e8); mask-image: linear-gradient(#000 75%, transparent); }
</style>
