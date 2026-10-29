// Вспомогательное для тестов: корень эффектов Svelte, чтобы пробовать контроллеры с `$effect` без компонента.
export function inRoot(fn: () => void): () => void {
  return $effect.root(fn)
}

/** Реактивная ячейка для подмены входов контроллера в тестах. */
export function cell<T>(initial: T) {
  let v = $state(initial)
  return {
    get value() {
      return v
    },
    set value(x: T) {
      v = x
    },
  }
}

export function watch(fn: () => void) {
  $effect(fn)
}
