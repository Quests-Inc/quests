<script>
  import HelpTip from './HelpTip.svelte'
  import OptionPills from './OptionPills.svelte'
  import { mirrorQuestId as findMirrorQuest } from '../js/steps.js'

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
  } = $props()

  const MODES = [
    { id: 'poll', label: 'опрос' },
    { id: 'watch', label: 'наблюдение' },
    { id: 'once', label: 'разово' },
  ]

  let hasCommand = $derived(Boolean(String(command || '').trim()))

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
    <input
      type="text"
      class="mono autocheck__cmd"
      placeholder="команда автопроверки (необязательно)"
      aria-label="Команда автопроверки"
      bind:value={command}
      spellcheck="false"
    />
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
      <input
        type="text"
        class="mono autocheck__runon"
        placeholder="устройство"
        title="Метка раннера, на котором запускать проверку (пусто — на сервере)"
        aria-label="Раннер"
        maxlength="32"
        bind:value={runOn}
        spellcheck="false"
      />
      <HelpTip label="Как работает автопроверка">
        <p><b>Опрос</b> — команда запускается каждые N сек (по умолчанию 60), число из stdout становится прогрессом шага. Можно вывести и JSON: <code>{`{"progress": 2, "total": 5, "description": "…"}`}</code>, все поля необязательны; <code>total</code> меняет максимум шага.</p>
        <p><b>Дочерний квест как шаг:</b> <code>quest=N</code> в названии или описании шага, режим «наблюдение» и команда <code>quests progress N</code> (кнопка-подсказка поставит всё сама).</p>
        <p><b>Наблюдение</b> — как опрос, но продолжается и после закрытия шага (не чаще раза в минуту): если число растёт, шаг открывается снова. Работает только пока квест активен.</p>
        <p><b>Разово</b> — один запуск: код 0 закрывает шаг, иначе квест проваливается.</p>
        <p><b>Ждать предыдущий</b> — шаг стартует только после предыдущего: линейный пайплайн.</p>
        <p><b>Устройство</b> — метка раннера (<code>quests-runner</code>), на котором запустится команда; пусто — на сервере. Если устройство офлайн, разовая проверка ждёт его до суток и не проваливает квест.</p>
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
  }

  .autocheck__row .autocheck__interval {
    flex: 0 0 4.5rem;
    width: 4.5rem;
  }

  .autocheck__opts .autocheck__runon {
    flex: 0 0 7rem;
    width: 7rem;
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
