<script lang="ts">
  import { untrack } from 'svelte'
  import { duplicateNote } from '@/shared/api/notes.ts'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  type Source = { id: number; title: string; parent_id?: number | null }
  let { source, notes, onClose, onCreated }: { source: Source; notes: Source[]; onClose: () => void; onCreated: (id: number) => Promise<void> } = $props()
  let title = $state(untrack(() => `${source.title} (копия)`))
  let parent = $state(untrack(() => source.parent_id == null ? '' : String(source.parent_id)))
  let busy = $state(false)
  let error = $state('')
  async function create() {
    if (busy || !title.trim()) return
    busy = true
    try {
      const created = await duplicateNote(source.id, { title: title.trim(), parent_id: parent ? Number(parent) : null })
      await onCreated(created.id)
      onClose()
    } catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { busy = false }
  }
</script>
<ModalShell open onClose={onClose} labelledby="sample-create-title" closeDisabled={busy}>
  <h2 id="sample-create-title">Создать по образцу</h2>
  <form onsubmit={e => { e.preventDefault(); void create() }}>
    <label class="field"><span class="label">Название</span><input bind:value={title} required aria-label="Название новой заметки" /></label>
    <label class="field"><span class="label">Родительская заметка</span><select bind:value={parent} aria-label="Родительская заметка"><option value="">Корень</option>{#each notes as n (n.id)}<option value={String(n.id)}>{n.title}</option>{/each}</select></label>
    {#if error}<p role="alert">{error}</p>{/if}
    <button type="submit" class="btn" disabled={busy || !title.trim()}>Создать</button>
  </form>
</ModalShell>
