package utils

import (
	"github.com/yaklang/yaklang/common/log"
	"sync/atomic"
	"testing"
	"time"
)

func TestCooldownFetcher(t *testing.T) {
	fetcher := NewCoolDownFetcher(10 * time.Second)
	count := int64(0)
	swg := NewSizedWaitGroup(200)
	for i := 0; i < 100; i++ {
		swg.Add()
		go func() {
			defer func() { swg.Done() }()
			fetcher.Fetch(func() (any, error) {
				log.Info("fetch ...")
				atomic.AddInt64(&count, 1)
				log.Info("fetch finished")
				return 1, nil
			})
		}()
	}
	swg.Wait()
	if count != 1 {
		t.Errorf("count should be 1, not: %v", count)
	}
}

func TestCooldownFetcher_2(t *testing.T) {
	fetcher := NewCoolDownFetcher(1 * time.Second)
	count := int64(0)
	swg := NewSizedWaitGroup(200)
	for i := 0; i < 100; i++ {
		swg.Add()
		go func() {
			defer func() { swg.Done() }()
			fetcher.Fetch(func() (any, error) {
				log.Info("fetch ...")
				atomic.AddInt64(&count, 1)
				log.Info("fetch finished")
				return 1, nil
			})
		}()
	}
	swg.Wait()

	if count != 1 {
		t.Errorf("count should be 1, not: %v", count)
	}

	swg = NewSizedWaitGroup(200)
	for i := 0; i < 100; i++ {
		swg.Add()
		go func() {
			defer func() { swg.Done() }()
			fetcher.Fetch(func() (any, error) {
				log.Info("fetch ...")
				atomic.AddInt64(&count, 1)
				log.Info("fetch finished")
				return 1, nil
			})
		}()
	}
	swg.Wait()
	if count != 1 {
		t.Errorf("count should be 1, not: %v", count)
	}

	time.Sleep(1500 * time.Millisecond)
	swg = NewSizedWaitGroup(200)
	for i := 0; i < 100; i++ {
		swg.Add()
		go func() {
			defer func() { swg.Done() }()
			fetcher.Fetch(func() (any, error) {
				log.Info("fetch ...")
				atomic.AddInt64(&count, 1)
				log.Info("fetch finished")
				return 1, nil
			})
		}()
	}
	swg.Wait()
	if count != 2 {
		t.Errorf("count should be 1, not: %v", count)
	}
}
