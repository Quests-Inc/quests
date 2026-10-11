package store

import "time"

// Server-side lifetimes. They live apart from the types so that the generated TypeScript types of
// the API (go/tygo.yaml) can leave them out: a time.Duration expression has no TypeScript form.
const (
	// RegistrationTTL is how long a fresh registration token stays valid.
	RegistrationTTL = 24 * time.Hour
)
