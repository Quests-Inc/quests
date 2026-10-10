// svelte-check с «храповиком»: число ошибок не должно расти относительно .svelte-check-baseline.
// Сократили ошибки — запустите `bun run check -- --update`, чтобы опустить планку.
import { readFileSync, writeFileSync } from 'node:fs'

const baselineFile = new URL('../.svelte-check-baseline', import.meta.url)
const proc = Bun.spawnSync(['bunx', 'svelte-check', '--tsconfig', './tsconfig.json', '--output', 'machine'], {
  stderr: 'inherit',
})
const out = proc.stdout.toString()
const errors = out.split('\n').filter((l) => / ERROR /.test(l))
const count = errors.length

if (process.argv.includes('--update')) {
  writeFileSync(baselineFile, `${count}\n`)
  console.log(`baseline = ${count}`)
  process.exit(0)
}

const baseline = Number(readFileSync(baselineFile, 'utf8').trim())
if (count > baseline) {
  console.error(errors.join('\n'))
  console.error(`\nsvelte-check: ${count} ошибок, базовая линия ${baseline} — число ошибок растёт`)
  process.exit(1)
}
console.log(
  `svelte-check: ${count} ошибок (базовая линия ${baseline})${count < baseline ? ' — обновите: bun run check -- --update' : ''}`
)
