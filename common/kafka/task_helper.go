package kafka

import "context"

type Process func(ctx context.Context, message *TaskRequestMessage, config *TaskConfig)

func PortScanProcess(ctx context.Context, message *TaskRequestMessage, config *TaskConfig) {

}
