import { expect, test } from 'bun:test'

import { isAudioFile } from '@/shared/lib/audioFile.ts'

test('isAudioFile recognises mime and extension', () => {
  expect(isAudioFile({ content_type_detected: 'audio/mpeg', filename: 'x.bin' })).toBe(true)
  expect(isAudioFile({ filename: 'theme.mp3' })).toBe(true)
  expect(isAudioFile({ filename: 'shot.png', mime: 'image/png' })).toBe(false)
  expect(isAudioFile(null)).toBe(false)
})
