package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/schema"
)

func reportedRiskFixture(name string) *schema.Risk {
	return &schema.Risk{Url: "https://example.com/" + name, RiskType: "sqli", Parameter: "input", Title: name, Severity: "high"}
}

func reportedRiskTimelineItems(timeline *Timeline) []*TimelineItem {
	timeline.mu.RLock()
	defer timeline.mu.RUnlock()
	var items []*TimelineItem
	for _, id := range timeline.idToTimelineItem.Keys() {
		item, _ := timeline.idToTimelineItem.Get(id)
		if item == nil || item.deleted {
			continue
		}
		if op, ok := item.value.(*PromotableTimelineItem); ok && op.Kind == TimelinePromotedKindReportedRisk {
			items = append(items, item)
		}
	}
	return items
}

func requireTimelineItemByID(t *testing.T, timeline *Timeline, id int64) *TimelineItem {
	t.Helper()
	timeline.mu.RLock()
	item, ok := timeline.idToTimelineItem.Get(id)
	timeline.mu.RUnlock()
	require.True(t, ok)
	require.NotNil(t, item)
	return item
}

func TestTimelineReportedRisksBroadcastAndCleanContext(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	require.True(t, parent.AppendReportedRisk(reportedRiskFixture("before-clean-child")))
	fork, err := parent.Timeline.ForkForTask("fork", "fork", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)
	clean := NewPrivateTimeline(parent, nil)
	opts := append(ConvertConfigToOptionsWithoutHotPatch(parent), WithTimeline(clean),
		WithSessionPromptState(parent.SessionPromptState.ForkForTaskOnlySubAgent()), WithPersistentSessionId(""))
	child := newTimelineBroadcastConfig(t, opts...)
	unrelated := newTimelineBroadcastConfig(t)
	require.Contains(t, RenderTimelineFrozenOpen(fork.Branch).PromptText(), "before-clean-child")
	require.Empty(t, RenderTimelineFrozenOpen(clean).PromptText())
	require.Empty(t, reportedRiskTimelineItems(clean))
	require.NotEmpty(t, child.GetReportedRisks(), "the runtime dedup ledger is shared without rendering inherited risks")

	require.True(t, child.AppendReportedRisk(reportedRiskFixture("child-discovery")))
	for _, timeline := range []*Timeline{parent.Timeline, fork.Branch, clean} {
		require.Contains(t, RenderTimelineFrozenOpen(timeline).Open, "child-discovery")
		items := reportedRiskTimelineItems(timeline)
		last := items[len(items)-1]
		var report ReportedRiskItem
		require.NoError(t, json.Unmarshal([]byte(last.value.(*PromotableTimelineItem).Payload), &report))
		require.Equal(t, "child-discovery", report.Title)
	}
	require.Empty(t, reportedRiskTimelineItems(unrelated.Timeline))
	require.False(t, parent.AppendReportedRisk(reportedRiskFixture("child-discovery")))
	require.Len(t, reportedRiskTimelineItems(parent.Timeline), 2)
	require.NotContains(t, RenderTimelineFrozenOpen(clean).PromptText(), "before-clean-child")
	_, err = fork.MergeBack()
	require.NoError(t, err, "broadcast reports already in the parent must not collide on merge")
}

func TestTimelineReportedRisksAppendWithoutChangingFrozenHistory(t *testing.T) {
	cfg := newTimelineBroadcastConfig(t)
	risk := reportedRiskFixture("oldest-immutable-report")
	require.True(t, cfg.AppendReportedRisk(risk))
	first := reportedRiskTimelineItems(cfg.Timeline)[0]
	original, err := json.Marshal(first)
	require.NoError(t, err)
	cfg.Timeline.FreezeAll()
	frozen := RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, frozen.PromotedReportedRisks, "oldest-immutable-report")
	require.NotContains(t, frozen.Open, "oldest-immutable-report")
	risk.Title = "caller changed the risk object"

	for i := 0; i < 100; i++ {
		risk := reportedRiskFixture(fmt.Sprintf("later-%03d", i))
		risk.Url += "/" + strings.Repeat(fmt.Sprintf("unique-path-%03d-", i), 8)
		require.True(t, cfg.AppendReportedRisk(risk))
	}
	pending := RenderTimelineFrozenOpen(cfg.Timeline)
	require.Equal(t, frozen.PromotedReportedRisks, pending.PromotedReportedRisks)
	require.Contains(t, pending.Open, "later-099")
	cfg.Timeline.FreezeAll()
	all := RenderTimelineFrozenOpen(cfg.Timeline)
	require.True(t, strings.HasPrefix(all.PromotedReportedRisks, frozen.PromotedReportedRisks+"\n\n"))
	require.Greater(t, MeasureTokens(all.PromotedReportedRisks), 4000)
	require.Len(t, reportedRiskTimelineItems(cfg.Timeline), 101)
	require.Less(t, strings.Index(all.PromotedReportedRisks, "oldest-immutable-report"), strings.Index(all.PromotedReportedRisks, "later-099"))
	after, err := json.Marshal(first)
	require.NoError(t, err)
	require.Equal(t, original, after)
	require.NotContains(t, all.PromptText(), "caller changed")
}

func TestTimelineReportedRisksAreExactAcrossCompression(t *testing.T) {
	cfg := newTimelineBroadcastConfig(t)
	cfg.Timeline.PushText(cfg.AcquireId(), "older observation")
	require.True(t, cfg.AppendReportedRisk(reportedRiskFixture("protected-report")))
	item := reportedRiskTimelineItems(cfg.Timeline)[0]
	before, err := json.Marshal(item)
	require.NoError(t, err)
	cfg.Timeline.PushText(cfg.AcquireId(), "later observation")
	source, err := cfg.Timeline.buildCompressionSnapshot()
	require.NoError(t, err)
	require.Contains(t, source.ExactItemIDs, item.GetID())
	require.Empty(t, source.PromotedItems)
	require.Equal(t, 1, strings.Count(source.InputText, reportedRiskPromptTitle))
	through := cfg.Timeline.GetMaxID()
	result, err := cfg.Timeline.ApplyCompressionSelection(&TimelineCompressionSelection{
		ThroughID: &through, Summary: "observations completed", RetiredPromotedEntries: []int64{item.GetID()},
	}, "request containing the report")
	require.NoError(t, err)
	require.NotContains(t, result.RetiredIDs, item.GetID())
	require.Empty(t, result.RetiredPromotedEntryIDs)
	view := RenderTimelineFrozenOpen(cfg.Timeline)
	require.Contains(t, view.PromotedReportedRisks, "protected-report")
	require.Empty(t, view.Open)
	after, err := json.Marshal(requireTimelineItemByID(t, cfg.Timeline, item.GetID()))
	require.NoError(t, err)
	require.Equal(t, before, after)
	filtered := RenderTimelineFrozenOpenWithOptions(cfg.Timeline, TimelinePromptOptions{UserInputOnly: true, ExcludeToolCache: true})
	require.Contains(t, filtered.PromotedReportedRisks, "protected-report")
}

func TestTimelineReportedRisksRestoreWithoutLedgerRebuildOrReplay(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	risk := reportedRiskFixture("historical-report")
	require.True(t, parent.AppendReportedRisk(risk))
	parent.Timeline.FreezeAll()
	raw, err := MarshalTimeline(parent.Timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	next := int64(500)
	restored.ReassignIDs(func() int64 { next++; return next })
	view := RenderTimelineFrozenOpen(restored)
	require.Equal(t, RenderTimelineFrozenOpen(parent.Timeline).PromotedReportedRisks, view.PromotedReportedRisks)
	cfg := newTimelineBroadcastConfig(t, WithTimeline(restored), WithSequence(next+1))
	require.Empty(t, cfg.GetReportedRisks(), "restoring Timeline must not rebuild the runtime risk ledger")
	old := reportedRiskTimelineItems(restored)[0]
	before, err := json.Marshal(old)
	require.NoError(t, err)
	clean := newTimelineBroadcastConfig(t, append(ConvertConfigToOptionsWithoutHotPatch(cfg), WithTimeline(NewPrivateTimeline(cfg, nil)))...)
	require.Empty(t, RenderTimelineFrozenOpen(clean.Timeline).PromptText(), "a newly registered Timeline must not replay historical risk reports")

	// A newly emitted report in a fresh runtime is a new event, even when its
	// business dedup key also appeared in the old runtime.
	require.True(t, cfg.AppendReportedRisk(risk))
	items := reportedRiskTimelineItems(restored)
	require.Len(t, items, 2)
	require.NotEqual(t, items[0].value.(*PromotableTimelineItem).Key, items[1].value.(*PromotableTimelineItem).Key)
	cfg.Timeline.FreezeAll()
	updated := RenderTimelineFrozenOpen(cfg.Timeline)
	require.True(t, strings.HasPrefix(updated.PromotedReportedRisks, view.PromotedReportedRisks+"\n\n"))
	require.Equal(t, 1, strings.Count(updated.PromotedReportedRisks, reportedRiskPromptTitle))
	require.Len(t, reportedRiskTimelineItems(clean.Timeline), 1)
	after, err := json.Marshal(old)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestTimelineReportedRisksDoNotUseLegacyDynamicRendering(t *testing.T) {
	cfg := newTimelineBroadcastConfig(t)
	require.True(t, cfg.SessionPromptState.AppendReportedRisk(reportedRiskFixture("legacy-ledger-only")))
	materials := &PromptMaterials{TaskInstruction: "instruction", Schema: `{"type":"object"}`}
	ApplyPromptFrozenOpenMaterials(materials, BuildPromptFrozenOpenMaterials(cfg))
	require.NotContains(t, materials.TimelineOpen, "legacy-ledger-only")
	require.Empty(t, materials.PromotedReportedRisks)
	prompt, err := NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(materials, "risk-test", "dynamic", nil, "nonce")
	require.NoError(t, err)
	require.NotContains(t, prompt, "legacy-ledger-only")
	require.NotContains(t, prompt, "## 已报告漏洞清单")

	require.True(t, cfg.AppendReportedRisk(reportedRiskFixture("timeline-only-report")))
	cfg.Timeline.FreezeAll()
	ApplyPromptFrozenOpenMaterials(materials, BuildPromptFrozenOpenMaterials(cfg))
	prompt, err = NewDefaultPromptPrefixBuilder().AssemblePromptWithDynamicSection(materials, "risk-test", "dynamic", nil, "nonce")
	require.NoError(t, err)
	sections := promptBuilderChunksBySection(t, prompt)
	var semi, open strings.Builder
	for _, chunk := range sections[aiprojection.SectionSemiDynamic1] {
		semi.WriteString(chunk.Content)
	}
	for _, chunk := range sections[aiprojection.SectionTimelineOpen] {
		open.WriteString(chunk.Content)
	}
	require.Contains(t, semi.String(), "timeline-only-report")
	require.NotContains(t, open.String(), "timeline-only-report")
	require.NotContains(t, prompt, "legacy-ledger-only")
}

func TestTimelineReportedRisksConcurrentDedupAndCallbackReentry(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("child", "child", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)
	child := newTimelineBroadcastConfig(t, append(ConvertConfigToOptionsWithoutHotPatch(parent), WithTimeline(fork.Branch))...)
	var wg sync.WaitGroup
	var added atomic.Int64
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := parent
			if i%2 == 0 {
				cfg = child
			}
			if cfg.AppendReportedRisk(reportedRiskFixture(fmt.Sprintf("concurrent-%d", i%8))) {
				added.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, 8, added.Load())
	require.Len(t, reportedRiskTimelineItems(parent.Timeline), 8)
	require.Len(t, reportedRiskTimelineItems(child.Timeline), 8)
	parent.Timeline.RegisterItemInputCallback("risk-reentry", func(event TimelineItemInputEvent) {
		if strings.Contains(event.ItemJSON, "reentry-trigger") {
			require.NotEmpty(t, parent.GetReportedRisks())
			require.True(t, child.AppendReportedRisk(reportedRiskFixture("nested-report")))
		}
	})
	require.True(t, parent.AppendReportedRisk(reportedRiskFixture("reentry-trigger")))
	require.Contains(t, RenderTimelineFrozenOpen(parent.Timeline).Open, "nested-report")
	require.Contains(t, RenderTimelineFrozenOpen(child.Timeline).Open, "nested-report")
}

func TestTimelineReportedRisksFailedBroadcastCanRetry(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("child", "child", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	ctx, cancel := context.WithCancel(context.Background())
	child := newTimelineBroadcastConfig(t, append(ConvertConfigToOptionsWithoutHotPatch(parent), WithTimeline(fork.Branch), WithContext(ctx))...)
	cancel()
	risk := reportedRiskFixture("retry-after-cancel")
	require.False(t, child.AppendReportedRisk(risk))
	require.Equal(t, `{"items":[]}`, parent.GetReportedRisks())
	require.Empty(t, reportedRiskTimelineItems(parent.Timeline))
	require.Empty(t, reportedRiskTimelineItems(fork.Branch))
	require.True(t, parent.AppendReportedRisk(risk))
	require.Len(t, reportedRiskTimelineItems(parent.Timeline), 1)
	require.Len(t, reportedRiskTimelineItems(fork.Branch), 1)
	require.False(t, parent.AppendReportedRisk(nil))
}
