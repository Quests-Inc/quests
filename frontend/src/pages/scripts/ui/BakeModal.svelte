<script lang="ts">
  import type { Candidate, ChosenField } from '@/entities/script'
  import type { CustomNode } from '@/shared/api/execution.gen.ts'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'

  /**
   * Bake the picked nodes into one: a name, and which of their parameters stay editable on the new node (a note's number, a
   * commit hash, a list of files). The links that leave the selection become the node's inputs and outputs.
   */
  let {
    open,
    count,
    candidates,
    nodeTitle,
    onClose,
    nodes,
    onBake,
  }: {
    open: boolean
    count: number
    candidates: Candidate[]
    nodeTitle: (id: number) => string
    onClose: () => void
    nodes: CustomNode[]
    onBake: (title: string, chosen: ChosenField[], replaceId?: number) => Promise<void>
  } = $props()

  let title = $state('')
  let replaceId = $state('')
  let on = $state<boolean[]>([])
  let labels = $state<string[]>([])
  let busy = $state(false)
  let error = $state('')

  $effect(() => {
    if (!open) return
    title = ''
    replaceId = ''
    error = ''
    on = candidates.map(() => false)
    labels = candidates.map((c) => c.param.replace(/^vars\./, ''))
  })

  async function submit() {
    busy = true
    error = ''
    try {
      const chosen = candidates.flatMap((c, i) => (on[i] ? [{ candidate: c, name: '', label: labels[i].trim() || c.param }] : []))
      await onBake(title, chosen, replaceId ? Number(replaceId) : undefined)
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="bake-title" maxWidth="36rem">
  <ModalHead id="bake-title" title="Запечь в ноду" icon="layers" {onClose} />
  <form
    class="modal__form"
    onsubmit={(e) => {
      e.preventDefault()
      void submit()
    }}
  >
    <p class="explain">Выбрано нод: {count}. Они станут одной нодой; связи наружу — её входы и выходы.</p>
    <label class="field">
      <span class="label">Название</span>
      <input type="text" bind:value={title} required maxlength="200" placeholder="например: Актуализировать заметку" />
    </label>
    <label class="field">
      <span class="label">Куда</span>
      <select bind:value={replaceId} onchange={() => { const n = nodes.find((x) => String(x.id) === replaceId); if (n) title = n.title }}>
        <option value="">новая нода</option>
        {#each nodes as n (n.id)}<option value={String(n.id)}>заменить «{n.title}» (скрипты с ней подхватят новую)</option>{/each}
      </select>
    </label>
    <div class="field">
      <span class="label">Редактируемые поля</span>
      {#each candidates as c, i (`${c.node}.${c.param}`)}
        <div class="cand">
          <label class="check">
            <input type="checkbox" bind:checked={on[i]} />
            <span><b>{nodeTitle(c.node)}</b> · <code>{c.param}</code></span>
          </label>
          <small class="val">{c.value === '' ? 'не задано' : c.value.length > 60 ? `${c.value.slice(0, 60)}…` : c.value}</small>
          {#if on[i]}<input type="text" bind:value={labels[i]} aria-label="Подпись поля" placeholder="подпись" />{/if}
        </div>
      {:else}
        <p class="explain">У выбранных нод нет параметров, которые можно вынести.</p>
      {/each}
    </div>
    {#if error}<p class="modal__error">{error}</p>{/if}
    <ModalFoot onCancel={onClose} submitLabel="Запечь" {busy} disabled={!title.trim()} />
  </form>
</ModalShell>

<style>
  .cand {
    display: grid;
    gap: 0.2rem;
    padding: 0.35rem 0;
    border-bottom: 1px solid var(--color-border, #333);
  }
  .val {
    color: var(--color-fg-muted, #9a9a9a);
    word-break: break-all;
  }
  .err {
    margin: 0 0 0.5rem;
    color: var(--color-danger, #b54a3a);
    font-size: var(--text-sm, 0.875rem);
  }
</style>
