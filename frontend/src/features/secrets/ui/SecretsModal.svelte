<script lang="ts">
  import { untrack } from 'svelte'
  import { SECRET_OWNER_KINDS } from '@/shared/api/client.ts'
  import { ValueTable } from '@/entities/value'
  import { MentionTextarea } from '@/entities/template'
  import { questCatalog } from '@/entities/quest'
  import { parseRefs } from '@/shared/lib/refs.ts'
  import ModalShell from '@/shared/ui/ModalShell.svelte'
  import ModalHead from '@/shared/ui/ModalHead.svelte'

  let { open = false, onClose, owner = null, questlines = [], labels = {} }: {
    open: boolean
    onClose: () => void
    owner?: { kind: string; id: number } | null
    questlines?: any[]
    labels?: Record<string, string>
  } = $props()
  let ownerText = $state('')
  let workspace = $state(false)
  const target = $derived(workspace ? { kind: 'workspace', id: 0 } : parseRefs(ownerText).find((r) => SECRET_OWNER_KINDS.includes(r.kind)))
  $effect(() => {
    if (!open) return
    untrack(() => { workspace = !owner || owner.kind === 'workspace'; ownerText = owner && owner.kind !== 'workspace' ? `${owner.kind}=${owner.id} ` : '' })
  })
</script>

<ModalShell {open} {onClose} labelledby="environment-modal-title" zIndex={80} maxWidth="40rem">
  <ModalHead id="environment-modal-title" title="Окружение" icon="key" {onClose} />
  <div class="modal__body">
    <label><input type="checkbox" bind:checked={workspace} /> Всё пространство</label>
    {#if !workspace}
      <MentionTextarea bind:value={ownerText} quests={questCatalog.list} {questlines} placeholder="@Название или note=12" ariaLabel="Чьё окружение" allowedKinds={SECRET_OWNER_KINDS} />
      {#if target}<p class="hint">{labels[`${target.kind}:${target.id}`] || `№${target.id}`}</p>{/if}
    {/if}
    {#if target}
      {#key `${target.kind}:${target.id}`}<ValueTable variant="panel" owner={target} />{/key}
    {:else}<p class="hint">Выберите заметку, квест, шаг, шаблон или квестлайн.</p>{/if}
  </div>
</ModalShell>
