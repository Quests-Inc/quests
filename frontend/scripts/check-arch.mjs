// Архитектурные гейты фронтенда (ADR-0001), которые не ловит check:fsd:
//  1. размер: файл не растёт сверх планки в .arch-baseline.json, новый файл — не длиннее MAX_LINES;
//  2. сегменты: model/ и lib/ не импортируют компоненты (.svelte), контроллеры `.svelte.ts` живут только в model/ и lib/;
//  3. сеть: fetch / XMLHttpRequest / WebSocket — только в shared/api и shared/lib/live.ts;
//  4. тесты: каждый файл model/ и shared/lib/ подключён хотя бы одним тестом (долг — в .arch-baseline.json).
// Запуск: `bun scripts/check-arch.mjs`; `--update` опускает планки (после сокращения файла или появления теста).
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

const ROOT = new URL('..', import.meta.url).pathname
const SRC = join(ROOT, 'src')
const TESTS = join(ROOT, 'tests')
const BASELINE = join(ROOT, '.arch-baseline.json')
const MAX_LINES = 800

const NET_ALLOWED = [/^shared\/api\//, /^shared\/lib\/live\.ts$/]

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else out.push(p)
  }
  return out
}

const rel = (p) => relative(SRC, p).split(sep).join('/')
const lines = (p) => readFileSync(p, 'utf8').split('\n').length - 1
const isCode = (f) => /\.(ts|js|svelte)$/.test(f) && !f.endsWith('.gen.ts') && !f.endsWith('.d.ts')

const files = walk(SRC).filter(isCode)
const tests = walk(TESTS).filter((f) => /\.(ts|mjs)$/.test(f))
const testText = tests.map((f) => readFileSync(f, 'utf8')).join('\n')

const baseline = (() => {
  try {
    return JSON.parse(readFileSync(BASELINE, 'utf8'))
  } catch {
    return { size: {}, untested: [] }
  }
})()

const problems = []
const nextBaseline = { size: {}, untested: [] }

for (const f of files) {
  const name = rel(f)
  const n = lines(f)
  // 1. размер
  if (n > MAX_LINES) {
    nextBaseline.size[name] = n
    const limit = baseline.size[name] ?? MAX_LINES
    if (n > limit)
      problems.push(
        `размер: ${name} — ${n} строк, планка ${limit}${baseline.size[name] ? '' : ` (новый файл не длиннее ${MAX_LINES})`}`
      )
  }
  const text = readFileSync(f, 'utf8')
  const segment = name.split('/').slice(0, 3)
  const inModel = /(^|\/)model\//.test(name)
  const inLib = /^shared\/lib\//.test(name)
  // 2. сегменты
  if ((inModel || inLib) && !f.endsWith('.svelte')) {
    for (const m of text.matchAll(/(?:from|import\()\s*['"]([^'"]+\.svelte)['"]/g)) {
      problems.push(`сегменты: ${name} импортирует компонент ${m[1]} — model/ и lib/ не знают про UI`)
    }
  }
  if (f.endsWith('.svelte.ts') && !inModel && !inLib) {
    problems.push(`сегменты: ${name} — контроллер .svelte.ts должен лежать в model/ или shared/lib/`)
  }
  // 3. сеть
  if (!NET_ALLOWED.some((re) => re.test(name))) {
    const code = text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1')
    if (/(?<![.\w])fetch\(|new XMLHttpRequest|new WebSocket\(/.test(code)) {
      problems.push(`сеть: ${name} ходит в сеть напрямую — только через shared/api`)
    }
  }
  // 4. тесты
  if ((inModel || inLib) && !/(^|\/)(index|types|constants)\.ts$/.test(name)) {
    const base = name.replace(/\.(svelte\.)?(ts|js)$/, '')
    const stem = base.split('/').pop()
    if (!testText.includes(`${base}`) && !new RegExp(`/${stem}(\\.svelte)?(\\.ts)?['"]`).test(testText)) {
      nextBaseline.untested.push(name)
      if (!baseline.untested.includes(name)) problems.push(`тесты: ${name} не подключён ни одним тестом`)
    }
  }
}

if (process.argv.includes('--update')) {
  nextBaseline.untested.sort()
  writeFileSync(BASELINE, JSON.stringify(nextBaseline, null, 2) + '\n')
  console.log(
    `arch baseline: файлов длиннее ${MAX_LINES} — ${Object.keys(nextBaseline.size).length}, без тестов — ${nextBaseline.untested.length}`
  )
  process.exit(0)
}

if (problems.length) {
  console.error(problems.join('\n') + `\n\narch: нарушений ${problems.length}`)
  process.exit(1)
}
const shrunk = Object.keys(baseline.size).filter(
  (k) => !(k in nextBaseline.size) || nextBaseline.size[k] < baseline.size[k]
).length
const tested = baseline.untested.filter((k) => !nextBaseline.untested.includes(k)).length
console.log(
  `arch: гейты соблюдены${shrunk || tested ? ' — можно опустить планки: bun scripts/check-arch.mjs --update' : ''}`
)
