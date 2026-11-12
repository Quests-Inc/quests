<script lang="ts">
  import { createEntitySearch } from '@/shared/lib/entitySearch.svelte.ts'
  import SearchState from '@/shared/ui/SearchState.svelte'
  import type { TemplateRead } from '@/shared/api/automation.gen'
  import PageLayout from '@/shared/ui/PageLayout.svelte'
  import Picker from '@/shared/ui/Picker.svelte'
  import PageToolbar from '@/shared/ui/PageToolbar.svelte'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import Icon from '@/shared/ui/Icon.svelte'
  import { getRunStats } from '@/shared/api/client.ts'
  import type { SourceRunStats } from '@/shared/api/execution.gen.ts'
  import { openTarget } from '@/shared/lib/openTarget.ts'

  /** Список шаблонов в окне: строка с названием и сводкой, быстрые действия и переключатель «включён». */
  let {
    standalone = false,
    error = '',
    loading,
    templates,
    rowMeta,
    onCreate,
    onOpen,
    onMenu,
    onEmit,
    onCopy,
    onToggleEnabled,
  }: {
    standalone?: boolean
    error?: string
    loading: boolean
    templates: TemplateRead[]
    rowMeta(t: TemplateRead): string
    onCreate: () => void
    onOpen(t: TemplateRead): void
    onMenu(e: MouseEvent, t: TemplateRead): void
    onEmit(t: TemplateRead, e: Event): void
    onCopy(t: TemplateRead, e: Event): void
    onToggleEnabled(t: TemplateRead): void
  } = $props()

  let search = $state('')
  let statusFilter = $state('all')
  let frequency = $state('all')
  const statusOptions = [{ id: 'all', label: 'Все' }, { id: 'on', label: 'Включены' }, { id: 'off', label: 'Выключены' }, { id: 'error', label: 'С ошибкой' }]

  /** The latest emit run of each template that prints its quest by a command or a script (quest=490). */
  let runs = $state<Record<string, SourceRunStats>>({})
  const eligible = $derived(templates.filter((t) => {
    const matches = true
    return matches && (statusFilter === 'all' || statusFilter === 'on' && t.enabled || statusFilter === 'off' && !t.enabled || statusFilter === 'error' && (t.emit_pool_last_outcome === 'error' || runs[String(t.id)]?.last_state === 'failed')) && (frequency === 'all' || t.freq === frequency)
  }))
  const remoteSearch = createEntitySearch(() => search.trim() ? {q:search,types:['template'],allowed:{template:eligible.map(t=>t.id)}} : null,{all:true})
  const shown = $derived(remoteSearch.select(eligible,'template'))
  const RUN_WORDS: Record<string, string> = { done: 'последний выпуск выполнен', failed: 'ошибка последнего выпуска', waiting: 'выпуск ждёт устройство' }
  $effect(() => {
    if (!templates.some((t) => t.emit_pool_command || t.script_id)) return
    void getRunStats('template')
      .then((m) => (runs = m ?? {}))
      .catch(() => {})
  })
</script>

{#snippet tools()}
    <SearchState pending={remoteSearch.pending} error={remoteSearch.error} />
    <PageToolbar>
      <input class="search" type="search" aria-label="Поиск шаблонов" placeholder="Найти шаблон…" bind:value={search} />
      <button type="button" class="btn btn--accent btn--icon-only" title="Новый шаблон" aria-label="Новый шаблон" onclick={onCreate}>
        <Icon name="add" size={16} />
      </button>
    </PageToolbar>
    <PageToolbar>
      <OptionPills options={statusOptions} bind:value={statusFilter} label="Состояние шаблонов" compact wrap />
      <Picker options={[{id:"daily",label:"Каждый день"},{id:"weekly",label:"По дням недели"}]} value={frequency === "all" ? "" : frequency} onChange={v => frequency = v || "all"} noneLabel="Любое расписание" label="Периодичность" />
      <span class="hint" role="status">{shown.length} из {templates.length}</span>
    </PageToolbar>
{/snippet}
{#snippet rows()}
    {#if error}<p class="modal__error">{error}</p>{/if}
    {#if loading}
      <p class="hint">Загрузка…</p>
    {:else if templates.length === 0}
      <p class="hint">Пока нет шаблонов — создай дейлик или еженедельный.</p>
    {:else if shown.length === 0}
      <p class="hint">Шаблоны не найдены. Измените поиск или фильтры.</p>
      <button type="button" class="btn btn--ghost" onclick={() => { search = ''; statusFilter = 'all'; frequency = 'all' }}>Сбросить фильтры</button>
    {:else}
      <ul class="tpl-list">
        {#each shown as t (t.id)}
          <li class="tpl-row" class:tpl-row--off={!t.enabled} oncontextmenu={(e) => onMenu(e, t)}>
            <button type="button" class="tpl-row__main" onclick={() => onOpen(t)}>
              <span class="tpl-row__title">
                {t.title}
                {#if t.emit_pool_last_outcome === 'error'}
                  <span class="tpl-row__err" title="Не удалось выпустить квест — попытки за период исчерпаны">
                    ⚠ Ошибка выпуска
                  </span>
                {/if}
              </span>
              <span class="tpl-row__meta">{rowMeta(t)}{!t.enabled ? ' · выключен' : ''}</span>
            </button>
            <div class="tpl-row__actions">
              {#if t.emit_pool_command || t.script_id}
                {@const last = runs[String(t.id)]}
                <button
                  type="button"
                  class="btn btn--ghost btn--icon tpl-row__runs tpl-row__runs--{last?.last_state ?? 'none'}"
                  onclick={() => openTarget({ kind: 'runs', source: { kind: 'template', id: t.id }, run: last?.last_id })}
                  title={last ? `${RUN_WORDS[last.last_state] ?? last.last_state}. Запуски выпуска` : 'Запуски выпуска'}
                  aria-label="Запуски выпуска"
                >
                  <Icon name="list" size={14} />
                </button>
              {/if}
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                onclick={(e) => onEmit(t, e)}
                title="Выпустить сейчас"
                aria-label="Выпустить сейчас"
              >
                <Icon name="renew" size={14} />
              </button>
              <button
                type="button"
                class="btn btn--ghost btn--icon"
                onclick={(e) => onCopy(t, e)}
                title="Копировать (копия будет выключена)"
                aria-label="Копировать шаблон"
              >
                <Icon name="copy" size={14} />
              </button>
              <button type="button" class="btn btn--ghost btn--icon" aria-label="Действия шаблона" title="Действия шаблона" onclick={(e) => onMenu(e, t)}><Icon name="more" size={14} /></button>
              <button
                type="button"
                class="switch"
                class:switch--on={t.enabled}
                role="switch"
                aria-checked={t.enabled}
                onclick={() => onToggleEnabled(t)}
                title={t.enabled ? 'Выключить' : 'Включить'}
                aria-label={t.enabled ? 'Выключить шаблон' : 'Включить шаблон'}
              >
                <span class="switch__knob"></span>
              </button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
{/snippet}
{#if standalone}
  <PageLayout>
    {#snippet toolbar()}{@render tools()}{/snippet}
    <div class="tpl-content">{@render rows()}</div>
  </PageLayout>
{:else}
  <div class="modal__body">{@render tools()}{@render rows()}</div>
{/if}

<style>
  .tpl-content { padding:0.5rem 0; font-size:var(--text-sm); }
  .tpl-row__title { overflow-wrap: anywhere; }
  @container (max-width: 40rem) {
    .tpl-row { flex-wrap: wrap; padding: 0.5rem 0.75rem; }
    .tpl-row__main { flex-basis: 100%; }
    .tpl-row__actions { width: 100%; justify-content: flex-end; padding: 0 0.35rem 0.35rem; }
  }
  .tpl-list {
    container-type: inline-size;
    display: grid;
    gap: 0;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .tpl-row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding:0.5rem 1rem;
    border-bottom:1px solid var(--color-border);
    background:transparent;
  }

  .tpl-row--off .tpl-row__title { color: var(--color-fg-muted); }

  .tpl-row__main {
    display: grid;
    flex: 1;
    gap: 0.15rem;
    min-width: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .tpl-row__title {
    font-weight: 600;
  }

  .tpl-row__err {
    display: inline-block;
    white-space: nowrap;
    margin-left: 0.4em;
    padding: 0.05em 0.4em;
    border-radius: var(--radius-md, 4px);
    background: color-mix(in srgb, var(--color-danger, #b54a3a) 12%, transparent);
    color: var(--color-danger, #b54a3a);
    font-size: var(--text-xs, 0.75rem);
    font-weight: 500;
  }

  .tpl-row__runs--done {
    color: var(--color-success, #7a9e3a);
  }
  .tpl-row__runs--failed {
    color: var(--color-danger, #b54a3a);
  }
  .tpl-row__meta {
    font-size: var(--text-xs, 0.75rem);
    color: var(--color-fg-muted, #9a9a9a);
  }

  .tpl-row__actions {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    flex-shrink: 0;
  }

  .switch {
    position: relative;
    flex-shrink: 0;
    width: 2.25rem;
    height: 1.25rem;
    margin: 0;
    padding: 0;
    border: 1px solid var(--color-border-strong, #4a4a4a);
    border-radius: 999px;
    background: var(--color-bg-muted, #242424);
    cursor: pointer;
    transition:
      background 0.15s ease,
      border-color 0.15s ease;
  }

  .switch__knob {
    position: absolute;
    top: 1px;
    left: 1px;
    width: calc(1.25rem - 4px);
    height: calc(1.25rem - 4px);
    border-radius: 999px;
    background: var(--color-fg-muted, #9a9a9a);
    transition:
      transform 0.15s ease,
      background 0.15s ease;
  }

  .switch--on {
    border-color: color-mix(in srgb, var(--color-accent, #c9a227) 55%, var(--color-border, #333));
    background: color-mix(in srgb, var(--color-accent, #c9a227) 28%, var(--color-bg-muted, #242424));
  }

  .switch--on .switch__knob {
    transform: translateX(1rem);
    background: var(--color-accent, #c9a227);
  }</style>
