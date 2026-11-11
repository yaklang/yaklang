package yakit

// HTTPFlowWriteObserver reports process-local write boundaries. A queued task
// can start before its producer observes the successful channel send.
type HTTPFlowWriteObserver func(stage HTTPFlowWriteStage, flowID uint, err error)

type HTTPFlowWriteStage uint8

const (
	HTTPFlowWriteSubmitting HTTPFlowWriteStage = iota + 1
	HTTPFlowWriteBlocked
	HTTPFlowWriteAccepted
	HTTPFlowWriteStarted
	HTTPFlowWriteFinished
)

func (o HTTPFlowWriteObserver) notify(stage HTTPFlowWriteStage, flowID uint, err error) {
	if o != nil {
		o(stage, flowID, err)
	}
}

func enqueueObservedHTTPFlowWrite(queue chan<- DbExecFunc, task DbExecFunc, observe HTTPFlowWriteObserver) {
	select {
	case queue <- task:
	default:
		observe.notify(HTTPFlowWriteBlocked, 0, nil)
		queue <- task
	}
	observe.notify(HTTPFlowWriteAccepted, 0, nil)
}
