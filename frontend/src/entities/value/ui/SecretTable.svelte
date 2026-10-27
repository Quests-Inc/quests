<script lang="ts">
  import { untrack } from 'svelte'
  import { deleteSecret, fetchWorkspaces, listSecrets, setSecret, type SecretSource } from '@/shared/api/client.ts'
  import Icon from '@/shared/ui/Icon.svelte'

  let { owner, readonly = false }: { owner: { kind: string; id: number }; readonly?: boolean } = $props()
  const kinds: Record<string, string> = { workspace: 'пространство', note: 'заметка', quest: 'квест', step: 'шаг', template: 'шаблон', questline: 'квестлайн' }
  let own = $state<SecretSource[]>([])
  let inherited = $state<SecretSource[]>([])
  let loading = $state(true)
  let error = $state('')
  let busy = $state(false)
  let canWrite = $state(false)
  let key = $state('')
  let value = $state('')
  let seq = 0

  function source(row: SecretSource) {
    return row.from === 'workspace' ? 'пространство' : `${kinds[row.from] || row.from} · ${row.title || `№${row.id}`}`
  }
  async function load() {
    const current = ++seq
    const target = { ...owner }
    loading = true
    error = ''
    own = []
    inherited = []
    canWrite = false
    try {
      const [list, spaces] = await Promise.all([listSecrets(target.kind, target.id), fetchWorkspaces()])
      if (current !== seq) return
      own = list.own
      inherited = list.inherited
      canWrite = spaces.workspaces.find((w) => w.id === spaces.current)?.role === 'owner'
    } catch (e) {
      if (current === seq) error = e instanceof Error ? e.message : String(e)
    } finally {
      if (current === seq) loading = false
    }
  }
  $effect(() => {
    void owner.kind
    void owner.id
    untrack(() => { key = ''; value = ''; void load() })
    return () => { seq++ }
  })
  async function write(remove = '') {
    if (busy || readonly || !canWrite) return
    if (!remove && (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key.trim()) || !value)) {
      error = 'Укажите имя секрета (латинские буквы, цифры и _) и новое значение.'
      return
    }
    busy = true
    error = ''
    try {
      if (remove) await deleteSecret(owner.kind, owner.id, remove)
      else await setSecret(owner.kind, owner.id, key.trim(), value)
      key = ''
      value = ''
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally { busy = false }
  }
  function enter(e: KeyboardEvent) {
    if (e.key !== 'Enter') return
    e.preventDefault()
    e.stopPropagation()
    void write()
  }
</script>

<p class="hint">Значения скрыты после записи. Секреты доступны только командам и заголовкам HTTP, в текст и формулы не подставляются.</p>
{#if error}<p class="modal__error" role="alert">{error}</p>{/if}
{#if loading}
  <p class="hint">Загрузка секретов…</p>
{:else}
  <ul class="secrets-list">
    {#each [...own, ...inherited] as row (row.key)}
      <li class="secret-row" class:inherited={!own.some((r) => r.key === row.key)}>
        <code>{row.key}</code>
        <span class="hint">{own.some((r) => r.key === row.key) ? 'свой' : source(row)}</span>
        {#if own.some((r) => r.key === row.key) && canWrite && !readonly}
          <button type="button" class="btn btn--ghost btn--icon" title="Заменить значение" aria-label="Заменить {row.key}" disabled={busy} onclick={() => { key = row.key; value = '' }}><Icon name="edit" size={14} /></button>
          <button type="button" class="btn btn--ghost btn--icon" title="Удалить" aria-label="Удалить {row.key}" disabled={busy} onclick={() => void write(row.key)}><Icon name="delete" size={14} /></button>
        {/if}
      </li>
    {:else}<li class="hint">Секретов пока нет.</li>{/each}
  </ul>
  {#if canWrite && !readonly}
    <div class="add-row" role="group" aria-label="Записать секрет">
      <input aria-label="Имя секрета" placeholder="API_TOKEN" bind:value={key} autocomplete="off" spellcheck="false" onkeydown={enter} />
      <input type="password" aria-label="Новое значение секрета" placeholder="Новое значение" bind:value autocomplete="new-password" onkeydown={enter} />
      <button type="button" class="btn" disabled={busy} onclick={() => void write()}>Записать</button>
    </div>
    <p class="hint">Чтобы заменить или переопределить секрет, запишите новое значение с тем же именем. Имена PATH, LD_*, QUESTS_* и другие служебные имена запрещены.</p>
  {:else}<p class="hint">Секреты записывает только владелец пространства.</p>{/if}
{/if}

<style>
  .secrets-list { display: grid; gap: 0.4rem; padding: 0; list-style: none; }
  .secret-row { display: flex; align-items: center; gap: 0.5rem; padding: 0.4rem 0.5rem; border: 1px solid var(--color-border, #333); border-radius: 8px; }
  .secret-row code { overflow-wrap: anywhere; }
  .secret-row .hint { flex: 1; }
  .inherited { border-style: dashed; }
  .add-row { display: flex; flex-wrap: wrap; gap: 0.5rem; }
  .add-row input { min-width: 5rem; flex: 1; width: 0; }
</style>
