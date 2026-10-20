<script lang="ts">
  import { type CodeField, type CodeSpec, codeNodeDef, codeSpecOf } from '@/entities/script'
  import { toast } from '@/shared/lib/toasts.svelte.ts'
  import { createCustomNode, deleteCustomNode, updateCustomNode } from '@/shared/api/client.ts'
  import type { CustomNode } from '@/shared/api/execution.gen.ts'
  import Icon from '@/shared/ui/Icon.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'

  /**
   * A node written as code: its inputs arrive as variables (the name in capitals), its fields are fixed variables the user edits on
   * the node. With `editing` the form opens on that node (only nodes of this shape can be edited here).
   */
  let {
    open,
    editing = null,
    onClose,
    onSaved,
  }: { open: boolean; editing?: CustomNode | null; onClose: () => void; onSaved: (n: CustomNode) => void } = $props()

  const blank = (): CodeSpec => ({
    title: '',
    description: '',
    language: '',
    text: '# значения входов — переменные в ВЕРХНЕМ регистре, поля — тоже\ntrue',
    inputs: [],
    fields: [],
  })
  let spec = $state<CodeSpec>(blank())
  let busy = $state(false)
  let error = $state('')

  $effect(() => {
    if (!open) return
    error = ''
    spec = (editing && codeSpecOf(editing)) || blank()
  })

  const LANGS = [
    { id: '', label: 'sh' },
    { id: 'bash', label: 'bash' },
    { id: 'python', label: 'python' },
    { id: 'node', label: 'node' },
  ]
  const addInput = () => (spec.inputs = [...spec.inputs, ''])
  const dropInput = (i: number) => (spec.inputs = spec.inputs.filter((_, j) => j !== i))
  const addField = () => (spec.fields = [...spec.fields, { name: '', label: '', type: 'str', value: '' } as CodeField])
  const dropField = (i: number) => (spec.fields = spec.fields.filter((_, j) => j !== i))

  async function submit() {
    busy = true
    error = ''
    try {
      const def = codeNodeDef(spec)
      const saved = editing ? await updateCustomNode(editing.id, { ...def, version: editing.version }) : await createCustomNode(def)
      onSaved(saved)
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  async function remove() {
    if (!editing) return
    busy = true
    try {
      await deleteCustomNode(editing.id)
      toast('Нода удалена', { kind: 'success' })
      onSaved(editing)
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="code-node-title" maxWidth="40rem">
  <ModalHead id="code-node-title" title={editing ? 'Нода из кода' : 'Новая нода из кода'} icon="edit" {onClose} />
  <form
    class="modal__form"
    onsubmit={(e) => {
      e.preventDefault()
      void submit()
    }}
  >
    <label class="field">
      <span class="label">Название</span>
      <input type="text" bind:value={spec.title} required maxlength="200" />
    </label>
    <label class="field">
      <span class="label">Описание</span>
      <input type="text" bind:value={spec.description} placeholder="необязательно" />
    </label>
    <div class="field">
      <span class="label">Входы</span>
      <p class="explain">Секреты нода получает всегда. Каждый вход приходит переменной окружения: имя входа заглавными.</p>
      {#each spec.inputs as _, i (i)}
        <div class="row">
          <input type="text" bind:value={spec.inputs[i]} placeholder="имя входа" aria-label="Имя входа" />
          <button type="button" class="btn btn--icon btn--ghost" onclick={() => dropInput(i)} title="Убрать" aria-label="Убрать вход">
            <Icon name="close" />
          </button>
        </div>
      {/each}
      <button type="button" class="btn btn--ghost" onclick={addInput}><Icon name="add" size={14} /> Вход</button>
    </div>
    <div class="field">
      <span class="label">Поля</span>
      <p class="explain">Значения, которые правят на самой ноде в скрипте (номер заметки, хеш коммита…). В коде — переменные того же имени.</p>
      {#each spec.fields as f, i (i)}
        <div class="row">
          <input type="text" bind:value={f.name} placeholder="имя" aria-label="Имя поля" />
          <input type="text" bind:value={f.label} placeholder="подпись" aria-label="Подпись поля" />
          <select bind:value={f.type} aria-label="Тип поля">
            <option value="str">текст</option>
            <option value="int">число</option>
            <option value="bool">флаг</option>
          </select>
          <input type="text" bind:value={f.value} placeholder="по умолчанию" aria-label="Значение по умолчанию" />
          <button type="button" class="btn btn--icon btn--ghost" onclick={() => dropField(i)} title="Убрать" aria-label="Убрать поле">
            <Icon name="close" />
          </button>
        </div>
      {/each}
      <button type="button" class="btn btn--ghost" onclick={addField}><Icon name="add" size={14} /> Поле</button>
    </div>
    <label class="field">
      <span class="label">Язык</span>
      <select bind:value={spec.language}>
        {#each LANGS as l (l.id)}<option value={l.id}>{l.label}</option>{/each}
      </select>
    </label>
    <label class="field">
      <span class="label">Код</span>
      <textarea class="mono" rows="10" spellcheck="false" bind:value={spec.text} required></textarea>
    </label>
    <p class="explain">Выходы: <code>stdout</code> (вывод при коде 0) и <code>failed</code> (любой другой исход документом). Исполняется на раннере.</p>
    {#if error}<p class="modal__error">{error}</p>{/if}
    <ModalFoot onCancel={onClose} submitLabel={editing ? 'Сохранить' : 'Создать'} {busy} disabled={!spec.title.trim()}>
      {#snippet left()}
        {#if editing}
          <button type="button" class="btn btn--danger" disabled={busy} onclick={() => void remove()}>Удалить</button>
        {/if}
      {/snippet}
    </ModalFoot>
  </form>
</ModalShell>

<style>
  .row {
    display: flex;
    gap: 0.3rem;
    margin-bottom: 0.3rem;
  }
  .row input,
  .row select {
    flex: 1 1 0;
    min-width: 0;
  }
  textarea {
    width: 100%;
    font-size: var(--text-xs, 0.75rem);
    line-height: 1.4;
  }
</style>
