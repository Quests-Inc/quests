package automation

import "time"

// RegistrationTTL is how long a fresh runner registration token stays valid. (Kept apart from the types so the
// generated TypeScript types of the API can leave it out: a time.Duration expression has no TypeScript form.)
const RegistrationTTL = 24 * time.Hour
