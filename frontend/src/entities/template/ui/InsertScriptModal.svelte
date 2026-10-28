<script lang="ts">
  import { getScripts } from '@/shared/api/client.ts'
  import type { Script } from '@/shared/api/execution.gen.ts'
  import { scriptInsertion } from '@/shared/lib/textCompletion.ts'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  let { open, onClose, onInsert }: { open: boolean; onClose: () => void; onInsert: (text: string) => void } = $props()
  let scripts = $state<Script[]>([])
  let id = $state(0), arg = $state(''), auto = $state(false), hide = $state(false), set = $state(''), on = $state(''), error = $state('')
  $effect(() => {
    if (!open) return
    let live = true
    id = 0; arg = ''; auto = false; hide = false; set = ''; on = ''; error = ''
    void getScripts().then((all) => { if (live) { scripts = all.filter((s) => s.purpose === 'embed'); id = scripts[0]?.id ?? 0 } }).catch((e) => { if (live) error = e.message || String(e) })
    return () => { live = false }
  })
  function insert() {
    try { onInsert(scriptInsertion(Number(id), arg, { auto, hide, set: set.trim(), on: on.trim() })); onClose() } catch (e) { error = e instanceof Error ? e.message : String(e) }
  }
</script>
<ModalShell {open} {onClose} labelledby="insert-script-title" zIndex={95} maxWidth="34rem">
  <ModalHead id="insert-script-title" title="Вставить скрипт" icon="edit" {onClose} />
  <form
    class="modal__form"
    onsubmit={(e) => {
      e.preventDefault()
      insert()
    }}
  >
    {#if error}<p class="modal__error" role="alert">{error}</p>{/if}
    <label class="field">
      <span class="label">Скрипт</span>
      <select bind:value={id} disabled={!scripts.length}>
        {#each scripts as s (s.id)}<option value={s.id}>{s.title}</option>{/each}
      </select>
    </label>
    {#if !scripts.length}
      <p class="explain">Нет скриптов с назначением «Вставки в текст». Создайте такой в «Автоматике» → «Скрипты».</p>
    {/if}
    <label class="field">
      <span class="label">Выражение</span>
      <input type="text" bind:value={arg} placeholder="2к6 + сила" maxlength="200" />
    </label>
    <label class="check">
      <input type="checkbox" bind:checked={auto} disabled={!!set || !!on} />
      Вычислять при показе
    </label>
    <label class="check">
      <input type="checkbox" bind:checked={hide} />
      Скрывать выражение
    </label>
    <label class="field">
      <span class="label">Записать результат в переменную</span>
      <input type="text" bind:value={set} placeholder="hp" />
    </label>
    <label class="field">
      <span class="label">Раннер</span>
      <input type="text" bind:value={on} placeholder="laptop" />
    </label>
    <p class="explain">
      Клик по вставке считает выражение заново. «Вычислять при показе» считает сразу, как только текст открыт, но недоступно с записью в
      переменную и с раннером. Имя переменной и раннер необязательны.
    </p>
    <ModalFoot onCancel={onClose} submitLabel="Вставить" submitIcon="add" disabled={!id || !arg.trim()} />
  </form>
</ModalShell>
