// Вспомогательное для тестов: корень эффектов Svelte, чтобы пробовать контроллеры с `$effect` без компонента.
export function inRoot(fn: () => void): () => void {
  return $effect.root(fn)
}
