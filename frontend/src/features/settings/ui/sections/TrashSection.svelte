<script lang="ts">
  import { onMount } from 'svelte'
  import SettingsBlock from './SettingsBlock.svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { emptyTrash, listTrash, purgeFromTrash, restoreFromTrash, type TrashItem } from '@/shared/api/client.ts'
  import { formatLocal, parseApiDate } from '@/shared/lib/time.ts'
  import { toast } from '@/shared/lib/toasts.svelte.ts'

  let items = $state<TrashItem[]>([])
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  /** What is about to be erased for good: one item, or everything. */
  let confirm = $state<{ item: TrashItem | null } | null>(null)

  async function refresh() {
    try {
      items = await listTrash()
      error = ''
    } catch (e) {
      error = (e as Error).message || String(e)
    } finally {
      loading = false
    }
  }

  onMount(() => {
    void refresh()
  })

  async function act(run: () => Promise<unknown>, done: string) {
    if (busy) return
    busy = true
    try {
      await run()
      toast(done, { kind: 'success' })
    } catch (e) {
      toast((e as Error).message || String(e), { kind: 'error' })
    } finally {
      busy = false
      await refresh()
    }
  }

  const restore = (t: TrashItem) => act(() => restoreFromTrash(t.kind, t.id), t.kind === 'quest' ? 'Квест восстановлен' : 'Заметка восстановлена')

  async function doPurge() {
    const c = confirm
    confirm = null
    if (!c) return
    if (c.item) await act(() => purgeFromTrash(c.item!.kind, c.item!.id), 'Удалено навсегда')
    else await act(() => emptyTrash(), 'Корзина очищена')
  }

  function daysLeft(t: TrashItem) {
    const d = parseApiDate(t.purge_at)
    return d ? Math.max(0, Math.ceil((d.getTime() - Date.now()) / 86_400_000)) : null
  }

  function meta(t: TrashItem) {
    const parts = [t.kind === 'quest' ? 'квест' : 'заметка']
    if (t.kind === 'quest' && t.steps_total) parts.push(`шаги ${t.steps_done ?? 0}/${t.steps_total}`)
    if (t.kind === 'note' && t.children) parts.push(`дочерних: ${t.children}`)
    parts.push(`удалено ${formatLocal(t.deleted_at, { year: undefined })}`)
    const left = daysLeft(t)
    if (left != null) parts.push(`сотрётся через ${left} дн.`)
    return parts.join(' · ')
  }

  const confirmText = $derived(
    confirm?.item
      ? `«${confirm.item.title}» будет стёрт без возможности восстановления вместе с вложениями.`
      : `Стереть всё, что в корзине (${items.length}), вместе с вложениями. Восстановить будет нельзя.`
  )
</script>

<SettingsBlock
  title="Удалённые квесты и заметки"
  hint="Удаление не стирает данные: квест или заметка лежит здесь 30 дней вместе со шагами, вложениями и секретами. Заметка возвращается под своего родителя вместе с дочерними."
>
  {#if loading}
    <p class="hint">Загрузка…</p>
  {:else if error}
    <p class="err" role="alert">{error}</p>
  {:else if !items.length}
    <p class="hint">Корзина пуста.</p>
  {:else}
    <ul class="trash">
      {#each items as t (t.kind + t.id)}
        <li class="row">
          <div class="row__text">
            <span class="row__title">{t.title || `${t.kind}=${t.id}`}</span>
            <span class="row__meta">{meta(t)}</span>
          </div>
          <button type="button" class="btn btn--ghost btn--icon" title="Восстановить" aria-label="Восстановить" disabled={busy} onclick={() => restore(t)}>
            <Icon name="undo" size={16} />
          </button>
          <button type="button" class="btn btn--ghost btn--icon" title="Стереть навсегда" aria-label="Стереть навсегда" disabled={busy} onclick={() => (confirm = { item: t })}>
            <Icon name="delete" size={16} />
          </button>
        </li>
      {/each}
    </ul>
    <div class="foot">
      <button type="button" class="btn" disabled={busy} onclick={() => (confirm = { item: null })}>Очистить корзину</button>
    </div>
  {/if}
</SettingsBlock>

<ConfirmModal
  open={!!confirm}
  title={confirm?.item ? 'Стереть навсегда?' : 'Очистить корзину?'}
  message={confirmText}
  confirmLabel="Стереть"
  {busy}
  onCancel={() => (confirm = null)}
  onConfirm={doPurge}
/>

<style>
  .trash {
    display: grid;
    gap: 0.35rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.35rem 0;
    border-bottom: 1px solid color-mix(in srgb, var(--color-border, #333) 60%, transparent);
  }
  .row__text {
    display: grid;
    flex: 1 1 auto;
    min-width: 0;
    gap: 0.1rem;
  }
  .row__title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row__meta,
  .hint {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.8rem;
  }
  .foot {
    display: flex;
    justify-content: flex-end;
  }
  .err {
    margin: 0;
    color: var(--color-danger, #b54a3a);
  }
</style>
