package aicommon

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestTimelineUserInputPromotionPreservesOriginalAndCacheBoundary(t *testing.T) {
	cfg := evidenceConfig(t)
	original := "  用户 Query\n\t保留缩进和末尾空格  \n"
	_, err := cfg.AppendUserInputHistory(original, time.Now())
	require.NoError(t, err)
	cfg.Timeline.PushUserInteraction(UserInteractionStage_Review, cfg.AcquireId(), "确认范围？", "仅检查 src\n不部署")
	cfg.Timeline.PushText(cfg.AcquireId(), "[current task user input] [task:child]:\nchild query")
	saveTestEvidence(cfg, "fact", "real tool observation")
	before := BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, before.TimelineOpen, original)
	require.Contains(t, before.TimelineOpen, "确认范围？")
	require.Contains(t, before.TimelineOpen, "child query")
	require.Empty(t, before.PromptedUserInputHistory)
	snapshot, err := cfg.Timeline.captureCompressionSnapshot()
	require.NoError(t, err)
	require.Empty(t, snapshot.Items, "all user inputs and evidence must stay out of AI summarization")
	require.Len(t, snapshot.ExactItemIDs, 4)
	receipt := cfg.Timeline.FreezeAll()
	require.Len(t, receipt.Promotions, 4)
	after := BuildPromptFrozenOpenMaterials(cfg)
	require.Contains(t, after.PromptedUserInputHistory, original)
	require.Contains(t, after.PromptedUserInputHistory, "确认范围？")
	require.Contains(t, after.PromptedUserInputHistory, "child query")
	require.Contains(t, after.SessionEvidenceSemiDynamic, "real tool observation")
	require.NotContains(t, after.SessionEvidenceSemiDynamic, original)
	require.Empty(t, after.TimelineOpen)
	require.Empty(t, after.TimelineFrozen, "exact inputs must not also remain in ordinary Frozen history")
	materials := &PromptMaterials{}
	ApplyPromptFrozenOpenMaterials(materials, after)
	prefix, err := NewDefaultPromptPrefixBuilder().AssemblePromptPrefix(materials)
	require.NoError(t, err)
	require.Contains(t, prefix.SemiDynamic, original)
	require.NotContains(t, prefix.TimelineOpen, original)
	raw, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		BuildPromptFrozenOpenMaterials(cfg)
	}
	again, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	require.JSONEq(t, raw, again, "prompt rendering must be read-only")
	_, err = cfg.AppendUserInputHistory("next input", time.Now())
	require.NoError(t, err)
	pending := BuildPromptFrozenOpenMaterials(cfg)
	require.Equal(t, after.PromptedUserInputHistory, pending.PromptedUserInputHistory)
	require.Contains(t, pending.TimelineOpen, "next input")
	cfg.Timeline.FreezeAll()
	require.Contains(t, BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory, "next input")
}

func TestTimelineUserInputHistoryImportRepeatedInputsAndRestore(t *testing.T) {
	cfg := evidenceConfig(t)
	cfg.Timeline.PushUserInteraction(UserInteractionStage_FreeInput, cfg.AcquireId(), "", "same input")
	history := []schema.AIAgentUserInputRecord{
		{Round: 1, Timestamp: time.Now(), UserInput: "same input"},
		{Round: 2, Timestamp: time.Now(), UserInput: "same input"},
		{Round: 3, Timestamp: time.Now(), UserInput: "third input"},
	}
	cfg.SetUserInputHistory(history)
	cfg.SetUserInputHistory(history)
	require.Equal(t, 3, cfg.Timeline.GetIdToTimelineItem().Len())
	cfg.Timeline.FreezeAll()
	blocks := RenderTimelineFrozenOpen(cfg.Timeline)
	require.Equal(t, 2, strings.Count(blocks.PromptedUserInputHistory, "same input"))
	raw, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, blocks, RenderTimelineFrozenOpen(restored))
	nextID := int64(100)
	restored.ReassignIDs(func() int64 { nextID++; return nextID })
	require.Equal(t, blocks.PromptedUserInputHistory, RenderTimelineFrozenOpen(restored).PromptedUserInputHistory)
}

func TestTimelineUserInputCompressionAndRollbackKeepExactHistory(t *testing.T) {
	cfg := evidenceConfig(t)
	input := strings.Repeat("原始输入 long query\n", 5000)
	_, err := cfg.AppendUserInputHistory(input, time.Now())
	require.NoError(t, err)
	result, err := cfg.Timeline.CompressOnce(TimelineCompressionOptions{Context: context.Background(), MaxInputTokens: 100000, MaxSummaryTokens: 1000})
	require.NoError(t, err, "exact-only compression must seal without calling an AI summarizer")
	require.Empty(t, result.RetiredIDs)
	require.Contains(t, BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory, input)
	checkpoint := cfg.Timeline.GetMaxID()
	_, err = cfg.AppendUserInputHistory("after checkpoint", time.Now())
	require.NoError(t, err)
	cfg.Timeline.FreezeAll()
	require.Contains(t, BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory, "after checkpoint")
	require.NoError(t, cfg.Timeline.TruncateAfter(checkpoint))
	require.NotContains(t, BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory, "after checkpoint")
	require.Contains(t, BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory, input)
}

func TestTimelineUserInputConcurrentAppendOrder(t *testing.T) {
	cfg := evidenceConfig(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cfg.AppendUserInputHistory("identical query", time.Now())
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	cfg.Timeline.FreezeAll()
	history := cfg.GetUserInputHistory()
	require.Len(t, history, 12)
	projection := BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory
	require.Equal(t, 12, strings.Count(projection, "identical query"))
	require.Less(t, strings.Index(projection, "Round 2\n"), strings.Index(projection, "Round 12\n"))
}

func TestTimelineUserInputForkMergeIsolation(t *testing.T) {
	parent := evidenceConfig(t)
	_, err := parent.AppendUserInputHistory("parent input", time.Now())
	require.NoError(t, err)
	parent.Timeline.FreezeAll()
	before := BuildPromptFrozenOpenMaterials(parent).PromptedUserInputHistory
	fork, err := parent.Timeline.ForkForTask("child", "user-input", nil, nil)
	require.NoError(t, err)
	fork.Branch.PushUserInteraction(UserInteractionStage_Review, parent.AcquireId(), "child question", "child reply")
	fork.Branch.FreezeAll()
	require.Equal(t, before, BuildPromptFrozenOpenMaterials(parent).PromptedUserInputHistory)
	require.Contains(t, RenderTimelineFrozenOpen(fork.Branch).PromptedUserInputHistory, "child reply")
	_, err = fork.MergeBack()
	require.NoError(t, err)
	require.Equal(t, before, BuildPromptFrozenOpenMaterials(parent).PromptedUserInputHistory)
	require.Contains(t, BuildPromptFrozenOpenMaterials(parent).TimelineOpen, "child reply")
	parent.Timeline.FreezeAll()
	require.Contains(t, BuildPromptFrozenOpenMaterials(parent).PromptedUserInputHistory, "child reply")
}

func TestTimelineUserInputAuditDoesNotChangeSealedPayload(t *testing.T) {
	cfg := evidenceConfig(t)
	id := cfg.AcquireId()
	cfg.Timeline.PushUserInteraction("", id, "", "default-stage input")
	cfg.Timeline.FreezeAll()
	before := BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory
	raw, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	cfg.Timeline.Dump()
	cfg.Timeline.ToTimelineItemOutputLastN(1)
	afterRaw, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	require.JSONEq(t, raw, afterRaw)
	item, ok := cfg.Timeline.GetIdToTimelineItem().Get(id)
	require.True(t, ok)
	require.Empty(t, item.value.(*UserInteraction).Stage)
	require.Equal(t, before, BuildPromptFrozenOpenMaterials(cfg).PromptedUserInputHistory)
	require.Contains(t, before, "Stage: free_input")
}
