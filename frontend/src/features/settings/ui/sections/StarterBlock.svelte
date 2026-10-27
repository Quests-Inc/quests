<script lang="ts">
  import { removeStarter, seedStarter } from '@/shared/api/client.ts'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  let { readonly = false }: { readonly?: boolean } = $props()
  let busy = $state(false), error = $state(''), result = $state(''), confirm = $state(false)
  async function run(remove = false) {
    if (busy || readonly) return
    busy = true; error = ''; result = ''; confirm = false
    try {
      if (remove) { const r = await removeStarter(); result = `В корзину перенесено: ${r.trashed}.` }
      else { const r = await seedStarter(); result = `Добавлено: ${r.added}, обновлено: ${r.updated}, сохранено без изменений: ${r.preserved}.` }
      window.dispatchEvent(new Event('quests:content-changed'))
    } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
</script>
<section class="starter">
  <h3>Примеры и гайды</h3>
  <p class="hint">Учебные квесты, заметки, скрипты, выключенные хук и шаблон. Повторное добавление сохраняет ваши правки и не возвращает удалённые примеры.</p>
  {#if error}<p class="modal__error" role="alert">{error}</p>{/if}
  {#if result}<p role="status">{result}</p>{/if}
  <button type="button" class="btn" disabled={busy || readonly} onclick={() => void run()}>Добавить примеры</button>
  <button type="button" class="btn btn--ghost" disabled={busy || readonly} onclick={() => (confirm = true)}>Убрать примеры…</button>
</section>
<ConfirmModal open={confirm} title="Убрать стартовые примеры?" message="Нетронутые примеры, шаблон и квестлайн отправятся в корзину. Изменённые вами примеры сохранятся. Повторное добавление не вернёт убранный набор." {busy} onCancel={() => (confirm = false)} onConfirm={() => void run(true)} />
