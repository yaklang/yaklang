package tests

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/kafka"
	"testing"
	"time"
)

func TestAgent(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Duration(10) * time.Second)
		cancelFunc()
	}()
	agent, err := kafka.NewScanAgent(uuid.NewString(), "127.0.0.1:9092", ctx)
	require.NoError(t, err)
	agent.Start()
	time.Sleep(time.Duration(3) * time.Second)
	agent.ShutDown()
}
