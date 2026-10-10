<script>
  import SettingsBlock from './SettingsBlock.svelte'
  import Toggle from '../../ui/Toggle.svelte'
  import { workspaceExportUrl } from '../../js/api.js'
  import { wsx, doImport } from './workspaceState.svelte.ts'

  let exportSecrets = $state(false)
  let importSecrets = $state(false)
  /** @type {HTMLInputElement | null} */
  let fileInput = $state(null)

  const isOwner = $derived(wsx.myRole === 'owner')

  async function onFile(event) {
    const input = event.currentTarget
    const file = input.files?.[0]
    input.value = ''
    if (file && !wsx.busy) await doImport(file, importSecrets)
  }
</script>

{#if isOwner}
  <SettingsBlock
    title="Экспорт"
    hint="Весь архив пространства одним zip: квесты, шаги, заметки, шаблоны, теги, журнал, вложения и иконки."
  >
    <div class="row">
      <a class="btn btn--accent" href={workspaceExportUrl(wsx.current, { secrets: exportSecrets })} download>Скачать архив</a>
      <Toggle bind:checked={exportSecrets} label="Включить секреты" hint="пароли и токены шагов; хранить архив как пароль" />
    </div>
  </SettingsBlock>
{/if}

<SettingsBlock
  title="Импорт"
  hint="Импорт всегда создаёт новое пространство и перенумеровывает сущности вместе со ссылками. Команды на сервере из архива не переносятся."
>
  <div class="row">
    <button type="button" class="btn btn--accent" disabled={wsx.busy} onclick={() => fileInput?.click()}>
      {wsx.busy ? 'Импортируем…' : 'Выбрать архив…'}
    </button>
    <Toggle bind:checked={importSecrets} label="Импортировать секреты" hint="если они есть в архиве" disabled={wsx.busy} />
    <input type="file" accept=".zip,application/zip" hidden bind:this={fileInput} onchange={onFile} />
  </div>
  {#if wsx.notice}<p class="ok">{wsx.notice}</p>{/if}
  {#if wsx.error}<p class="err" role="alert">{wsx.error}</p>{/if}
</SettingsBlock>

<style>
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.75rem 1.25rem;
  }
  .row :global(.btn) {
    white-space: nowrap;
    text-decoration: none;
  }
  .ok,
  .err {
    margin: 0;
    font-size: 0.82rem;
  }
  .ok {
    color: var(--color-accent, #c9a227);
  }
  .err {
    color: var(--color-danger, #b54a3a);
  }
</style>
