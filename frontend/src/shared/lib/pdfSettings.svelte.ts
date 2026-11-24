import { renderPdf } from '@/shared/api/exportPdf.ts'
import { fetchAuthState, fetchWorkspaces } from '@/shared/api/identity.ts'

import type { PdfDocument } from './pdfExport.ts'
import { builtinPdfPresets, defaultPdfOptions, restorePdfOptions, savePdfBlob, type PdfPreset } from './pdfOptions.ts'

export const pdfPreferences = $state({ name: 'Стандартный' })
export interface PdfPreferences {
  quick: PdfPreset
  custom: PdfPreset[]
}
export interface PdfExportSession {
  document: PdfDocument
  filename: string
  storageKey: string
  preferences: PdfPreferences
  buildBook?: () => Promise<PdfDocument>
  resolve: (saved: boolean) => void
}
export const pdfSettings = $state({ session: null as PdfExportSession | null })

export function readPdfPreferences(key: string): PdfPreferences {
  const fallback: PdfPreferences = { quick: { ...builtinPdfPresets[0], options: { ...defaultPdfOptions } }, custom: [] }
  try {
    const saved = JSON.parse(localStorage.getItem(key) || 'null')
    if (!saved) return fallback
    const parse = (p: PdfPreset): PdfPreset | null => {
      const options = restorePdfOptions(p?.options)
      return options && typeof p?.name === 'string' && p.name.length <= 64 && typeof p.id === 'string'
        ? { id: p.id, name: p.name, options }
        : null
    }
    const quick = parse(saved.quick)
    const custom = Array.isArray(saved.custom)
      ? saved.custom.slice(0, 20).flatMap((p: PdfPreset) => {
          const v = parse(p)
          return v ? [v] : []
        })
      : []
    return { quick: quick ?? fallback.quick, custom }
  } catch {
    return fallback
  }
}
export function writePdfPreferences(key: string, value: PdfPreferences) {
  localStorage.setItem(key, JSON.stringify(value))
  pdfPreferences.name = value.quick.name
}
async function preferencesContext() {
  const [auth, workspaces] = await Promise.all([fetchAuthState(), fetchWorkspaces()])
  const key = `quests.pdf.v1:${encodeURIComponent(auth.username || 'local')}:${workspaces.current}`
  const preferences = readPdfPreferences(key)
  pdfPreferences.name = preferences.quick.name
  return { key, preferences }
}
export async function refreshPdfPreferences() {
  try {
    await preferencesContext()
  } catch {
    /* export will report errors when actually requested */
  }
}
export async function exportPdfDocument(
  document: PdfDocument,
  filename: string,
  options: { configure?: boolean; buildBook?: () => Promise<PdfDocument> } = {}
) {
  const { key, preferences } = await preferencesContext()
  if (options.configure) {
    if (pdfSettings.session) throw new Error('Закройте открытые настройки PDF.')
    return new Promise<boolean>((resolve) => {
      pdfSettings.session = { document, filename, storageKey: key, preferences, buildBook: options.buildBook, resolve }
    })
  }
  const blob = await renderPdf({ ...document, options: preferences.quick.options }, filename)
  savePdfBlob(blob, filename)
  return true
}
export function closePdfSettings(saved = false) {
  const session = pdfSettings.session
  pdfSettings.session = null
  session?.resolve(saved)
}
