<script>
  import SettingsBlock from './SettingsBlock.svelte'
  import { getMyMfa, setupMyMfa, enableMyMfa, disableMyMfa, regenerateRecoveryCodes } from '../../js/api.ts'

  /** @type {any} */
  let st = $state(null)
  let loadError = $state('')
  let busy = $state(false)
  let error = $state('')
  /** Enrolment in progress: the secret and its QR code. */
  let setup = $state(/** @type {{ secret: string, uri: string, qr: string } | null} */ (null))
  /** Recovery codes, visible only right after they were made. */
  let codes = $state(/** @type {string[] | null} */ (null))
  let code = $state('')
  /** Which action waits for a code: 'disable' | 'recovery' | null. */
  let ask = $state(/** @type {'disable' | 'recovery' | null} */ (null))
  let copied = $state(false)

  async function refresh() {
    try {
      st = await getMyMfa()
    } catch (e) {
      loadError = e.message || String(e)
    }
  }
  $effect(() => {
    void refresh()
  })

  async function run(fn) {
    busy = true
    error = ''
    try {
      await fn()
    } catch (e) {
      error = e.message || String(e)
    } finally {
      busy = false
    }
  }

  const start = () =>
    run(async () => {
      const r = await setupMyMfa()
      const { default: QRCode } = await import('qrcode')
      const qr = await QRCode.toDataURL(r.uri, { margin: 1, width: 192, errorCorrectionLevel: 'M' })
      setup = { secret: r.secret, uri: r.uri, qr }
      code = ''
    })

  const confirm = () =>
    run(async () => {
      const r = await enableMyMfa(code.trim())
      codes = r.recovery_codes
      setup = null
      code = ''
      await refresh()
    })

  const submitAsk = () =>
    run(async () => {
      if (ask === 'disable') {
        await disableMyMfa(code.trim())
        codes = null
      } else if (ask === 'recovery') {
        codes = (await regenerateRecoveryCodes(code.trim())).recovery_codes
      }
      ask = null
      code = ''
      await refresh()
    })

  const groups = (s) => s.replace(/(.{4})/g, '$1 ').trim()

  async function copyCodes() {
    try {
      await navigator.clipboard.writeText((codes ?? []).join('\n'))
      copied = true
      setTimeout(() => (copied = false), 2000)
    } catch {
      /* the codes stay on screen to be copied by hand */
    }
  }
</script>

{#if loadError}
  <SettingsBlock title="Двухфакторный вход"><p class="err">{loadError}</p></SettingsBlock>
{:else if st?.available}
  <SettingsBlock
    title="Двухфакторный вход"
    hint="После пароля вход просит одноразовый код из приложения-аутентификатора (Google Authenticator, Aegis, 1Password и т. п.). Токены для HUD, CLI и бота это не затрагивает."
  >
    {#if codes}
      <p class="warn">Сохраните коды восстановления: каждый подходит один раз и показывается только сейчас. Они нужны, если телефон потерян.</p>
      <ul class="codes">
        {#each codes as c (c)}<li>{c}</li>{/each}
      </ul>
      <div class="actions">
        <button type="button" class="btn" onclick={copyCodes}>{copied ? 'Скопировано' : 'Копировать'}</button>
        <button type="button" class="btn btn--primary" onclick={() => (codes = null)}>Я сохранил коды</button>
      </div>
    {:else if setup}
      <p class="muted">Отсканируйте код в приложении или введите ключ вручную, затем введите шесть цифр из приложения.</p>
      <div class="enrol">
        <img class="qr" src={setup.qr} alt="QR-код для приложения-аутентификатора" width="192" height="192" />
        <div class="enrol__side">
          <span class="muted">Ключ вручную</span>
          <code class="secret">{groups(setup.secret)}</code>
          <label class="field">
            <span>Код из приложения</span>
            <input type="text" inputmode="numeric" autocomplete="one-time-code" spellcheck="false" bind:value={code} placeholder="123456" />
          </label>
        </div>
      </div>
      <div class="actions">
        <button type="button" class="btn btn--primary" disabled={busy || !code.trim()} onclick={confirm}>Включить</button>
        <button type="button" class="btn" disabled={busy} onclick={() => ((setup = null), (error = ''))}>Отмена</button>
      </div>
    {:else if st.enabled}
      <p class="ok">Включён. Неиспользованных кодов восстановления: {st.recovery_left}.</p>
      {#if ask}
        <label class="field">
          <span>{ask === 'disable' ? 'Код для отключения' : 'Код для новых кодов восстановления'} (из приложения или код восстановления)</span>
          <input type="text" autocomplete="one-time-code" spellcheck="false" bind:value={code} />
        </label>
        <div class="actions">
          <button
            type="button"
            class={ask === 'disable' ? 'btn btn--danger-solid' : 'btn btn--primary'}
            disabled={busy || !code.trim()}
            onclick={submitAsk}
          >{ask === 'disable' ? 'Отключить' : 'Выпустить новые'}</button>
          <button type="button" class="btn" disabled={busy} onclick={() => ((ask = null), (code = ''), (error = ''))}>Отмена</button>
        </div>
      {:else}
        <div class="actions">
          <button type="button" class="btn" onclick={() => (ask = 'recovery')}>Новые коды восстановления</button>
          <button type="button" class="btn btn--danger-solid" onclick={() => (ask = 'disable')}>Отключить</button>
        </div>
      {/if}
    {:else}
      <p class="muted">Выключен: вход только по паролю.</p>
      <div class="actions">
        <button type="button" class="btn btn--primary" disabled={busy} onclick={start}>Включить</button>
      </div>
    {/if}
    {#if error}<p class="err">{error}</p>{/if}
  </SettingsBlock>
{/if}

<style>
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .muted {
    margin: 0;
    font-size: 0.8rem;
    line-height: 1.4;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .ok {
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-success, #8ec07c);
  }
  .warn {
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-warning, #d79921);
  }
  .err {
    margin: 0;
    font-size: 0.85rem;
    color: var(--color-danger, #b54a3a);
  }
  .field {
    display: grid;
    gap: 0.25rem;
    font-size: var(--text-sm, 0.875rem);
  }
  .field > span {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.78rem;
  }
  .enrol {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    align-items: flex-start;
  }
  .enrol__side {
    display: grid;
    gap: 0.5rem;
    min-width: 12rem;
    flex: 1;
  }
  .qr {
    background: #fff;
    border-radius: var(--radius-md, 6px);
    padding: 4px;
  }
  .secret {
    font-family: var(--font-mono, monospace);
    font-size: 0.85rem;
    letter-spacing: 0.04em;
    word-break: break-all;
    user-select: all;
  }
  .codes {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.25rem 1rem;
    margin: 0;
    padding: 0.5rem 0.75rem;
    list-style: none;
    font-family: var(--font-mono, monospace);
    font-size: 0.9rem;
    border: 1px dashed var(--color-border, #333);
    border-radius: var(--radius-md, 6px);
    user-select: all;
  }
</style>
