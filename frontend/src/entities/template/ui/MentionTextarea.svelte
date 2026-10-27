<script lang="ts">
  import { getScripts, listValues } from '@/shared/api/client.ts'
  import { completeText, textCompletionToken, type TextToken } from '@/shared/lib/textCompletion.ts'
  import InsertScriptModal from './InsertScriptModal.svelte'
  import { activeMentionToken, applyMention, matchMentions } from '@/shared/lib/mentionSuggest.ts'
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
        ? (await getScripts()).filter((s) => s.purpose === 'embed' && `${s.id} ${s.title}`.toLowerCase().includes(token.query.toLowerCase())).map((s) => ({ kind: 'script', id: s.id, title: s.title, label: 'скрипт', insertion: `script=${s.id}` }))
        : (await listValues(host?.kind ?? 'workspace', host?.id ?? 0)).effective.filter((v) => v.name.startsWith(token.query)).map((v, i) => ({ kind: 'value', id: i, title: v.name, label: 'переменная', insertion: `{{${v.name}}}` }))
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

  function onFieldKeydown(event: KeyboardEvent) {
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
  class={['mention-field', placement === 'inside' && 'mention-field--inside']}
>
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
  ></textarea>
  {#if mentionOpen}
    <ul class="mentions" role="listbox">
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
            <span class="mentions__kind">{item.label}</span>
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
