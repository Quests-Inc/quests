import { expect, test } from 'bun:test'

import {
  defaultPdfOptions,
  builtinPdfPresets,
  pdfOptionsError,
  restorePdfOptions,
} from '../../src/shared/lib/pdfOptions.ts'
test('built-in presets are valid and standard preserves historical layout', () => {
  for (const preset of builtinPdfPresets) expect(pdfOptionsError(preset.options)).toBe('')
  expect(defaultPdfOptions).toMatchObject({
    width: 210,
    height: 297,
    left: 16,
    right: 16,
    top: 16,
    bottom: 18,
    font_size: 10,
    line_spacing: 1.45,
  })
})
test('invalid geometry, numbers and persisted data are rejected', () => {
  expect(pdfOptionsError({ ...defaultPdfOptions, width: 80, left: 30, right: 30 })).toContain('40')
  expect(pdfOptionsError({ ...defaultPdfOptions, font_size: NaN })).not.toBe('')
  expect(pdfOptionsError({ ...defaultPdfOptions, bottom: 0 })).toContain('номер')
  expect(pdfOptionsError({ ...defaultPdfOptions, bottom: 0, number_format: 'none' })).toBe('')
  expect(restorePdfOptions({ font: 'nonexistent' })).toBeNull()
  expect(restorePdfOptions({ width: '210' })).toBeNull()
  expect(restorePdfOptions({ left: 0 })).toMatchObject({ left: 0, width: 210, font_size: 10 })
})
