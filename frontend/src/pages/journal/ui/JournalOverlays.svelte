<script lang="ts">
  import { attachmentCatalog, flattenAttachmentIndex } from '@/entities/attachment'
  import { noteCatalog } from '@/entities/note'
  import { questCatalog } from '@/entities/quest'
  import { QuestlineModal } from '@/features/questline-edit'
  import { QuestModal, StepModal } from '@/features/quest-edit'
  import ConfirmModal from '@/shared/ui/ConfirmModal.svelte'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import { createJournalDesk } from '../model/desk.svelte.ts'

  let {
    desk,
    questlines = [],
    selected = null,
  }: {
    desk: ReturnType<typeof createJournalDesk>
    questlines?: { id: number; title: string }[]
    selected?: { id: number; title?: string } | null
  } = $props()

  let quests = $derived(questCatalog.list)
  let notes = $derived(noteCatalog.list)
  let attachments = $derived(flattenAttachmentIndex(attachmentCatalog.index).filter((a) => !a.hidden))
</script>

<QuestModal
  open={desk.modalOpen}
  mode={desk.modalMode}
  quest={desk.modalMode === 'edit' ? selected : null}
  defaults={desk.modalMode === 'create' ? desk.modalDefaults : null}
  {quests}
  {notes}
  {attachments}
  onClose={desk.closeQuestModal}
  onSaved={desk.onSaved}
  onDeleted={desk.onDeleted}
/>

<StepModal
  open={desk.stepModalOpen}
  questId={desk.stepModalQuestId}
  step={desk.stepModalStep}
  {quests}
  {questlines}
  {notes}
  {attachments}
  onClose={desk.closeStepModal}
  onSaved={(q) => desk.applyQuest(q, { select: false })}
/>

<QuestlineModal
  open={desk.lineModalOpen}
  mode={desk.lineModalMode}
  line={desk.lineModalMode === 'edit' ? desk.lineModalTarget : null}
  {quests}
  {questlines}
  {notes}
  {attachments}
  onClose={desk.closeLineModal}
  onSaved={desk.onQuestlineSaved}
  onDeleted={desk.onQuestlineDeleted}
/>

<ContextMenu
  open={desk.menu.open}
  x={desk.menu.x}
  y={desk.menu.y}
  items={desk.menu.items}
  onSelect={desk.menu.select}
  onClose={desk.menu.close}
/>

<ConfirmModal
  open={desk.del.questAsk}
  title="Удалить квест?"
  message={selected ? `Удалить квест «${selected.title}»?` : ''}
  busy={desk.del.questBusy}
  onCancel={desk.del.cancelQuest}
  onConfirm={desk.del.confirmQuest}
/>

<ConfirmModal
  open={desk.del.stepOpen}
  title="Удалить шаг?"
  message={desk.del.step ? `Удалить шаг «${desk.del.step.title}»?` : ''}
  busy={desk.del.stepBusy}
  onCancel={desk.del.cancelStep}
  onConfirm={desk.del.confirmStep}
/>

<ConfirmModal
  open={desk.del.lineAsk}
  title="Удалить квестлайн?"
  message="Квесты останутся, но отвяжутся от линии."
  busy={desk.del.lineBusy}
  onCancel={desk.del.cancelLine}
  onConfirm={desk.del.confirmLine}
/>
