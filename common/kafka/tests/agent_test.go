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
		time.Sleep(time.Duration(5) * time.Second)
	}()
	manager := kafka.NewManager(uuid.NewString(), "127.0.0.1:9092")
	err := manager.Start(ctx)
	require.NoError(t, err)
}
