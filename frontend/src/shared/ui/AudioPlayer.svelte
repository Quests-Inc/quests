<script lang="ts">
  import { fetchId3Tags } from '@/shared/api/audioTags.ts'
  import { focusGraphPresence, graphPresence } from '@/shared/lib/graphPresence.svelte.ts'
  import {
    audioSrc,
    player,
    setLoop,
    setMuted,
    setPinned,
    setVolume,
    stopPlayback,
  } from '@/shared/lib/player.svelte.ts'
  import Icon from '@/shared/ui/Icon.svelte'

  let {
    owner = null,
    onOpenAttachment,
    onOpenOwner,
    onShowMap,
  }: {
    owner?: { kind: string; id: number } | null
    onOpenAttachment?: (id: number) => void
    onOpenOwner?: (kind: string, id: number) => void
    onShowMap?: () => void
  } = $props()

  let audio = $state<HTMLAudioElement | null>(null)
  let dockEl = $state<HTMLElement | null>(null)
  let tabEl = $state<HTMLButtonElement | null>(null)
  let fullFits = $state(true)
  let src = $derived(player.track ? audioSrc(player.track) : '')
  let heading = $derived(player.title || player.track?.filename || '')
  let seekPct = $derived(player.duration > 0 ? `${(player.current / player.duration) * 100}%` : '0%')
  let volPct = $derived(`${(player.muted ? 0 : player.volume) * 100}%`)
  let fullTime = $derived(`${clock(player.current)}/${clock(player.duration)}`)
  let leftTime = $derived(`-${clock(Math.max(0, (player.duration || 0) - (player.current || 0)))}`)
  let tabTime = $derived(fullFits ? fullTime : leftTime)
  let onMap = $derived(
    !!player.track && graphPresence.keys.includes(`attachment:${player.track.id}`),
  )
  const OWNER_TITLE: Record<string, string> = {
    quest: 'Квест',
    questline: 'Квестлайн',
    note: 'Заметка',
  }

  function clock(seconds: number) {
    if (!Number.isFinite(seconds) || seconds < 0) return '0:00'
    const t = Math.floor(seconds)
    const h = Math.floor(t / 3600)
    const m = Math.floor((t % 3600) / 60)
    const s = t % 60
    if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
    return `${m}:${String(s).padStart(2, '0')}`
  }

  function togglePlay() {
    player.playing = !player.playing
  }

  function toStart() {
    if (audio) audio.currentTime = 0
    player.current = 0
  }

  // A click leaves focus on a button; :focus-within would keep the card open after the cursor leaves.
  function blurDock() {
    const active = document.activeElement
    if (active instanceof HTMLElement && dockEl?.contains(active)) active.blur()
  }

  function goAttachment() {
    if (!player.track) return
    onOpenAttachment?.(player.track.id)
  }

  function goOwner() {
    if (!owner) return
    onOpenOwner?.(owner.kind, owner.id)
  }

  function goMap() {
    if (!player.track || !onMap) return
    const key = `attachment:${player.track.id}`
    onShowMap?.()
    focusGraphPresence(key)
  }

  $effect(() => {
    const el = audio
    const playing = player.playing
    const current = src
    if (!el || !current) return
    el.loop = player.loop
    el.volume = player.volume
    el.muted = player.muted
    if (playing && el.paused) {
      const started = el.play()
      void Promise.resolve(started).catch(() => {
        player.playing = false
        player.error = 'Не удалось воспроизвести'
      })
    } else if (!playing && !el.paused) el.pause()
  })

  $effect(() => {
    const current = src
    if (!current) return
    const ac = new AbortController()
    void fetchId3Tags(current, ac.signal)
      .then((tags) => {
        if (ac.signal.aborted || !player.track || audioSrc(player.track) !== current) return
        player.title = tags.title
        player.artist = tags.artist
        if (tags.cover) {
          if (player.cover) URL.revokeObjectURL(player.cover)
          player.cover = URL.createObjectURL(tags.cover)
        }
      })
      .catch(() => {
        /* Abort or network — keep the filename as the title. */
      })
    return () => ac.abort()
  })

  // The full "elapsed/total" line is used when it fits the bookmark; otherwise the shorter remaining time.
  $effect(() => {
    const label = fullTime
    const box = tabEl
    if (!box || box.clientHeight < 8) return
    const probe = box.querySelector('.dock__probe')
    if (!(probe instanceof HTMLElement)) return
    probe.textContent = label
    fullFits = probe.scrollWidth <= box.clientHeight - 4
  })

  $effect(() => {
    const session = navigator.mediaSession
    if (!session || !player.track) return
    if (typeof MediaMetadata === 'function') {
      session.metadata = new MediaMetadata({
        title: heading,
        artist: player.artist,
        artwork: player.cover ? [{ src: player.cover }] : [],
      })
    }
    session.playbackState = player.playing ? 'playing' : 'paused'
    try {
      session.setActionHandler('play', () => { player.playing = true })
      session.setActionHandler('pause', () => { player.playing = false })
      session.setActionHandler('stop', () => stopPlayback())
    } catch {
      /* This browser has no handler for one of the actions. */
    }
  })
</script>

{#if player.track}
  <aside class="dock" class:dock--pinned={player.pinned} aria-label="Плеер" bind:this={dockEl} onpointerleave={blurDock}>
    <div class="dock__card">
      <div class="dock__head">
        <button
          type="button"
          class="dock__cover"
          class:dock__cover--empty={!player.cover}
          aria-label={player.playing ? 'Пауза' : 'Играть'}
          onclick={togglePlay}
        >
          {#if player.cover}
            <img src={player.cover} alt="" />
          {:else}
            <svg class="dock__note" viewBox="0 0 16 16" aria-hidden="true">
              <path d="M9.15 1.7h1.2v7.05a2.2 2.2 0 1 1-1.2-1.95V1.7z" />
            </svg>
          {/if}
          <span class="dock__cover-mark" aria-hidden="true">
            <Icon name={player.playing ? 'pause' : 'play'} size={16} />
          </span>
        </button>
        <div class="dock__meta">
          <p class="dock__title" title={heading}>{heading}</p>
          {#if player.artist}<p class="dock__artist">{player.artist}</p>{/if}
        </div>
        <div class="dock__jumps">
          <button
            type="button"
            class="dock__btn"
            aria-label="Открыть вложение"
            title="Вложение"
            onclick={goAttachment}
          >
            <Icon name="attachment" size={13} />
          </button>
          {#if owner}
            <button
              type="button"
              class="dock__btn"
              aria-label={`Открыть ${OWNER_TITLE[owner.kind] || 'владельца'}`}
              title={OWNER_TITLE[owner.kind] || 'Владелец'}
              onclick={goOwner}
            >
              <Icon name="folder" size={13} />
            </button>
          {/if}
          {#if onMap}
            <button
              type="button"
              class="dock__btn"
              aria-label="Показать на карте"
              title="На карте"
              onclick={goMap}
            >
              <Icon name="map" size={13} />
            </button>
          {/if}
          <button
            type="button"
            class="dock__btn dock__pin"
            class:dock__btn--on={player.pinned}
            aria-pressed={player.pinned}
            aria-label={player.pinned ? 'Открепить плеер' : 'Закрепить плеер'}
            title={player.pinned ? 'Открепить' : 'Закрепить'}
            onclick={() => setPinned(!player.pinned)}
          >
            <Icon name={player.pinned ? 'pin-filled' : 'pin'} size={13} />
          </button>
        </div>
      </div>
      <div class="dock__transport">
        <label class="dock__seek">
          <span class="sr">{clock(player.current)} / {clock(player.duration)}</span>
          <input
            class="dock__range"
            type="range"
            min="0"
            max={player.duration || 0}
            step="0.1"
            value={player.current}
            style:--pct={seekPct}
            aria-label="Позиция"
            oninput={(e) => {
              const v = Number(e.currentTarget.value)
              if (audio) audio.currentTime = v
              player.current = v
            }}
          />
        </label>
        <div class="dock__row">
          <span class="dock__time">{clock(player.current)}</span>
          <span class="dock__time dock__time--end">{clock(player.duration)}</span>
          <button type="button" class="dock__btn dock__stop-btn" aria-label="Стоп" onclick={stopPlayback}>
            <span class="dock__stop" aria-hidden="true"></span>
          </button>
          <div class="dock__mid">
            <button type="button" class="dock__btn" aria-label="К началу" title="К началу" onclick={toStart}>
              <Icon name="to-start" size={13} />
            </button>
            <button type="button" class="dock__btn dock__play" aria-label={player.playing ? 'Пауза' : 'Играть'} onclick={togglePlay}>
              <Icon name={player.playing ? 'pause' : 'play'} size={18} />
            </button>
            <div class="dock__vol">
              <button
                type="button"
                class="dock__btn"
                aria-pressed={player.muted || player.volume === 0}
                aria-label={player.muted ? 'Со звуком' : 'Без звука'}
                onclick={() => setMuted(!player.muted)}
              >{player.muted || player.volume === 0 ? '×' : '♪'}</button>
              <input
                class="dock__range dock__volume"
                type="range"
                min="0"
                max="1"
                step="0.01"
                value={player.muted ? 0 : player.volume}
                style:--pct={volPct}
                aria-label="Громкость"
                oninput={(e) => setVolume(Number(e.currentTarget.value))}
              />
            </div>
          </div>
          <button
            type="button"
            class="dock__btn dock__loop"
            class:dock__btn--on={player.loop}
            aria-pressed={player.loop}
            aria-label="Повтор"
            title="Повтор"
            onclick={() => setLoop(!player.loop)}
          >↺</button>
        </div>
      </div>
      {#if player.error}<p class="dock__error">{player.error}</p>{/if}
    </div>
    {#if !player.pinned}
      <button type="button" class="dock__tab" aria-label="Показать плеер" bind:this={tabEl}>
        <span class="dock__probe" aria-hidden="true">{fullTime}</span>
        <span class="dock__tab-time" aria-hidden="true">{tabTime}</span>
      </button>
    {/if}
  </aside>
  <audio
    bind:this={audio}
    {src}
    preload="auto"
    ontimeupdate={() => { if (audio) player.current = audio.currentTime }}
    onloadedmetadata={() => { if (audio) player.duration = Number.isFinite(audio.duration) ? audio.duration : 0 }}
    onended={() => { if (!player.loop) stopPlayback() }}
    onerror={() => { player.playing = false; player.error = 'Не удалось воспроизвести' }}
  ></audio>
{/if}

<style>
  .dock {
    --dock-h: 5.15rem;
    position: fixed;
    left: 0;
    bottom: 1rem;
    z-index: 40;
    transition: left 0.2s ease;
  }
  .dock--pinned { left: 0.75rem; }
  .dock__card {
    position: absolute;
    left: 0;
    bottom: 0;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    width: min(19rem, calc(100vw - 2.5rem));
    height: var(--dock-h);
    padding: 0.28rem 0.28rem 0.22rem 0.42rem;
    border: 1px solid var(--color-border, #333);
    border-left: 0;
    border-radius: 0 10px 10px 0;
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg, #e8e8e8);
    transform: translateX(calc(-100% - 2px));
    transition: transform 180ms ease, border-radius 0.2s ease, border-left-width 0.2s ease;
  }
  .dock--pinned .dock__card {
    border-left: 1px solid var(--color-border, #333);
    border-radius: 10px;
  }
  .dock--pinned .dock__card,
  .dock:hover .dock__card,
  .dock:focus-within .dock__card {
    transform: none;
  }
  .dock__tab {
    position: relative;
    display: grid;
    place-items: center;
    width: 1.45rem;
    height: var(--dock-h);
    padding: 0.3rem 0;
    overflow: hidden;
    border: 1px solid var(--color-border, #333);
    border-left: 0;
    border-radius: 0 8px 8px 0;
    background: var(--color-bg-raised, #1a1a1a);
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }
  .dock--pinned .dock__tab,
  .dock:hover .dock__tab,
  .dock:focus-within .dock__tab { visibility: hidden; }
  .dock__probe {
    position: absolute;
    white-space: nowrap;
    visibility: hidden;
    pointer-events: none;
    font-size: 0.68rem;
    font-variant-numeric: tabular-nums;
  }
  .dock__tab-time {
    writing-mode: vertical-rl;
    transform: rotate(180deg);
    font-size: 0.68rem;
    font-variant-numeric: tabular-nums;
    letter-spacing: 0.03em;
    line-height: 1;
  }
  .dock__head { display: flex; align-items: center; gap: 0.4rem; min-width: 0; min-height: 0; padding-right: 0.1rem; }
  .dock__cover {
    position: relative;
    display: grid;
    place-items: center;
    flex: 0 0 auto;
    width: 1.85rem;
    height: 1.85rem;
    padding: 0;
    overflow: hidden;
    border: 0;
    border-radius: 3px;
    background: transparent;
    cursor: pointer;
  }
  .dock__cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .dock__cover--empty {
    color: var(--color-accent, var(--color-fg));
    background: color-mix(in srgb, var(--color-accent, var(--color-fg)) 22%, var(--color-bg, #121212));
  }
  .dock__note { width: 1.15rem; height: 1.15rem; display: block; fill: currentColor; }
  .dock__cover-mark {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    opacity: 0;
    color: color-mix(in srgb, var(--color-fg, #e8e8e8) 82%, transparent);
    background: color-mix(in srgb, var(--color-bg, #121212) 35%, transparent);
    transition: opacity 120ms ease;
  }
  .dock__cover:hover .dock__cover-mark,
  .dock__cover:focus-visible .dock__cover-mark { opacity: 1; }
  .dock__meta { min-width: 0; flex: 1; }
  .dock__title, .dock__artist, .dock__error { margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .dock__title { font-size: 0.75rem; font-weight: 650; line-height: 1.15; }
  .dock__artist, .dock__error { font-size: 0.64rem; line-height: 1.15; color: var(--color-fg-muted, #9a9a9a); }
  .dock__error { color: var(--color-danger, #b54a3a); }
  .dock__jumps {
    display: flex;
    flex: 0 0 auto;
    align-items: center;
    align-self: flex-start;
    gap: 0.05rem;
    margin-top: -0.05rem;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .dock__transport { display: grid; gap: 0; }
  .dock__seek { display: block; margin: 0; }
  .dock__seek input { display: block; width: 100%; height: 0.85rem; margin: 0; }
  .dock__range {
    -webkit-appearance: none;
    appearance: none;
    background: transparent;
    cursor: pointer;
  }
  .dock__range::-webkit-slider-runnable-track {
    height: 2px;
    border-radius: 99px;
    background:
      linear-gradient(var(--color-accent, #c9a227), var(--color-accent, #c9a227)) 0 0 / var(--pct, 0%) 100% no-repeat,
      color-mix(in srgb, var(--color-fg-muted, #9a9a9a) 38%, transparent);
  }
  .dock__range::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 8px;
    height: 8px;
    margin-top: -3px;
    border: 0;
    border-radius: 50%;
    background: var(--color-fg, #e8e8e8);
  }
  .dock__range::-moz-range-track {
    height: 2px;
    border: 0;
    border-radius: 99px;
    background: color-mix(in srgb, var(--color-fg-muted, #9a9a9a) 38%, transparent);
  }
  .dock__range::-moz-range-progress {
    height: 2px;
    border-radius: 99px;
    background: var(--color-accent, #c9a227);
  }
  .dock__range::-moz-range-thumb {
    width: 8px;
    height: 8px;
    border: 0;
    border-radius: 50%;
    background: var(--color-fg, #e8e8e8);
  }
  .dock__row { position: relative; height: 1.45rem; }
  .dock__time {
    position: absolute;
    top: 0;
    left: 0;
    font-size: 0.64rem;
    line-height: 1.45rem;
    font-variant-numeric: tabular-nums;
    color: var(--color-fg-muted, #9a9a9a);
    pointer-events: none;
  }
  .dock__time--end { left: auto; right: 0; }
  .dock__stop-btn {
    position: absolute;
    left: 2.35rem;
    top: 50%;
    transform: translateY(-50%);
  }
  .dock__mid {
    position: absolute;
    left: 50%;
    top: 0;
    display: flex;
    align-items: center;
    height: 100%;
    transform: translateX(-50%);
  }
  .dock__loop {
    position: absolute;
    right: 2.35rem;
    top: 50%;
    transform: translateY(-50%);
  }
  .dock__vol { position: relative; display: flex; align-items: center; height: 100%; }
  .dock__volume {
    position: absolute;
    left: 100%;
    width: 2.6rem;
    height: 0.85rem;
    margin: 0 0 0 0.15rem;
    opacity: 0;
    pointer-events: none;
  }
  .dock__vol:hover .dock__volume,
  .dock__vol:focus-within .dock__volume {
    opacity: 1;
    pointer-events: auto;
  }
  .dock__btn {
    display: inline-grid;
    place-items: center;
    width: 1.15rem;
    height: 1.15rem;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    cursor: pointer;
    font-size: 0.85rem;
    line-height: 1;
  }
  .dock__play {
    width: 1.45rem;
    height: 1.45rem;
  }
  .dock__btn--on { color: var(--color-accent, #c9a227); }
  .dock__stop {
    width: 0.48rem;
    height: 0.48rem;
    background: currentColor;
    border-radius: 1px;
  }
  .dock__btn:focus-visible, .dock__cover:focus-visible, .dock__tab:focus-visible {
    outline: 2px solid var(--color-accent, #c9a227);
    outline-offset: 1px;
  }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
  audio { display: none; }
</style>
