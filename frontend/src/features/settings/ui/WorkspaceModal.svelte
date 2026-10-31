<script lang="ts">
  import { untrack } from 'svelte'
  import { ColorIconPicker } from '@/entities/questline'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'
  import ModalFoot from '@/shared/ui/ModalFoot.svelte'
  import { wsx, addWorkspace, saveWorkspaceDetails } from '../model/workspaceState.svelte.ts'

  let { open, workspace = null, onClose }: {
    open: boolean,
    workspace?: { id: number; name: string; color?: string; icon?: string } | null,
    onClose: () => void,
  } = $props()
  let name = $state('')
  let color = $state('#9a9a9a')
  let icon = $state('layers')
  let error = $state('')
  $effect(() => {
    if (!open) return
    untrack(() => {
      name = workspace?.name || ''
      color = workspace?.color || '#9a9a9a'
      icon = workspace?.icon || 'layers'
      error = ''
    })
  })
  async function save(event: SubmitEvent) {
    event.preventDefault()
    if (wsx.busy) return
    if (!name.trim()) { error = 'Нужно название пространства'; return }
    const ok = workspace
      ? await saveWorkspaceDetails(workspace.id, name, color, icon)
      : await addWorkspace(name, { color, icon })
    if (ok) onClose()
    else error = wsx.error
  }
</script>

<ModalShell {open} {onClose} labelledby="workspace-modal-title" maxWidth="30rem" zIndex={60}>
  <ModalHead id="workspace-modal-title" title={workspace ? 'Редактировать пространство' : 'Новое пространство'} icon="layers" {onClose} />
  <form class="modal__form" onsubmit={save}>
    <label class="field"><span class="label">Название</span><input type="text" bind:value={name} maxlength="80" required disabled={wsx.busy} /></label>
    <fieldset disabled={wsx.busy}>
      <ColorIconPicker bind:color bind:icon allowUpload={false} />
    </fieldset>
    {#if error}<p class="modal__error" role="alert">{error}</p>{/if}
    <ModalFoot onCancel={onClose} submitLabel={workspace ? 'Сохранить' : 'Создать'} submitIcon="save" busy={wsx.busy} />
  </form>
</ModalShell>
<style>fieldset { display: grid; gap: 1rem; min-width: 0; padding: 0; margin: 0; border: 0; }</style>
