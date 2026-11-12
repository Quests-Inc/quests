<script lang="ts">
  import DurationInput from './DurationInput.svelte'
  import HelpTip from './HelpTip.svelte'
  let { hours=$bindable(''),minutes=$bindable(''),expire=$bindable(true),showWindow=true,label='Помечать просроченным после срока' }: {hours?:string;minutes?:string;expire?:boolean;showWindow?:boolean;label?:string}=$props()
</script>
{#if showWindow}
  <div class="inline-line">
    <span>Окно срочности</span>
    <DurationInput bind:hours bind:minutes label="Окно срочности" />
    <HelpTip label="Как работает окно срочности">
      <p>За это время до срока квест становится срочным. Цвет таймера меняется по оставшейся доле окна.</p>
      <p>Пустое или нулевое окно — нейтральный таймер без напоминания о начале окна. Автопросрочка настраивается отдельно.</p>
    </HelpTip>
  </div>
{/if}
<label class="check"><input type="checkbox" bind:checked={expire} />{label}</label>
<p class="hint">{expire ? 'После срока незавершённый активный квест становится просроченным.' : 'Срок служит ориентиром; после него квест остаётся активным.'}</p>
