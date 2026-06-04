package browser

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubSessionTracker struct {
	mu  sync.Mutex
	ids []string
}

func (s *stubSessionTracker) TrackBrowserSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids = append(s.ids, id)
}

func TestWrapBrowserOpen_TracksInstanceID(t *testing.T) {
	defer CloseAll()

	tracker := &stubSessionTracker{}
	origin := Open
	wrappedAny := wrapBrowserOpen(tracker)(origin)
	wrapped, ok := wrappedAny.(func(...BrowserOption) (*BrowserInstance, error))
	require.True(t, ok)

	_, err := wrapped(WithID("hook-test-session"), WithHeadless(true), WithTimeout(5))
	require.NoError(t, err)

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	require.Contains(t, tracker.ids, "hook-test-session")
}

func TestConfigIDFromOptions(t *testing.T) {
	require.Equal(t, "my-session", ConfigIDFromOptions(WithID("my-session")))
	require.Equal(t, defaultBrowserID, ConfigIDFromOptions())
}
