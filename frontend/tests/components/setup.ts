// Среда компонентных тестов: DOM от happy-dom и компилятор Svelte для `.svelte` и `.svelte.ts`.
// Запуск: `bun test --conditions browser tests/components` (условие browser выбирает клиентскую сборку svelte).
import { GlobalRegistrator } from '@happy-dom/global-registrator'

import './../sveltePlugin.ts'

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
