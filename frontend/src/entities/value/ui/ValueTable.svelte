<script lang="ts">
  import { untrack } from 'svelte'
  import { deleteValue, fetchWorkspaces, listValues, putValue, rollValue, type ValueRow, type ValueType } from '@/shared/api/client.ts'
  import { ANY_HOST, hostKey, publishValues, touchValues, valueRev } from '@/shared/lib/valueLive.svelte.ts'
  import { readValueInput } from '@/shared/lib/valueInput.ts'
  import { valueDetail, valueTypeColor, valueTypeLabel } from '@/shared/lib/valueTip.ts'
  import { effectiveText, formatValue, valueSummary } from '../model/format.ts'
  import FormSection from '@/shared/ui/FormSection.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import SecretTable from './SecretTable.svelte'
  import ValueEditDialog from '@/shared/ui/ValueEditDialog.svelte'

  let {
    owner,
    readonly = false,
    variant = 'text',
    onEffective,
  }: {
    owner: { kind: string; id: number }
    readonly?: boolean
    variant?: 'text' | 'icon' | 'hidden' | 'inline' | 'panel'
    onEffective?: (text: Record<string, string>) => void
  } = $props()

  const NAME = /^[a-z][a-z0-9_]{0,23}$/
  const HINT: Record<ValueType, string> = {
    int: '(str_score - 10) / 2',
    float: 'hp / 2.0',
    str: 'name + " волка"',
    bool: 'hp < 10',
  }
  const TYPES: ValueType[] = ['int', 'float', 'str', 'bool']

  let workspaceReadonly = $state(true)
  const editReadonly = $derived(readonly || (owner.kind === 'workspace' && workspaceReadonly))
  let own = $state([] as ValueRow[])
  let inherited = $state([] as ValueRow[])
  let tab = $state<'values' | 'secrets'>('values')
  let open = $state(false)
  let sectionOpen = $state(false)
  let error = $state('')
  let busy = $state(false)
  let truncated = $state(false)
  let name = $state('')
  let type = $state('int' as ValueType)
  let raw = $state('')
  let editing = $state('')
  let seenKey = ''

  const summary = $derived(
    valueSummary(
      [...own, ...inherited]
        .filter((row) => !row.error && row.value != null)
        .map((row) => ({ name: row.name, text: formatValue(row.type, row.value) })),
    ),
  )
  const label = $derived(own.length ? `Окружение · ${own.length}` : 'Окружение')
  const title = $derived(summary ? `Окружение: ${summary}` : 'Окружение')

  function source(row: ValueRow) {
    if (row.from === 'workspace') return 'пространство'
    return row.title || `${row.from}=${row.id}`
  }

  function liveOf(rows: ValueRow[]) {
    return rows.map((row) => ({
      name: row.name,
      type: row.type,
      text: row.value == null ? '' : formatValue(row.type, row.value),
      formula: row.formula ?? '',
      error: row.error ?? '',
      roll: row.roll?.arg ?? '',
    }))
  }

  async function load() {
    const kind = owner.kind
    const id = owner.id
    const key = hostKey(kind, id)
    onEffective?.({})
    error = ''
    try {
      const list = await listValues(kind, id)
      if (kind === 'workspace') {
        const spaces = await fetchWorkspaces()
        if (hostKey(owner.kind, owner.id) !== key) return
        workspaceReadonly = spaces.workspaces.find((w) => w.id === spaces.current)?.role !== 'owner'
      }
      if (hostKey(owner.kind, owner.id) !== key) return
      own = list.own ?? []
      inherited = list.inherited ?? []
      truncated = !!list.truncated
      const effective = (list.effective ?? []).filter((row) => !row.error && row.value != null)
      onEffective?.(effectiveText(effective))
      publishValues(key, liveOf(list.effective ?? []))
    } catch (e) {
      if (hostKey(owner.kind, owner.id) !== key) return
      error = e instanceof Error ? e.message : String(e)
      own = []
      inherited = []
      publishValues(key, [])
    }
  }

  $effect(() => {
    const key = hostKey(owner.kind, owner.id)
    const rev = valueRev.n
    const revHost = valueRev.host
    untrack(() => {
      if (key !== seenKey) {
        seenKey = key
        void load()
        return
      }
      if ((revHost === key || revHost === ANY_HOST) && rev) void load()
    })
  })

  async function add() {
    if (!NAME.test(name) || name === 'expr' || name === 'true' || name === 'false') {
      error = 'имя: строчные латинские, цифры и _, не expr, true, false'
      return
    }
    const read = readValueInput(type, raw)
    if ('error' in read) {
      error = read.error
      return
    }
    await write(name, type, read.value, read.formula)
    if (!error) {
      name = ''
      raw = ''
    }
  }

  async function write(rowName: string, rowType: ValueType, value: number | string | boolean, formula: string) {
    busy = true
    error = ''
    try {
      await putValue(owner.kind, owner.id, rowName, rowType, value, formula)
      touchValues(hostKey(owner.kind, owner.id))
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  /** The add row is not a form: it may sit inside another one (a quest's edit form), where Enter must not save that. */
  function onAddKey(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    e.stopPropagation()
    void add()
  }

  async function reroll(rowName: string) {
    busy = true
    error = ''
    try {
      await rollValue(owner.kind, owner.id, rowName)
      touchValues(hostKey(owner.kind, owner.id))
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  async function remove(rowName: string) {
    busy = true
    error = ''
    try {
      await deleteValue(owner.kind, owner.id, rowName)
      touchValues(hostKey(owner.kind, owner.id))
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

{#snippet panel()}
  <div class="environment-tabs" class:environment-tabs--secrets={tab === 'secrets'} role="group" aria-label="Вид окружения">
    <button type="button" class="environment-tabs__option" aria-pressed={tab === 'values'} onclick={() => (tab = 'values')}>Переменные</button>
    <button type="button" class="environment-tabs__option" aria-pressed={tab === 'secrets'} onclick={() => (tab = 'secrets')}>Секреты</button>
  </div>
  {#if tab === 'secrets'}<SecretTable {owner} {readonly} />{/if}
  <div class="environment-values" hidden={tab !== 'values'}>
    <p class="values-hint">Имя пишется как здесь (str). В команде оно заглавное (STR).</p>
    {#if truncated}<p class="values-hint">Дальние переменные не влезли в область видимости.</p>{/if}
    {#if own.length || inherited.length}
      <ul class="values-list">
        {#each own as row (row.name)}
          <li class="values-card" data-type={row.type}>
            <button type="button" class="values-card__main" onclick={() => (editing = row.name)}>
              <span class="values-type">{valueTypeLabel(row.type)}</span>
              <span class="values-name"><code>{row.name}</code></span>
              <span class="values-detail" class:values-formula={!!row.formula} class:values-bad={!!row.error} title={valueDetail({ formula: row.formula, text: formatValue(row.type, row.value), error: row.error, roll: row.roll?.arg })}>
                {valueDetail({ formula: row.formula, text: formatValue(row.type, row.value), error: row.error, roll: row.roll?.arg })}
              </span>
            </button>
            <span class="values-actions">
{#if !readonly && row.roll}
              <button
                type="button"
                class="values-x"
                disabled={busy}
                aria-label="Обновить {row.name} из источника"
                title="Обновить из источника: {row.roll.arg}"
                onclick={() => void reroll(row.name)}
              >
                <Icon name="repeat" size={14} />
              </button>
            {/if}
            {#if !editReadonly}
              <button
                type="button"
                class="values-x"
                disabled={busy}
                aria-label="Удалить {row.name}"
                title="Удалить"
                onclick={() => void remove(row.name)}
              >
                <Icon name="close" size={14} />
              </button>
            {/if}
            </span>
          </li>
        {/each}
        {#each inherited as row (row.name)}
          <li class="values-card values-card--inherited" data-type={row.type}>
            <button type="button" class="values-card__main" onclick={() => (editing = row.name)}>
              <span class="values-type">{valueTypeLabel(row.type)}</span>
              <span class="values-name">
                <code>{row.name}</code>
                <small title="Из: {source(row)}">· {source(row)}</small>
              </span>
              <span class="values-detail" class:values-formula={!!row.formula} class:values-bad={!!row.error} title={valueDetail({ formula: row.formula, text: formatValue(row.type, row.value), error: row.error, roll: row.roll?.arg })}>
                {valueDetail({ formula: row.formula, text: formatValue(row.type, row.value), error: row.error, roll: row.roll?.arg })}
              </span>
            </button>
            <span class="values-actions"></span>
          </li>
        {/each}
      </ul>
    {/if}
    {#if !editReadonly}
      <div class="values-add" role="group" aria-label="Новая переменная">
        <select bind:value={type} aria-label="Тип">
          {#each TYPES as item (item)}
            <option value={item} style:color={valueTypeColor(item)}>{valueTypeLabel(item)}</option>
          {/each}
        </select>
        <input type="text" bind:value={name} placeholder="имя" aria-label="Имя" maxlength="24" onkeydown={onAddKey} />
        <input type="text" bind:value={raw} placeholder={HINT[type]} aria-label="Значение или формула" maxlength="1024" onkeydown={onAddKey} />
        <button type="button" class="values-x values-x--add" disabled={busy} aria-label="Добавить" title="Добавить" onclick={() => void add()}>
          <Icon name="add" size={14} />
        </button>
      </div>
    {/if}
  </div>
{/snippet}

{#if variant === 'panel'}
  {#if error}<p class="modal__error">{error}</p>{/if}
  {@render panel()}
{:else if variant === 'inline'}
  <FormSection title="Окружение" summary={own.length ? `${own.length} · ${summary}` : ''} bind:open={sectionOpen}>
    {#if error}<p class="modal__error">{error}</p>{/if}
    <div class="values-inline">{@render panel()}</div>
  </FormSection>
{:else}
{#if variant !== 'hidden'}
<button
  type="button"
  class={variant === 'icon' ? 'btn btn--icon values-open values-open--icon' : 'values-open'}
  {title}
  aria-label="Окружение"
  aria-haspopup="dialog"
  onclick={() => (open = true)}
>
  {#if variant === 'icon'}
    <Icon name="key" size={16} />
  {:else}
    {label}
  {/if}
</button>
{/if}

<ModalShell {open} onClose={() => (open = false)} labelledby="values-modal-title" zIndex={80} maxWidth="36rem">
  <ModalHead id="values-modal-title" title="Окружение" onClose={() => (open = false)} />
  {#if error}<p class="modal__error">{error}</p>{/if}
  <div class="modal__body">
    {@render panel()}
  </div>
</ModalShell>
{/if}

{#if editing}
  <ValueEditDialog
    open
    {owner}
    readonly={editReadonly}
    name={editing}
    onClose={() => (editing = '')}
  />
{/if}

<style>
  .environment-tabs {
    position: relative;
    isolation: isolate;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 100%;
    padding: 0.25rem;
    box-sizing: border-box;
    margin-bottom: 0.75rem;
    border-radius: var(--radius-lg, 12px);
    background: var(--color-bg, #121212);
  }
  .environment-tabs::before {
    content: '';
    position: absolute;
    z-index: -1;
    inset: 0.25rem auto 0.25rem 0.25rem;
    width: calc((100% - 0.5rem) / 2);
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-accent, #c9a227) 25%, transparent);
    transition: transform 180ms ease;
  }
  .environment-tabs--secrets::before { transform: translateX(100%); }
  .environment-tabs__option {
    min-width: 0;
    height: 2.5rem;
    border: 0;
    border-radius: var(--radius-md, 4px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    font: inherit;
    cursor: pointer;
  }
  .environment-tabs__option[aria-pressed='true'] { color: var(--color-accent, #c9a227); }
  .environment-tabs__option:focus-visible { outline: 1px solid var(--color-accent, #c9a227); outline-offset: -2px; }
  @media (prefers-reduced-motion: reduce) { .environment-tabs::before { transition: none; } }
  .environment-values:not([hidden]) { display: grid; gap: 0.6rem; }
  .values-open {
    height: 2rem;
    padding: 0 0.7rem;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }
  .values-open:hover {
    color: var(--color-accent, #c9a227);
  }
  .values-open--icon {
    width: 2rem;
    min-width: 2rem;
    padding: 0;
    font-family: var(--font-mono, monospace);
  }
  .values-hint {
    margin: 0;
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.85rem;
  }
  .values-list {
    display: grid;
    gap: 0.35rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .values-card {
    --vt: var(--color-fg-muted, #9a9a9a);
    display: flex;
    align-items: center;
    gap: 0.35rem;
    min-width: 0;
    padding: 0.35rem 0.35rem 0.35rem 0.55rem;
    border: 1px solid var(--color-border, #333);
    border-radius: 8px;
    background: transparent;
  }
  .values-card:hover {
    background: color-mix(in srgb, var(--color-fg, #e8e8e8) 6%, transparent);
  }
  .values-card[data-type='int'] { --vt: #4aa3df; }
  .values-card[data-type='float'] { --vt: #5cc4c4; }
  .values-card[data-type='str'] { --vt: #c9a227; }
  .values-card[data-type='bool'] { --vt: #b06ad8; }
  .values-card__main {
    display: grid;
    grid-template-columns: 5rem minmax(5.5rem, 36%) minmax(0, 1fr);
    align-items: center;
    gap: 0.55rem;
    flex: 1;
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .values-type {
    color: var(--vt);
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.02em;
    white-space: nowrap;
  }
  .values-name {
    display: flex;
    align-items: baseline;
    gap: 0.4rem;
    min-width: 0;
    overflow: hidden;
    font-family: var(--font-mono, monospace);
    white-space: nowrap;
  }
  .values-name code {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .values-name small {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--color-fg-muted, #9a9a9a);
    font-family: var(--font-ui, sans-serif);
    font-size: 0.72rem;
  }
  /* The same slot in every card, with or without buttons, so the values end on one line. */
  .values-actions {
    display: flex;
    flex: none;
    justify-content: flex-end;
    width: 3.1rem;
  }
  .values-card--inherited .values-name,
  .values-card--inherited .values-detail {
    color: var(--color-fg-muted, #9a9a9a);
  }
  .values-detail {
    overflow: hidden;
    text-align: right;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .values-formula {
    font-family: var(--font-mono, monospace);
    font-size: 0.85em;
  }
  .values-bad {
    color: var(--color-danger, #b54a3a);
  }
  .values-x {
    display: inline-flex;
    flex: none;
    padding: 0.15rem;
    border: 0;
    background: none;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }
  .values-x:hover {
    color: var(--color-danger, #b54a3a);
  }
  .values-x--add:hover {
    color: var(--color-accent, #c9a227);
  }
  .values-inline {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 0.25rem 0 0.5rem;
  }
  .values-add {
    display: flex;
    flex-wrap: nowrap;
    align-items: center;
    gap: 0.4rem;
  }
  .values-add select {
    flex: 0 0 6.6rem;
    width: 6.6rem;
  }
  .values-add input[type='text'] {
    flex: 1 1 0;
    width: 0;
    min-width: 0;
  }
</style>
