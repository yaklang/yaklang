package coordinator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func settledDeliveryController(t *testing.T) *Controller {
	t.Helper()
	c := New(context.Background(), &testHost{}, 1)
	t.Cleanup(c.Close)
	c.patchDir = filepath.Join(t.TempDir(), "plan-patches")
	c.state.Phase, c.state.Plan = PhaseExec, testPlan()
	for _, task := range c.state.Plan.Tasks {
		c.state.Attempts[task.ID] = Attempt{Task: task, ID: 1, State: Accepted, Result: Result{Summary: task.Name + " verified", EvidenceIDs: []string{"source." + task.ID}}}
	}
	return c
}

func TestDeliveryCannotCompleteBeforeReviewOrWithStaleInput(t *testing.T) {
	c := settledDeliveryController(t)
	basis := c.Snapshot()
	a := c.state.Attempts["a"]
	a.State = AwaitingReview
	c.state.Attempts["a"] = a
	_, err := c.saveDelivery(context.Background(), basis, Delivery{Content: "not approved"})
	require.ErrorIs(t, err, errDeliveryChanged)
	a.State = Accepted
	c.state.Attempts["a"] = a
	c.state.UserRevision++
	_, err = c.saveDelivery(context.Background(), basis, Delivery{Content: "obsolete"})
	require.ErrorIs(t, err, errDeliveryChanged)
	require.False(t, c.Snapshot().Report.Submitted)
}

func TestDeliveryPersistsBusinessResultAndObjectiveFailureHistory(t *testing.T) {
	c := settledDeliveryController(t)
	c.state.History = map[string][]Attempt{"a": {{Task: c.state.Attempts["a"].Task, ID: 0, State: Failed, Result: Result{Error: "source unavailable"}}}}
	const output = `{"@action":"business","payload":{"array":[1,"two",null]}}`
	r, err := c.saveDelivery(context.Background(), c.Snapshot(), Delivery{Content: output, ContentType: "application/json", Summary: "checked"})
	require.NoError(t, err)
	require.True(t, r.Submitted)
	data, err := os.ReadFile(r.DeliveryPath)
	require.NoError(t, err)
	require.Equal(t, output, string(data))
	proof, err := os.ReadFile(r.Path)
	require.NoError(t, err)
	require.Contains(t, string(proof), "source unavailable")
	require.Contains(t, string(proof), "source.a")
	require.True(t, strings.HasSuffix(r.DeliveryPath, ".json"))
	require.NoError(t, c.FinalizeReport())
	require.True(t, c.Snapshot().Finished)
}

func TestDeliveryDoesNotSwallowArtifactOrCancellationFailure(t *testing.T) {
	c := settledDeliveryController(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.saveDelivery(ctx, c.Snapshot(), Delivery{Content: "partial"})
	require.ErrorIs(t, err, context.Canceled)
	file := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(file, []byte("file"), 0600))
	c.patchDir = filepath.Join(file, "plan-patches")
	_, err = c.saveDelivery(context.Background(), c.Snapshot(), Delivery{Content: "partial"})
	require.Error(t, err)
	require.False(t, errors.Is(err, errDeliveryChanged))
	require.False(t, c.Snapshot().Report.Submitted)
}
