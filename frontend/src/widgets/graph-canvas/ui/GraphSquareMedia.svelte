<script lang="ts">
  import { fetchId3Tags } from '@/shared/api/audioTags.ts'
  import { enqueueMedia } from '@/shared/lib/mediaQueue.ts'
  import { isAudioFile } from '@/shared/lib/audioFile.ts'
  import { attachmentImage, showImages, type GalleryImage } from '@/shared/lib/imageViewer.svelte.ts'
  import { audioSrc } from '@/shared/lib/player.svelte.ts'
  import FileKindIcon from '@/shared/ui/FileKindIcon.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import PlayFile from '@/shared/ui/PlayFile.svelte'
  import GraphImage from './GraphImage.svelte'

  let {
    id,
    title,
    mime,
    raster = false,
    imagePixels = 128,
    imageGallery = [],
    loadCover = true,
  }: {
    id: number | string
    title: string
    mime?: string
    raster?: boolean
    imagePixels?: number
    imageGallery?: GalleryImage[]
    loadCover?: boolean
  } = $props()

  let audioFile = $derived(
    isAudioFile({ filename: title, mime }) ? { id, filename: title, mime } : null,
  )
  let coverUrl = $state('')
  $effect(() => {
    const file = audioFile
    coverUrl = ''
    if (!file?.id || !loadCover) return
    const ac = new AbortController()
    let objectUrl = ''
    let open: (() => void) | null = null
    const release = () => {
      open?.()
      open = null
    }
    const cancel = enqueueMedia((done) => {
      open = done
      void fetchId3Tags(
        audioSrc({ id: Number(file.id), filename: file.filename || '', hidden: false, revision: 0 }),
        ac.signal,
      )
        .then((tags) => {
          if (ac.signal.aborted || !tags.cover) return
          objectUrl = URL.createObjectURL(tags.cover)
          coverUrl = objectUrl
        })
        .catch(() => {})
        .finally(release)
    })
    return () => {
      ac.abort()
      cancel()
      release()
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  })
</script>

{#if raster}
  <div class="media">
    <GraphImage {id} pixels={imagePixels} hold={!loadCover} paced={loadCover} />
    <button
      type="button"
      class="media__open"
      aria-label={`Открыть изображение ${title}`}
      title="Открыть изображение"
      onpointerdown={(e) => e.stopPropagation()}
      onclick={(e) => {
        e.stopPropagation()
        const image = attachmentImage({ id, filename: title })
        showImages(imageGallery.length ? imageGallery : [image], image.src)
      }}
    >
      <Icon name="fullscreen" size={18} />
    </button>
  </div>
{:else if audioFile}
  <div class="media" class:media--cover={!!coverUrl}>
    {#if coverUrl}
      <img class="media__cover" src={coverUrl} alt="" />
    {:else}
      <FileKindIcon att={{ filename: title, content_type_detected: mime }} size={30} />
    {/if}
    <PlayFile file={audioFile} />
  </div>
{:else}
  <FileKindIcon att={{ filename: title, content_type_detected: mime }} size={30} />
{/if}

<style>
  .media {
    position: relative;
    display: grid;
    place-items: center;
    min-height: 0;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }
  .media--cover {
    background: color-mix(in srgb, var(--c, #888) 12%, var(--color-bg, #121212));
  }
  .media__cover {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .media :global(.play-file) {
    position: relative;
    z-index: 1;
    width: 1.85rem;
    height: 1.85rem;
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 92%, transparent);
  }
  .media--cover :global(.play-file) {
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 95%, transparent);
    filter: drop-shadow(0 0 2px color-mix(in srgb, var(--color-bg, #121212) 70%, transparent));
  }
  .media__open {
    position: absolute;
    inset: 0;
    z-index: 1;
    display: grid;
    place-items: center;
    width: 100%;
    height: 100%;
    margin: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 88%, transparent);
    cursor: zoom-in;
    opacity: 0;
    transition: opacity 120ms ease;
  }
  .media:hover .media__open,
  .media__open:focus-visible {
    opacity: 1;
  }
  .media__open:focus-visible {
    outline: 2px solid var(--color-accent, #c9a227);
    outline-offset: -3px;
  }
</style>
