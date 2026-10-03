package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func observationPrompt(f *actionFixture) string {
	f.c.publish()
	m := aicommon.BuildPromptFrozenOpenMaterials(f.cfg)
	return aicommon.WrapPromptMessageSection(aicommon.PromptSectionSemiDynamic1, m.SessionEvidenceSemiDynamic, "") +
		aicommon.WrapPromptMessageSection(aicommon.PromptSectionTimelineOpen, m.TimelineOpen, "")
}

func settleObservation(t *testing.T, f *actionFixture) Attempt {
	t.Helper()
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return f.c.Snapshot().Attempts["a"].State == AwaitingReview }, time.Second, time.Millisecond)
	f.c.publish()
	return f.c.Snapshot().Attempts["a"]
}

func TestCoordinatorTaskResultPublishedBeforeReview(t *testing.T) {
	f := newActionFixture(t, true)
	a := settleObservation(t, f)
	id, content := taskResultEvidence(f.cfg, taskResultRecords([]Attempt{a})[0])
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), content)
	require.Error(t, f.c.ReviewTask("a", a.ID+1, "accept", "wrong attempt"))
	f.cfg.Timeline.FreezeAll()
	require.Contains(t, observationPrompt(f), "[id: "+id+"]")
	require.NoError(t, f.c.ReviewTask("a", a.ID, "reject", "retry source"))
	_, err := f.c.RetryTask("a", a.ID, "new source")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return f.c.Snapshot().Attempts["a"].State == AwaitingReview }, time.Second, time.Millisecond)
	f.c.publish()
	require.Error(t, f.c.ReviewTask("a", a.ID, "accept", "stale attempt"))
	next := f.c.Snapshot().Attempts["a"]
	require.NoError(t, f.c.ReviewTask("a", next.ID, "accept", "new source confirmed"))
}

func TestCoordinatorTaskObservationFailuresAndCancellation(t *testing.T) {
	for _, mode := range []string{"error", "panic", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newActionFixture(t, true)
			f.c.host.(*testHost).execute = func(ctx context.Context, _ Attempt) (Result, error) {
				switch mode {
				case "panic":
					panic("worker panic")
				case "cancel":
					<-ctx.Done()
					return Result{}, ctx.Err()
				default:
					return Result{Summary: "partial evidence", EvidenceIDs: []string{"partial"}}, errors.New("worker error")
				}
			}
			_, err := f.c.StartTasks([]string{"a"})
			require.NoError(t, err)
			if mode == "cancel" {
				err = f.c.CancelTasks([]string{"a"}, "cancel test")
				require.NoError(t, err)
			}
			require.Eventually(t, func() bool { a := f.c.Snapshot().Attempts["a"]; return a.State == Failed || a.State == Cancelled }, time.Second, time.Millisecond)
			f.c.publish()
			a := f.c.Snapshot().Attempts["a"]
			_, content := taskResultEvidence(f.cfg, taskResultRecords([]Attempt{a})[0])
			require.Contains(t, f.cfg.GetSessionEvidenceRendered(), content)
			require.NotEmpty(t, a.Result.Error)
		})
	}
}

func TestCoordinatorTaskObservationDoesNotDowngradeFromActionReceipt(t *testing.T) {
	f := newActionFixture(t, true)
	a := settleObservation(t, f)
	stale := a
	stale.State, stale.Result = Running, Result{}
	before := f.cfg.GetSessionEvidenceRendered()
	_ = taskResultReferences(f.loop, []Attempt{stale})
	require.Equal(t, before, f.cfg.GetSessionEvidenceRendered())
	require.NoError(t, f.c.attachResultTimeline(f.cfg))
	require.Equal(t, before, f.cfg.GetSessionEvidenceRendered())
}

func TestCoordinatorTaskObservationPersistenceFailure(t *testing.T) {
	f := newActionFixture(t, true)
	settleObservation(t, f)
	other := aicommon.NewConfig(context.Background(), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()))
	require.Error(t, f.c.attachResultTimeline(discardEvidenceConfig{other}))
	require.ErrorContains(t, f.c.ReviewTask("a", f.c.Snapshot().Attempts["a"].ID, "accept", "unpersisted"), "Timeline result publication failed")
}

func TestCoordinatorTaskObservationDoesNotReinsertDeletedHistory(t *testing.T) {
	f := newActionFixture(t, true)
	a := settleObservation(t, f)
	id, _ := taskResultEvidence(f.cfg, taskResultRecords([]Attempt{a})[0])
	f.cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "delete", ID: id}})
	before := f.cfg.GetSessionEvidenceRendered()
	f.c.Wake()
	require.Equal(t, before, f.cfg.GetSessionEvidenceRendered(), "unrelated revisions must not reinsert unchanged deleted/evicted results")
}
