<script lang="ts">
  import { untrack } from 'svelte'
  import { listValues, putValue, type ValueRow, type ValueType } from '@/shared/api/client.ts'
  import { touchValues, hostKey } from '@/shared/lib/valueLive.svelte.ts'
  import { readValueInput, valueInputText } from '@/shared/lib/valueInput.ts'
  import { valueTypeColor, valueTypeLabel } from '@/shared/lib/valueTip.ts'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'

  let {
    open = false,
    owner,
    name,
    readonly = false,
    onClose,
    onSaved,
  }: {
    open?: boolean
    owner: { kind: string; id: number }
    name: string
    readonly?: boolean
    onClose: () => void
    onSaved?: () => void
  } = $props()

  const HINT: Record<ValueType, string> = {
    int: '(str_score - 10) / 2',
    float: 'hp / 2.0',
    str: 'name + " волка"',
    bool: 'hp < 10',
  }
  const TYPES: ValueType[] = ['int', 'float', 'str', 'bool']

  let type = $state('int' as ValueType)
  let raw = $state('')
  let from = $state('')
  let error = $state('')
  let busy = $state(false)

  function fill(row: ValueRow | undefined) {
    type = row?.type ?? 'int'
    raw = row ? valueInputText(row) : ''
    from = row?.from ? (row.from === 'workspace' ? 'пространство' : row.title || `${row.from}=${row.id}`) : ''
    error = ''
  }

  $effect(() => {
    if (!open) return
    const kind = owner.kind
    const id = owner.id
    const valueName = name
    untrack(() => {
      void listValues(kind, id)
        .then((list) => fill([...(list.own ?? []), ...(list.inherited ?? [])].find((row) => row.name === valueName)))
        .catch((e) => {
          error = e instanceof Error ? e.message : String(e)
        })
    })
  })

  async function save() {
    if (readonly || busy) return
    const read = readValueInput(type, raw)
    if ('error' in read) {
      error = read.error
      return
    }
    busy = true
    error = ''
    try {
      await putValue(owner.kind, owner.id, name, type, read.value, read.formula)
      touchValues(hostKey(owner.kind, owner.id))
      onSaved?.()
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<ModalShell {open} {onClose} labelledby="value-edit-title" zIndex={90} maxWidth="28rem">
  <ModalHead id="value-edit-title" title={name} onClose={onClose} />
  {#if error}<p class="modal__error">{error}</p>{/if}
  <form
    class="modal__form"
    onsubmit={(e) => {
      e.preventDefault()
      e.stopPropagation() // the table may sit inside a quest's form
      void save()
    }}
  >
    {#if from}<p class="value-edit__from">Сейчас из {from}. Запись перекроет его здесь.</p>{/if}
    <div class="value-edit__top">
      <label class="field">
        <span class="label">Тип</span>
        <select bind:value={type} disabled={readonly || busy}>
          {#each TYPES as item (item)}
            <option value={item} style:color={valueTypeColor(item)}>{valueTypeLabel(item)}</option>
          {/each}
        </select>
      </label>
    </div>
    <p class="value-edit__hint">Число или да/нет, либо формула: + − * / % , ! && || и сравнения, деление целых округляет вниз. Строка: текст как есть; с + или сравнением — формула; в кавычках — всегда текст.</p>
    <label class="field">
      <span class="label">Значение или формула</span>
      <input type="text" bind:value={raw} placeholder={HINT[type]} disabled={readonly || busy} maxlength="1024" />
    </label>
    {#if !readonly}
      <button type="submit" class="btn btn--accent" disabled={busy}>Сохранить</button>
    {/if}
  </form>
</ModalShell>

<style>
  .value-edit__from,
  .value-edit__hint {
    margin: 0;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.85rem;
  }
  .value-edit__top {
    display: flex;
    flex-wrap: nowrap;
    align-items: flex-end;
    gap: 0.75rem;
  }
  .value-edit__top > .field {
    flex: 1 1 auto;
    min-width: 0;
  }
</style>
