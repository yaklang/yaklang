package asyncexecmanager

import (
	"context"
	"fmt"
)

// Inspector reads authoritative observations without consuming them. It must
// honor the observation context, provide concurrency-safe snapshots and remain
// usable after Execute ends. It must not expose mutable resource-owned buffers.
type Inspector interface {
	Inspect(ctx context.Context) (Observation, error)
}

func inspect(ctx context.Context, observer Inspector) (observation Observation, err error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("inspect execution: %v", value)
		}
	}()
	return observer.Inspect(ctx)
}
