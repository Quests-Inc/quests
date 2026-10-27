export type TextToken = { kind: 'script' | 'value'; query: string; start: number; end: number }
export function textCompletionToken(text: string, caret: number): TextToken | null {
  const before = text.slice(0, caret)
  const script = /\bscript=([^\s{}]*)$/.exec(before)
  if (script) return { kind: 'script', query: script[1], start: caret - script[0].length, end: caret }
  const value = /\{\{([a-z0-9_]*)$/.exec(before)
  return value ? { kind: 'value', query: value[1], start: caret - value[0].length, end: caret } : null
}
export function completeText(text: string, token: TextToken, insertion: string) {
  let end = token.end
  if (token.kind === 'value' && text.slice(end, end + 2) === '}}') end += 2
  return { text: text.slice(0, token.start) + insertion + text.slice(end), caret: token.start + insertion.length }
}
export function scriptInsertion(
  id: number,
  arg: string,
  flags: { auto: boolean; hide: boolean; set: string; on: string }
) {
  if (!Number.isInteger(id) || id < 1 || !arg.trim() || arg.length > 200 || /[{}[\]\n]/.test(arg))
    throw new Error('Укажите скрипт и выражение до 200 символов без скобок {} и [].')
  if (flags.set && !/^[a-z][a-z0-9_]{0,23}$/.test(flags.set))
    throw new Error('Имя переменной: строчные латинские, цифры и _, до 24 символов.')
  if (flags.on && !/^[a-z0-9][a-z0-9_-]{0,31}(,[a-z0-9][a-z0-9_-]{0,31}){0,7}$/.test(flags.on))
    throw new Error('Укажите метку раннера или несколько через запятую.')
  const query = [
    flags.auto && !flags.set && !flags.on && 'auto',
    flags.hide && 'hide',
    flags.set && `set=${flags.set}`,
    flags.on && `on=${flags.on}`,
  ]
    .filter(Boolean)
    .join('&')
  return `script=${id}{${arg}}${query ? '?' + query : ''}`
}
