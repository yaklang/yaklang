package yakit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestHTTPFlowWriteObserverReportsActualFullQueue(t *testing.T) {
	queue := make(chan DbExecFunc, 1)
	write := func(*gorm.DB) error { return nil }
	queue <- write
	stages := make(chan HTTPFlowWriteStage, 2)
	go enqueueObservedHTTPFlowWrite(queue, write, func(stage HTTPFlowWriteStage, _ uint, _ error) {
		stages <- stage
	})
	select {
	case stage := <-stages:
		require.Equal(t, HTTPFlowWriteBlocked, stage)
	case <-time.After(time.Second):
		t.Fatal("write was not reported blocked")
	}
	require.Empty(t, stages)
	<-queue
	select {
	case stage := <-stages:
		require.Equal(t, HTTPFlowWriteAccepted, stage)
	case <-time.After(time.Second):
		t.Fatal("write was not reported accepted")
	}
	require.Len(t, queue, 1)
}
