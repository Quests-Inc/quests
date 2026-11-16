/** Applied server budgets shared by interactive rendering and exports. Loaded before app data. */
export const serverLimits = {
  embed_auto: 16,
  foreign_values: 8,
  values_own: 32,
  values_bytes: 8192,
  values_effective: 64,
  /** One attachment. Bytes, same budget as the server's upload_bytes (25 MiB). */
  upload_bytes: 25 * 1024 * 1024,
}
