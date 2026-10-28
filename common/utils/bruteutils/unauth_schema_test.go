package bruteutils

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetBruteHandlerChecksUnauthOncePerTarget(t *testing.T) {
	var unauthChecks, bruteChecks int32
	service := &DefaultServiceAuthInfo{
		UnAuthVerify: func(item *BruteItem) *BruteItemResult {
			atomic.AddInt32(&unauthChecks, 1)
			result := item.Result()
			result.Ok = true
			return result
		},
		BrutePass: func(item *BruteItem) *BruteItemResult {
			atomic.AddInt32(&bruteChecks, 1)
			return item.Result()
		},
	}
	handler := service.GetBruteHandler()
	const attempts = 32
	results := make(chan *BruteItemResult, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- handler(&BruteItem{Target: "127.0.0.1:2080", Username: "admin", Password: "admin"})
		}()
	}
	wg.Wait()
	close(results)
	if got := atomic.LoadInt32(&unauthChecks); got != 1 {
		t.Fatalf("unauthenticated checks = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&bruteChecks); got != attempts-1 {
		t.Fatalf("credential checks = %d, want %d", got, attempts-1)
	}
	for result := range results {
		if result.Ok && (result.Username != "" || result.Password != "") {
			t.Fatal("unauthenticated success must have empty credentials")
		}
	}
}

func TestGetBruteHandlerRechecksUnauthInNewHandler(t *testing.T) {
	for _, firstOpen := range []bool{false, true} {
		name := "previously-authenticated"
		if firstOpen {
			name = "previously-open"
		}
		t.Run(name, func(t *testing.T) {
			checks := 0
			open := firstOpen
			service := &DefaultServiceAuthInfo{
				UnAuthVerify: func(item *BruteItem) *BruteItemResult {
					checks++
					result := item.Result()
					result.Ok = open
					return result
				},
				BrutePass: func(item *BruteItem) *BruteItemResult { return item.Result() },
			}
			for scan := 0; scan < 2; scan++ {
				if scan > 0 {
					open = true
				}
				handler := service.GetBruteHandler()
				result := handler(&BruteItem{Target: "127.0.0.1:2080", Username: "admin", Password: "admin"})
				if result.Ok != open {
					t.Fatalf("scan %d: success = %v, want %v", scan, result.Ok, open)
				}
				if result.Ok && (result.Username != "" || result.Password != "") {
					t.Fatalf("scan %d: unauthenticated success must have empty credentials", scan)
				}
			}
			if checks != 2 {
				t.Fatalf("unauthenticated checks = %d, want 2", checks)
			}
		})
	}
}

func TestGetBruteHandlerChecksDifferentTargetsConcurrently(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	defer close(release)
	service := &DefaultServiceAuthInfo{
		UnAuthVerify: func(item *BruteItem) *BruteItemResult {
			started <- item.Target
			<-release
			return item.Result()
		},
	}
	handler := service.GetBruteHandler()
	go handler(&BruteItem{Target: "127.0.0.1:2080"})
	go handler(&BruteItem{Target: "127.0.0.1:2081"})
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-timer.C:
			t.Fatal("different targets must start before either probe finishes")
		}
	}
}
