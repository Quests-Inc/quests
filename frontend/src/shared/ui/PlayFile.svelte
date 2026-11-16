<script lang="ts">
  import { isAudioFile, type AudioFile } from '@/shared/lib/audioFile.ts'
  import { playAttachment, trackOf } from '@/shared/lib/player.svelte.ts'
  import Icon from '@/shared/ui/Icon.svelte'

  let { file, floating = false }: { file: AudioFile; floating?: boolean } = $props()
  let state = $derived(trackOf(Number(file?.id)))
  let name = $derived(file?.filename || file?.title || 'запись')
</script>

{#if isAudioFile(file)}
  <button
    type="button"
    class="play-file"
    class:play-file--on={state === 'playing'}
    class:play-file--float={floating}
    aria-pressed={state === 'playing'}
    aria-label={state === 'playing' ? `Пауза: ${name}` : `Играть: ${name}`}
    title={state === 'playing' ? 'Пауза' : 'Играть'}
    onpointerdown={(e) => e.stopPropagation()}
    onclick={(e) => {
      e.stopPropagation()
      e.preventDefault()
      playAttachment(file)
    }}
  >
    <Icon name={state === 'playing' ? 'pause' : 'play'} size={15} />
  </button>
{/if}

<style>
  .play-file {
    display: inline-grid;
    place-items: center;
    flex: 0 0 auto;
    width: 1.25rem;
    height: 1.25rem;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }
  .play-file--on,
  .play-file:hover {
    color: var(--color-accent, #c9a227);
  }
  .play-file--float {
    position: absolute;
    top: 4px;
    right: 4px;
  }
  .play-file:focus-visible {
    outline: 2px solid var(--color-accent, #c9a227);
    outline-offset: 2px;
  }
</style>
