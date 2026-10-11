<script lang="ts">
  import { SECRET_OWNER_KINDS, listSecrets, setSecret, deleteSecret } from '@/shared/api/client.ts'
  import { parseRefs } from '@/shared/lib/refs.ts'
  import Icon from '@/shared/ui/Icon.svelte'
  import { MentionTextarea } from '@/entities/template'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import { untrack } from 'svelte'

  /**
   * Secrets manager — GitHub-style: names are visible, values never are, only
   * add/overwrite/delete. A secret belongs to one entity (questline, template,
   * quest or step) and is inherited step > quest > template > questline, so
   * the field below takes `@name` (resolved to `quest=12`) or a pasted ref.
   * `owner` pre-fills it (context menu → «Секреты»); only entities that can
   * own secrets are offered (a note owns its own, which nothing inherits).
   */
  let { open = false, onClose, owner = null, quests = [], questlines = [], labels = {} }: {
    open: boolean,
    onClose: () => void,
    owner?: { kind: string, id: number } | null,
    quests?: any[],
    questlines?: any[],
    labels?: Record<string, string>,
  } = $props()

  const KIND_LABEL: Record<string, string> = { questline: 'квестлайн', template: 'шаблон', quest: 'квест', step: 'шаг', note: 'заметка' }

  let ownerText = $state('')
  /** the entity the field resolves to: the first owner-capable ref in the text */
  let target = $derived.by(() => {
    const hit = parseRefs(ownerText).find((r) => SECRET_OWNER_KINDS.includes(r.kind))
    return hit ?? null
  })
  let targetLabel = $derived(
    target ? labels[`${target.kind}:${target.id}`] || `${target.kind}=${target.id}` : '',
  )

  let keys = $state(([] as string[]))
  let inherited = $state(([] as { key: string, from: string, id: number }[]))
  let keysLoading = $state(false)
  let keysError = $state('')

  let newKey = $state('')
  let newValue = $state('')
  let saving = $state(false)

  /** key being overwritten inline, or null */
  let editingKey = $state((null as string | null))
  let editValue = $state('')

  const KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/

  /** Bumped on every refreshKeys() call so a slower, older request can't
   * clobber a newer one's result if responses arrive out of order. */
  let keysRequestSeq = 0

  async function refreshKeys() {
    const t = target
    if (!t) {
      keys = []
      inherited = []
      keysError = ''
      return
    }
    const seq = ++keysRequestSeq
    keysLoading = true
    keysError = ''
    try {
      const result = await listSecrets(t.kind, t.id)
      if (seq !== keysRequestSeq) return
      keys = result.keys
      inherited = result.inherited
    } catch (e) {
      if (seq !== keysRequestSeq) return
      keysError = e.message || String(e)
      keys = []
      inherited = []
    } finally {
      if (seq === keysRequestSeq) keysLoading = false
    }
  }

  // Reset on open — only track `open`.
  $effect(() => {
    if (!open) return
    untrack(() => {
      newKey = ''
      newValue = ''
      editingKey = null
      keysError = ''
      ownerText = owner ? `${owner.kind}=${owner.id} ` : ''
    })
  })

  // Reload names whenever the resolved entity changes.
  $effect(() => {
    if (!open) return
    void target?.kind
    void target?.id
    untrack(() => {
      editingKey = null
      void refreshKeys()
    })
  })

  async function onAdd(event: { preventDefault: () => void }) {
    event.preventDefault()
    if (!target || saving) return
    const key = newKey.trim()
    if (!KEY_PATTERN.test(key)) {
      keysError = 'Имя ключа — как переменная окружения: [A-Za-z_][A-Za-z0-9_]*'
      return
    }
    if (!newValue) {
      keysError = 'Нужно значение'
      return
    }
    saving = true
    keysError = ''
    try {
      await setSecret(target.kind, target.id, key, newValue)
      newKey = ''
      newValue = ''
      await refreshKeys()
    } catch (e) {
      keysError = e.message || String(e)
    } finally {
      saving = false
    }
  }

  function startEdit(key: string) {
    editingKey = key
    editValue = ''
    keysError = ''
  }

  function cancelEdit() {
    editingKey = null
    editValue = ''
  }

  async function saveEdit(key: string) {
    if (!target || saving) return
    if (!editValue) {
      keysError = 'Нужно значение'
      return
    }
    saving = true
    keysError = ''
    try {
      await setSecret(target.kind, target.id, key, editValue)
      editingKey = null
      editValue = ''
      await refreshKeys()
    } catch (e) {
      keysError = e.message || String(e)
    } finally {
      saving = false
    }
  }

  async function onDelete(key: string) {
    if (!target || saving) return
    saving = true
    keysError = ''
    try {
      await deleteSecret(target.kind, target.id, key)
      if (editingKey === key) editingKey = null
      await refreshKeys()
    } catch (e) {
      keysError = e.message || String(e)
    } finally {
      saving = false
    }
  }

  function sourceLabel(ref: { from: string, id: number }) {
    const name = labels[`${ref.from}:${ref.id}`]
    return `${KIND_LABEL[ref.from] || ref.from}${name ? ` «${name}»` : ''} (${ref.from}=${ref.id})`
  }
</script>

<ModalShell {open} {onClose} labelledby="secrets-modal-title" zIndex={60} maxWidth="30rem">
  <ModalHead id="secrets-modal-title" title="Секреты" icon="key" {onClose} />

  <div class="modal__body">
    <div class="field">
      <span class="label">Чьи секреты</span>
      <MentionTextarea
        bind:value={ownerText}
        {quests}
        {questlines}
        notes={[]}
        attachments={[]}
        rows={1}
        placeholder="@квест, @шаг, @шаблон или @квестлайн; можно вставить quest=12"
      />
      {#if target}
        <span class="hint">
          {KIND_LABEL[target.kind]} · <b>{targetLabel}</b> <code>{target.kind}={target.id}</code>
        </span>
      {:else}
        <span class="hint">Секрет принадлежит квестлайну, шаблону, квесту или шагу.</span>
      {/if}
    </div>

    {#if target}
      {#if keysError}
        <p class="modal__error">{keysError}</p>
      {/if}

      {#if keysLoading}
        <p class="hint">Загрузка ключей…</p>
      {:else}
        <ul class="secrets-list">
          {#each keys as key (key)}
            <li class="secret-row">
              <code class="secret-row__key">{key}</code>
              {#if editingKey === key}
                <input
                  type="password"
                  class="secret-row__input"
                  placeholder="новое значение"
                  bind:value={editValue}
                  autocomplete="off"
                />
                <button type="button" class="btn btn--ghost btn--icon" aria-label="Сохранить" onclick={() => saveEdit(key)} disabled={saving}>
                  <Icon name="checkmark" size={14} />
                </button>
                <button type="button" class="btn btn--ghost btn--icon" aria-label="Отмена" onclick={cancelEdit} disabled={saving}>
                  <Icon name="close" size={14} />
                </button>
              {:else}
                <span class="secret-row__value" aria-hidden="true">••••••••</span>
                <button type="button" class="btn btn--ghost btn--icon" aria-label="Изменить" onclick={() => startEdit(key)} disabled={saving}>
                  <Icon name="edit" size={14} />
                </button>
                <button type="button" class="btn btn--ghost btn--icon" aria-label="Удалить" onclick={() => onDelete(key)} disabled={saving}>
                  <Icon name="delete" size={14} />
                </button>
              {/if}
            </li>
          {:else}
            <li class="hint">Своих секретов нет.</li>
          {/each}
          {#each inherited as ref (ref.key)}
            <li class="secret-row secret-row--inherited">
              <code class="secret-row__key">{ref.key}</code>
              <span class="secret-row__from">унаследован: {sourceLabel(ref)}</span>
            </li>
          {/each}
        </ul>
      {/if}

      <form class="add-row" onsubmit={onAdd}>
        <input
          type="text"
          class="add-row__key"
          placeholder="ИМЯ_КЛЮЧА"
          bind:value={newKey}
          spellcheck="false"
          autocomplete="off"
        />
        <input
          type="password"
          class="add-row__value"
          placeholder="значение"
          bind:value={newValue}
          autocomplete="off"
        />
        <button type="submit" class="btn btn--accent" disabled={saving}>
          <Icon name="add" size={14} />
          <span class="btn__text">Добавить</span>
        </button>
      </form>

      <p class="hint">
        Секрет доступен командам этой сущности и всего, что ниже: шаг &gt; квест &gt; шаблон &gt; квестлайн
        (одноимённый секрет более частного уровня перекрывает общий, добавьте тот же ключ, чтобы переопределить
        унаследованный). Значения попадают в окружение команды только в момент запуска, а в её выводе
        заменяются на ***. Ни один инструмент (включая этот список) их обратно не показывает, только имена.
        Кто может написать команду шага, может вывести её секреты, держите секрет на самом узком уровне.
      </p>
    {/if}
  </div>
</ModalShell>

<style>

  .secrets-list {
    display: grid;
    gap: 0.4rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .secret-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.35rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-sm, 2px);
    background: var(--color-bg-muted, #242424);
  }

  .secret-row__key {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-sm, 0.875rem);
  }

  .secret-row__value {
    flex: 1;
    color: var(--color-fg-subtle, #6e6e6e);
    letter-spacing: 0.15em;
  }

  .secret-row__input {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono, monospace);
    font-size: var(--text-xs, 0.75rem);
  }

  .secret-row--inherited {
    background: transparent;
    border-style: dashed;
  }

  .secret-row__from {
    flex: 1;
    min-width: 0;
    color: var(--color-fg-subtle, #6e6e6e);
    font-size: var(--text-xs, 0.75rem);
  }
  .add-row {
    display: grid;
    grid-template-columns: 1fr 1fr auto;
    gap: 0.5rem;
  }

  .add-row__key {
    font-family: var(--font-mono, monospace);
    font-size: var(--text-sm, 0.875rem);
  }

</style>
