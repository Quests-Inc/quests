<script>
  import { onMount, tick } from 'svelte'
  import Icon from './Icon.svelte'

  /** Table of contents that floats over a note being read: shown while the
   * page is scrolled past a threshold, then fades out a couple of seconds
   * after scrolling stops. Landscape screens only.
   * @type {{
   *   scroller: HTMLElement | null,
   *   content: HTMLElement | null,
   *   enabled?: boolean,
   *   dep?: string,
   *   threshold?: number,
   *   hold?: number,
   * }} */
  let { scroller, content, enabled = true, dep = '', threshold = 280, hold = 2400 } = $props()

  const WIDE = '(orientation: landscape) and (min-width: 820px)'
  let wide = $state(false)
  /** @type {{ el: HTMLElement, text: string, level: number }[]} */
  let heads = $state([])
  let current = $state(-1)
  let visible = $state(false)
  let hovered = false
  const FOLD_KEY = 'quests.notes.outline.folded'
  /** Folded to a single line: just the heading the reader is at. */
  let folded = $state(
    (() => {
      try {
        return localStorage.getItem(FOLD_KEY) === '1'
      } catch {
        return false
      }
    })(),
  )
  function setFolded(v) {
    folded = v
    try {
      localStorage.setItem(FOLD_KEY, v ? '1' : '0')
    } catch {
      /* the choice is just not remembered */
    }
  }
  /** @type {number | undefined} */
  let timer

  onMount(() => {
    const mq = window.matchMedia(WIDE)
    wide = mq.matches
    const on = () => (wide = mq.matches)
    mq.addEventListener('change', on)
    return () => {
      mq.removeEventListener('change', on)
      clearTimeout(timer)
    }
  })

  function collect() {
    if (!content) {
      heads = []
      return
    }
    const list = [...content.querySelectorAll('h1, h2, h3')]
      .map((el) => ({ el: /** @type {HTMLElement} */ (el), text: (el.textContent || '').trim(), level: Number(el.tagName[1]) }))
      .filter((h) => h.text)
    heads = list
    spy()
  }

  // Headings are re-read whenever the rendered text changes.
  $effect(() => {
    const c = content
    void dep
    if (!c) return
    void tick().then(collect)
    const mo = new MutationObserver(() => collect())
    mo.observe(c, { childList: true, subtree: true })
    return () => mo.disconnect()
  })

  /** The heading the reader is at: the last one above a line near the top. */
  function spy() {
    if (!scroller || !heads.length) {
      current = -1
      return
    }
    const line = scroller.getBoundingClientRect().top + 90
    let idx = 0
    for (let i = 0; i < heads.length; i++) {
      if (heads[i].el.getBoundingClientRect().top <= line) idx = i
      else break
    }
    current = idx
  }

  function arm() {
    clearTimeout(timer)
    if (hovered) return
    timer = window.setTimeout(() => (visible = false), hold)
  }

  function onScroll() {
    if (!scroller || !enabled || !wide) return
    spy()
    if (scroller.scrollTop >= threshold) {
      visible = true
      arm()
    } else {
      visible = false
      clearTimeout(timer)
    }
  }

  $effect(() => {
    const s = scroller
    if (!s) return
    s.addEventListener('scroll', onScroll, { passive: true })
    return () => s.removeEventListener('scroll', onScroll)
  })

  // A different note: start hidden.
  $effect(() => {
    void dep
    visible = false
  })

  const minLevel = $derived(heads.length ? Math.min(...heads.map((h) => h.level)) : 1)

  /** @param {{ el: HTMLElement }} h */
  function go(h) {
    h.el.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
</script>

{#if enabled && wide && heads.length > 1}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <nav
    class="outline"
    class:outline--on={visible}
    class:outline--folded={folded}
    aria-label="Оглавление заметки"
    aria-hidden={!visible}
    onmouseenter={() => {
      hovered = true
      clearTimeout(timer)
    }}
    onmouseleave={() => {
      hovered = false
      arm()
    }}
  >
    {#if folded}
      <button
        type="button"
        class="outline__fold"
        title="Развернуть оглавление"
        tabindex={visible ? 0 : -1}
        onclick={() => setFolded(false)}
      >
        <span class="outline__now">{heads[Math.max(0, current)]?.text}</span>
        <Icon name="chevron-down" size={12} />
      </button>
    {:else}
      <button
        type="button"
        class="outline__fold outline__fold--up"
        title="Свернуть в строку"
        aria-label="Свернуть в строку"
        tabindex={visible ? 0 : -1}
        onclick={() => setFolded(true)}
      >
        <span class="outline__now">Оглавление</span>
        <Icon name="chevron-down" size={12} />
      </button>
    {/if}
    {#each folded ? [] : heads as h, i (i)}
      <button
        type="button"
        class="outline__item"
        class:outline__item--cur={i === current}
        style:padding-left="{0.5 + (h.level - minLevel) * 0.8}rem"
        tabindex={visible ? 0 : -1}
        onclick={() => go(h)}
      >
        {h.text}
      </button>
    {/each}
  </nav>
{/if}

<style>
  .outline {
    position: absolute;
    z-index: 4;
    top: 0.9rem;
    right: 1.4rem;
    box-sizing: border-box;
    width: 15rem;
    max-height: min(70%, 28rem);
    overflow-y: auto;
    padding: 0.4rem 0.2rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--pill-radius, 10px);
    background: color-mix(in srgb, var(--color-bg-raised, #1a1a1a) 72%, transparent);
    backdrop-filter: blur(6px);
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.35s ease;
  }

  .outline--on {
    opacity: 1;
    pointer-events: auto;
  }

  .outline--folded {
    overflow: hidden;
  }

  .outline__fold {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
    width: 100%;
    box-sizing: border-box;
    padding: 0.2rem 0.6rem;
    border: 0;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font: inherit;
    font-size: 0.74rem;
    cursor: pointer;
  }

  .outline__fold:hover {
    color: var(--color-fg, #ddd);
  }

  .outline--folded .outline__fold {
    color: var(--color-accent, #c9a227);
  }

  .outline__fold--up :global(svg) {
    transform: rotate(180deg);
  }

  .outline__now {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .outline__item {
    display: block;
    width: 100%;
    box-sizing: border-box;
    padding: 0.22rem 0.5rem;
    border: 0;
    border-left: 2px solid transparent;
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font: inherit;
    font-size: 0.78rem;
    line-height: 1.3;
    text-align: left;
    cursor: pointer;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .outline__item:hover {
    color: var(--color-fg, #ddd);
  }

  .outline__item--cur {
    border-left-color: var(--color-accent, #c9a227);
    color: var(--color-accent, #c9a227);
  }
</style>
