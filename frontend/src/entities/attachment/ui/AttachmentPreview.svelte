<script lang="ts">
  import { onMount } from 'svelte'
  import type { AttachmentView } from '@/shared/api/knowledge_http.gen.ts'
  import { getAttachmentPreview } from '@/shared/api/attachments.ts'
  import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
  import { fileKind } from '@/shared/lib/fileKind.ts'
  import { fileCodeLanguage, highlightCode } from '@/shared/lib/codeHighlight.ts'
  import { extOf, isRaster, thumbSrc } from '../model/list.ts'

  let { a }: { a: AttachmentView } = $props()
  let host = $state<HTMLDivElement | null>(null)
  let visible = $state(false)
  let excerpt = $state('')
  let kind = $state('')
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
      <pre class="file-code">{#if language}{@html highlighted}{:else}{excerpt}{/if}</pre>
    </div>
  {:else}
    <div class="file-preview__icon">
      <FileKindIcon att={a} size={36} />
      <span>{extOf(a) || 'FILE'}</span>
    </div>
  {/if}
</div>

<style>
  .file-preview { width: 100%; height: 100%; min-width: 0; min-height: 0; overflow: hidden; background: var(--attachment-preview-bg, color-mix(in srgb, var(--file-color) 8%, var(--color-bg-raised, #1a1a1a))); }
  .file-preview img { width: 100%; height: 100%; object-fit: contain; }
  .file-preview__icon { display: grid; place-content: center; justify-items: center; gap: 0.4rem; height: 100%; }
  .file-preview__icon span { font: var(--text-xs, 0.75rem) var(--font-mono, monospace); color: var(--color-fg-muted, #9a9a9a); }
  .file-preview__text { display: flex; flex-direction: column; width: 100%; height: 100%; padding: 0.6rem; text-align: left; }
  pre { flex: 1; min-height: 0; overflow: hidden; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: 0.7rem/1.5 var(--font-mono, monospace); color: var(--color-fg, #e8e8e8); mask-image: linear-gradient(#000 75%, transparent); }
</style>
