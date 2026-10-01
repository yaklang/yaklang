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

func userInputPromptMaterials(cfg *Config) PromptFrozenOpenMaterials {
	return BuildPromptFrozenOpenMaterialsWithOptions(cfg, TimelinePromptOptions{PromoteUserInput: true})
}

func userInputPromptBlocks(timeline *Timeline) TimelineFrozenOpenBlocks {
	return RenderTimelineFrozenOpenWithOptions(timeline, TimelinePromptOptions{PromoteUserInput: true})
}

func TestTimelineUserInputTaskIngressReusesHistoryAcrossFreezeAndRestore(t *testing.T) {
	cfg := evidenceConfig(t)
	original := "  original current input\n\tkeep whitespace  \n"
	_, err := cfg.AppendUserInputHistory(original, time.Now())
	require.NoError(t, err)
	cfg.Timeline.EnsureTaskUserInput("root", original, cfg.AcquireId)
	require.Equal(t, 1, cfg.Timeline.GetIdToTimelineItem().Len())
	cfg.Timeline.FreezeAll()
	before := userInputPromptMaterials(cfg).PromptedUserInputHistory
	cfg.Timeline.EnsureTaskUserInput("root", original, cfg.AcquireId)
	require.Equal(t, before, userInputPromptMaterials(cfg).PromptedUserInputHistory)
	require.Empty(t, userInputPromptMaterials(cfg).TimelineOpen)
	raw, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	nextID := int64(100)
	restored.ReassignIDs(func() int64 { nextID++; return nextID })
	restored.EnsureTaskUserInput("root", original, cfg.AcquireId)
	require.Equal(t, before, userInputPromptBlocks(restored).PromptedUserInputHistory)
	require.Empty(t, userInputPromptBlocks(restored).Open)
	require.Equal(t, 1, restored.GetIdToTimelineItem().Len())

	// A real second submission stays a second journal record, even if its text
	// is identical; prompt assembly only avoids adding a third startup copy.
	_, err = cfg.AppendUserInputHistory(original, time.Now())
	require.NoError(t, err)
	cfg.Timeline.EnsureTaskUserInput("root-2", original, cfg.AcquireId)
	require.Equal(t, 2, cfg.Timeline.GetIdToTimelineItem().Len())
	require.Contains(t, userInputPromptMaterials(cfg).TimelineOpen, original)
	cfg.Timeline.FreezeAll()
	require.Equal(t, 2, strings.Count(userInputPromptMaterials(cfg).PromptedUserInputHistory, original))
}

func TestTimelineUserInputTaskIngressPreservesTaskAndConcurrentAssembly(t *testing.T) {
	cfg := evidenceConfig(t)
	input := "  nested task input\n\tkeep exact body  \n"
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg.Timeline.EnsureTaskUserInput("child-a", input, cfg.AcquireId)
		}()
	}
	wg.Wait()
	require.Equal(t, 1, cfg.Timeline.GetIdToTimelineItem().Len())
	open := userInputPromptMaterials(cfg).TimelineOpen
	require.Contains(t, open, "[current task user input] [task:child-a]:\n"+input)
	cfg.Timeline.FreezeAll()
	before := userInputPromptMaterials(cfg).PromptedUserInputHistory
	cfg.Timeline.EnsureTaskUserInput("child-a", input, cfg.AcquireId)
	require.Empty(t, userInputPromptMaterials(cfg).TimelineOpen)
	cfg.Timeline.EnsureTaskUserInput("child-b", input, cfg.AcquireId)
	require.Equal(t, 2, cfg.Timeline.GetIdToTimelineItem().Len())
	require.Contains(t, userInputPromptMaterials(cfg).TimelineOpen, "[task:child-b]:\n"+input)
	require.Equal(t, before, userInputPromptMaterials(cfg).PromptedUserInputHistory)
	cfg.Timeline.EnsureTaskUserInput("child-a", "updated input", cfg.AcquireId)
	require.Equal(t, 3, cfg.Timeline.GetIdToTimelineItem().Len())
	cfg.Timeline.EnsureTaskUserInput("child-c", " \n\t", cfg.AcquireId)
	require.Equal(t, 3, cfg.Timeline.GetIdToTimelineItem().Len())
}

func TestTimelineUserInputPromotionPreservesOriginalAndCacheBoundary(t *testing.T) {
	cfg := evidenceConfig(t)
	original := "  用户 Query\n\t保留缩进和末尾空格  \n"
	_, err := cfg.AppendUserInputHistory(original, time.Now())
	require.NoError(t, err)
	cfg.Timeline.PushUserInteraction(UserInteractionStage_Review, cfg.AcquireId(), "确认范围？", "仅检查 src\n不部署")
	cfg.Timeline.PushText(cfg.AcquireId(), "[current task user input] [task:child]:\nchild query")
	saveTestEvidence(cfg, "fact", "real tool observation")
	before := userInputPromptMaterials(cfg)
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
	after := userInputPromptMaterials(cfg)
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
		userInputPromptMaterials(cfg)
	}
	again, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	require.JSONEq(t, raw, again, "prompt rendering must be read-only")
	_, err = cfg.AppendUserInputHistory("next input", time.Now())
	require.NoError(t, err)
	pending := userInputPromptMaterials(cfg)
	require.Equal(t, after.PromptedUserInputHistory, pending.PromptedUserInputHistory)
	require.Contains(t, pending.TimelineOpen, "next input")
	cfg.Timeline.FreezeAll()
	require.Contains(t, userInputPromptMaterials(cfg).PromptedUserInputHistory, "next input")
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
	blocks := userInputPromptBlocks(cfg.Timeline)
	require.Equal(t, 2, strings.Count(blocks.PromptedUserInputHistory, "same input"))
	raw, err := MarshalTimeline(cfg.Timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, blocks, userInputPromptBlocks(restored))
	nextID := int64(100)
	restored.ReassignIDs(func() int64 { nextID++; return nextID })
	require.Equal(t, blocks.PromptedUserInputHistory, userInputPromptBlocks(restored).PromptedUserInputHistory)
}

func TestTimelineUserInputCompressionAndRollbackKeepExactHistory(t *testing.T) {
	cfg := evidenceConfig(t)
	input := strings.Repeat("原始输入 long query\n", 5000)
	_, err := cfg.AppendUserInputHistory(input, time.Now())
	require.NoError(t, err)
	result, err := cfg.Timeline.CompressOnce(TimelineCompressionOptions{Context: context.Background(), MaxInputTokens: 100000, MaxSummaryTokens: 1000})
	require.NoError(t, err, "exact-only compression must seal without calling an AI summarizer")
	require.Empty(t, result.RetiredIDs)
	require.Contains(t, userInputPromptMaterials(cfg).PromptedUserInputHistory, input)
	checkpoint := cfg.Timeline.GetMaxID()
	_, err = cfg.AppendUserInputHistory("after checkpoint", time.Now())
	require.NoError(t, err)
	cfg.Timeline.FreezeAll()
	require.Contains(t, userInputPromptMaterials(cfg).PromptedUserInputHistory, "after checkpoint")
	require.NoError(t, cfg.Timeline.TruncateAfter(checkpoint))
	require.NotContains(t, userInputPromptMaterials(cfg).PromptedUserInputHistory, "after checkpoint")
	require.Contains(t, userInputPromptMaterials(cfg).PromptedUserInputHistory, input)
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
	projection := userInputPromptMaterials(cfg).PromptedUserInputHistory
	require.Equal(t, 12, strings.Count(projection, "identical query"))
	require.Less(t, strings.Index(projection, "Round 2\n"), strings.Index(projection, "Round 12\n"))
}

func TestTimelineUserInputForkMergeIsolation(t *testing.T) {
	parent := evidenceConfig(t)
	_, err := parent.AppendUserInputHistory("parent input", time.Now())
	require.NoError(t, err)
	parent.Timeline.FreezeAll()
	before := userInputPromptMaterials(parent).PromptedUserInputHistory
	fork, err := parent.Timeline.ForkForTask("child", "user-input", nil, nil)
	require.NoError(t, err)
	fork.Branch.PushUserInteraction(UserInteractionStage_Review, parent.AcquireId(), "child question", "child reply")
	fork.Branch.FreezeAll()
	require.Equal(t, before, userInputPromptMaterials(parent).PromptedUserInputHistory)
	require.Contains(t, userInputPromptBlocks(fork.Branch).PromptedUserInputHistory, "child reply")
	_, err = fork.MergeBack()
	require.NoError(t, err)
	require.Equal(t, before, userInputPromptMaterials(parent).PromptedUserInputHistory)
	require.Contains(t, userInputPromptMaterials(parent).TimelineOpen, "child reply")
	parent.Timeline.FreezeAll()
	require.Contains(t, userInputPromptMaterials(parent).PromptedUserInputHistory, "child reply")
}

func TestTimelineUserInputAuditDoesNotChangeSealedPayload(t *testing.T) {
	cfg := evidenceConfig(t)
	id := cfg.AcquireId()
	cfg.Timeline.PushUserInteraction("", id, "", "default-stage input")
	cfg.Timeline.FreezeAll()
	before := userInputPromptMaterials(cfg).PromptedUserInputHistory
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
	require.Equal(t, before, userInputPromptMaterials(cfg).PromptedUserInputHistory)
	require.Contains(t, before, "Stage: free_input")
}
