<script>
  import SettingsBlock from './SettingsBlock.svelte'
  import {
    wsx,
    currentWorkspaceName,
    pickWorkspace,
    addWorkspace,
    saveName,
    removeCurrent,
    newInvite,
    dropMember,
  } from './workspaceState.svelte.js'

  /** @type {{ username?: string }} */
  let { username = '' } = $props()

  let newName = $state('')
  let renameTo = $state(currentWorkspaceName())
  let deleting = $state(false)
  let deleteConfirm = $state('')
  let inviteUrl = $state('')
  let copied = $state(false)

  // Follow the current workspace (loaded after the section opens, and changed by renames).
  $effect(() => {
    renameTo = currentWorkspaceName()
    deleting = false
    deleteConfirm = ''
    inviteUrl = ''
  })

  const isOwner = $derived(wsx.myRole === 'owner')

  async function makeInvite() {
    inviteUrl = await newInvite()
    copied = false
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(inviteUrl)
      copied = true
    } catch {
      /* the field is selectable — copy by hand */
    }
  }
</script>

<SettingsBlock title="Ваши пространства" hint="Отдельные наборы квестов, заметок и шаблонов: данные не пересекаются.">
  <ul class="chips">
    {#each wsx.list as w (w.id)}
      <li>
        <button
          type="button"
          class="btn chip"
          class:btn--accent={w.id === wsx.current}
          disabled={wsx.busy}
          aria-pressed={w.id === wsx.current}
          onclick={() => pickWorkspace(w.id)}
        >
          {w.name}
        </button>
      </li>
    {/each}
  </ul>
  <form
    class="row"
    onsubmit={(e) => {
      e.preventDefault()
      addWorkspace(newName)
    }}
  >
    <input type="text" class="grow" placeholder="Название нового пространства" maxlength="80" bind:value={newName} disabled={wsx.busy} />
    <button type="submit" class="btn" disabled={wsx.busy || !newName.trim()}>Создать</button>
  </form>
</SettingsBlock>

{#if wsx.members.length}
  <SettingsBlock title="Это пространство">
    {#if isOwner}
      <form
        class="row"
        onsubmit={(e) => {
          e.preventDefault()
          saveName(renameTo)
        }}
      >
        <input type="text" class="grow" maxlength="80" bind:value={renameTo} disabled={wsx.busy} aria-label="Название" />
        <button type="submit" class="btn" disabled={wsx.busy || !renameTo.trim() || renameTo.trim() === currentWorkspaceName()}>
          Переименовать
        </button>
      </form>
    {/if}

    <h5 class="sub">Участники</h5>
    <ul class="members">
      {#each wsx.members as m (m.user_id)}
        <li class="members__row">
          <span class="members__name">{m.username}</span>
          {#if m.role === 'owner'}<span class="members__role">владелец</span>{/if}
          {#if isOwner || m.username === username}
            <button type="button" class="btn btn--ghost members__x" onclick={() => dropMember(m, username)}>
              {m.username === username ? 'Выйти' : 'Убрать'}
            </button>
          {/if}
        </li>
      {/each}
    </ul>

    {#if isOwner}
      <h5 class="sub">Приглашение</h5>
      {#if inviteUrl}
        <div class="row">
          <input type="text" class="grow" readonly value={inviteUrl} onfocus={(e) => e.currentTarget.select()} aria-label="Ссылка-приглашение" />
          <button type="button" class="btn btn--accent" onclick={copy}>{copied ? 'Скопировано' : 'Копировать'}</button>
        </div>
        <p class="note">Одноразовая ссылка, действует 7 дней. Открыв её, человек создаёт аккаунт или входит в свой и попадает сюда.</p>
      {:else}
        <div class="row">
          <button type="button" class="btn btn--accent" disabled={wsx.busy} onclick={makeInvite}>Пригласить по ссылке</button>
        </div>
      {/if}
    {/if}
  </SettingsBlock>

  {#if isOwner}
    <SettingsBlock
      title="Опасная зона"
      danger
      hint="Пространство скроется сразу, а его данные удалятся через 7 дней (до этого их может вернуть администратор сервера). Доверенное пространство удалить нельзя."
    >
      {#if deleting}
        <p class="note">Введите название «{currentWorkspaceName()}», чтобы подтвердить.</p>
        <form
          class="row"
          onsubmit={(e) => {
            e.preventDefault()
            removeCurrent(deleteConfirm)
          }}
        >
          <input type="text" class="grow" bind:value={deleteConfirm} disabled={wsx.busy} aria-label="Название для подтверждения" />
          <button type="submit" class="btn btn--danger-solid" disabled={wsx.busy || deleteConfirm.trim() !== currentWorkspaceName()}>
            Удалить
          </button>
          <button type="button" class="btn btn--ghost" onclick={() => (deleting = false)}>Отмена</button>
        </form>
      {:else}
        <div class="row">
          <button type="button" class="btn btn--danger-solid" onclick={() => (deleting = true)}>Удалить пространство…</button>
        </div>
      {/if}
    </SettingsBlock>
  {/if}
{/if}

{#if wsx.error}<p class="err" role="alert">{wsx.error}</p>{/if}

<style>
  .chips {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .chip {
    white-space: nowrap;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
  }
  .row .btn {
    white-space: nowrap;
  }
  .grow {
    flex: 1 1 12rem;
    min-width: 0;
  }
  .sub {
    margin: var(--space-2, 0.5rem) 0 0;
    font-size: 0.78rem;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .members {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.25rem;
  }
  .members__row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    min-height: 2rem;
  }
  .members__name {
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg);
  }
  .members__role {
    font-size: 0.75rem;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .members__x {
    margin-left: auto;
  }
  .note {
    margin: 0;
    font-size: 0.8rem;
    line-height: 1.4;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .err {
    margin: 0;
    font-size: 0.82rem;
    color: var(--color-danger, #b54a3a);
  }
</style>
