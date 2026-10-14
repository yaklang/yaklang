package asyncexecmanager

import "context"

type execution struct {
	adopted   bool // Execute joins work that already exists, even after cancellation.
	summary   Summary
	resource  Resource
	inspector Inspector
	ctx       context.Context
	cancel    context.CancelCauseFunc
	done      chan struct{}
}
