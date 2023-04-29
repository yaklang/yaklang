package healthinfo

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

//
//func TestGenerateHealthInfo(t *testing.T) {
//	for {
//		select {
//		case <-time.Tick(1 * time.Second):
//			manager := NewHealthInfo()
//			log.Info(spew.Sdump(manager))
//		case <-time.After(10 * time.Second):
//			return
//		}
//	}
//}

func TestNewHealthInfoManager(t *testing.T) {
	manager, err := NewHealthInfoManager(1*time.Second, 10*time.Minute)
	assert.Nil(t, err)
	_ = manager

	after := time.After(5 * time.Second)
LOOP:
	for {
		select {
		case <-after:
			break LOOP
		}
	}
	assert.Greater(t, len(manager.GetHealthInfos()), 0)
}
