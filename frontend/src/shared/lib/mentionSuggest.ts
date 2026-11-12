/** `@partial` token ending at `caret` in `text`, or null if caret isn't inside one. */
export function activeMentionToken(text: any, caret: number) {
  const upto = text.slice(0, caret)
  const m = upto.match(/@([^\s@]*)$/)
  if (!m) return null
  return { query: m[1], start: caret - m[0].length }
}

/** Replace the `@query` token with `kind=id ` (space after, cursor lands there). */
export function applyMention(text: any, token: { start: number; query: any }, item: { kind: any; id: any }) {
  const tagEnd = token.start + 1 + token.query.length
  const before = text.slice(0, token.start)
  const after = text.slice(tagEnd)
  const tag = `${item.kind}=${item.id}`
  return { text: `${before}${tag} ${after}`, caret: before.length + tag.length + 1 }
}
