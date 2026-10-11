package asyncexecmanager

import "context"

type execution struct {
	summary   Summary
	resource  Resource
	inspector Inspector
	ctx       context.Context
	cancel    context.CancelCauseFunc
	done      chan struct{}
}
