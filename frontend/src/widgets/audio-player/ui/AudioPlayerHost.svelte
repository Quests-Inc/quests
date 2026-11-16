<script lang="ts">
  import { attachmentCatalog, flattenAttachmentIndex, ownerKind } from '@/entities/attachment'
  import { player } from '@/shared/lib/player.svelte.ts'
  import AudioPlayer from '@/shared/ui/AudioPlayer.svelte'

  let {
    onOpenAttachment,
    onOpenOwner,
    onShowMap,
  }: {
    onOpenAttachment?: (id: number) => void
    onOpenOwner?: (kind: string, id: number) => void
    onShowMap?: () => void
  } = $props()

  let owner = $derived.by(() => {
    const id = player.track?.id
    if (id == null) return null
    const meta = flattenAttachmentIndex(attachmentCatalog.index).find((a) => a.id === id)
    if (!meta) return null
    const kind = ownerKind(meta)
    if (kind === 'workspace') return null
    const ownerId = Number(meta.owner_id)
    return Number.isFinite(ownerId) && ownerId > 0 ? { kind, id: ownerId } : null
  })
</script>

<AudioPlayer {owner} {onOpenAttachment} {onOpenOwner} {onShowMap} />
