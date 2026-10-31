<script lang="ts">
  import { deleteValue, getRunners, listValues, putValue } from '@/shared/api/client.ts'
  import RunnerPicker from '@/shared/ui/RunnerPicker.svelte'
  import { hostKey, touchValues } from '@/shared/lib/valueLive.svelte.ts'

  /** The runner a text's code goes to when the link names none: the workspace value `runner`, a chain tried in turn. */
  let { readonly = false }: { readonly?: boolean } = $props()

  let raw = $state('')
  let saved = $state('')
  let runners = $state([] as { label: string; online: boolean; registered: boolean }[])
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
      runners = (await getRunners()).filter((r) => r.registered && !r.revoked).map((r) => ({ label: r.label, online: r.online, registered: true }))
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }
  void load()

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
<form class="default-runner-form" onsubmit={(e) => { e.preventDefault(); void save() }}>
  <fieldset disabled={readonly || busy}>
    <RunnerPicker bind:value={raw} {runners} label="Раннеры по умолчанию" />
    {#if !runners.length}<p class="note">Нет зарегистрированных раннеров.</p>{/if}
  </fieldset>
  {#if !readonly}
    <button type="submit" class="btn" disabled={busy || raw.trim() === saved.trim()}>Сохранить</button>
  {/if}
</form>

<style>
  .default-runner-form { display: flex; align-items: flex-start; flex-wrap: wrap; gap: 0.75rem; }
  fieldset { flex: 1 1 14rem; min-width: 0; border: 0; padding: 0; margin: 0; }
  .btn { flex-shrink: 0; }
</style>
