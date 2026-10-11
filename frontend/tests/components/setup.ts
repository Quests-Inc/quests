// Среда компонентных тестов: DOM от happy-dom и компилятор Svelte для `.svelte` и `.svelte.ts`.
// Запуск: `bun test --conditions browser tests/components` (условие browser выбирает клиентскую сборку svelte).
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'

import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { plugin } from 'bun'
import { compile, compileModule } from 'svelte/compiler'

GlobalRegistrator.register({ url: 'http://localhost/' })

// Анимации Web Animations в happy-dom отклоняют `finished` при отмене — Svelte это не ловит и тест падает на «необработанной» ошибке.
// Подменяем на мгновенную анимацию: onfinish вызывается сразу после установки, отмена безвредна.
Element.prototype.animate = function () {
  const anim: any = {
    currentTime: 0,
    playState: 'finished',
    finished: Promise.resolve(),
    ready: Promise.resolve(),
    cancel() {},
    finish() {},
    play() {},
    pause() {},
    reverse() {},
    addEventListener() {},
    removeEventListener() {},
    _onfinish: null as null | (() => void),
    get onfinish() {
      return this._onfinish
    },
    set onfinish(fn: null | (() => void)) {
      this._onfinish = fn
      if (fn) queueMicrotask(() => fn.call(anim))
    },
  }
  return anim
} as typeof Element.prototype.animate

const ts = new Bun.Transpiler({ loader: 'ts' })

// Vite отдаёт `import x from './a.svg?raw'` строкой; Bun запрос `?raw` не знает, поэтому вклеиваем файл сами.
function inlineRaw(source: string, from: string): string {
  return source.replace(/import (\w+) from '([^']+)\?raw'/g, (_m, name: string, spec: string) => {
    const file = spec.startsWith('@/')
      ? resolve(import.meta.dir, '../../src', spec.slice(2))
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
