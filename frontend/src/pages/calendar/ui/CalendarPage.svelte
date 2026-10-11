<script lang="ts">
  import { questCatalog } from '@/entities/quest'
  import { ActivityCalendar } from '@/widgets/activity-calendar'

  let {
    labels = {},
    onRef,
    onSelectQuest,
    onQuestContextMenu,
    onOpenHistory,
  }: {
    labels?: Record<string, string>
    onRef?: (kind: string, id: number) => void
    onSelectQuest?: (id: number) => void
    onQuestContextMenu?: (event: MouseEvent, id: number) => void
    onOpenHistory?: (dayKey: string) => void
  } = $props()

  let quests = $derived(questCatalog.list)
</script>

<div class="journal__calendar">
  <ActivityCalendar {quests} {labels} {onRef} {onSelectQuest} {onQuestContextMenu} {onOpenHistory} />
</div>
