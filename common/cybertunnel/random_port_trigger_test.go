package cybertunnel

import (
	"testing"
	"time"
)

func TestRandomTrigger(t *testing.T) {
	trigger, err := NewRandomPortTrigger()
	if err != nil {
		t.Error(err)
		return
	}

	go func() {
		for {
			time.Sleep(time.Second)
			res, err := trigger.GetTriggerNotification(64334)
			if err != nil {
				continue
			}

			res.Show()
		}
	}()

	err = trigger.Run()
	if err != nil {
		t.Error(err)
		return
	}
}
