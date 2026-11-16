import { serverLimits } from './serverLimits.ts'

function limitLabel() {
  const mb = serverLimits.upload_bytes / (1024 * 1024)
  return Number.isInteger(mb) ? String(mb) : mb.toFixed(1)
}

/** Refuse before the request. A body past the server cap is cut off mid-upload, and the browser reports a network error instead of the limit. */
export function assertUploadSize(file: Blob) {
  if (file.size > serverLimits.upload_bytes) {
    throw new Error(`Файл больше ${limitLabel()} МБ`)
  }
}
