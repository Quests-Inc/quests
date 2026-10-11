<script lang="ts">
  import HelpTip from './HelpTip.svelte'
  import OptionPills from './OptionPills.svelte'
  import { mirrorQuestId as findMirrorQuest } from '../js/steps.ts'
  import { loadRunners, runnersList } from '../js/runnersList.svelte.ts'

  /** A step's auto-check: command, poll interval, mode, pipeline gate.
   * The mode/gate controls only appear once there is a command to run.
   * `text` (read-only) is the step's title + description: a quest=N in it offers
   * the "mirror that quest" check (`quests progress N`).
   * @type {{ command?: string, interval?: string, waitPrevious?: boolean, runMode?: string, runOn?: string, text?: string }} */
  let {
    command = $bindable(''),
    interval = $bindable(''),
    waitPrevious = $bindable(false),
    runMode = $bindable('poll'),
    runOn = $bindable(''),
    text = '',
  }: { command?: string, interval?: string, waitPrevious?: boolean, runMode?: string, runOn?: string, text?: string } = $props()

  const MODES = [
    { id: 'poll', label: 'опрос' },
    { id: 'watch', label: 'наблюдение' },
    { id: 'once', label: 'разово' },
  ]

  let hasCommand = $derived(Boolean(String(command || '').trim()))

  // Devices to pick from; a label that is not (or no longer) registered stays selectable so it is not lost silently.
  $effect(() => {
    if (hasCommand) {
      void loadRunners()
    }
  })
  let missing = $derived(runOn && !runnersList.items.some((r) => r.label === runOn) ? runOn : '')

  /** First quest=N in the step's title/description, offered while no command is set. */
  let mirrorQuestId = $derived(hasCommand ? null : findMirrorQuest(text))

  function useMirrorCheck() {
    if (mirrorQuestId == null) return
    command = `quests progress ${mirrorQuestId}`
    runMode = 'watch'
  }
</script>

<div class="autocheck">
  <div class="autocheck__row">
    <textarea
      class="mono autocheck__cmd"
      rows="1"
      placeholder={'команда автопроверки (необязательно); можно несколько строк или скрипт: #!/usr/bin/env python3 …'}
      aria-label="Команда автопроверки"
      bind:value={command}
      spellcheck="false"
    ></textarea>
    {#if hasCommand && runMode !== 'once'}
      <input
        type="number"
        class="autocheck__interval"
        min="15"
        step="15"
        placeholder="сек"
        title="Интервал опроса, сек (мин. 15)"
        aria-label="Интервал опроса, сек"
        bind:value={interval}
      />
    {/if}
  </div>
  {#if mirrorQuestId != null}
    <button type="button" class="btn btn--ghost autocheck__suggest" onclick={useMirrorCheck}>
      Следить за quest={mirrorQuestId}: <code>quests progress {mirrorQuestId}</code>
    </button>
  {/if}
  {#if hasCommand}
    <div class="autocheck__opts">
      <OptionPills options={MODES} bind:value={runMode} label="Режим автопроверки" compact />
      <label class="check">
        <input type="checkbox" bind:checked={waitPrevious} />
        ждать предыдущий
      </label>
      <select class="autocheck__runon" title="Где запускать проверку" aria-label="Устройство" bind:value={runOn}>
        <option value="" disabled>выберите устройство</option>
        {#each runnersList.items as r (r.label)}
          <option value={r.label}>{r.label}{r.online ? '' : r.registered ? ' (офлайн)' : ' (не подключён)'}</option>
        {/each}
        {#if missing}
          <option value={missing}>{missing} (нет такого раннера)</option>
        {/if}
      </select>
      {#if !runnersList.items.length}
        <span class="autocheck__note">Команды запускаются на раннерах: подключите раннер во вкладке «Раннеры».</span>
      {/if}
      <HelpTip label="Как работает автопроверка">
        <p><b>Опрос</b> — команда запускается каждые N сек (по умолчанию 60), число из stdout становится прогрессом шага. Можно вывести и JSON: <code>{`{"progress": 2, "total": 5, "description": "…"}`}</code>, все поля необязательны; <code>total</code> меняет максимум шага.</p>
        <p><b>Дочерний квест как шаг:</b> <code>quest=N</code> в названии или описании шага, режим «наблюдение» и команда <code>quests progress N</code> (кнопка-подсказка поставит всё сама).</p>
        <p><b>Наблюдение</b> — как опрос, но продолжается и после закрытия шага (не чаще раза в минуту): если число растёт, шаг открывается снова. Работает только пока квест активен.</p>
        <p><b>Разово</b> — один запуск: код 0 закрывает шаг, иначе квест проваливается.</p>
        <p><b>Ждать предыдущий</b> — шаг стартует только после предыдущего: линейный пайплайн.</p>
        <p><b>Устройство</b> — раннер (<code>quests-runner</code>), на котором запустится команда; команды выполняются только на раннерах. Раннеры добавляются во вкладке «Раннеры». Если устройство офлайн, разовая проверка ждёт его до суток и не проваливает квест.</p>
      </HelpTip>
    </div>
  {/if}
</div>

<style>
  .autocheck {
    display: grid;
    gap: 0.4rem;
  }

  .autocheck__row {
    display: flex;
    gap: 0.4rem;
  }

  /* Two classes deep to outrank the shared .modal input rule. */
  .autocheck__row .autocheck__cmd {
    flex: 1 1 auto;
    font-size: var(--text-xs, 0.75rem);
    resize: vertical;
    min-height: 2.1rem;
    line-height: 1.4;
  }

  .autocheck__row .autocheck__interval {
    flex: 0 0 4.5rem;
    width: 4.5rem;
  }

  .autocheck__note {
    flex-basis: 100%;
    font-size: 0.78rem;
    color: var(--color-fg-muted);
  }

  .autocheck__opts .autocheck__runon {
    flex: 0 1 11rem;
    min-width: 0;
    font-size: var(--text-xs, 0.75rem);
  }

  .autocheck__suggest {
    justify-self: start;
    font-size: var(--text-xs, 0.75rem);
  }

  .autocheck__opts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem 0.75rem;
  }
</style>
