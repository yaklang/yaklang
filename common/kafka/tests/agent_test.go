package tests

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/kafka"
	"github.com/yaklang/yaklang/common/log"
	"testing"
	"time"
)

func TestAgent(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
	//go func() {
	//	time.Sleep(time.Duration(10) * time.Second)
	//	cancelFunc()
	//}()
	manager := kafka.NewManager(uuid.NewString(), "8.155.8.3:9092")
	go func() {
		time.Sleep(time.Duration(3) * time.Second)
		manager.Finish()
		log.Info("context done")
	}()
	err := manager.Start(ctx)
	require.NoError(t, err)
}
