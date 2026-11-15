<script lang="ts">
  import { uiPreferences } from '@/shared/model/uiPreferences.svelte.ts'
  import Toggle from '@/shared/ui/Toggle.svelte'
  import SettingsBlock from './SettingsBlock.svelte'
</script>

<SettingsBlock title="Заметки" hint="Настройки сохраняются в этом браузере.">
  <Toggle label="Скрыть строку истории заметок" align="end" bind:checked={uiPreferences.hideNoteHistory} />
  <Toggle label="Скрыть значки в истории заметок" align="end" bind:checked={uiPreferences.hideNoteHistoryIcons} />
</SettingsBlock>
