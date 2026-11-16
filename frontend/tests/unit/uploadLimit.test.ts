import { expect, test } from 'bun:test'

import { serverLimits } from '@/shared/lib/serverLimits.ts'
import { assertUploadSize } from '@/shared/lib/uploadLimit.ts'

test('a file over the upload budget is refused before the request', () => {
  const old = serverLimits.upload_bytes
  try {
    serverLimits.upload_bytes = 1.5 * 1024 * 1024
    expect(() => assertUploadSize(new Blob([new Uint8Array(serverLimits.upload_bytes)]))).not.toThrow()
    expect(() => assertUploadSize(new Blob([new Uint8Array(serverLimits.upload_bytes + 1)]))).toThrow(
      'Файл больше 1.5 МБ'
    )
  } finally {
    serverLimits.upload_bytes = old
  }
})

test('the default budget is the server 25 MiB cap', () => {
  expect(serverLimits.upload_bytes).toBe(25 * 1024 * 1024)
  expect(() => assertUploadSize(new Blob([new Uint8Array(1)]))).not.toThrow()
})
