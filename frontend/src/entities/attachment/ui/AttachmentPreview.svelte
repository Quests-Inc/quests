<script lang="ts">
  import { onMount } from 'svelte'
  import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'
  import { getAttachmentPreview } from '@/shared/api/attachments.ts'
  import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
  import { fileKind } from '@/shared/lib/fileKind.ts'
  import { extOf, isRaster, thumbSrc } from '../model/list.ts'

  let { a }: { a: AttachmentView } = $props()
  let host = $state<HTMLDivElement | null>(null)
  let visible = $state(false)
  let excerpt = $state('')
  let kind = $state('')
  let imageFailed = $state(false)
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
    const id = a.id, revision = a.revision || 1
    void a.uploaded_at
    imageFailed = false
    excerpt = ''
    kind = ''
    if (!visible || isRaster(a) || a.available === false || a.scan_status !== 'clean') return
    const abort = new AbortController()
    void getAttachmentPreview(id, revision, abort.signal).then((result) => {
      if (!abort.signal.aborted) { excerpt = result.text; kind = result.kind }
    }).catch(() => { /* Keep the file icon when preview is unavailable. */ })
    return () => abort.abort()
  })
</script>

<div class="file-preview" style:--file-color={fileKind(a).color} bind:this={host}>
  {#if isRaster(a) && !imageFailed}
    <img src={thumbSrc(a)} alt={a.filename} loading="lazy" onerror={() => (imageFailed = true)} />
  {:else if excerpt}
    <div class="file-preview__text" title={LABELS[kind] || 'Превью'}>
      <div class="file-preview__label">
        <FileKindIcon att={a} size={12} />
        <span>{LABELS[kind] || 'Превью'}</span>
        <span class="file-preview__ext">{extOf(a)}</span>
      </div>
      <pre>{excerpt}</pre>
    </div>
  {:else}
    <div class="file-preview__icon">
      <FileKindIcon att={a} size={36} />
      <span>{extOf(a) || 'FILE'}</span>
    </div>
  {/if}
</div>

<style>
  .file-preview { width: 100%; height: 100%; min-width: 0; min-height: 0; overflow: hidden; background: var(--pill-bg, #151515); }
  .file-preview img { width: 100%; height: 100%; object-fit: contain; }
  .file-preview__icon { display: grid; place-content: center; justify-items: center; gap: 0.4rem; height: 100%; }
  .file-preview__icon span { font: var(--text-xs, 0.75rem) var(--font-mono, monospace); color: var(--color-fg-muted, #9a9a9a); }
  .file-preview__text { display: flex; flex-direction: column; width: 100%; height: 100%; padding: 0.6rem; text-align: left; }
  .file-preview__label { display: flex; align-items: center; gap: 0.35rem; flex: none; margin-bottom: 0.4rem; font: var(--text-xs, 0.75rem) var(--font-ui, sans-serif); color: var(--color-fg-muted, #9a9a9a); }
  .file-preview__ext { margin-left: auto; padding: 0.05rem 0.25rem; border-radius: var(--radius-sm, 2px); background: color-mix(in srgb, var(--file-color) 8%, transparent); color: var(--file-color); font: 0.65rem var(--font-mono, monospace); }
  pre { flex: 1; min-height: 0; overflow: hidden; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: 0.7rem/1.5 var(--font-mono, monospace); color: var(--color-fg, #e8e8e8); mask-image: linear-gradient(#000 75%, transparent); }
</style>
