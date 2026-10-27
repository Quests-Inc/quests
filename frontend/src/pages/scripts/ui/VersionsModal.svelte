<script lang="ts">
  import { untrack } from 'svelte'
  import { getScriptVersion } from '@/shared/api/client.ts'
  import type { Graph } from '@/shared/api/scriptgraph.gen.ts'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  let { open, id, version, graph, dirty, onClose, onRestore }: {
    open: boolean; id: number | null; version: number; graph: Graph; dirty: boolean
    onClose: () => void; onRestore: (graph: Graph) => Promise<void>
  } = $props()
  let selected = $state(1)
  let snapshot = $state<Graph | null>(null)
  let error = $state('')
  let busy = $state(false)
  let loading = $state(false)
  let confirm = $state(false)
  let seq = 0
  $effect(() => {
    if (open) untrack(() => { selected = version; confirm = false })
  })
  $effect(() => {
    if (!open || id == null) return
    const script = id, n = selected, request = ++seq
    snapshot = null; error = ''; loading = true
    void getScriptVersion(script, n).then((v) => { if (request === seq) snapshot = v.graph }).catch((e) => { if (request === seq) error = e.message || String(e) }).finally(() => { if (request === seq) loading = false })
    return () => { seq++ }
  })
  async function restore() {
    if (!snapshot) return
    busy = true; error = ''; confirm = false
    try { await onRestore(snapshot); onClose() } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
  const same = $derived(snapshot && JSON.stringify(snapshot) === JSON.stringify(graph))
</script>
<ModalShell {open} {onClose} labelledby="versions-title" zIndex={75} maxWidth="60rem">
  <ModalHead id="versions-title" title="Версии скрипта" {onClose} />
  <div class="modal__body">
    <label>Версия <select value={selected} onchange={(e) => (selected = Number(e.currentTarget.value))} disabled={busy}>{#each Array.from({ length: version }, (_, i) => version - i) as n (n)}<option value={n}>{n}</option>{/each}</select></label>
    {#if error}<p class="modal__error" role="alert">{error}</p>{/if}
    {#if loading}<p class="hint">Загрузка версии…</p>{:else if snapshot}
      <p class="hint">{same ? 'Схема совпадает с редактором.' : 'Слева сохранённая версия, справа текущая схема редактора.'}</p>
      <div class="version-diff"><section><h3>Версия {selected}</h3><pre>{JSON.stringify(snapshot, null, 2)}</pre></section><section><h3>Редактор{dirty ? ' · не сохранён' : ''}</h3><pre>{JSON.stringify(graph, null, 2)}</pre></section></div>
      <button type="button" class="btn" disabled={busy || !!same} onclick={() => (confirm = true)}>Откатить к версии {selected}</button>
    {/if}
  </div>
</ModalShell>
<ConfirmModal zIndex={90} confirmLabel="Откатить" open={confirm} title="Откатить скрипт?" message={`Будет создана новая версия со схемой версии ${selected}.${dirty ? ' Несохранённые правки редактора будут заменены.' : ''}`} {busy} onCancel={() => (confirm = false)} onConfirm={() => void restore()} />
<style>
  .version-diff { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
  .version-diff section { min-width: 0; }
  pre { max-height: 50vh; overflow: auto; font-size: 0.8rem; }
  @media (max-width: 600px) { .version-diff { grid-template-columns: 1fr; } }
</style>
