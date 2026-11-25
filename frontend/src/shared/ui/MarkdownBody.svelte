<script lang="ts">
  import {repositoryRefs} from '@/shared/lib/repositoryRefs.svelte.ts'
  import { showImages } from '@/shared/lib/imageViewer.svelte.ts'
  import { untrack } from 'svelte'
  import { listValues, type ValueRow } from '@/shared/api/client.ts'
  import { foreignValueRefs, renderMarkdown, taskMarksFromSource } from '@/shared/lib/markdown.ts'
  import { hostKey, publishValues, valueRev } from '@/shared/lib/valueLive.svelte.ts'
  import { valueHover, type ValueEditTarget } from '@/shared/lib/valueHover.ts'
  import { proseValue } from '@/shared/lib/valueText.ts'
  import ValueEditDialog from './ValueEditDialog.svelte'
  import { refQuotes, scrollFragment } from '@/shared/lib/markdownSpans.ts'
  import { setPendingFragment, validNoteHash } from '@/shared/lib/noteFragment.ts'
  import { parseRefHref } from '@/shared/lib/refs.ts'
  import { copyText } from '@/shared/lib/clipboard.ts'
  import { tailFade } from '@/shared/lib/typewriter.ts'
  import { refPreview } from '@/shared/lib/refPreview.ts'
  import { scriptEmbed } from '@/shared/lib/scriptEmbed.ts'
  import { refIcons } from '@/shared/lib/refIcons.svelte.ts'
  import { renderMermaidDiagrams } from '@/shared/lib/markdownMermaid.ts'
  import { attachmentIsAudio } from '@/shared/api/stt.ts'
  import { playAttachment, player } from '@/shared/lib/player.svelte.ts'
  import pauseSvg from '@/assets/icons/pause.svg?raw'
  import playSvg from '@/assets/icons/play.svg?raw'
  import { mapJumps, type MapJump } from '@/shared/lib/mapJumps.ts'
  import copySvg from '@/assets/icons/copy.svg?raw'
  import checkSvg from '@/assets/icons/checkmark.svg?raw'

  let {
    source = '',
    class: className = '',
    labels = {},
    values = {},
    valuesLoading = false,
    host,
    ownValues = false,
    onRef,
    typing = false,
    mapJump = null,
    imageThumb = undefined,
    onTaskState,
    onSaveMarks,
    onAudioMenu,
  }: {
    source?: string,
    class?: string,
    labels?: Record<string, string>,
    values?: Record<string, string>,
    valuesLoading?: boolean,
    host?: { kind: string; id: number },
    /** The text reads the named values of `host` itself: load them here (a page that has its own value table passes `values`). */
    ownValues?: boolean,
    onRef?: (kind: string, id: number) => void,
    typing?: boolean,
    /** When the linked entity is on the same map, a button after the link focuses it there. */
    mapJump?: MapJump | null,
    /** Longest side of the preview, in pixels. Unset keeps the original attachment image. */
    imageThumb?: number,
    /** Report the task-tick state so the host can render its own bar (a sticky bar cannot
     * escape an overflow-clipped ancestor, so the plate must live outside this element). */
    onTaskState?: (state: { count: number; edited: boolean }) => void,
    onSaveMarks?: (marks: boolean[]) => void,
    /** Right click on an inline player. The host opens its menu. */
    onAudioMenu?: (id: number, point: { x: number; y: number }) => void,
  } = $props()

  let foreign = $state({} as Record<string, string>)
  let edit = $state(null as ValueEditTarget | null)

  function hoverOpts() {
    return { host, onEdit: (target: ValueEditTarget) => (edit = target) }
  }

  $effect(() => {
    void valueRev.n
    const refs = foreignValueRefs(source)
    if (!refs.length) {
      foreign = {}
      return
    }
    const owners = new Map<string, { kind: string; id: number }>()
    for (const ref of refs) owners.set(`${ref.kind}:${ref.id}`, { kind: ref.kind, id: ref.id })
    let dead = false
    Promise.all(
      [...owners.values()].map(async (owner) => {
        try {
          const list = await listValues(owner.kind, owner.id)
          return { owner, rows: list.effective ?? [] }
        } catch {
          return { owner, rows: [] as ValueRow[] }
        }
      }),
    ).then((groups) => {
      if (dead) return
      const next: Record<string, string> = {}
      for (const group of groups) {
        publishValues(
          hostKey(group.owner.kind, group.owner.id),
          group.rows.map((row) => ({
            name: row.name,
            type: row.type,
            text: row.value == null ? '' : proseValue(row.type, row.value),
            formula: row.formula ?? '',
            error: row.error ?? '',
            roll: row.roll?.arg ?? '',
          })),
        )
      }
      for (const ref of refs) {
        const group = groups.find((g) => g.owner.kind === ref.kind && g.owner.id === ref.id)
        const row = group?.rows.find((r) => r.name === ref.name)
        if (row) next[ref.key] = proseValue(row.type, row.value)
      }
      foreign = next
    })
    return () => {
      dead = true
    }
  })

  const foreignTruncated = $derived(foreignValueRefs(source, Infinity).length > foreignValueRefs(source).length)
  let own = $state({} as Record<string, string>)
  $effect(() => {
    void valueRev.n
    const h = host
    if (!ownValues || !h) {
      own = {}
      return
    }
    let dead = false
    listValues(h.kind, h.id)
      .then((list) => {
        if (dead) return
        const rows = list.effective ?? []
        publishValues(
          hostKey(h.kind, h.id),
          rows.map((row) => ({
            name: row.name,
            type: row.type,
            text: row.value == null ? '' : proseValue(row.type, row.value),
            formula: row.formula ?? '',
            error: row.error ?? '',
            roll: row.roll?.arg ?? '',
          }))
        )
        own = Object.fromEntries(rows.map((row) => [row.name, row.value == null ? '' : proseValue(row.type, row.value)]))
      })
      .catch(() => {
        if (!dead) own = {}
      })
    return () => {
      dead = true
    }
  })

  let merged = $derived({ ...foreign, ...own, ...values })
  let html = $derived(renderMarkdown(source, { labels:{...repositoryRefs.labels,...labels}, values: merged, valuesLoading }))

  /** A plain click is scratch state (a star until saved). Ctrl+click writes that one tick into the note immediately. */
  let taskCount = $state(0)
  let taskEdited = $state(false)

  function taskInputs(): HTMLInputElement[] {
    return root ? Array.from(root.querySelectorAll('input.md-task')) : []
  }

  /** Sync the per-item phantom stars and the bar's buttons with the DOM ticks vs the source. */
  function refreshTaskMarks() {
    const defaults = taskMarksFromSource(source)
    let edited = false
    for (const [i, input] of taskInputs().entries()) {
      const li = input.closest('li') ?? input.parentElement
      const changed = input.checked !== (defaults[i] ?? false)
      li?.classList.toggle('md-task--edited', changed)
      if (!li) continue
      if (changed && !li.querySelector('.md-task__unsaved')) {
        const star = document.createElement('span')
        star.className = 'md-task__unsaved'
        star.textContent = '*'
        star.setAttribute('title', 'Отметка не в тексте — ЛКМ сохранить, ПКМ сбросить')
        const holder = li.querySelector('p:last-of-type') ?? li
        holder.append(star)
      } else if (!changed) {
        li.querySelector('.md-task__unsaved')?.remove()
      }
      if (changed) edited = true
    }
    taskEdited = edited
  }

  function saveMarks() {
    onSaveMarks?.(taskInputs().map((input) => input.checked))
  }

  /** Save a single item's tick (the phantom star's LMB): defaults + this one change. */
  function saveOneMark(input: HTMLInputElement) {
    const i = taskInputs().indexOf(input)
    if (i === -1) return
    const marks = taskMarksFromSource(source)
    if (marks[i] === undefined) return
    marks[i] = input.checked
    onSaveMarks?.(marks)
  }

  /** Reset a single item's tick back to the source (the phantom star's RMB). */
  function resetOneMark(input: HTMLInputElement) {
    const defaults = taskMarksFromSource(source)
    const i = taskInputs().indexOf(input)
    if (i === -1 || defaults[i] === undefined) return
    input.checked = defaults[i]
    refreshTaskMarks()
  }

  /** Reset every tick back to the source (the bar's «Сбросить»). */
  function resetMarks() {
    const defaults = taskMarksFromSource(source)
    for (const [i, input] of taskInputs().entries()) if (defaults[i] !== undefined) input.checked = defaults[i]
    refreshTaskMarks()
  }
  let valueSig = $derived(
    Object.keys(merged)
      .sort()
      .map((name) => `${encodeURIComponent(name)}=${encodeURIComponent(merged[name])}`)
      .join('&')
  )
  let root: HTMLDivElement | undefined = $state()
  let fragmentKey = ''
  let fragmentScrolls = 0
  $effect(() => {
    void html
    if (!root || host?.kind !== 'note' || !validNoteHash(location.hash)) return
    if (location.search !== `?note=${host.id}`) return
    const key = `${host.id}:${location.hash}`
    if (fragmentKey !== key) {
      fragmentKey = key
      fragmentScrolls = 0
    }
    // The first paint can be replaced when titles arrive. Scroll that one too, then stop.
    if (fragmentScrolls >= 2) return
    const node = root
    const hash = location.hash.slice(1)
    queueMicrotask(() => {
      if (fragmentScrolls >= 2 || !node.isConnected) return
      if (scrollFragment(node, hash)) fragmentScrolls++
    })
  })

  $effect(() => {
    const el = root
    const _ = html
    if (!el) return
    untrack(() => {
      enhanceCodeBlocks(el)
      void renderMermaidDiagrams(el)
    })
  })

  $effect(() => {
    void player.playing
    void player.current
    void player.track?.id
    const el = root
    if (!el) return
    paintAudioButtons(el)
  })

  // A new render replaces the DOM: scratch ticks and their «edited» flag reset with it.
  $effect(() => {
    void html
    taskCount = untrack(() => taskInputs().length)
    taskEdited = false
  })

  // The host renders the bar (outside this clipped element) from this state.
  $effect(() => {
    onTaskState?.({ count: taskCount, edited: taskEdited })
  })

  function enhanceCodeBlocks(rootEl: HTMLElement) {
    for (const block of rootEl.querySelectorAll('.md-codeblock')) {
      if (block.querySelector(':scope > .md-codeblock__copy')) continue
      const pre = block.querySelector(':scope > pre')
      if (!pre) continue

      const btn = document.createElement('button')
      btn.type = 'button'
      btn.className = 'md-codeblock__copy'
      btn.setAttribute('aria-label', 'Копировать')
      btn.title = 'Копировать'
      btn.innerHTML = copySvg

      btn.addEventListener('click', async (event) => {
        event.preventDefault()
        event.stopPropagation()
        const code = pre.querySelector('code')
        const text = code?.textContent ?? pre.textContent ?? ''
        try {
          await copyText(text)
          btn.classList.add('md-codeblock__copy--done')
          btn.innerHTML = checkSvg
          btn.setAttribute('aria-label', 'Скопировано')
          btn.title = 'Скопировано'
          window.setTimeout(() => {
            btn.classList.remove('md-codeblock__copy--done')
            btn.innerHTML = copySvg
            btn.setAttribute('aria-label', 'Копировать')
            btn.title = 'Копировать'
          }, 1400)
        } catch {
          /* ignore */
        }
      })

      block.prepend(btn)
    }
  }

  /**
   * `error` doesn't bubble, so listen in the capture phase: a removed/missing
   * attachment (or a non-image one) becomes a visible placeholder instead of a
   * blank hole.
   */
  function paintAudioButtons(el: HTMLElement) {
    const track = player.track
    for (const btn of el.querySelectorAll<HTMLButtonElement>('button.md-audio')) {
      const id = Number(btn.dataset.audioId)
      const on = track?.id === id && player.playing
      const name = btn.dataset.audioName || `attachment=${id}`
      const time = track?.id === id && player.duration > 0 ? clock(player.current) : ''
      const icon = btn.querySelector('.md-audio__icon')
      const label = btn.querySelector('.md-audio__name')
      const clockEl = btn.querySelector('.md-audio__time')
      if (icon) icon.innerHTML = on ? pauseSvg : playSvg
      if (label) label.textContent = name
      if (clockEl) clockEl.textContent = time
      btn.setAttribute('aria-label', `${on ? 'Пауза' : 'Играть'}: ${name}`)
    }
  }

  function clock(seconds: number) {
    const t = Math.max(0, Math.floor(seconds))
    return `${Math.floor(t / 60)}:${String(t % 60).padStart(2, '0')}`
  }

  function audioButton(id: string, alt: string) {
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'md-audio'
    btn.dataset.audioId = id
    btn.dataset.audioName = alt || `attachment=${id}`
    btn.innerHTML = '<span class="md-audio__icon" aria-hidden="true"></span><span class="md-audio__name"></span><span class="md-audio__time"></span>'
    btn.addEventListener('click', (event) => {
      event.preventDefault()
      event.stopPropagation()
      playAttachment({ id: Number(id), filename: btn.dataset.audioName || `attachment-${id}.webm`, mime: 'audio/webm' })
    })
    btn.addEventListener('contextmenu', (event) => {
      event.preventDefault()
      event.stopPropagation()
      onAudioMenu?.(Number(id), { x: event.clientX, y: event.clientY })
    })
    return btn
  }

  async function onImgError(event: Event) {
    const img = event.target
    if (!(img instanceof HTMLImageElement) || !img.classList.contains('md-attachment-img')) return
    const m = /\/api\/attachments\/(\d+)\/image/.exec(img.getAttribute('src') || '')
    const alt = img.getAttribute('alt') || ''
    if (m) {
      try {
        if (await attachmentIsAudio(m[1])) {
          const btn = audioButton(m[1], alt)
          img.replaceWith(btn)
          if (root) paintAudioButtons(root)
          return
        }
      } catch {
        /* Fall through to the missing-image placeholder. */
      }
    }
    const ph = document.createElement('span')
    ph.className = 'md-img-missing'
    ph.textContent = `▢ вложение недоступно${m ? ` (attachment=${m[1]})` : ''}${alt ? ` — ${alt}` : ''}`
    img.replaceWith(ph)
  }

  function captureImgErrors(node: HTMLElement) {
    node.addEventListener('error', onImgError, true)
    return { destroy: () => node.removeEventListener('error', onImgError, true) }
  }

  function fitMapImages(node: HTMLElement, spec: { thumb?: number; html: string }) {
    const apply = (next: { thumb?: number; html: string }) => {
      for (const img of node.querySelectorAll<HTMLImageElement>('img.md-attachment-img')) {
        const match = /\/api\/attachments\/(\d+)\/image/.exec(img.getAttribute('src') || '')
        if (!match) continue
        const full = `/api/attachments/${match[1]}/image`
        img.dataset.full = full
        const src = next.thumb ? `${full}?preview=1&thumb=${next.thumb}` : full
        if (img.getAttribute('src') !== src) img.setAttribute('src', src)
      }
    }
    apply(spec)
    return { update: apply }
  }
  function imageAccess(node:HTMLElement,_html:string) {
    const apply=()=>{for(const img of node.querySelectorAll<HTMLImageElement>('img.md-attachment-img')){img.tabIndex=0;img.setAttribute('role','button');img.setAttribute('aria-label',`Открыть изображение ${img.alt}`);img.style.cursor='zoom-in'}}
    const key=(event:KeyboardEvent)=>{if((event.key==='Enter'||event.key===' ')&&event.target instanceof HTMLImageElement){event.preventDefault();event.target.click()}}
    node.addEventListener('keydown',key);queueMicrotask(apply)
    return {update(){queueMicrotask(apply)},destroy(){node.removeEventListener('keydown',key)}}
  }
  /** RMB on a phantom star resets that item's tick back to the source. */
  function onTaskContext(event: MouseEvent) {
    const audio = event.target instanceof Element ? event.target.closest('button.md-audio') : null
    if (audio instanceof HTMLButtonElement) {
      event.preventDefault()
      const id = Number(audio.dataset.audioId)
      if (id) onAudioMenu?.(id, { x: event.clientX, y: event.clientY })
      return
    }
    const star = event.target instanceof Element ? event.target.closest('.md-task__unsaved') : null
    if (!star) return
    event.preventDefault()
    const input = star.closest('li')?.querySelector<HTMLInputElement>('input.md-task')
    if (input) resetOneMark(input)
  }

  function onClick(event: MouseEvent) {
    const image=event.target
    if(image instanceof HTMLImageElement && image.classList.contains('md-attachment-img')) {
      event.preventDefault();event.stopPropagation()
      const images=[...root?.querySelectorAll<HTMLImageElement>('img.md-attachment-img')||[]].map(img=>({src:img.dataset.full||img.getAttribute('src')||'',title:img.alt||'Изображение'}))
      showImages(images,image.dataset.full||image.getAttribute('src')||'');return
    }
    if (event.target instanceof Element && event.target.closest('input.md-task')) {
      const input = event.target.closest('input.md-task')
      if (input instanceof HTMLInputElement && (event.ctrlKey || event.metaKey)) {
        saveOneMark(input)
        return
      }
      refreshTaskMarks()
      return
    }
    if (event.target instanceof Element && event.target.closest('.md-task__unsaved')) {
      const input = event.target.closest('li')?.querySelector<HTMLInputElement>('input.md-task')
      if (input) saveOneMark(input)
      return
    }
    const a = event.target instanceof Element ? event.target.closest('a') : null
    if (!a) return
    const href = a.getAttribute('href') || ''
    const ref = parseRefHref(href)
    if (!ref) return
    event.preventDefault()
    if (ref.fragment && ref.kind === 'note' && host?.kind === 'note' && host.id === ref.id && root) {
      scrollFragment(root, ref.fragment)
      const hash = `#${ref.fragment}`
      if (location.hash !== hash) history.replaceState(history.state, '', `${location.pathname}${location.search}${hash}`)
      return
    }
    if (ref.fragment && ref.kind === 'note') setPendingFragment(ref.id, ref.fragment)
    onRef?.(ref.kind, ref.id)
  }
</script>

{#if html}
  <div
    class={['md', className]}
    data-value-host={host ? `${host.kind}:${host.id}` : undefined}
    data-value-sig={valueSig}
    bind:this={root}
    onclick={onClick}
    oncontextmenu={onTaskContext}
    use:imageAccess={html}
    use:fitMapImages={{ thumb: imageThumb, html }}
    use:captureImgErrors
    use:refIcons
    use:scriptEmbed={merged}
    use:refQuotes={{ source, noteId: host?.kind === 'note' ? host.id : null, html }}
    use:refPreview={onRef}
    use:tailFade={typing}
    use:valueHover={hoverOpts}
    use:mapJumps={mapJump}
  >{@html html}</div>
{/if}
{#if edit}
  <ValueEditDialog open owner={{ kind: edit.kind, id: edit.id }} name={edit.name} onClose={() => (edit = null)} />
{/if}

{#if foreignTruncated}<p class="hint" role="status">Достигнут лимит внешних переменных: часть значений не загружена.</p>{/if}
