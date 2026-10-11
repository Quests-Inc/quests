// Границы слоёв FSD (ADR-0001): app → pages → widgets → features → entities → shared, импорт только вниз,
// слайсы одного слоя друг друга не знают, в слайс заходят только через index.ts.
// Разбирает .ts и скрипты .svelte (steiger не видит импортов, у которых целью служит .svelte-файл).
// Запуск: `bun scripts/check-fsd.mjs [корень]`; код выхода 1 при нарушениях.
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs'
import { dirname, join, relative, resolve, sep } from 'node:path'

import ts from 'typescript'

export const LAYERS = ['app', 'pages', 'widgets', 'features', 'entities', 'shared']
// В этих слоях нет слайсов: код разложен сразу по сегментам.
const SEGMENTED = new Set(['app', 'shared'])
const EXT = ['.ts', '.js', '.svelte', '.svelte.ts', '.svelte.js']

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      if (name === 'node_modules') continue
      walk(p, out)
    } else if (/\.(ts|js|svelte)$/.test(name) && !name.endsWith('.d.ts')) out.push(p)
  }
  return out
}

function importsOf(file) {
  let code = readFileSync(file, 'utf8')
  if (file.endsWith('.svelte')) {
    code = [...code.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]).join('\n')
  }
  const sf = ts.createSourceFile(file, code, ts.ScriptTarget.ESNext, true, ts.ScriptKind.TS)
  const found = []
  const visit = (n) => {
    if (
      (ts.isImportDeclaration(n) || ts.isExportDeclaration(n)) &&
      n.moduleSpecifier &&
      ts.isStringLiteral(n.moduleSpecifier)
    ) {
      found.push(n.moduleSpecifier.text)
    } else if (
      ts.isCallExpression(n) &&
      n.expression.kind === ts.SyntaxKind.ImportKeyword &&
      n.arguments[0] &&
      ts.isStringLiteral(n.arguments[0])
    ) {
      found.push(n.arguments[0].text)
    }
    ts.forEachChild(n, visit)
  }
  visit(sf)
  return found
}

function resolveSpec(spec, from, root) {
  let base
  if (spec.startsWith('@/')) base = join(root, spec.slice(2))
  else if (spec.startsWith('.')) base = resolve(dirname(from), spec)
  else return null // пакет
  const candidates = [base, ...EXT.map((e) => base + e), ...EXT.map((e) => join(base, 'index' + e))]
  return candidates.find((c) => existsSync(c) && statSync(c).isFile()) ?? base
}

/** Куда в слоистой структуре попадает файл: { layer, slice, segment, rest } либо null (вне слоёв). */
export function locate(file, root) {
  const parts = relative(root, file).split(sep)
  const layer = parts[0]
  if (!LAYERS.includes(layer) || parts.length < 2) return null
  if (SEGMENTED.has(layer)) return { layer, slice: null, segment: parts[1], rest: parts.slice(2) }
  return { layer, slice: parts[1], rest: parts.slice(2) }
}

export function checkFsd(root) {
  const violations = []
  if (!existsSync(root)) return violations
  for (const file of walk(root)) {
    const from = locate(file, root)
    if (!from) continue
    for (const spec of importsOf(file)) {
      const target = resolveSpec(spec, file, root)
      if (!target) continue
      const to = locate(target, root)
      if (!to) continue
      const at = `${relative(root, file)} → ${spec}`
      const fromIdx = LAYERS.indexOf(from.layer)
      const toIdx = LAYERS.indexOf(to.layer)
      if (toIdx < fromIdx) {
        violations.push(`${at}: слой «${from.layer}» не может импортировать вышележащий «${to.layer}»`)
        continue
      }
      if (from.layer === to.layer && !SEGMENTED.has(from.layer) && from.slice !== to.slice) {
        // кросс-импорт между сущностями допустим только через @x
        const viaX = to.layer === 'entities' && to.rest[0] === '@x'
        if (!viaX)
          violations.push(`${at}: слайсы слоя «${from.layer}» друг друга не знают (${from.slice} → ${to.slice})`)
        continue
      }
      const sameSlice = from.layer === to.layer && from.slice === to.slice
      if (!sameSlice && !SEGMENTED.has(to.layer)) {
        const isPublic = to.rest.length === 1 && /^index\.(ts|js)$/.test(to.rest[0])
        const viaX = to.layer === 'entities' && to.rest[0] === '@x'
        if (!isPublic && !viaX)
          violations.push(`${at}: в слайс «${to.layer}/${to.slice}» заходят только через index.ts`)
      }
    }
  }
  return violations
}

if (import.meta.main) {
  const root = resolve(process.argv[2] ?? new URL('../src', import.meta.url).pathname)
  const v = checkFsd(root)
  if (v.length) {
    console.error(v.join('\n') + `\n\nFSD: нарушений ${v.length}`)
    process.exit(1)
  }
  console.log('FSD: границы слоёв соблюдены')
}
