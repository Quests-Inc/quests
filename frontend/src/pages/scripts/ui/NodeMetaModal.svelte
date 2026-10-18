<script lang="ts">
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { deleteCustomNode, updateCustomNode } from '@/shared/api/client.ts'
  import type { CustomNode } from '@/shared/api/execution.gen.ts'
  import type { CustomField } from '@/shared/api/scriptgraph.gen.ts'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'

  /**
   * A baked node's name, description and fields (the label and the baked value of each; a field can be dropped). Its inside and its
   * ports change by unfolding it into the script and baking again over the same node.
   */
  let {
    open,
    node,
    onClose,
    onSaved,
  }: { open: boolean; node: CustomNode | null; onClose: () => void; onSaved: (n: CustomNode | null) => void } = $props()

  let title = $state('')
  let description = $state('')
  let fields = $state<CustomField[]>([])
  let busy = $state(false)
  let error = $state('')

  $effect(() => {
    if (!open || !node) return
    title = node.title
    description = node.description ?? ''
    fields = node.fields.map((f) => ({ ...f }))
    error = ''
  })

  const text = (f: CustomField) => (f.default === undefined || f.default === null ? '' : String(f.default))
  function setDefault(f: CustomField, v: string | boolean) {
    if (typeof v === 'boolean') f.default = v
    else if (v === '') f.default = undefined
    else f.default = f.type === 'int' ? Math.trunc(Number(v)) : v
  }

  async function submit() {
    if (!node) return
    busy = true
    error = ''
    try {
      const saved = await updateCustomNode(node.id, {
        title: title.trim(),
        description: description.trim() || undefined,
        version: node.version,
        inputs: node.inputs,
        outputs: node.outputs,
        fields,
        graph: node.graph,
      })
      onSaved(saved)
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  async function remove() {
    if (!node) return
    busy = true
    try {
      await deleteCustomNode(node.id)
      toast('Нода удалена', { kind: 'success' })
      onSaved(null)
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="node-meta-title" maxWidth="36rem">
  <ModalHead id="node-meta-title" title="Нода" icon="edit" {onClose} />
  <form
    class="modal__form"
    onsubmit={(e) => {
      e.preventDefault()
      void submit()
    }}
  >
    <label class="field">
      <span class="label">Название</span>
      <input type="text" bind:value={title} required maxlength="200" />
    </label>
    <label class="field">
      <span class="label">Описание</span>
      <input type="text" bind:value={description} placeholder="необязательно" />
    </label>
    <div class="field">
      <span class="label">Поля</span>
      {#each fields as f, i (f.name)}
        <div class="row">
          <input type="text" bind:value={f.label} placeholder={f.name} aria-label="Подпись поля" />
          {#if f.type === 'bool'}
            <label class="check"><input type="checkbox" checked={f.default === true} onchange={(e) => setDefault(f, e.currentTarget.checked)} />по умолчанию</label>
          {:else}
            <input
              type={f.type === 'int' ? 'number' : 'text'}
              value={text(f)}
              placeholder="по умолчанию"
              aria-label="Значение по умолчанию"
              onchange={(e) => setDefault(f, e.currentTarget.value)}
            />
          {/if}
          <button type="button" class="btn btn--ghost" title="Убрать поле: на ноде останется запечённое значение" onclick={() => (fields = fields.filter((_, j) => j !== i))}>×</button>
        </div>
      {:else}
        <p class="explain">Редактируемых полей нет.</p>
      {/each}
    </div>
    <p class="explain">Чтобы поменять, что внутри или порты: «Развернуть в ноды» в свойствах ноды, правка и «Запечь» поверх этой ноды.</p>
    {#if error}<p class="modal__error">{error}</p>{/if}
    <ModalFoot onCancel={onClose} submitLabel="Сохранить" {busy} disabled={!title.trim()}>
      {#snippet left()}
        <button type="button" class="btn btn--danger" disabled={busy} onclick={() => void remove()}>Удалить</button>
      {/snippet}
    </ModalFoot>
  </form>
</ModalShell>

<style>
  .row {
    display: flex;
    gap: 0.3rem;
    margin-bottom: 0.3rem;
    align-items: center;
  }
  .row input[type='text'],
  .row input[type='number'] {
    flex: 1 1 0;
    min-width: 0;
  }
</style>
