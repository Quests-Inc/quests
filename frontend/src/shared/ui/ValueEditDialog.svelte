<script lang="ts">
  import { untrack } from 'svelte'
  import { listValues, putValue, type ValueRow, type ValueType } from '@/shared/api/client.ts'
  import { touchValues, hostKey } from '@/shared/lib/valueLive.svelte.ts'
  import { valueTypeColor, valueTypeLabel } from '@/shared/lib/valueTip.ts'
  import ModalShell from './ModalShell.svelte'
  import ModalHead from './ModalHead.svelte'
  import Toggle from './Toggle.svelte'

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
  let flag = $state(false)
  let asFormula = $state(false)
  let from = $state('')
  let error = $state('')
  let busy = $state(false)

  function literal(): number | string | boolean | null {
    if (type === 'bool') return flag
    if (type === 'str') return raw
    if (raw.trim() === '' || Number.isNaN(Number(raw))) return null
    return type === 'int' ? Math.trunc(Number(raw)) : Number(raw)
  }

  function fill(row: ValueRow | undefined) {
    type = row?.type ?? 'int'
    asFormula = Boolean(row?.formula)
    raw = row?.formula || (row && row.type !== 'bool' && row.value != null ? String(row.value) : '')
    flag = row?.value === true
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
    if (asFormula && !raw.trim()) {
      error = 'нужна формула'
      return
    }
    const value = asFormula ? '' : literal()
    if (!asFormula && value == null) {
      error = 'нужно значение'
      return
    }
    busy = true
    error = ''
    try {
      await putValue(owner.kind, owner.id, name, type, value ?? '', asFormula ? raw.trim() : '')
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
      <div class="value-edit__switch">
        <Toggle label="формула" bind:checked={asFormula} disabled={readonly || busy} />
      </div>
    </div>
    {#if asFormula}
      <p class="value-edit__hint">Числа: + − * / % , деление целых округляет вниз. Да/нет: ! && || и сравнение. Строки склеиваются через +.</p>
      <label class="field">
        <span class="label">Формула</span>
        <input type="text" bind:value={raw} placeholder={HINT[type]} disabled={readonly || busy} maxlength="1024" />
      </label>
    {:else if type === 'bool'}
      <Toggle label="да" bind:checked={flag} disabled={readonly || busy} />
    {:else}
      <label class="field">
        <span class="label">Значение</span>
        <input type="text" bind:value={raw} disabled={readonly || busy} />
      </label>
    {/if}
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
  .value-edit__switch {
    flex: none;
    padding-bottom: 0.3rem;
  }
</style>
