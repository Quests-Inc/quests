<script lang="ts">
  import type { SourceRunStats } from '@/shared/api/execution.gen.ts'
  import type { Step } from '@/shared/api/journal.gen.ts'
  import { openTarget } from '@/shared/lib/openTarget.ts'
  import { ago } from '@/shared/lib/time.ts'

  /**
   * The chip of a step that checks itself (a command or a script, quest=490): how often, on which runner, and a dot with how
   * its latest run went. A click opens the step's runs.
   */
  let { step, waiting = false, last }: { step: StepLike; waiting?: boolean; last?: SourceRunStats } = $props()

  type StepLike = Pick<Step, 'id' | 'check_command' | 'script_id' | 'run_status' | 'run_mode' | 'check_interval_seconds' | 'run_on'>
  const RUN_WORDS: Record<string, string> = { done: 'готово', failed: 'ошибка', waiting: 'ждёт раннер' }
</script>

<button
  type="button"
  class="step__auto step__auto--btn"
  class:step__auto--run={step.run_status === 'running'}
  class:step__auto--wait={waiting}
  title={(step.script_id ? `Скрипт ${step.script_id}` : step.check_command) +
    (last ? `\nПоследний запуск: ${RUN_WORDS[last.last_state] ?? last.last_state}, ${ago(last.last_at)}` : '\nЕщё не запускался') +
    '\nНажмите, чтобы открыть запуски'}
  onclick={() => openTarget({ kind: 'runs', source: { kind: 'step', id: step.id }, run: last?.last_id })}
>
  {#if last}<span class="step__dot step__dot--{last.last_state}" aria-hidden="true"></span>{/if}
  {step.script_id ? '⚙' : ''}
  {#if waiting}
    ждёт
  {:else if step.run_status === 'running'}
    идёт
  {:else if step.run_status === 'fail'}
    сбой
  {:else if step.run_mode === 'once'}
    разово
  {:else if step.run_mode === 'watch'}
    наблюдение {step.check_interval_seconds || '?'} с
  {:else}
    каждые {step.check_interval_seconds || '?'} с
  {/if}
  {#if step.run_on}
    · {step.run_on}
  {/if}
</button>
