package kafka

import "context"

type Processor interface {
	Process(context.Context, *TaskRequestMessage)
	Type() TaskType
	Init(ctx context.Context, config *TaskConfig)
}
