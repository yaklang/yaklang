package cybertunnel

import (
	"testing"
	"time"
)

func TestIcmpTrigger(t *testing.T) {
	trigger, _ := NewICMPTrigger()

	go func() {
		for {
			time.Sleep(time.Second)
			res, err := trigger.GetICMPTriggerNotification(100)
			if err != nil {
				continue
			}
			res.Show()
		}
	}()

	err := trigger.Run()
	if err != nil {
		t.Error(err)
		return
	}
}
