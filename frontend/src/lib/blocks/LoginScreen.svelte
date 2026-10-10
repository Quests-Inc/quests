<script>
  // Sign-in gate. Rendered instead of the journal whenever the instance
  // requires accounts and no valid session cookie is present.
  import { login, loginMfa } from '../js/api.ts'

  /** @type {{ onAuthenticated: (username: string) => void }} */
  let { onAuthenticated } = $props()

  let username = $state('')
  let password = $state('')
  let busy = $state(false)
  let error = $state('')
  /** Set once the password was right and the account wants a second factor. */
  let mfaToken = $state('')
  let code = $state('')

  async function submit(event) {
    event.preventDefault()
    if (busy) return
    error = ''
    busy = true
    try {
      const res = mfaToken ? await loginMfa(mfaToken, code.trim()) : await login(username.trim(), password)
      password = ''
      if (res?.mfa_required) {
        mfaToken = res.mfa_token
        return
      }
      code = ''
      onAuthenticated(res?.username ?? username.trim())
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
      password = ''
      code = ''
      // The ticket is single-purpose and short-lived: a wrong code stays on this step, anything
      // else (expired, out of attempts) sends the person back to the password.
      if (mfaToken && !/код/i.test(error) || /истекла|кончились/.test(error)) mfaToken = ''
    } finally {
      busy = false
    }
  }
</script>

<div class="login-shell">
  <form class="login-card" onsubmit={submit}>
    <h1>Quests</h1>
    <p class="hint">Журнал закрыт — войдите в аккаунт.</p>

    {#if mfaToken}
      <p class="hint">Введите код из приложения-аутентификатора или один из кодов восстановления.</p>
      <label>
        <span>Код</span>
        <!-- svelte-ignore a11y_autofocus -->
        <input
          type="text"
          inputmode="numeric"
          autocomplete="one-time-code"
          autocapitalize="none"
          spellcheck="false"
          autofocus
          bind:value={code}
          disabled={busy}
        />
      </label>
    {:else}
    <label>
      <span>Логин</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input
        type="text"
        autocomplete="username"
        autocapitalize="none"
        autocorrect="off"
        spellcheck="false"
        autofocus
        bind:value={username}
        disabled={busy}
      />
    </label>

    <label>
      <span>Пароль</span>
      <input
        type="password"
        autocomplete="current-password"
        bind:value={password}
        disabled={busy}
      />
    </label>

    {/if}

    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}

    <button type="submit" disabled={busy || (mfaToken ? !code.trim() : !username.trim() || !password)}>
      {busy ? 'Проверяем…' : mfaToken ? 'Подтвердить' : 'Войти'}
    </button>
    {#if mfaToken}
      <button type="button" class="link" disabled={busy} onclick={() => { mfaToken = ''; code = ''; error = '' }}>
        ← другой аккаунт
      </button>
    {/if}
  </form>
</div>

<style>
  .login-shell {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 24px;
    background: var(--color-bg);
  }

  .login-card {
    width: min(360px, 100%);
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 28px;
    background: var(--color-bg-raised);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
  }

  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 1.6rem;
    color: var(--color-fg);
  }

  .hint {
    margin: -6px 0 4px;
    font-family: var(--font-ui);
    font-size: 0.85rem;
    color: var(--color-fg-muted);
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-family: var(--font-ui);
    font-size: 0.82rem;
    color: var(--color-fg-muted);
  }

  input {
    padding: 9px 11px;
    font-family: var(--font-body);
    font-size: 0.95rem;
    color: var(--color-fg);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
  }

  input:focus {
    outline: none;
    border-color: var(--color-accent);
  }

  button {
    margin-top: 4px;
    padding: 10px;
    font-family: var(--font-ui);
    font-size: 0.9rem;
    color: var(--color-accent-fg);
    background: var(--color-accent);
    border: none;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  button.link {
    margin-top: 0;
    padding: 4px;
    color: var(--color-fg-muted);
    background: transparent;
    font-size: 0.8rem;
  }

  button:disabled {
    opacity: 0.55;
    cursor: default;
  }

  .error {
    margin: 0;
    font-family: var(--font-ui);
    font-size: 0.82rem;
    color: var(--color-danger);
  }
</style>
