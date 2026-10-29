<script lang="ts">
  import type { RevisionRead } from '@/shared/api/knowledge.gen'
  import type { AttachmentView } from '@/shared/api/knowledge_http.gen'
  import { EntityHistory } from '@/entities/history'
  import { AttachmentPreview, formatSize, formatWhen } from '@/entities/attachment'
  import Icon from '@/shared/ui/Icon.svelte'

  /** Панель выбранного вложения: превью, действия, сведения, комментарий, версии и история. */
  let {
    a,
    busy = false,
    revs,
    revsBusy,
    commentDraft = $bindable(''),
    ownerKindLabel,
    ownerLabel,
    onClose,
    onDownload,
    onToggleHidden,
    onPickVersion,
    onDelete,
    onOpenOwner,
    onSaveComment,
    onMakeCurrent,
    onDeleteRevision,
    onCopyRef,
  }: {
    a: AttachmentView
    busy?: boolean
    revs: RevisionRead[]
    revsBusy: boolean
    commentDraft?: string
    ownerKindLabel: string
    ownerLabel: string
    onClose: () => void
    onDownload: (revision?: number | null) => void
    onToggleHidden: () => void
    onPickVersion: () => void
    onDelete: () => void
    onOpenOwner: () => void
    onSaveComment: () => void
    onMakeCurrent: (revision: number) => void
    onDeleteRevision: (revision: number) => void
    onCopyRef: () => void
  } = $props()
</script>

    <aside class="attpage__panel" aria-label="Вложение">
      <div class="attpage__phead">
        <button type="button" class="attpage__back" onclick={onClose} aria-label="Закрыть">
          <Icon name="close" size={16} />
        </button>
        <span class="attpage__pref">attachment={a.id}</span>
      </div>

      <div class="attpage__preview" class:attpage__preview--hidden={a.hidden}>
        <AttachmentPreview {a} />
        {#if a.hidden}
          <span class="attpage__hiddenbadge"><Icon name="eye-off" size={14} /> скрыто</span>
        {/if}
      </div>

      <h2 class="attpage__ptitle">{a.filename || `attachment=${a.id}`}</h2>

      <div class="attpage__pactions">
        <button type="button" class="attpage__btn attpage__btn--main" onclick={() => onDownload()}>
          <Icon name="download" size={14} /> Скачать
        </button>
        <button type="button" class="attpage__btn" disabled={busy} onclick={onToggleHidden}>
          <Icon name={a.hidden ? 'eye' : 'eye-off'} size={14} />
          {a.hidden ? 'Вернуть' : 'Скрыть'}
        </button>
        <button type="button" class="attpage__btn" disabled={busy} onclick={onPickVersion}>
          <Icon name="renew" size={14} /> Новая версия
        </button>
        <button type="button" class="attpage__btn attpage__btn--danger" onclick={onDelete}>
          <Icon name="delete" size={14} /> Удалить
        </button>
      </div>

      <dl class="attpage__facts">
        <dt>Владелец</dt>
        <dd>
          <button type="button" class="attpage__link" onclick={onOpenOwner}>
            {ownerKindLabel} · {ownerLabel}
          </button>
        </dd>
        <dt>Размер</dt>
        <dd>{formatSize(a.size_bytes)}</dd>
        <dt>Тип</dt>
        <dd class="attpage__mono">{a.content_type_detected || a.content_type_declared || '—'}</dd>
        <dt>Загружено</dt>
        <dd>{formatWhen(a.uploaded_at, true)}</dd>
        <dt>Проверка</dt>
        <dd>{a.scan_status || '—'}</dd>
      </dl>

      <label class="attpage__lbl" for="att-comment">Комментарий</label>
      <textarea
        id="att-comment"
        class="attpage__comment"
        rows="2"
        maxlength="500"
        placeholder="зачем этот файл"
        bind:value={commentDraft}
        onblur={onSaveComment}
      ></textarea>

      <div class="attpage__lbl">Версии ({revs.length || a.revision_count || 1})</div>
      {#if revsBusy && revs.length === 0}
        <p class="attpage__muted">загрузка…</p>
      {:else}
        <ul class="attpage__revs">
          {#each revs as r (r.revision)}
            <li class="attpage__rev" class:attpage__rev--cur={r.is_current}>
              <span class="attpage__rev-v">v{r.revision}</span>
              <span class="attpage__rev-info">
                {formatWhen(r.uploaded_at)} · {formatSize(r.size_bytes)}
                {#if r.is_current}<b>· текущая</b>{/if}
              </span>
              <button type="button" class="attpage__ico" title="Скачать версию" onclick={() => onDownload(r.revision)}>
                <Icon name="download" size={14} />
              </button>
              {#if !r.is_current}
                <button
                  type="button"
                  class="attpage__ico"
                  title="Сделать текущей"
                  disabled={busy}
                  onclick={() => onMakeCurrent(r.revision)}
                >
                  <Icon name="arrow-up" size={14} />
                </button>
                <button
                  type="button"
                  class="attpage__ico attpage__ico--danger"
                  title="Удалить версию"
                  onclick={() => onDeleteRevision(r.revision)}
                >
                  <Icon name="delete" size={14} />
                </button>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      <EntityHistory
        entityType="attachment"
        entityId={a.id}
        version={`${a.revision}-${a.hidden}-${a.uploaded_at}`}
        labelClass="attpage__lbl"
      />

      <button type="button" class="attpage__link attpage__copy" onclick={onCopyRef}>
        <Icon name="copy" size={12} /> Копировать attachment={a.id}
      </button>
    </aside>

<style>


  .attpage__btn,
  :global(.attpage__ico),
  .attpage__back,
  .attpage__link {
    font: inherit;
    color: inherit;
    cursor: pointer;
  }

  /* Detail panel */
  .attpage__panel {
    overflow: auto;
    min-height: 0;
    padding: 0.75rem 1rem 1.5rem;
    border-left: 1px solid var(--color-border, #333);
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .attpage__phead {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .attpage__pref {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__back {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.9rem;
    height: 1.9rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__back:hover {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .attpage__preview {
    position: relative;
    display: grid;
    place-items: center;
    height: 16rem;
    overflow: hidden;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 5%, transparent);
  }

  .attpage__preview--hidden :global(.file-preview) {
    opacity: 0.55;
  }

  .attpage__hiddenbadge {
    position: absolute;
    top: 0.4rem;
    left: 0.4rem;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.1rem 0.45rem;
    border-radius: var(--radius-sm, 3px);
    background: var(--color-bg-raised, #1a1a1a);
    border: 1px solid var(--color-border, #333);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__ptitle {
    margin: 0;
    font-size: var(--text-md, 1rem);
    font-weight: 600;
    word-break: break-word;
  }

  .attpage__pactions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .attpage__btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.3rem 0.65rem;
    border: 1px solid var(--color-border, #3a3a3a);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__btn:hover:not(:disabled) {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 8%, transparent);
  }

  .attpage__btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .attpage__btn--main {
    color: var(--color-accent, #c9a227);
    border-color: var(--color-accent, #c9a227);
  }

  .attpage__btn--danger:hover:not(:disabled) {
    color: var(--color-danger, #b54a3a);
    border-color: var(--color-danger, #b54a3a);
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 12%, transparent);
  }

  .attpage__facts {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 0.25rem 0.8rem;
    margin: 0.2rem 0;
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__facts dt {
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__facts dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .attpage__mono {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
  }

  .attpage__link {
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    color: var(--color-accent, #c9a227);
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__link:hover {
    text-decoration: underline;
  }

  .attpage__copy {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    margin-top: 0.4rem;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__lbl {
    margin-top: 0.3rem;
    font-size: 0.68rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--color-fg-subtle, #6e6e6e);
  }

  .attpage__comment {
    width: 100%;
    resize: vertical;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: inherit;
    font: inherit;
    font-size: var(--text-sm, 0.875rem);
  }

  .attpage__muted {
    margin: 0;
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__revs {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.15rem;
  }

  .attpage__rev {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.1rem 0.3rem;
    border-radius: var(--radius-sm, 3px);
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .attpage__rev--cur {
    color: var(--color-fg, #e8e8e8);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 10%, transparent);
  }

  .attpage__rev-v {
    flex: 0 0 auto;
    font-family: var(--font-mono, monospace);
  }

  .attpage__rev-info {
    flex: 1 1 auto;
    min-width: 0;
  }

  .attpage__rev :global(.attpage__ico) {
    width: 1.6rem;
    height: 1.6rem;
  }@media (max-width: 860px) {


    .attpage__panel {
      border-left: 0;
    }
}
</style>
