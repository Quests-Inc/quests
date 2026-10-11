package knowledge

import "time"

// ReadDedupe is the window in which repeated opens of a note count once. (Kept apart from the types so the generated
// TypeScript types of the API can leave it out: a time.Duration expression has no TypeScript form.)
const ReadDedupe = 10 * time.Minute
