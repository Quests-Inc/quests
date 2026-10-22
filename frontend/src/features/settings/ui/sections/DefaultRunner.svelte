<script lang="ts">
  import { deleteValue, getRunners, listValues, putValue } from '@/shared/api/client.ts'
  import { hostKey, touchValues } from '@/shared/lib/valueLive.svelte.ts'

  /** The runner a text's code goes to when the link names none: the workspace value `runner`, a chain tried in turn. */
  let { readonly = false }: { readonly?: boolean } = $props()

  let raw = $state('')
  let saved = $state('')
  let runners = $state([] as { label: string; online: boolean }[])
  let error = $state('')
  let busy = $state(false)

  const chain = $derived(
    raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
  )

  async function load() {
    error = ''
    try {
      const list = await listValues('workspace')
      const row = (list.own ?? []).find((r) => r.name === 'runner' && r.type === 'str' && !r.formula)
      saved = raw = row ? String(row.value ?? '') : ''
      runners = (await getRunners()).filter((r) => r.registered && !r.revoked).map((r) => ({ label: r.label, online: r.online }))
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }
  void load()

  function add(label: string) {
    if (!chain.includes(label)) raw = [...chain, label].join(', ')
  }

  async function save() {
    const unknown = chain.filter((l) => !runners.some((r) => r.label === l))
    if (unknown.length) {
      error = `Нет раннера: ${unknown.join(', ')}`
      return
    }
    busy = true
    error = ''
    try {
      if (chain.length) await putValue('workspace', 0, 'runner', 'str', chain.join(','))
      else await deleteValue('workspace', 0, 'runner')
      saved = raw = chain.join(', ')
      touchValues(hostKey('workspace', 0))
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

{#if error}<p class="note note--err">{error}</p>{/if}
<form
  class="row"
  onsubmit={(e) => {
    e.preventDefault()
    void save()
  }}
>
  <input
    type="text"
    class="grow"
    bind:value={raw}
    disabled={readonly || busy}
    placeholder={runners.length === 1 ? `${runners[0].label} (единственный)` : 'main-pc, laptop'}
    aria-label="Раннеры по умолчанию"
  />
  {#if !readonly}
    <button type="submit" class="btn" disabled={busy || raw.trim() === saved.trim()}>Сохранить</button>
  {/if}
</form>
{#if runners.length}
  <ul class="chips">
    {#each runners as r (r.label)}
      <li>
        <button type="button" class="btn chip" disabled={readonly || busy} title={r.online ? 'в сети' : 'не в сети'} onclick={() => add(r.label)}>
          {r.label}{r.online ? '' : ' · нет в сети'}
        </button>
      </li>
    {/each}
  </ul>
{/if}
