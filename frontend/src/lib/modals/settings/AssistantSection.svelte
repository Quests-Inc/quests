<script lang="ts">
  import SettingsBlock from './SettingsBlock.svelte'
  import { getMyLLM, saveMyLLM, deleteMyLLM, testMyLLM } from '../../js/api.ts'

  let info: any = $state(null)
  let loadError = $state('')
  let provider = $state('groq')
  let baseUrl = $state('')
  let model = $state('')
  let apiKey = $state('')
  let busy = $state(false)
  let note: { kind: 'ok' | 'err', text: string } | null = $state(null)

  const preset = $derived(info?.providers?.find((p: { id: string }) => p.id === provider))
  const SOURCE: Record<string, string> = {
    own: 'Используется ваш ключ',
    server: 'Используется ключ сервера',
    none: 'Ассистент не настроен',
  }

  function apply(v: { provider: string; base_url: string; model: string }) {
    info = v
    provider = v.provider || 'groq'
    baseUrl = v.base_url || ''
    model = v.model || ''
    apiKey = ''
  }

  $effect(() => {
    getMyLLM().then(apply).catch((e) => (loadError = e.message || String(e)))
  })

  const form = () => ({ provider, base_url: baseUrl, model, api_key: apiKey })

  async function run(fn: { (): Promise<void>; (): Promise<void>; (): Promise<void>; (): any }) {
    busy = true
    note = null
    try {
      await fn()
    } catch (e) {
      note = { kind: 'err', text: e.message || String(e) }
    } finally {
      busy = false
    }
  }

  const save = () =>
    run(async () => {
      apply(await saveMyLLM(form()))
      note = { kind: 'ok', text: 'Сохранено' }
    })

  const test = () =>
    run(async () => {
      const r = await testMyLLM(form())
      note = r.ok ? { kind: 'ok', text: 'Провайдер отвечает, ключ принят' } : { kind: 'err', text: r.detail || 'Не удалось' }
    })

  const remove = () =>
    run(async () => {
      apply(await deleteMyLLM())
      note = { kind: 'ok', text: 'Ключ удалён' }
    })
</script>

{#if loadError}
  <p class="err">{loadError}</p>
{:else if info}
  <SettingsBlock
    title="Ассистент (текст → квесты)"
    hint="Разбирает свободный текст в набор квестов и шагов. Работает на вашем ключе: запросы идут от вашего имени и считаются у вашего провайдера."
  >
    <p class="source" data-source={info.source}>{SOURCE[info.source]}</p>

    {#if !info.storage}
      <p class="muted">На этом сервере личный ключ не хранится — ассистент берёт ключ из окружения сервера.</p>
    {:else}
      <label class="field">
        <span>Провайдер</span>
        <select bind:value={provider}>
          {#each info.providers as p (p.id)}
            <option value={p.id}>{p.label}</option>
          {/each}
        </select>
      </label>

      {#if preset?.editable_base}
        <label class="field">
          <span>Адрес API</span>
          <input type="text" bind:value={baseUrl} placeholder="https://api.example.com/v1" autocomplete="off" spellcheck="false" />
        </label>
      {/if}

      <label class="field">
        <span>Модель</span>
        <input type="text" bind:value={model} placeholder={preset?.default_model || 'название модели'} autocomplete="off" spellcheck="false" />
      </label>

      <label class="field">
        <span>Ключ API</span>
        <input
          type="password"
          bind:value={apiKey}
          autocomplete="new-password"
          spellcheck="false"
          placeholder={info.has_key ? `задан ${info.key_hint} — введите новый, чтобы заменить` : 'вставьте ключ'}
        />
      </label>

      <div class="actions">
        <button type="button" class="btn" disabled={busy} onclick={test}>Проверить</button>
        <button type="button" class="btn btn--primary" disabled={busy || (!info.has_key && !apiKey)} onclick={save}>Сохранить</button>
        {#if info.has_key}
          <button type="button" class="btn btn--danger-solid" disabled={busy} onclick={remove}>Удалить ключ</button>
        {/if}
      </div>
      {#if note}<p class={note.kind === 'ok' ? 'ok' : 'err'}>{note.text}</p>{/if}
    {/if}
  </SettingsBlock>

  <SettingsBlock title="Что уходит провайдеру">
    <p class="muted">
      Текст, который вы вводите, и названия ваших разделов (подсказка для модели). Ключ хранится
      в зашифрованном виде и не показывается обратно; до {info.text_limit} символов за запрос, не больше 20 запросов за 10 минут.
    </p>
  </SettingsBlock>
{/if}

<style>
  .field {
    display: grid;
    gap: 0.25rem;
    font-size: var(--text-sm, 0.875rem);
  }
  .field > span {
    color: var(--color-fg-muted, #9a9a9a);
    font-size: 0.78rem;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-top: 0.25rem;
  }
  .source {
    margin: 0;
    font-size: var(--text-sm, 0.875rem);
    color: var(--color-fg);
  }
  .source[data-source='none'] {
    color: var(--color-warning, #d79921);
  }
  .muted {
    margin: 0;
    font-size: 0.8rem;
    line-height: 1.4;
    color: var(--color-fg-muted, #9a9a9a);
  }
  .ok {
    margin: 0;
    font-size: 0.85rem;
    color: var(--color-success, #8ec07c);
  }
  .err {
    margin: 0;
    font-size: 0.85rem;
    color: var(--color-danger, #b54a3a);
  }
</style>
