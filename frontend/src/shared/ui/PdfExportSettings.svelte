<script lang="ts">
 import { onMount } from 'svelte'
 onMount(()=>{void refreshPdfPreferences()})
  import { pdfSettings, refreshPdfPreferences } from '@/shared/lib/pdfSettings.svelte.ts'
  import PdfSettingsDialog from './PdfSettingsDialog.svelte'
</script>
{#if pdfSettings.session}
  {#key pdfSettings.session}
    <PdfSettingsDialog session={pdfSettings.session} />
  {/key}
{/if}
