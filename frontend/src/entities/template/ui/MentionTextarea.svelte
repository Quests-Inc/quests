<script lang="ts">
  import { getScripts, listValues } from '@/shared/api/client.ts'
  import { proseValue } from '@/shared/lib/valueText.ts'
  import { completeText, textCompletionToken, type TextToken } from '@/shared/lib/textCompletion.ts'
  import InsertScriptModal from './InsertScriptModal.svelte'
  import { activeMentionToken, applyMention, matchMentions } from '@/shared/lib/mentionSuggest.ts'
  import { pairBackspace, pairKey, type PairEdit } from '@/shared/lib/autoPair.ts'
  import { highlightMarkdown } from '@/shared/lib/mdHighlight.ts'
  import { caretXY, placePopup } from '@/shared/lib/caretXY.ts'
  import { templateStore } from '../model/templatesStore.svelte.ts'

  let {
    value = $bindable(''),
    quests = [],
    questlines = [],
    notes = [],
    attachments = [],
    placeholder = '',
    rows = 3,
    class: className = '',
    disabled = false,
    ariaLabel = '',
    allowedKinds = [],
    host,
    placement = 'below',
    highlight = true,
    pairs = true,
    el = $bindable(null),
    onkeydown,
    oninput,
  }: {
    value?: string,
    quests?: any[],
    questlines?: any[],
    notes?: any[],
    attachments?: any[],
    placeholder?: string,
    rows?: number,
    class?: string,
    disabled?: boolean,
    ariaLabel?: string,
    allowedKinds?: string[],
    host?: { kind: string; id: number },
    placement?: 'below' | 'inside',
    /** Colour the markdown and our inserts as they are typed (a layer under the field). Off for fields of references only. */
    highlight?: boolean,
    /** Close `*` `**` `{{` `(` `[` … by itself while typing. */
    pairs?: boolean,
    el?: HTMLTextAreaElement | null,
    onkeydown?: (event: KeyboardEvent) => void,
    oninput?: (event: Event) => void,
  } = $props()

  let mentionOpen = $state(false)
  let mentionItems = $state(([] as any[]))
  let mentionIndex = $state(0)
  let mentionToken = $state((null as { query: string, start: number } | null))

  let completion = $state<TextToken | null>(null)
  let insertOpen = $state(false)
  let insertAt = 0
  let completionSeq = 0
  async function refreshCompletion(token: TextToken) {
    const seq = ++completionSeq
    try {
      const items = token.kind === 'script'
        ? (await getScripts()).filter((s) => s.purpose === 'embed' && `${s.id} ${s.title}`.toLowerCase().includes(token.query.toLowerCase())).map((s) => ({ kind: 'script', id: s.id, title: s.title, label: '', hint: `№${s.id}`, insertion: `script=${s.id}` }))
        : (await listValues(host?.kind ?? 'workspace', host?.id ?? 0)).effective.filter((v) => v.name.startsWith(token.query)).map((v, i) => ({ kind: 'value', id: i, title: v.name, label: '', hint: v.error ? 'ошибка' : v.value == null ? '' : proseValue(v.type, v.value), insertion: `{{${v.name}}}` }))
      if (seq !== completionSeq) return
      mentionItems = items.slice(0, 8)
      mentionIndex = 0
      mentionOpen = mentionItems.length > 0
    } catch { if (seq === completionSeq) mentionOpen = false }
  }
  function refreshMentions(caret: number) {
    completionSeq++
    completion = allowedKinds.length ? null : textCompletionToken(value, caret)
    if (completion) { mentionOpen = false; void refreshCompletion(completion); return }
    const token = activeMentionToken(value, caret)
    if (!token) {
      mentionOpen = false
      mentionToken = null
      return
    }
    mentionToken = token
    mentionItems = matchMentions(token.query, {
      quests,
      questlines,
      notes,
      attachments,
      templates: templateStore.list,
    }).filter((item) => !allowedKinds.length || allowedKinds.includes(item.kind))
    mentionIndex = 0
    mentionOpen = mentionItems.length > 0
  }

  function onFieldInput(event: Event) {
    const node = (event.currentTarget as HTMLTextAreaElement)
    refreshMentions(node.selectionStart ?? value.length)
    oninput?.(event)
  }

  function pickMention(item: { kind: any; id: any; insertion?: string }) {
    if (!item || (!mentionToken && !completion)) return
    const { text: nextText, caret } = completion && item.insertion ? completeText(value, completion, item.insertion) : applyMention(value, mentionToken!, item)
    value = nextText
    mentionOpen = false
    mentionToken = null
    completion = null
    completionSeq++
    queueMicrotask(() => {
      el?.focus()
      el?.setSelectionRange(caret, caret)
    })
  }

  // The suggestion list stands at the caret: under its line, or above it when the screen leaves too little room below.
  let popupStyle = $state('')
  const POPUP = { w: 384, rowH: 34, maxH: 192 }
  $effect(() => {
    void value
    const n = mentionItems.length
    const ta = el
    const wrap = ta?.parentElement
    if (!mentionOpen || !ta || !wrap) {
      popupStyle = ''
      return
    }
    const c = caretXY(ta)
    if (!c) {
      popupStyle = ''
      return
    }
    const y = ta.offsetTop + c.top - ta.scrollTop
    const r = ta.getBoundingClientRect()
    const screenY = r.top + c.top - ta.scrollTop
    const p = placePopup(
      { left: ta.offsetLeft + c.left - ta.scrollLeft, top: y, height: c.height },
      { w: wrap.offsetWidth, h: wrap.offsetHeight },
      { w: POPUP.w, h: Math.min(POPUP.maxH, n * POPUP.rowH + 10) },
      { below: window.innerHeight - (screenY + c.height), above: screenY }
    )
    popupStyle = `left:${p.left}px;width:${p.width}px;right:auto;margin:0;${p.top != null ? `top:${p.top}px;bottom:auto` : `bottom:${p.bottom}px;top:auto`}`
  })

  // Highlighting: a layer under a transparent field, with the field's own font and padding copied to it.
  const markdownField = $derived(highlight && !allowedKinds.length)
  const MAX_HIGHLIGHT = 80_000
  let layer = $state(null as HTMLDivElement | null)
  const layerHtml = $derived(markdownField && value.length <= MAX_HIGHLIGHT ? highlightMarkdown(value) : '')
  const useLayer = $derived(markdownField && value.length <= MAX_HIGHLIGHT)

  function syncLayer() {
    const ta = el
    const lay = layer
    if (!ta || !lay) return
    const cs = getComputedStyle(ta)
    const set = lay.style
    // The layer sits exactly over the field's box, whatever else the wrapper holds (the «insert script» button, a hint).
    set.top = `${ta.offsetTop}px`
    set.left = `${ta.offsetLeft}px`
    set.width = `${ta.offsetWidth}px`
    set.height = `${ta.offsetHeight}px`
    set.font = cs.font
    set.letterSpacing = cs.letterSpacing
    set.lineHeight = cs.lineHeight
    set.tabSize = cs.tabSize
    set.textTransform = cs.textTransform
    set.paddingTop = cs.paddingTop
    set.paddingLeft = cs.paddingLeft
    set.paddingBottom = cs.paddingBottom
    // The field's scrollbar narrows its text; the layer has none, so it gives the same width away.
    const bars = ta.offsetWidth - ta.clientWidth - parseFloat(cs.borderLeftWidth) - parseFloat(cs.borderRightWidth)
    set.paddingRight = `${parseFloat(cs.paddingRight) + Math.max(0, bars)}px`
    set.borderWidth = cs.borderWidth
    set.borderRadius = cs.borderRadius
    set.backgroundColor = cs.backgroundColor === 'rgba(0, 0, 0, 0)' ? '' : cs.backgroundColor
    lay.scrollTop = ta.scrollTop
    lay.scrollLeft = ta.scrollLeft
  }
  $effect(() => {
    void layerHtml
    void useLayer
    if (!useLayer) return
    syncLayer()
  })
  $effect(() => {
    const ta = el
    if (!ta || !useLayer) return
    const ro = new ResizeObserver(() => syncLayer())
    ro.observe(ta)
    return () => ro.disconnect()
  })

  /** Apply a pair edit through the browser's own typing, so Ctrl+Z still undoes it. */
  function applyPair(edit: PairEdit) {
    const ta = el
    if (!ta) return
    if (edit.kind === 'edit') {
      ta.setSelectionRange(edit.start, edit.end)
      const done = edit.text ? document.execCommand('insertText', false, edit.text) : document.execCommand('delete')
      if (!done) {
        ta.setRangeText(edit.text, edit.start, edit.end, 'end')
        value = ta.value
      }
      ta.setSelectionRange(edit.caret, edit.selEnd ?? edit.caret)
    } else {
      ta.setSelectionRange(edit.caret, edit.caret)
    }
    refreshMentions(ta.selectionStart ?? edit.caret)
  }

  function onFieldKeydown(event: KeyboardEvent) {
    if (pairs && markdownField && !disabled && !mentionOpen && !event.isComposing && !event.ctrlKey && !event.metaKey && !event.altKey && el) {
      const a = el.selectionStart ?? 0
      const b = el.selectionEnd ?? a
      const edit = event.key === 'Backspace' ? pairBackspace(el.value, a, b) : pairKey(el.value, a, b, event.key)
      if (edit) {
        event.preventDefault()
        applyPair(edit)
        return
      }
    }
    if (mentionOpen) {
      if (event.key === 'ArrowDown') {
        event.preventDefault()
        mentionIndex = (mentionIndex + 1) % mentionItems.length
        return
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault()
        mentionIndex = (mentionIndex - 1 + mentionItems.length) % mentionItems.length
        return
      }
      if (event.key === 'Enter' || event.key === 'Tab') {
        event.preventDefault()
        pickMention(mentionItems[mentionIndex])
        return
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        event.stopPropagation()
        mentionOpen = false
        return
      }
    }
    onkeydown?.(event)
  }
</script>

<div
  class={['mention-field', placement === 'inside' && 'mention-field--inside', useLayer && 'mention-field--hl']}
>
  {#if useLayer}
    <div class="mdh-layer" bind:this={layer} aria-hidden="true">{@html layerHtml}</div>
  {/if}
  <textarea
    aria-label={ariaLabel || undefined}
    bind:this={el}
    bind:value
    {rows}
    {placeholder}
    {disabled}
    class={className}
    oninput={onFieldInput}
    onkeydown={onFieldKeydown}
    onscroll={syncLayer}
  ></textarea>
  {#if mentionOpen}
    <ul class="mentions" role="listbox" style={popupStyle || undefined}>
      {#each mentionItems as item, i (`${item.kind}:${item.id}`)}
        <li>
          <button
            type="button"
            class="mentions__opt"
            class:mentions__opt--on={i === mentionIndex}
            role="option"
            aria-selected={i === mentionIndex}
            onmousedown={(e) => e.preventDefault()}
            onclick={() => pickMention(item)}
          >
            {#if item.label}<span class="mentions__kind">{item.label}</span>{/if}
            <span class="mentions__title">{item.title}</span>
            {#if item.hint}<span class="mentions__hint">{item.hint}</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  {#if !allowedKinds.length && !disabled}
    <button type="button" class="insert-script" onclick={() => { insertAt = el?.selectionStart ?? value.length; insertOpen = true }}>Вставить скрипт…</button>
  {/if}
</div>
<InsertScriptModal open={insertOpen} onClose={() => (insertOpen = false)} onInsert={(text) => {
  value = value.slice(0, insertAt) + text + value.slice(insertAt)
  queueMicrotask(() => { el?.focus(); el?.setSelectionRange(insertAt + text.length, insertAt + text.length) })
}} />

<style>
  .insert-script { border: 0; background: none; color: var(--color-fg-muted, #9a9a9a); font: inherit; font-size: 0.75rem; cursor: pointer; padding: 0.25rem 0; }
  .mention-field--inside .insert-script { position: absolute; right: 1rem; top: 0.25rem; }
  .mention-field {
    position: relative;
    display: block;
    width: 100%;
  }

  .mention-field--inside {
    height: 100%;
    min-height: 18rem;
  }

  .mdh-layer {
    position: absolute;
    box-sizing: border-box;
    overflow: hidden;
    border: 1px solid transparent;
    color: var(--color-fg, #e8e8e8);
    white-space: pre-wrap;
    overflow-wrap: break-word;
    pointer-events: none;
  }

  /* The field itself stays, but its glyphs are drawn by the layer under it (the caret and the selection are the field's). */
  .mention-field--hl textarea {
    position: relative;
    background: transparent !important;
    color: transparent !important;
    -webkit-text-fill-color: transparent;
    caret-color: var(--color-fg, #e8e8e8);
  }

  .mention-field--hl textarea::selection {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 35%, transparent);
  }

  textarea {
    display: block;
    width: 100%;
    box-sizing: border-box;
    resize: vertical;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg, #121212);
    color: inherit;
    padding: 0.55rem 0.65rem;
    font: inherit;
  }

  .mention-field--inside textarea {
    height: 100%;
    min-height: 18rem;
  }

  .mentions {
    position: absolute;
    z-index: 8;
    top: 100%;
    left: 0;
    right: 0;
    margin: 0.25rem 0 0;
    max-height: 12rem;
    overflow-y: auto;
    list-style: none;
    padding: 0.25rem;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg-raised, #1a1a1a);
    box-shadow: 0 8px 24px color-mix(in srgb, #000 45%, transparent);
  }

  .mention-field--inside .mentions {
    top: auto;
    bottom: 0.4rem;
    left: 0.4rem;
    right: 0.4rem;
    margin: 0;
  }

  .mentions__opt {
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
    width: 100%;
    border: 0;
    border-radius: var(--radius-sm, 2px);
    background: transparent;
    color: var(--color-fg, #e8e8e8);
    padding: 0.3rem 0.4rem;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .mentions__opt--on {
    background: color-mix(in srgb, var(--color-accent, #c9a227) 18%, transparent);
  }

  .mentions__kind {
    flex-shrink: 0;
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .mentions__title {
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm, 0.875rem);
  }

  .mentions__hint {
    flex-shrink: 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }
</style>
