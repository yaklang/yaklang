package kafka

import "context"

type Processor interface {
	Process(context.Context, *TaskRequestMessage, *TaskConfig)
}
