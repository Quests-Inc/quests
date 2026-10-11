// Запускает тесты по группам; компонентные — каждый файл в своём процессе: `mock.module` в bun действует на весь процесс
// и просачивался бы из одного файла в другой (подмена API-клиента меняла бы поведение чужих тестов).
import { readdirSync } from 'node:fs'
import { join } from 'node:path'

const root = new URL('..', import.meta.url).pathname
const only = process.argv[2]

const groups = [
  { name: 'unit', args: ['tests/unit'], perFile: false },
  {
    name: 'components',
    args: ['--conditions', 'browser', '--preload', './tests/components/setup.ts'],
    dir: 'tests/components',
    perFile: true,
  },
  {
    name: 'security',
    args: ['--conditions', 'browser', '--preload', './tests/security/setup.ts'],
    dir: 'tests/security',
    perFile: true,
  },
].filter((g) => !only || g.name === only)

let failed = 0
for (const g of groups) {
  const targets = g.perFile
    ? readdirSync(join(root, g.dir))
        .filter((f) => f.endsWith('.test.ts'))
        .map((f) => `${g.dir}/${f}`)
    : [null]
  for (const target of targets) {
    const args = g.perFile ? [...g.args, target] : g.args
    const p = Bun.spawnSync(['bun', 'test', ...args], {
      cwd: root,
      stdout: 'pipe',
      stderr: 'pipe',
      timeout: 120_000,
      killSignal: 'SIGKILL',
    })
    const out = p.stdout.toString() + p.stderr.toString()
    if (p.exitCode !== 0) {
      failed++
      const hung = p.exitCode === null || p.signalCode === 'SIGKILL'
      console.error(
        `\n✗ ${g.name}${target ? ` · ${target}` : ''}${hung ? ' — ЗАВИС (больше 120 с: похоже на зацикленный эффект)' : ''}\n${out}`
      )
    } else {
      const m = /(\d+) pass/.exec(out)
      console.log(`✓ ${g.name}${target ? ` · ${target.split('/').pop()}` : ''} — ${m ? m[1] : '?'} pass`)
    }
  }
}
if (failed) {
  console.error(`\nтестов-групп с ошибками: ${failed}`)
  process.exit(1)
}
