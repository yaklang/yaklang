package yakgrpc

import (
	"context"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_ChannelControlTest(t *testing.T) {
	errC := make(chan error, 1)
	close(errC)

	if utils.TryWriteChannel(errC, utils.Error("test")) {
		t.FailNow()
	}
}

func TestGRPCMUSTPASS_HybridScan_PROGRESS(t *testing.T) {
	id := uuid.NewV4()
	manager, err := CreateHybridTask(id.String(), context.Background())
	if err != nil {
		t.Error(err)
		return
	}

	val := 0
	go func() {
		for {
			log.Infof("CURRENT TASK: %v", val)
			time.Sleep(time.Second)
			select {
			case <-manager.Context().Done():
				log.Info("DONE")
				return
			default:

			}
		}
	}()
	go func() {
		time.Sleep(2*time.Second + 300*time.Millisecond)
		manager.Pause()
		log.Infof("VAL: %v", val)
		time.Sleep(time.Second * 6)
		log.Infof("VAL: %v", val)
		manager.Resume()
		log.Infof("VAL: %v", val)

		time.Sleep(time.Second)
		log.Infof("VAL: %v", val)

		time.Sleep(time.Second)
		log.Infof("VAL: %v", val)

		time.Sleep(time.Second)
		log.Infof("VAL: %v", val)

		manager.Stop()
		log.Infof("VAL: %v", val)

	}()

	swg := utils.NewSizedWaitGroup(10)
	for i := 0; i < 1000; i++ {
		manager.Checkpoint()
		if err := swg.AddWithContext(manager.Context()); err != nil {
			return
		}
		val = i
		go func() {
			defer func() {
				swg.Done()
			}()
			time.Sleep(time.Second * 2)
		}()
	}
	swg.Wait()

	if val != 40 {
		t.Error("val > 50, process control failed")
		t.Failed()
	}
}
