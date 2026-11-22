// Text of a private note the person chose to see. Lives while this map page is open, then the page clears it.

let bodies = $state<Record<string, string>>({})

export function revealedBody(key: string) {
  return bodies[key]
}

export function revealedEntries(): [string, string][] {
  return Object.entries(bodies)
}

export function rememberRevealed(key: string, text: string) {
  bodies[key] = text
}

export function clearRevealedNotes() {
  bodies = {}
}
