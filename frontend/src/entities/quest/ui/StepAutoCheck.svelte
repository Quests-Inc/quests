<script lang="ts">
  import RunnerPicker from '@/shared/ui/RunnerPicker.svelte'
  import HelpTip from '@/shared/ui/HelpTip.svelte'
  import OptionPills from '@/shared/ui/OptionPills.svelte'
  import { mirrorQuestId as findMirrorQuest } from '../model/steps.ts'
  import { loadRunners, runnersList } from '@/entities/runner/@x/quest'
  import { ScriptPicker } from '@/entities/script/@x/quest'

  /**
   * A step's auto-check: command, poll interval, mode, pipeline gate.
   * The mode/gate controls only appear once there is a command to run.
   * `text` (read-only) is the step's title + description: a quest=N in it offers
   * the "mirror that quest" check (`quests progress N`).
   */
  let {
    command = $bindable(''),
    interval = $bindable(''),
    waitPrevious = $bindable(false),
    runMode = $bindable('poll'),
    runOn = $bindable(''),
    scriptId = $bindable(''),
    text = '',
    stepId,
  }: {
    command?: string
    interval?: string
    waitPrevious?: boolean
    runMode?: string
    runOn?: string
    scriptId?: string
    text?: string
    /** The step itself, when it exists already: a script made here names it as its source. */
    stepId?: number
  } = $props()

  const MODES = [
    { id: 'poll', label: 'опрос' },
    { id: 'watch', label: 'наблюдение' },
    { id: 'once', label: 'разово' },
  ]

  let scripted = $derived(Boolean(String(scriptId || '').trim()))
  // The mode, interval and device apply to a command and to a script alike.
  let hasCommand = $derived(scripted || Boolean(String(command || '').trim()))

  // Devices to pick from; a label that is not (or no longer) registered stays selectable so it is not lost silently.
  $effect(() => {
    if (hasCommand) {
      void loadRunners()
    }
  })

  /** First quest=N in the step's title/description, offered while no command is set. */
  let mirrorQuestId = $derived(hasCommand || scripted ? null : findMirrorQuest(text))

  function useMirrorCheck() {
    if (mirrorQuestId == null) return
    command = `quests progress ${mirrorQuestId}`
    runMode = 'watch'
  }
</script>

<div class="autocheck formscope">
  <div class="autocheck__script">
    <ScriptPicker kind="step" entityId={stepId} bind:value={scriptId} legacy={command} onClearLegacy={() => (command = '')} />
  </div>
  <div class="autocheck__row">
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
  {#if mirrorQuestId != null && !scripted}
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
      <span class="autocheck__runon"><RunnerPicker bind:value={runOn} runners={runnersList.items} /></span>
      {#if !runnersList.items.length}
        <span class="autocheck__note">Команды запускаются на раннерах: подключите раннер во вкладке «Раннеры».</span>
      {/if}
      <HelpTip label="Как работает автопроверка">
        <p><b>Опрос</b> — команда запускается каждые N сек (по умолчанию 60), число из stdout становится прогрессом шага. Можно вывести и JSON: <code>{`{"progress": 2, "total": 5, "description": "…"}`}</code>, все поля необязательны; <code>total</code> меняет максимум шага.</p>
        <p><b>Дочерний квест как шаг:</b> <code>quest=N</code> в названии или описании шага, режим «наблюдение» и команда <code>quests progress N</code> (кнопка-подсказка поставит всё сама).</p>
        <p><b>Наблюдение</b> — как опрос, но продолжается и после закрытия шага (не чаще раза в минуту): если число растёт, шаг открывается снова. Работает только пока квест активен.</p>
        <p><b>Разово</b> — один запуск: код 0 закрывает шаг, иначе квест проваливается.</p>
        <p><b>Ждать предыдущий</b> — шаг стартует только после предыдущего: линейный пайплайн.</p>
        <p><b>Устройство</b> — раннер (<code>quests-runner</code>), на котором запустится команда; команды выполняются только на раннерах. Можно выбрать несколько: команда уйдёт на первое доступное сверху и перейдёт к следующему, если оно не отвечает (подходит, когда команде всё равно, на каком устройстве идти). Раннеры добавляются во вкладке «Раннеры». Если устройство офлайн, разовая проверка ждёт его до суток и не проваливает квест.</p>
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
    flex: 1 1 11rem;
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
