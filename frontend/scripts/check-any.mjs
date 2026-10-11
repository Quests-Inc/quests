// Явный `any` — долг: число вхождений не должно расти относительно .any-baseline.
// Сузили типы — запустите `bun run check:any -- --update`, чтобы опустить планку.
import { readFileSync, writeFileSync } from 'node:fs'

const baselineFile = new URL('../.any-baseline', import.meta.url)
const proc = Bun.spawnSync(['bunx', 'oxlint', 'src', '--format', 'json'], { stderr: 'inherit' })
const diags = JSON.parse(proc.stdout.toString()).diagnostics.filter((d) => d.code === 'typescript(no-explicit-any)')
const count = diags.length

if (process.argv.includes('--update')) {
  writeFileSync(baselineFile, `${count}\n`)
  console.log(`any baseline = ${count}`)
  process.exit(0)
}

const baseline = Number(readFileSync(baselineFile, 'utf8').trim())
if (count > baseline) {
  const byFile = new Map()
  for (const d of diags) byFile.set(d.filename, (byFile.get(d.filename) ?? 0) + 1)
  console.error(
    [...byFile]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 15)
      .map(([f, n]) => `${n}\t${f}`)
      .join('\n')
  )
  console.error(`\nexplicit any: ${count}, базовая линия ${baseline} — число растёт`)
  process.exit(1)
}
console.log(
  `explicit any: ${count} (базовая линия ${baseline})${count < baseline ? ' — обновите: bun run check:any -- --update' : ''}`
)
