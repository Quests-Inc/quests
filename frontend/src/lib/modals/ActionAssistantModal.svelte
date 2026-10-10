<script>
  import { previewActionBatch, applyActionBatch } from '../js/api.ts'
  import { buildPreviewTree } from '../js/actionsPreview.ts'
  import Icon from '../ui/Icon.svelte'
  import MentionTextarea from '../ui/MentionTextarea.svelte'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import ModalFoot from './ModalFoot.svelte'

  /** @type {{
   *   open: boolean,
   *   quests: any[],
   *   questlines: any[],
   *   notes?: any[],
   *   attachments?: any[],
   *   onClose: () => void,
   *   onApplied: () => void,
   * }} */
  let { open = false, quests = [], questlines = [], notes = [], attachments = [], onClose, onApplied } = $props()

  let text = $state('')
  /** @type {'input' | 'loading' | 'preview' | 'applying'} */
  let phase = $state('input')
  let clarifyQuestion = $state('')
  let batch = $state(/** @type {any | null} */ (null))
  let preview = $state(/** @type {any[]} */ ([]))
  let errorMsg = $state('')
  let textareaEl = $state(/** @type {HTMLTextAreaElement | null} */ (null))

  let tree = $derived(
    phase === 'preview'
      ? buildPreviewTree(preview, { quests, questlines })
      : { questlines: [], looseQuests: [] },
  )

  $effect(() => {
    if (!open) return
    text = ''
    phase = 'input'
    clarifyQuestion = ''
    batch = null
    preview = []
    errorMsg = ''
    queueMicrotask(() => textareaEl?.focus())
  })

  async function generate() {
    const t = text.trim()
    if (!t || phase === 'loading') return
    phase = 'loading'
    errorMsg = ''
    try {
      // one clarifying question per request: after it the model must commit
      const res = await previewActionBatch(t, { noClarify: !!clarifyQuestion })
      if (res?.needs_clarification) {
        clarifyQuestion = res.clarify_question || 'Уточни, пожалуйста, запрос.'
        phase = 'input'
        return
      }
      clarifyQuestion = ''
      if (!res.batch?.actions?.length) {
        errorMsg = 'Модель не предложила действий. Переформулируй запрос.'
        phase = 'input'
        return
      }
      batch = res.batch
      preview = res.preview || []
      phase = 'preview'
    } catch (e) {
      errorMsg = e.message || String(e)
      phase = 'input'
    }
  }

  async function apply() {
    if (!batch || phase === 'applying') return
    phase = 'applying'
    errorMsg = ''
    try {
      await applyActionBatch(batch)
      onApplied?.()
      onClose()
    } catch (e) {
      errorMsg = e.message || String(e)
      phase = 'preview'
    }
  }

  function backToEdit() {
    phase = 'input'
    batch = null
    preview = []
  }

  function onPromptKeydown(event) {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault()
      generate()
    }
  }

  function fmtVal(v) {
    if (v === null || v === undefined || v === '') return '—'
    if (typeof v === 'boolean') return v ? 'да' : 'нет'
    return String(v)
  }
</script>

<ModalShell {open} {onClose} labelledby="aa-modal-title" zIndex={50} maxWidth="34rem">
      <ModalHead id="aa-modal-title" title="Командная строка журнала" icon="terminal" {onClose} />

      {#if errorMsg}
        <p class="modal__error">{errorMsg}</p>
      {/if}

      {#if phase !== 'preview'}
        <div class="modal__body">
          {#if clarifyQuestion}
            <p class="clarify">{clarifyQuestion}</p>
          {/if}
          <MentionTextarea
            bind:value={text}
            bind:el={textareaEl}
            {quests}
            {questlines}
            {notes}
            {attachments}
            rows={3}
            placeholder="Например: создай квестлайн Бэкапы и закинь туда @Про…"
            disabled={phase === 'loading'}
            onkeydown={onPromptKeydown}
          />
          <p class="hint">
            @ + название — подставить существующий квест/квестлайн/шаг. Ctrl/Cmd+Enter — сгенерировать план.
            Ничего не запишется без подтверждения.
          </p>
          <ModalFoot
            onCancel={onClose}
            submitLabel="Сгенерировать"
            onSubmit={generate}
            busy={phase === 'loading'}
            busyLabel="Думаю…"
            disabled={!text.trim()}
          />
        </div>
      {:else}
        <div class="modal__body">
          <p class="hint">Проверь план перед применением — можно вернуться и переформулировать.</p>
          <ul class="tree">
            {#each tree.questlines as line (line.id)}
              <li class="node node--questline">
                <span class="node__row">
                  {#if line.isNew}<span class="badge badge--new">NEW</span>{/if}
                  <Icon name="flag" size={13} />
                  <span class="node__label">{line.label}</span>
                </span>
                {#if line.quests.length}
                  <ul class="tree">
                    {#each line.quests as q (q.id)}
                      <li class="node node--quest">
                        <span class="node__row">
                          {#if q.isNew}<span class="badge badge--new">NEW</span>{/if}
                          <span class="node__label">{q.label}</span>
                        </span>
                        {#if q.changes.length}
                          <ul class="changes">
                            {#each q.changes as c}
                              <li>
                                <span class="change__field">{c.field}:</span>
                                {fmtVal(c.from)} → <strong>{fmtVal(c.to)}</strong>
                              </li>
                            {/each}
                          </ul>
                        {/if}
                        {#if q.steps.length}
                          <ul class="tree">
                            {#each q.steps as s (s.id)}
                              <li class="node node--step">
                                <span class="node__row">
                                  {#if s.isNew}<span class="badge badge--new">NEW</span>{/if}
                                  {#if s.deleted}<span class="badge badge--danger">DEL</span>{/if}
                                  <span class="node__label" class:node__label--deleted={s.deleted}>{s.label}</span>
                                </span>
                                {#if s.changes.length}
                                  <ul class="changes">
                                    {#each s.changes as c}
                                      <li>
                                        <span class="change__field">{c.field}:</span>
                                        {fmtVal(c.from)} → <strong>{fmtVal(c.to)}</strong>
                                      </li>
                                    {/each}
                                  </ul>
                                {/if}
                              </li>
                            {/each}
                          </ul>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>
            {/each}

            {#each tree.looseQuests as q (q.id)}
              <li class="node node--quest">
                <span class="node__row">
                  {#if q.isNew}<span class="badge badge--new">NEW</span>{/if}
                  <span class="node__label">{q.label}</span>
                </span>
                {#if q.changes.length}
                  <ul class="changes">
                    {#each q.changes as c}
                      <li>
                        <span class="change__field">{c.field}:</span>
                        {fmtVal(c.from)} → <strong>{fmtVal(c.to)}</strong>
                      </li>
                    {/each}
                  </ul>
                {/if}
                {#if q.steps.length}
                  <ul class="tree">
                    {#each q.steps as s (s.id)}
                      <li class="node node--step">
                        <span class="node__row">
                          {#if s.isNew}<span class="badge badge--new">NEW</span>{/if}
                          {#if s.deleted}<span class="badge badge--danger">DEL</span>{/if}
                          <span class="node__label">{s.label}</span>
                        </span>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>
            {/each}
          </ul>

          <ModalFoot
            onCancel={backToEdit}
            cancelLabel="Назад"
            submitLabel="Применить"
            submitIcon="checkmark"
            onSubmit={apply}
            busy={phase === 'applying'}
            busyLabel="Применяю…"
          />
        </div>
      {/if}
</ModalShell>

<style>
  .clarify {
    margin: 0;
    padding: 0.5rem 0.65rem;
    border: 1px solid color-mix(in srgb, var(--color-accent, #c9a227) 45%, var(--color-border, #333));
    border-radius: var(--radius-sm, 2px);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 10%, transparent);
    color: var(--color-fg, #e8e8e8);
    font-size: var(--text-sm, 0.875rem);
  }

  .tree,
  .changes {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .tree .tree,
  .tree .changes {
    margin-top: 0.3rem;
    padding-left: 1.1rem;
    border-left: 1px dashed var(--color-border, #333);
  }

  .node {
    margin-bottom: 0.4rem;
  }

  .node__row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg, #e8e8e8);
  }

  .node--questline .node__row {
    font-weight: 600;
    color: var(--color-accent, #c9a227);
  }

  .node__label--deleted {
    text-decoration: line-through;
    color: var(--color-danger, #b54a3a);
  }

  .badge {
    font-size: 0.65rem;
    font-weight: 700;
    letter-spacing: 0.03em;
    padding: 0.05rem 0.35rem;
    border-radius: var(--radius-sm, 2px);
  }

  .badge--new {
    color: #1a1a1a;
    background: var(--color-accent, #c9a227);
  }

  .badge--danger {
    color: #fff;
    background: var(--color-danger, #b54a3a);
  }

  .changes {
    margin-top: 0.15rem;
  }

  .changes li {
    font-size: var(--text-xs, 0.8rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .change__field {
    color: var(--color-fg, #e8e8e8);
  }

</style>
