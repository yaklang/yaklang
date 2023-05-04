package bruteutils

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mixer"
	"testing"
	"time"
)

func TestBruteUtils(t *testing.T) {
	Targets := utils.ParseStringToHosts("4.1.1.1/24")
	Usernames := utils.ParseStringToHosts("1.1.1.1/30")
	Passwords := utils.ParseStringToHosts("234.2.2.2/30")

	m, err := mixer.NewMixer(Targets, Usernames, Passwords)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	bu, err := NewMultiTargetBruteUtil(
		200, 0, 0, func(item *BruteItem) *BruteItemResult {
			log.Infof("bruting %s %s %s", item.Target, item.Username, item.Password)
			return &BruteItemResult{
				Ok: true,
			}
		})
	if err != nil {
		t.FailNow()
	}

	log.Info("start to feed data")

	for {
		results := m.Value()
		if len(results) != 3 {
			err := m.Next()
			if err != nil {
				break
			}
		}

		bu.Feed(&BruteItem{
			Target:   results[0],
			Username: results[1],
			Password: results[2],
		})

		err := m.Next()
		if err != nil {
			break
		}
	}

	log.Info("feed finished")

	log.Info("start to run")

	if err := bu.Run(); err != nil {
		t.FailNow()
	}
}

func TestNewMultiTargetBruteUtilWithContext(t *testing.T) {
	Targets := utils.ParseStringToHosts("4.1.1.1/30")
	Usernames := utils.ParseStringToHosts("1.1.1.1/30")
	Passwords := utils.ParseStringToHosts("234.2.2.2/30")

	m, err := mixer.NewMixer(Targets, Usernames, Passwords)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	ctx, _ := context.WithTimeout(context.Background(), 2*time.Second)
	bu, err := NewMultiTargetBruteUtil(
		200, 0, 0, func(item *BruteItem) *BruteItemResult {
			return &BruteItemResult{
				Ok: true,
			}
		})
	if err != nil {
		t.FailNow()
	}

	log.Info("start to feed data")

	for {
		results := m.Value()
		if len(results) != 3 {
			err := m.Next()
			if err != nil {
				break
			}
		}

		bu.Feed(&BruteItem{
			Target:   results[0],
			Username: results[1],
			Password: results[2],
		})

		err := m.Next()
		if err != nil {
			break
		}
	}

	log.Info("feed finished")
	log.Info("start to run")

	_ = bu.RunWithContext(ctx)
}

func TestNewMultiTargetBruteUtilWithContext_Tomcat(t *testing.T) {
	Targets := utils.ParseStringToHosts("4.1.1.1/30")
	Usernames := utils.ParseStringToHosts("1.1.1.1/30")
	Passwords := utils.ParseStringToHosts("234.2.2.2/30")

	m, err := mixer.NewMixer(Targets, Usernames, Passwords)
	if err != nil {
		t.Logf("build mixer failed: %s", err)
		t.FailNow()
	}

	ctx, _ := context.WithTimeout(context.Background(), 2*time.Second)
	bu, err := NewMultiTargetBruteUtil(
		200, 0, 0, func(item *BruteItem) *BruteItemResult {
			return &BruteItemResult{
				Ok: true,
			}
		})
	if err != nil {
		t.FailNow()
	}

	log.Info("start to feed data")

	for {
		results := m.Value()
		if len(results) != 3 {
			err := m.Next()
			if err != nil {
				break
			}
		}

		bu.Feed(&BruteItem{
			Target:   results[0],
			Username: results[1],
			Password: results[2],
		})

		err := m.Next()
		if err != nil {
			break
		}
	}

	log.Info("feed finished")
	log.Info("start to run")

	_ = bu.RunWithContext(ctx)
}
