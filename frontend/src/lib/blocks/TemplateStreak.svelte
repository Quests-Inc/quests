<script lang="ts">
  import { getTemplateStreak } from '../js/api.ts'

  /** Streak row for a quest emitted by a template; renders nothing until the
   * template has closed periods. `status` refetches when the quest closes.
   * @type {{ templateId: number, status?: string }} */
  let { templateId, status = '' }: { templateId: number, status?: string } = $props()

  let streak = $state((null as any | null))

  $effect(() => {
    const id = templateId
    void status
    if (!id) return
    let stale = false
    getTemplateStreak(id)
      .then((r) => {
        if (!stale) streak = r
      })
      .catch(() => {})
    return () => {
      stale = true
    }
  })
</script>

{#if streak && streak.closed}
  <div>
    <dt>Серия</dt>
    <dd>
      {streak.current_streak} подряд · лучшая {streak.longest_streak} · закрыто {Math.round(
        (streak.close_rate || 0) * 100,
      )}%
    </dd>
  </div>
{/if}
