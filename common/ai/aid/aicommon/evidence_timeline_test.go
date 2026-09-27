package aicommon

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func evidenceConfig(t *testing.T) *Config {
	t.Helper()
	cfg := NewConfig(context.Background())
	cfg.Timeline.SetTimelineBucketByteSize(-1)
	return cfg
}
func saveTestEvidence(c *Config, id, content string) {
	c.ApplySessionEvidenceOps([]EvidenceOperation{{Op: "add", ID: id, Content: content}})
}

func TestTimelineEvidenceFreezeUpdateDeleteAndReadOnlyPrompt(t *testing.T) {
	c := evidenceConfig(t)
	saveTestEvidence(c, "a", "first finding")
	before := BuildPromptFrozenOpenMaterials(c)
	require.Contains(t, before.TimelineOpen, "first finding")
	require.Empty(t, before.SessionEvidenceSemiDynamic)
	require.NotContains(t, before.TimelineOpen, "CACHE_TOOL_CALL", "evidence must not create an empty tool-cache block")
	raw, err := MarshalTimeline(c.Timeline)
	require.NoError(t, err)
	id := c.Timeline.GetMaxID()
	for i := 0; i < 3; i++ {
		BuildPromptFrozenOpenMaterials(c)
	}
	after, err := MarshalTimeline(c.Timeline)
	require.NoError(t, err)
	require.JSONEq(t, raw, after)
	saveTestEvidence(c, "a", "first finding")
	require.Equal(t, id, c.Timeline.GetMaxID(), "identical evidence must not grow the journal")
	receipt := c.Timeline.FreezeAll()
	require.Len(t, receipt.Promotions, 1)
	require.Equal(t, TimelinePromotedKindEvidence, receipt.Promotions[0].Kind)
	sealed := BuildPromptFrozenOpenMaterials(c)
	require.Empty(t, sealed.TimelineOpen)
	require.Contains(t, sealed.SessionEvidenceSemiDynamic, "first finding")
	saveTestEvidence(c, "a", "updated finding")
	pending := BuildPromptFrozenOpenMaterials(c)
	require.Equal(t, sealed.SessionEvidenceSemiDynamic, pending.SessionEvidenceSemiDynamic)
	require.Contains(t, pending.TimelineOpen, "[UPSERT]")
	require.Contains(t, pending.TimelineOpen, "updated finding")
	require.NotContains(t, c.GetSessionEvidenceRendered(), "first finding")
	c.Timeline.FreezeAll()
	require.NotContains(t, BuildPromptFrozenOpenMaterials(c).SessionEvidenceSemiDynamic, "first finding")
	sealed = BuildPromptFrozenOpenMaterials(c)
	c.ApplySessionEvidenceOps([]EvidenceOperation{{Op: "delete", ID: "a"}})
	pending = BuildPromptFrozenOpenMaterials(c)
	require.Equal(t, sealed.SessionEvidenceSemiDynamic, pending.SessionEvidenceSemiDynamic)
	require.Contains(t, pending.TimelineOpen, "[TOMBSTONE]")
	require.Empty(t, c.GetSessionEvidenceRendered())
	c.Timeline.FreezeAll()
	require.Empty(t, BuildPromptFrozenOpenMaterials(c).SessionEvidenceSemiDynamic)
	require.Empty(t, BuildPromptFrozenOpenMaterials(c).TimelineOpen)
}

func TestTimelineEvidenceAndToolPromoteInSameTransaction(t *testing.T) {
	c := evidenceConfig(t)
	saveTestEvidence(c, "a", "finding")
	require.True(t, c.Timeline.PushPromotable(c.AcquireId(), TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1, "tool", TimelinePromotedOperationUpsert, "TOOL_SCHEMA"))
	receipt := c.Timeline.FreezeAll()
	require.Len(t, receipt.Promotions, 2)
	materials := BuildPromptFrozenOpenMaterials(c)
	require.Contains(t, materials.SessionEvidenceSemiDynamic, "finding")
	require.Contains(t, materials.PromotedSemiDynamic1, "TOOL_SCHEMA")
	require.Empty(t, materials.TimelineOpen)
	require.Empty(t, c.Timeline.getActiveTimelineItemIDs(), "neither evidence nor schema is AI reducer input")
}

func TestTimelineEvidencePayloadTriggersFreeze(t *testing.T) {
	c := evidenceConfig(t)
	c.Timeline.SetTimelineBucketByteSize(256)
	saveTestEvidence(c, "large", strings.Repeat("confirmed observation ", 100))
	blocks := BuildPromptFrozenOpenMaterials(c)
	require.NotEmpty(t, blocks.SessionEvidenceSemiDynamic)
	require.Empty(t, blocks.TimelineOpen)
	require.NotEmpty(t, c.Timeline.FreezeSnapshot().Batches)
}

func TestTimelineEvidenceBudgetEvictionWaitsForFreeze(t *testing.T) {
	c := evidenceConfig(t)
	saveTestEvidence(c, "old", strings.Repeat("long evidence ", 20000))
	c.Timeline.FreezeAll()
	before := BuildPromptFrozenOpenMaterials(c).SessionEvidenceSemiDynamic
	saveTestEvidence(c, "new", "short finding")
	pending := BuildPromptFrozenOpenMaterials(c)
	require.Equal(t, before, pending.SessionEvidenceSemiDynamic)
	require.Contains(t, pending.TimelineOpen, "[id: old]")
	require.Contains(t, pending.TimelineOpen, "[TOMBSTONE]")
	require.Contains(t, pending.TimelineOpen, "short finding")
	require.NotContains(t, c.GetSessionEvidenceRendered(), "[id: old]")
	c.Timeline.FreezeAll()
	promoted := BuildPromptFrozenOpenMaterials(c).SessionEvidenceSemiDynamic
	require.Contains(t, promoted, "short finding")
	require.NotContains(t, promoted, "long evidence")
}

func TestTimelineEvidenceRestoreForkRollback(t *testing.T) {
	c := evidenceConfig(t)
	saveTestEvidence(c, "a", "original")
	c.Timeline.FreezeAll()
	checkpoint := c.Timeline.GetMaxID()
	saveTestEvidence(c, "b", "pending")
	raw, err := MarshalTimeline(c.Timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, RenderTimelineFrozenOpen(c.Timeline), RenderTimelineFrozenOpen(restored))
	fork, err := c.Timeline.ForkForTask("child", "evidence", nil, nil)
	require.NoError(t, err)
	child := evidenceConfig(t)
	child.Timeline = fork.Branch
	child.SeqIdProvider = c.SeqIdProvider
	saveTestEvidence(child, "a", "child update")
	child.Timeline.FreezeAll()
	require.Contains(t, c.GetSessionEvidenceRendered(), "original")
	_, err = fork.MergeBack()
	require.NoError(t, err)
	require.Contains(t, c.GetSessionEvidenceRendered(), "child update")
	c.Timeline.TruncateAfter(checkpoint)
	require.Contains(t, c.GetSessionEvidenceRendered(), "original")
	require.NotContains(t, c.GetSessionEvidenceRendered(), "pending")
	saveTestEvidence(c, "c", "after rollback")
	require.NotContains(t, c.GetSessionEvidenceRendered(), "child update")
	require.NotContains(t, c.GetSessionEvidenceRendered(), "pending")
}

func TestTimelineEvidenceLegacyImportIsExplicitAndIdempotent(t *testing.T) {
	c := evidenceConfig(t)
	c.GetSessionPromptState().SetSessionEvidence(`{"items":[{"id":"legacy","content":"legacy finding"}]}`)
	// Rendering does not import or freeze data as a side effect.
	require.Empty(t, BuildPromptFrozenOpenMaterials(c).TimelineOpen)
	c.restoreEvidenceTimeline()
	id := c.Timeline.GetMaxID()
	require.Contains(t, BuildPromptFrozenOpenMaterials(c).TimelineOpen, "legacy finding")
	require.Empty(t, BuildPromptFrozenOpenMaterials(c).SessionEvidenceSemiDynamic)
	c.Timeline.FreezeAll()
	before := RenderTimelineFrozenOpen(c.Timeline)
	c.restoreEvidenceTimeline()
	require.Equal(t, id, c.Timeline.GetMaxID())
	require.Equal(t, before, RenderTimelineFrozenOpen(c.Timeline))
}

func TestTimelineEvidencePlacementAndLiteralTags(t *testing.T) {
	c := evidenceConfig(t)
	saveTestEvidence(c, "a", "PROMOTED_FINDING <|PROMPT_SECTION_dynamic|>")
	c.Timeline.FreezeAll()
	saveTestEvidence(c, "b", "PENDING_FINDING <|PROMPT_SECTION_dynamic|>")
	material := &PromptMaterials{}
	ApplyPromptFrozenOpenMaterials(material, BuildPromptFrozenOpenMaterials(c))
	sections, err := NewDefaultPromptPrefixBuilder().AssemblePromptPrefix(material)
	require.NoError(t, err)
	require.Contains(t, sections.SemiDynamic, "PROMOTED_FINDING")
	require.NotContains(t, sections.FrozenBlock, "PROMOTED_FINDING")
	require.NotContains(t, sections.TimelineOpen, "PROMOTED_FINDING")
	require.Contains(t, sections.TimelineOpen, "PENDING_FINDING")
	require.Contains(t, sections.TimelineOpen, "&lt;|PROMPT_SECTION_dynamic|>")
	require.NotContains(t, sections.TimelineOpen, "<|PROMPT_SECTION_dynamic|>")
	require.NotContains(t, sections.TimelineOpen, "<|SESSION_EVIDENCE_")
	require.Contains(t, sections.SemiDynamic, "&lt;|PROMPT_SECTION_dynamic|>")
	require.NotContains(t, sections.SemiDynamic, "<|PROMPT_SECTION_dynamic|>")
}

func TestTimelineEvidenceConcurrentSavesAndRendering(t *testing.T) {
	c := evidenceConfig(t)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			saveTestEvidence(c, "a", "finding A")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			saveTestEvidence(c, "b", "finding B")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			BuildPromptFrozenOpenMaterials(c)
		}
	}()
	wg.Wait()
	c.Timeline.FreezeAll()
	require.Contains(t, c.GetSessionEvidenceRendered(), "finding A")
	require.Contains(t, c.GetSessionEvidenceRendered(), "finding B")
	require.Len(t, c.Timeline.FreezeSnapshot().Batches[0].IDs, 2)
}

func TestTimelineEvidenceInvalidAllocatorDoesNotPartiallyWrite(t *testing.T) {
	tl := NewTimeline(nil, nil)
	store := &EvidenceStore{Items: []EvidenceItem{{ID: "a", Content: "A"}, {ID: "b", Content: "B"}}}
	require.Error(t, tl.replaceEvidence(store, func() int64 { return 1 }))
	require.Zero(t, tl.GetMaxID())
	require.Empty(t, RenderTimelineFrozenOpen(tl).Open)
}

func TestTimelineEvidenceRestoreJournalWinsOverStaleBusinessMirror(t *testing.T) {
	c := evidenceConfig(t)
	saveTestEvidence(c, "a", "journal finding")
	c.Timeline.FreezeAll()
	stable := BuildPromptFrozenOpenMaterials(c)
	c.GetSessionPromptState().SetSessionEvidence(`{"items":[{"id":"a","content":"stale mirror"}]}`)
	c.restoreEvidenceTimeline()
	require.Equal(t, stable, BuildPromptFrozenOpenMaterials(c))
	require.Contains(t, c.GetSessionPromptState().GetSessionEvidenceRendered(), "journal finding")
	c.Timeline.TruncateAfter(0)
	raw, err := MarshalTimeline(c.Timeline)
	require.NoError(t, err)
	c.Timeline, err = UnmarshalTimeline(raw)
	require.NoError(t, err)
	c.restoreEvidenceTimeline()
	require.Empty(t, c.GetSessionEvidenceRendered())
	require.Empty(t, c.GetSessionPromptState().GetSessionEvidenceRendered())
}

func TestTimelineEvidenceRollbackAllDoesNotResurrectBusinessMirror(t *testing.T) {
	c := evidenceConfig(t)
	saveTestEvidence(c, "a", "removed by rollback")
	c.Timeline.TruncateAfter(0)
	require.Empty(t, c.GetSessionEvidenceRendered())
	saveTestEvidence(c, "b", "new finding")
	require.NotContains(t, c.GetSessionEvidenceRendered(), "removed by rollback")
	require.Contains(t, c.GetSessionEvidenceRendered(), "new finding")
}

// TestTimelineEvidenceMixedHistoryPromotionLifecycle is a small executable
// walkthrough. Fixed timestamps and explicit freeze avoid sleeps, AI, Config
// initialization and DB access. The journal keeps every operation; the prompt
// retains every open delta in place and folds by semantic key only on freeze.
func TestTimelineEvidenceMixedHistoryPromotionLifecycle(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(-1)
	base := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	var id int64
	appendNote := func(text string) {
		id++
		injectTimelineItem(tl, id, base.Add(time.Duration(id)*time.Second), &TextTimelineItem{ID: id, Text: text})
	}
	appendEvidence := func(key, operation, content string) {
		id++
		payload := ""
		if operation == TimelinePromotedOperationUpsert {
			encoded, err := json.Marshal(newEvidenceItem(key, content, base.Unix()))
			require.NoError(t, err)
			payload = string(encoded)
		}
		injectTimelineItem(tl, id, base.Add(time.Duration(id)*time.Second), &PromotableTimelineItem{
			ID: id, Kind: TimelinePromotedKindEvidence, TargetSection: TimelinePromotedTargetSemiDynamic1,
			Key: key, Operation: operation, Payload: payload, PayloadHash: promotedPayloadHash(payload),
		})
	}
	// Check the real rendered timeline prefix after every append. Only the closing
	// Timeline tag moves; all prior entry bytes must remain unchanged until freeze.
	var previousPrefix string
	assertAppendOnly := func() {
		prompt := RenderTimelineFrozenOpen(tl).Open
		end := strings.LastIndex(prompt, "<|TIMELINE_END_")
		require.GreaterOrEqual(t, end, 0)
		require.True(t, strings.HasPrefix(prompt, previousPrefix), "appending must not rewrite an earlier delta")
		previousPrefix = prompt[:end]
		require.NotContains(t, prompt, "<|SESSION_EVIDENCE_")
	}
	// 1. Ordinary history and evidence share the same ordered journal and bucket.
	appendNote("[user]:\n检查服务的访问控制和健康状态") // #1
	assertAppendOnly()
	appendEvidence("auth", TimelinePromotedOperationUpsert, "未登录请求返回 403") // #2
	assertAppendOnly()
	appendNote("[tool_result]:\n完成第二次访问控制检查") // #3
	assertAppendOnly()
	appendEvidence("temporary", TimelinePromotedOperationUpsert, "候选判断，待复核") // #4
	assertAppendOnly()
	appendEvidence("auth", TimelinePromotedOperationUpsert, "复核确认：未登录请求返回 401") // #5
	assertAppendOnly()
	appendEvidence("temporary", TimelinePromotedOperationDelete, "") // #6
	assertAppendOnly()
	appendEvidence("health", TimelinePromotedOperationUpsert, "健康检查返回 200") // #7
	assertAppendOnly()
	appendNote("[note]:\n本轮检查结束") // #8
	assertAppendOnly()
	require.Equal(t, []int64{1, 2, 3, 4, 5, 6, 7, 8}, tl.idToTimelineItem.Keys())
	audit := tl.GetTimelineOutput()
	require.Len(t, audit, 8, "user-facing history must include evidence operations in their original positions")
	require.Contains(t, audit[0].Content, "检查服务")
	require.Contains(t, audit[1].Content, "[evidence upsert: auth]")
	require.Contains(t, audit[1].Content, "403")
	require.Contains(t, audit[4].Content, "401")
	require.Contains(t, audit[5].Content, "[evidence delete: temporary]")
	for _, evidenceID := range []int64{2, 4, 5, 6, 7} {
		item, _ := tl.idToTimelineItem.Get(evidenceID)
		human := ParseTimelineItemHumanReadable(item) // same serializer used by timeline_item events
		require.Equal(t, evidenceID, human.ID)
		require.Equal(t, "evidence", human.EntryType)
		require.NotEmpty(t, human.Content)
	}
	t.Log("步骤 1：用户时间线可见普通事件与新增、修正、删除证据，共 8 条记录。")

	// 2. Before freeze, the prompt shows current open mutations and no semi snapshot.
	render := func() (string, string, string) {
		blocks := RenderTimelineFrozenOpen(tl)
		material := &PromptMaterials{TimelineFrozen: blocks.Frozen, TimelineOpen: blocks.Open,
			SessionEvidenceSemiDynamic: blocks.EvidenceSemiDynamic}
		prefix, err := NewDefaultPromptPrefixBuilder().AssemblePromptPrefix(material)
		require.NoError(t, err)
		return strings.TrimSpace(prefix.FrozenBlock), strings.TrimSpace(prefix.SemiDynamic), strings.TrimSpace(prefix.TimelineOpen)
	}
	_, semi, open := render()
	require.Empty(t, semi)
	require.Contains(t, open, "检查服务")
	require.Contains(t, open, "401")
	require.Contains(t, open, "403", "open evidence retains earlier versions to preserve the append-only prefix")
	require.Contains(t, open, "[TOMBSTONE]")
	require.Contains(t, open, "健康检查返回 200")
	require.NotContains(t, open, "<|SESSION_EVIDENCE_")
	lastPosition := -1
	for _, text := range []string{"检查服务的访问控制", "未登录请求返回 403", "完成第二次访问控制检查", "候选判断", "复核确认", "[TOMBSTONE]", "健康检查返回 200", "本轮检查结束"} {
		position := strings.Index(open, text)
		require.Greater(t, position, lastPosition, "delta must retain its original position: %s", text)
		lastPosition = position
	}
	require.Empty(t, tl.Freeze().NewlyFrozenIDs, "the single under-budget time bucket is still open")
	t.Logf("步骤 2：冻结前 Timeline Open：\n%s", open)

	// 3. One freeze seals ordinary facts and materializes all five evidence mutations.
	receipt := tl.FreezeAll()
	require.Equal(t, []int64{1, 2, 3, 4, 5, 6, 7, 8}, receipt.NewlyFrozenIDs)
	require.Len(t, receipt.Promotions, 5)
	require.Zero(t, receipt.PendingBytes)
	require.Equal(t, int64(1), receipt.Version)
	frozen, semi, open := render()
	require.Contains(t, frozen, "检查服务")
	require.NotContains(t, frozen, "401", "exact evidence belongs to semi, not the ordinary frozen narrative")
	require.Empty(t, open)
	require.Contains(t, semi, "401")
	require.Contains(t, semi, "健康检查返回 200")
	require.NotContains(t, semi, "403")
	require.NotContains(t, semi, "temporary")
	require.Equal(t, 1, strings.Count(semi, "[id: auth]"))
	require.Equal(t, 1, strings.Count(semi, "[id: health]"))
	aggregate := tl.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindEvidence]
	require.Len(t, aggregate, 2)
	require.Equal(t, int64(5), aggregate["auth"].SourceItemID)
	require.Equal(t, int64(7), aggregate["health"].SourceItemID)
	require.Equal(t, audit, tl.GetTimelineOutput(), "promotion must preserve the user's original audit trail")
	require.Equal(t, []int64{1, 3, 8}, tl.getActiveTimelineItemIDs(), "AI compression only receives ordinary facts")
	t.Logf("步骤 3：一次 freeze 后，semi 中聚合为 auth 和 health 两条有效证据：\n%s", semi)

	// 4. New edits remain open without rewriting the stable snapshot until the next freeze.
	stable := semi
	appendEvidence("auth", TimelinePromotedOperationUpsert, "携带有效凭据后返回 200") // #9
	appendEvidence("health", TimelinePromotedOperationDelete, "")            // #10
	_, semi, open = render()
	require.Equal(t, stable, semi)
	require.Contains(t, open, "携带有效凭据后返回 200")
	require.Contains(t, open, "[TOMBSTONE]")
	t.Log("步骤 4：后续修改先留在 Open，semi 保持字节一致。")
	next := tl.FreezeAll()
	require.Equal(t, []int64{9, 10}, next.NewlyFrozenIDs)
	_, semi, open = render()
	require.Empty(t, open)
	require.Contains(t, semi, "携带有效凭据后返回 200")
	require.NotContains(t, semi, "401")
	require.NotContains(t, semi, "[id: health]")
	require.Len(t, tl.GetTimelineOutput(), 10)
	require.Empty(t, tl.FreezeAll().NewlyFrozenIDs, "repeated freeze cannot promote the same operations twice")
	t.Logf("步骤 5：第二次 freeze 后仅保留最新 auth；用户历史仍保留全部 10 条记录：\n%s", semi)
	// 6. Verify the real write path emits the same readable event to consumers.
	t.Run("live_timeline_event", func(t *testing.T) {
		events := make(chan *schema.AiOutputEvent, 1)
		live := NewTimeline(nil, nil)
		live.config = &Config{Emitter: NewEmitter("evidence-walkthrough", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			events <- event
			return event, nil
		})}
		_, err := live.applyEvidenceOperations([]EvidenceOperation{{Op: "add", ID: "live", Content: "用户可见的证据记录"}}, "", func() int64 { return 1 })
		require.NoError(t, err)
		select {
		case event := <-events:
			require.Equal(t, "timeline_item", event.NodeId)
			var human TimelineItemHumanReadable
			require.NoError(t, json.Unmarshal(event.Content, &human))
			require.Equal(t, int64(1), human.ID)
			require.Equal(t, "evidence", human.EntryType)
			require.Contains(t, human.Content, "用户可见的证据记录")
		case <-time.After(time.Second):
			t.Fatal("evidence write did not emit its user-facing timeline event")
		}
	})

}
