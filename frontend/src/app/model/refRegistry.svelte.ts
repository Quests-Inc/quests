// Tells the shared link widgets what the app knows: the icon of every entity (refIcons) and the card a hover shows (refPreview).
// Called once from App; both registries follow the catalogs.
import { refIconStore } from '@/shared/lib/refIcons.svelte.ts'
import { setRefPreviewProvider } from '@/shared/lib/refPreview.ts'

import { buildRefIcons, buildRefPreview } from './refLabels.ts'

type Catalogs = Parameters<typeof buildRefIcons>[0] & Parameters<typeof buildRefPreview>[0]

export function bindRefRegistries(get: () => Catalogs & { labels: Record<string, string> }) {
  $effect(() => {
    refIconStore.map = buildRefIcons(get())
  })
  $effect(() => {
    const c = get()
    setRefPreviewProvider(buildRefPreview(c), (k) => c.labels[k])
    return () => setRefPreviewProvider(null)
  })
}
