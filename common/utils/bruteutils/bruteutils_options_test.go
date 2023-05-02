package bruteutils

import (
	"sync/atomic"
	"testing"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mixer"
)

func TestNewMultiTargetBruteUtilEx_WithTargetsConcurrentOption(t *testing.T) {
	log.SetLevel(log.TraceLevel)

	targets := utils.ParseStringToHosts("1.1.1.1/24") // 256
	users := []string{"user"}
	passes := []string{"pass"}

	dw, err := WithDelayerWaiter(0, 0)
	if err != nil {
		t.Logf("build delayer waiter failed: %s", err)
		t.FailNow()
	}

	bu, err := NewMultiTargetBruteUtilEx(
		WithTargetsConcurrent(150),
		dw,
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			time.Sleep(time.Second)
			return &BruteItemResult{
				Ok: true,
			}
		}),
	)
	if err != nil {
		t.Logf("build brute utils failed: %s", err)
		t.FailNow()
	}

	mx, err := mixer.NewMixer(targets, users, passes)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	for {
		results := mx.Value()
		target, user, pass := results[0], results[1], results[2]

		bu.Feed(&BruteItem{"", target, user, pass})

		err := mx.Next()
		if err != nil {
			break
		}
	}

	start := time.Now()
	err = bu.Run()
	if err != nil {
		t.Logf("run brute failed: %s", err)
		t.FailNow()
	}
	end := time.Now()

	interval := end.Sub(start)
	if interval > 3*time.Second || interval < 2*time.Second {
		t.Logf("brute timeout failed: %#v", interval.Seconds())
		t.FailNow()
	}
}

func TestNewMultiTargetBruteUtilEx_WithTargetTasksConcurrentOption(t *testing.T) {
	targets := utils.ParseStringToHosts("1.1.1.1/24") // 256
	users := []string{"user", "user2"}
	passes := []string{"pass"}

	dw, err := WithDelayerWaiter(0, 0)
	if err != nil {
		t.Logf("build delayer waiter failed: %s", err)
		t.FailNow()
	}

	bu, err := NewMultiTargetBruteUtilEx(
		WithTargetTasksConcurrent(2),
		WithTargetsConcurrent(150),
		dw,
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			time.Sleep(time.Second)
			log.Infof("buring %s:%s@%s", item.Username, item.Password, item.Target)
			return &BruteItemResult{
				Ok: true,
			}
		}),
	)
	if err != nil {
		t.Logf("build brute utils failed: %s", err)
		t.FailNow()
	}

	mx, err := mixer.NewMixer(targets, users, passes)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	for {
		results := mx.Value()
		target, user, pass := results[0], results[1], results[2]

		bu.Feed(&BruteItem{"", target, user, pass})

		err := mx.Next()
		if err != nil {
			break
		}
	}

	start := time.Now()
	err = bu.Run()
	if err != nil {
		t.Logf("run brute failed: %s", err)
		t.FailNow()
	}
	end := time.Now()

	interval := end.Sub(start)
	if interval > 3*time.Second || interval < 2*time.Second {
		t.Logf("brute timeout")
		t.FailNow()
	}
}

func TestNewMultiTargetBruteUtilEx_WithOkToStop(t *testing.T) {
	targets := []string{"user"}
	users := utils.ParseStringToHosts("1.1.1.1/24") // 256
	passes := []string{"pass"}

	dw, err := WithDelayerWaiter(0, 0)
	if err != nil {
		t.Logf("build delayer waiter failed: %s", err)
		t.FailNow()
	}

	var count int32 = 0
	bu, err := NewMultiTargetBruteUtilEx(
		WithOkToStop(true),
		WithTargetsConcurrent(150),
		dw,
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			log.Infof("bruting %s:%s@%s", item.Username, item.Password, item.Target)
			atomic.AddInt32(&count, 1)
			return &BruteItemResult{
				Ok: true,
			}
		}),
	)
	if err != nil {
		t.Logf("build brute utils failed: %s", err)
		t.FailNow()
	}

	mx, err := mixer.NewMixer(targets, users, passes)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	for {
		results := mx.Value()
		target, user, pass := results[0], results[1], results[2]

		bu.Feed(&BruteItem{"", target, user, pass})

		err := mx.Next()
		if err != nil {
			break
		}
	}

	err = bu.Run()
	if err != nil {
		t.Logf("run brute failed: %s", err)
		t.FailNow()
	}

	if int(atomic.LoadInt32(&count)) > 3 || int(atomic.LoadInt32(&count)) < 1 {
		t.Logf("Ok to Stop is invalid: %v", int(atomic.LoadInt32(&count)))
		t.FailNow()
	}

}

func TestNewMultiTargetBruteUtilEx_WithFinishingThreshold(t *testing.T) {
	targets := []string{"user"}
	users := utils.ParseStringToHosts("1.1.1.1/24") // 256
	passes := []string{"pass"}

	dw, err := WithDelayerWaiter(0, 0)
	if err != nil {
		t.Logf("build delayer waiter failed: %s", err)
		t.FailNow()
	}

	var count int32 = 0
	bu, err := NewMultiTargetBruteUtilEx(
		WithOkToStop(true),
		WithFinishingThreshold(10),
		WithTargetsConcurrent(150),
		dw,
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			log.Infof("bruting %s:%s@%s", item.Username, item.Password, item.Target)
			atomic.AddInt32(&count, 1)
			return &BruteItemResult{
				Ok:       false,
				Finished: true,
			}
		}),
	)
	if err != nil {
		t.Logf("build brute utils failed: %s", err)
		t.FailNow()
	}

	mx, err := mixer.NewMixer(targets, users, passes)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	for {
		results := mx.Value()
		target, user, pass := results[0], results[1], results[2]

		bu.Feed(&BruteItem{"", target, user, pass})

		err := mx.Next()
		if err != nil {
			break
		}
	}

	err = bu.Run()
	if err != nil {
		t.Logf("run brute failed: %s", err)
		t.FailNow()
	}

	if int(atomic.LoadInt32(&count)) > 12 || int(atomic.LoadInt32(&count)) < 9 {
		t.Logf("FinishingThreshold is invalid: %v", int(atomic.LoadInt32(&count)))
		t.FailNow()
	}

}

func TestNewMultiTargetBruteUtilEx_WithOnlyNeedPassword(t *testing.T) {
	targets := []string{"user"}
	users := utils.ParseStringToHosts("1.1.1.1/24") // 256
	passes := []string{"pass", "pass1", "pass2"}

	dw, err := WithDelayerWaiter(0, 0)
	if err != nil {
		t.Logf("build delayer waiter failed: %s", err)
		t.FailNow()
	}

	var count int32 = 0
	bu, err := NewMultiTargetBruteUtilEx(
		WithOkToStop(true),
		WithFinishingThreshold(10),
		WithOnlyNeedPassword(true),
		WithTargetsConcurrent(150),
		dw,
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			log.Infof("bruting %s:%s@%s", item.Username, item.Password, item.Target)
			atomic.AddInt32(&count, 1)
			return &BruteItemResult{
				Ok:       false,
				Finished: true,
			}
		}),
	)
	if err != nil {
		t.Logf("build brute utils failed: %s", err)
		t.FailNow()
	}

	mx, err := mixer.NewMixer(targets, users, passes)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	for {
		results := mx.Value()
		target, user, pass := results[0], results[1], results[2]

		bu.Feed(&BruteItem{"", target, user, pass})

		err := mx.Next()
		if err != nil {
			break
		}
	}

	err = bu.Run()
	if err != nil {
		t.Logf("run brute failed: %s", err)
		t.FailNow()
	}

	if int(atomic.LoadInt32(&count)) != 3 {
		t.Logf("FinishingThreshold is invalid: %v", int(atomic.LoadInt32(&count)))
		t.FailNow()
	}

}

func TestNewMultiTargetBruteUtilEx_EliminatedUser(t *testing.T) {
	targets := []string{"user"}
	users := []string{"user1", "user2"}
	passes := []string{"pass", "pass1", "pass2"}

	dw, err := WithDelayerWaiter(0, 0)
	if err != nil {
		t.Logf("build delayer waiter failed: %s", err)
		t.FailNow()
	}

	var count int32 = 0
	bu, err := NewMultiTargetBruteUtilEx(
		WithOkToStop(true),
		WithFinishingThreshold(10),
		WithTargetsConcurrent(150),
		dw,
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			log.Infof("bruting %s:%s@%s", item.Username, item.Password, item.Target)
			atomic.AddInt32(&count, 1)

			if item.Username == "user1" {
				return &BruteItemResult{
					Ok:             false,
					Finished:       true,
					UserEliminated: true,
				}
			}

			return &BruteItemResult{
				Ok:       false,
				Finished: true,
			}
		}),
	)
	if err != nil {
		t.Logf("build brute utils failed: %s", err)
		t.FailNow()
	}

	mx, err := mixer.NewMixer(targets, users, passes)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	for {
		results := mx.Value()
		target, user, pass := results[0], results[1], results[2]

		bu.Feed(&BruteItem{"", target, user, pass})

		err := mx.Next()
		if err != nil {
			break
		}
	}

	err = bu.Run()
	if err != nil {
		t.Logf("run brute failed: %s", err)
		t.FailNow()
	}

	if int(atomic.LoadInt32(&count)) != 4 {
		t.Logf("FinishingThreshold is invalid: %v", int(atomic.LoadInt32(&count)))
		t.FailNow()
	}

}

func TestNewMultiTargetBruteUtilEx_WithBeforeBruteCallback(t *testing.T) {
	log.SetLevel(log.TraceLevel)

	targets := []string{"target1", "target2"}
	users := []string{"user1", "user2"}
	passes := []string{"pass", "pass1", "pass2"}

	dw, err := WithDelayerWaiter(0, 0)
	if err != nil {
		t.Logf("build delayer waiter failed: %s", err)
		t.FailNow()
	}

	var count int32 = 0
	bu, err := NewMultiTargetBruteUtilEx(
		WithOkToStop(true),
		WithFinishingThreshold(10),
		WithTargetsConcurrent(150),
		dw,
		WithBeforeBruteCallback(func(s string) bool {
			return s == "target1"
		}),
		WithBruteCallback(func(item *BruteItem) *BruteItemResult {
			log.Infof("bruting %s:%s@%s", item.Username, item.Password, item.Target)
			atomic.AddInt32(&count, 1)

			if item.Username == "user1" {
				return &BruteItemResult{
					Ok:             false,
					Finished:       true,
					UserEliminated: true,
				}
			}

			return &BruteItemResult{
				Ok:       false,
				Finished: true,
			}
		}),
	)
	if err != nil {
		t.Logf("build brute utils failed: %s", err)
		t.FailNow()
	}

	mx, err := mixer.NewMixer(targets, users, passes)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	for {
		results := mx.Value()
		target, user, pass := results[0], results[1], results[2]

		bu.Feed(&BruteItem{"", target, user, pass})

		err := mx.Next()
		if err != nil {
			break
		}
	}

	err = bu.Run()
	if err != nil {
		t.Logf("run brute failed: %s", err)
		t.FailNow()
	}

	if int(atomic.LoadInt32(&count)) != 4 {
		t.Logf("FinishingThreshold is invalid: %v", int(atomic.LoadInt32(&count)))
		t.FailNow()
	}

}
