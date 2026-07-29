package yakit

import (
	"testing"
	"time"
)

func receiveCallback(t *testing.T, callbacks <-chan string, want string) {
	t.Helper()
	select {
	case got := <-callbacks:
		if got != want {
			t.Fatalf("unexpected callback: got %q, want %q", got, want)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for callback %q", want)
	}
}

func TestHTTPFlowBroadcastThrottleKeepsLatestTrailingWakeup(t *testing.T) {
	callbacks := make(chan string, 4)
	caller := newBroadcastTypeCaller(ServerPushType_HttpFlow, 20*time.Millisecond)

	caller(func() { callbacks <- "leading" })
	caller(func() { callbacks <- "superseded" })
	caller(func() { callbacks <- "trailing" })

	receiveCallback(t, callbacks, "leading")
	receiveCallback(t, callbacks, "trailing")
}

func TestOtherBroadcastTypesRemainLeadingOnly(t *testing.T) {
	callbacks := make(chan string, 2)
	caller := newBroadcastTypeCaller(ServerPushType_Risk, 20*time.Millisecond)

	caller(func() { callbacks <- "leading" })
	caller(func() { callbacks <- "unexpected-trailing" })
	receiveCallback(t, callbacks, "leading")

	select {
	case got := <-callbacks:
		t.Fatalf("unexpected trailing callback: %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}
