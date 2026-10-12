<script lang="ts">
  import Icon from './Icon.svelte'

  /**
   * Where a command runs: one device or an ordered list. With several, the command goes to the first one that is
   * online and falls to the next when it does not answer, so it must not depend on one particular device.
   * `value` is the stored `run_on`: labels separated by commas.
   */
  let {
    value = $bindable(''),
    runners = [],
    onchange,
    label = 'Устройство',
  }: {
    value?: string
    runners?: { label: string; online?: boolean; registered?: boolean }[]
    onchange?: () => void
    label?: string
  } = $props()

  let chosen = $derived(
    String(value || '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
  )
  let rest = $derived(runners.filter((r) => !chosen.includes(r.label)))

  function state(l: string) {
    const r = runners.find((x) => x.label === l)
    if (!r) return ' (нет такого раннера)'
    return r.online ? '' : r.registered ? ' (офлайн)' : ' (не подключён)'
  }

  function set(next: string[]) {
    value = next.join(',')
    onchange?.()
  }

  function move(i: number, d: number) {
    const next = [...chosen]
    const j = i + d
    if (j < 0 || j >= next.length) return
    ;[next[i], next[j]] = [next[j], next[i]]
    set(next)
  }
</script>

<div class="rpick">
  {#if chosen.length}
    <ol class="rpick__list">
      {#each chosen as l, i (l)}
        <li class="rpick__item">
          {#if chosen.length > 1}<span class="rpick__n">{i + 1}</span>{/if}
          <span class="rpick__name">{l}<span class="rpick__state">{state(l)}</span></span>
          {#if chosen.length > 1}
            <button type="button" class="rpick__btn" disabled={i === 0} aria-label="Выше" title="Выше: пробуется раньше" onclick={() => move(i, -1)}>
              <Icon name="arrow-up" size={12} />
            </button>
            <button type="button" class="rpick__btn rpick__btn--down" disabled={i === chosen.length - 1} aria-label="Ниже" title="Ниже: пробуется позже" onclick={() => move(i, 1)}>
              <Icon name="arrow-up" size={12} />
            </button>
          {/if}
          <button type="button" class="rpick__btn" aria-label="Убрать {l}" title="Убрать" onclick={() => set(chosen.filter((x) => x !== l))}>
            <Icon name="close" size={12} />
          </button>
        </li>
      {/each}
    </ol>
  {/if}
  {#if rest.length || !chosen.length}
    <select
      class="rpick__add"
      aria-label={chosen.length ? 'Добавить устройство' : label}
      value=""
      onchange={(e) => {
        const v = e.currentTarget.value
        if (v) set([...chosen, v])
        e.currentTarget.value = ''
      }}
    >
      <option value="" disabled>{chosen.length ? '+ ещё устройство (запасное)' : 'выберите устройство'}</option>
      {#each rest as r (r.label)}
        <option value={r.label}>{r.label}{r.online ? '' : r.registered ? ' (офлайн)' : ' (не подключён)'}</option>
      {/each}
    </select>
  {/if}
  {#if chosen.length > 1}
    <p class="rpick__hint">Команда уйдёт на первое доступное устройство сверху и перейдёт к следующему, если оно не отвечает.</p>
  {/if}
</div>

<style>
  .rpick {
    display: grid;
    gap: 0.3rem;
    min-width: 0;
  }
  .rpick__list {
    display: grid;
    gap: 0.2rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .rpick__item {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.1rem 0.2rem 0.1rem 0.5rem;
    border: 1px solid var(--color-border, #333);
    border-radius: var(--radius-md, 4px);
    font-size: var(--text-sm, 0.875rem);
  }
  .rpick__n {
    color: var(--color-fg-subtle, #6e6e6e);
    font-variant-numeric: tabular-nums;
  }
  .rpick__name {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .rpick__state {
    color: var(--color-fg-muted, #9a9a9a);
  }
  .rpick__btn {
    display: grid;
    place-items: center;
    width: 1.5rem;
    height: 1.5rem;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm, 3px);
    background: transparent;
    color: var(--color-fg-muted, #9a9a9a);
    cursor: pointer;
  }
  .rpick__btn--down :global(svg) {
    transform: rotate(180deg);
  }
  .rpick__btn:hover:not(:disabled) {
    color: var(--color-fg, #e8e8e8);
    background: rgba(127, 127, 127, 0.2);
  }
  .rpick__btn:disabled {
    opacity: 0.3;
    cursor: default;
  }
  .rpick__hint {
    margin: 0;
    font-size: 0.75rem;
    color: var(--color-fg-muted, #9a9a9a);
  }
</style>
