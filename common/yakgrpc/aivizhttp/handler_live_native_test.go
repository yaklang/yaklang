package aivizhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
)

func TestLiveSessionsIncludesNativeCoordinatorUntilActualExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started := make(chan struct{})
	var once sync.Once
	sid := "live-native-" + uuid.NewString()
	session, err := coordinator.NewSession(ctx, "prepare a plan", aicommon.WithPersistentSessionId(sid),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true),
		aicommon.WithNoOpMemoryTriage(), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			once.Do(func() { close(started) })
			<-c.GetContext().Done()
			return nil, c.GetContext().Err()
		}))
	require.NoError(t, err)
	defer session.Close()
	finished := make(chan error, 1)
	go func() { finished <- session.RunPlanOnly() }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("native coordinator did not start")
	}
	require.True(t, isSessionLive(sid))
	require.True(t, isSessionLive(session.Id))
	recorder := httptest.NewRecorder()
	(&VizHTTPServer{}).handleListLiveSessions(recorder, httptest.NewRequest(http.MethodGet, "/live", nil))
	var response LiveSessionsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	found := 0
	for _, live := range response.Sessions {
		if live.SessionID == sid {
			found++
			require.True(t, live.IsCoordinator)
			require.Equal(t, session.Id, live.CoordinatorID)
		}
	}
	require.Equal(t, 1, found)
	cancel()
	select {
	case err := <-finished:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancelled native coordinator did not exit")
	}
	require.False(t, isSessionLive(sid))
	require.False(t, isSessionLive(session.Id))
}
