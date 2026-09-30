package browser

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
)

// A CDP operation that does not complete on its own. This checks the real Rod
// context boundary without launching Chrome or accessing any network endpoint.
type stalledWaitCDP struct{ events chan *cdp.Event }

func (c *stalledWaitCDP) Event() <-chan *cdp.Event { return c.events }
func (c *stalledWaitCDP) Call(ctx context.Context, session, method string, params interface{}) ([]byte, error) {
	if strings.HasPrefix(method, "Runtime.") {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if method == "Target.attachToTarget" {
		return []byte(`{"sessionId":"fixture"}`), nil
	}
	return []byte(`{}`), nil
}

func TestWaitFunctionUsesOperationTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	client := &stalledWaitCDP{events: make(chan *cdp.Event)}
	t.Cleanup(func() { close(client.events) })
	b := rod.New().Context(ctx).Client(client).NoDefaultDevice()
	require.NoError(t, b.Connect())
	page, err := b.PageFromTarget(proto.TargetTargetID("fixture"))
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		timeout time.Duration
		budget  []float64
	}{
		{"configured", 20 * time.Millisecond, nil},
		{"shorter readiness budget", time.Second, []float64{0.02}},
		{"budget cannot extend timeout", 20 * time.Millisecond, []float64{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &BrowserPage{page: page, timeout: tc.timeout}
			start := time.Now()
			require.ErrorIs(t, p.WaitFunction("false", tc.budget...), context.DeadlineExceeded)
			require.Less(t, time.Since(start), 500*time.Millisecond, "per-operation timeout was ignored")
		})
	}
}

func TestWaitFunctionRejectsPendingDialog(t *testing.T) {
	p := &BrowserPage{pendingDialog: &JavaScriptDialog{Type: "alert", Message: "blocked"}}
	var dialogError *DialogBlockingError
	require.ErrorAs(t, p.WaitFunction("true"), &dialogError)
	require.Equal(t, "blocked", dialogError.Dialog.Message)
	require.ErrorContains(t, p.WaitFunction(""), "cannot be empty")
}
