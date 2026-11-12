<script lang="ts">
  let { pending = false, error = '' }: { pending?: boolean; error?: string } = $props()
</script>
{#if pending}<p class="hint" role="status">Поиск…</p>{:else if error}<p class="hint" role="alert">Не удалось выполнить поиск: {error}</p>{/if}
