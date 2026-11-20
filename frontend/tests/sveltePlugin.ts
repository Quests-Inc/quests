// Bun не компилирует руны сам. Тот же приём, что у компонентных тестов: `.svelte` и `.svelte.ts` → клиентский код.
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'

import { plugin } from 'bun'
import { compile, compileModule } from 'svelte/compiler'

const ts = new Bun.Transpiler({ loader: 'ts' })

function inlineRaw(source: string, from: string): string {
  return source.replace(/import (\w+) from '([^']+)\?raw'/g, (_m, name: string, spec: string) => {
    const file = spec.startsWith('@/')
      ? resolve(import.meta.dir, '../src', spec.slice(2))
      : resolve(dirname(from), spec)
    return `const ${name} = ${JSON.stringify(readFileSync(file, 'utf8'))}`
  })
}

plugin({
  name: 'svelte-client',
  setup(build) {
    build.onLoad({ filter: /\.svelte$/ }, async ({ path }) => {
      const source = inlineRaw(await Bun.file(path).text(), path)
      const { js } = compile(source, { filename: path, generate: 'client', css: 'injected', dev: false })
      return { contents: js.code, loader: 'js' }
    })
    build.onLoad({ filter: /\.svelte\.(ts|js)$/ }, async ({ path }) => {
      const source = await Bun.file(path).text()
      const plain = path.endsWith('.ts') ? ts.transformSync(source) : source
      const { js } = compileModule(plain, { filename: path, generate: 'client', dev: false })
      return { contents: js.code, loader: 'js' }
    })
  },
})
