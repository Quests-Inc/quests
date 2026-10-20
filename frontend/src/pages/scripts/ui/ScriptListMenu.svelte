<script lang="ts">
  import { PURPOSE_NONE, PURPOSES } from '@/entities/script'
  import type { Script } from '@/shared/api/execution.gen.ts'
  import ContextMenu from '@/shared/ui/ContextMenu.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalShell from '@/shared/ui/ModalShell.svelte'

  /**
   * Right click on a script in the list: rename, what it is for (a second menu), bake it into a custom node, delete. The
   * actions are the page's; this holds the menu and the one-line rename form.
   */
  let {
    menu,
    scripts,
    onClose,
    onRename,
    onPurpose,
    onBake,
    onDelete,
    onCopy,
    onDuplicate,
    group = [],
    groupItems = [],
    onGroup,
  }: {
    menu: { open: boolean; x: number; y: number; id: number | null }
    scripts: Script[]
    onClose: () => void
    onRename: (id: number, title: string) => Promise<void>
    onPurpose: (id: number, purpose: string) => void
    onBake: (id: number) => void
    onDelete: (id: number) => void
    onCopy: (id: number) => void
    onDuplicate: (id: number) => void
    /** Picked scripts the menu is about when it was opened on one of them (more than one). */
    group?: number[]
    groupItems?: { id: string; label?: string; sep?: boolean; danger?: boolean; children?: { id: string; label?: string }[] }[]
    onGroup?: (action: string, ids: number[]) => void
  } = $props()

  const current = $derived(scripts.find((s) => s.id === menu.id))
  const items = $derived(group.length > 1 ? groupItems : [
    { id: 'copy-id', label: `Копировать script=${menu.id}` },
    { id: 'sep-ref', sep: true },
    { id: 'rename', label: 'Переименовать…' },
    { id: 'duplicate', label: 'Дублировать' },
    {
      id: 'purpose',
      label: 'Принадлежность',
      children: [...PURPOSES, PURPOSE_NONE].map((p) => ({ id: `p:${p.id}`, label: p.title, checked: (current?.purpose ?? '') === p.id })),
    },
    { id: 'bake', label: 'Запечь в кастомную ноду…' },
    { id: 'sep', sep: true },
    { id: 'del', label: 'Удалить', danger: true },
  ])

  let renaming = $state<{ id: number; title: string } | null>(null)
  let busy = $state(false)

  function select(id: string): void {
    const sid = menu.id
    if (sid == null) return
    if (group.length > 1) return void onGroup?.(id, group)
    if (id === 'copy-id') onCopy(sid)
    else if (id === 'duplicate') onDuplicate(sid)
    else if (id === 'rename') renaming = { id: sid, title: current?.title ?? '' }
    else if (id === 'bake') onBake(sid)
    else if (id === 'del') onDelete(sid)
    else if (id.startsWith('p:')) onPurpose(sid, id.slice(2))
  }

  async function submit() {
    if (!renaming || !renaming.title.trim()) return
    busy = true
    try {
      await onRename(renaming.id, renaming.title.trim())
      renaming = null
    } finally {
      busy = false
    }
  }
</script>

<ContextMenu open={menu.open} x={menu.x} y={menu.y} {items} onSelect={select} {onClose} />

<ModalShell open={renaming !== null} onClose={() => (renaming = null)} labelledby="rename-script-title" maxWidth="26rem">
  <ModalHead id="rename-script-title" title="Переименовать скрипт" icon="edit" onClose={() => (renaming = null)} />
  {#if renaming}
    <form
      class="modal__form"
      onsubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      <label class="field">
        <span class="label">Название</span>
        <input type="text" bind:value={renaming.title} required maxlength="200" />
      </label>
      <ModalFoot onCancel={() => (renaming = null)} submitLabel="Сохранить" {busy} disabled={!renaming.title.trim()} />
    </form>
  {/if}
</ModalShell>
