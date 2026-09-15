package store

import "context"

// Pinger is implemented by persistent stores for readiness probes.
type Pinger interface {
	Ping(context.Context) error
}
