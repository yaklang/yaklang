package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSubForgeInteractiveInputRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := NewConfig(ctx)
	var parent, child []string
	cfg.InputEventManager.RegisterAfterInputEvent("parent", func(e *ypb.AIInputEvent) {
		parent = append(parent, e.GetInteractiveId())
	})
	closeRoute := cfg.InputEventManager.RouteInteractiveInput("child", func(e *ypb.AIInputEvent) bool {
		if e.GetInteractiveId() != "child-review" {
			return false
		}
		child = append(child, e.GetInteractiveId())
		return true
	})
	send := func(id string) {
		require.NoError(t, cfg.processInputEvent(&ypb.AIInputEvent{
			IsInteractiveMessage: true,
			InteractiveId:        id,
			InteractiveJSONInput: `{"suggestion":"continue"}`,
		}))
	}
	send("wrong-review")
	send("child-review")
	closeRoute("child-review")
	send("child-review")
	require.Equal(t, []string{"child-review"}, child)
	require.Equal(t, []string{"wrong-review"}, parent)
}
