package aicommon

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPromotedSkillContentConcatenatesSameNameAndSkipsSameHash(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(-1)
	pages := []string{"FIRST_PAGE", "SECOND_PAGE", "SECOND_PAGE"}
	timestamp := time.Now()
	var lastHash string
	for i, content := range pages {
		payload, err := json.Marshal(skillContentPayload{
			Name: "demo-skill", Path: "SKILL.md", Content: content, Source: SkillContentSourceManual,
		})
		require.NoError(t, err)
		lastHash = promotedPayloadHash(string(payload))
		id := int64(i + 1)
		importFreezeItem(tl, id, timestamp.Add(time.Duration(i)*time.Millisecond), &PromotableTimelineItem{
			ID: id, Kind: TimelinePromotedKindSkillContent, TargetSection: TimelinePromotedTargetSemiDynamic1,
			Key: skillPartIdentity(skillContentPayload{Name: "demo-skill", Path: "SKILL.md"}), Operation: TimelinePromotedOperationUpsert,
			Payload: string(payload), PayloadHash: lastHash,
		})
	}
	tl.FreezeAll()
	entries := tl.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindSkillContent]
	require.Len(t, entries, 1)
	entry := entries[skillPartIdentity(skillContentPayload{Name: "demo-skill", Path: "SKILL.md"})]
	require.NotNil(t, entry, "same path without a range stays one promoted part")
	require.Equal(t, lastHash, entry.PayloadHash, "compare the next payload against the last appended hash")
	require.Equal(t, int64(1), entry.SourceItemID)
	require.Equal(t, 1, strings.Count(entry.Payload, "FIRST_PAGE"))
	require.Equal(t, 1, strings.Count(entry.Payload, "SECOND_PAGE"))
	require.Less(t, strings.Index(entry.Payload, "FIRST_PAGE"), strings.Index(entry.Payload, "SECOND_PAGE"))
	require.Len(t, tl.idToTimelineItem.Keys(), 3, "projection deduplication must not delete original items")

	before := RenderTimelineFrozenOpen(tl)
	require.Contains(t, before.SkillsContext, "FIRST_PAGE")
	require.Contains(t, before.SkillsContext, "SECOND_PAGE")
	require.True(t, tl.PushSkillFileContent(4, "demo-skill", "references/api.md", "RESOURCE_PAGE", SkillContentSourceForced, 1, 1, 1))
	effective := tl.effectivePromotedEntries(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindSkillContent)
	require.Len(t, effective, 2, "a different path is a separate promoted part")
	joined := effective[0].Payload + "\n" + effective[1].Payload
	require.Contains(t, joined, "RESOURCE_PAGE")
	require.Contains(t, joined, "FIRST_PAGE")
	require.Equal(t, before.SkillsContext, RenderTimelineFrozenOpen(tl).SkillsContext,
		"reading Open content must not mutate the promoted snapshot")
	tl.FreezeAll()
	entries = tl.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindSkillContent]
	require.Len(t, entries, 2, "different paths stay separate after promotion")
	rendered := tl.RenderSkillContentForPrompt()
	require.Equal(t, 1, strings.Count(rendered, "=== Skill: demo-skill ==="))
	require.Contains(t, rendered, "RESOURCE_PAGE")
	require.NotContains(t, rendered, "[source: user-forced]", "source is shown on metadata, not repeated on content ranges")

	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, RenderTimelineFrozenOpen(tl).SkillsContext, RenderTimelineFrozenOpen(restored).SkillsContext)
	require.True(t, tl.DeleteSkillContent("demo-skill"))
	require.Empty(t, RenderTimelineFrozenOpen(tl).SkillsContext)
}

func TestSkillContentFlowsThroughSharedPromptMaterials(t *testing.T) {
	for _, test := range []struct {
		name    string
		options TimelinePromptOptions
	}{
		{name: "main", options: TimelinePromptOptions{PromoteUserInput: true}},
		{name: "helper", options: TimelinePromptOptions{ExcludeToolCache: true}},
		{name: "lightweight", options: TimelinePromptOptions{UserInputOnly: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true))
			tl := cfg.GetTimeline()
			tl.SetTimelineBucketByteSize(-1)
			require.True(t, tl.PushSkillFileContent(cfg.AcquireId(), "demo-skill", "SKILL.md", "SHARED_SKILL_BODY", SkillContentSourceManual, 1, 1, 1))
			for _, frozen := range []bool{false, true} {
				if frozen {
					tl.FreezeAll()
				}
				boundary := tl.FreezeSnapshot()
				frozenOpen := BuildPromptFrozenOpenMaterialsWithOptions(cfg, test.options)
				materials := &PromptMaterials{Nonce: "shared-skill"}
				ApplyPromptFrozenOpenMaterials(materials, frozenOpen)
				prompt, err := NewDefaultPromptPrefixBuilder().AssemblePromptPrefix(materials)
				require.NoError(t, err)
				require.Equal(t, 1, strings.Count(prompt.Prompt, "SHARED_SKILL_BODY"))
				require.NotContains(t, prompt.FrozenBlock, "SHARED_SKILL_BODY")
				if frozen {
					require.Contains(t, prompt.SemiDynamic, "SHARED_SKILL_BODY")
					require.NotContains(t, prompt.TimelineOpen, "SHARED_SKILL_BODY")
				} else {
					require.Empty(t, materials.SkillsContext)
					require.Contains(t, prompt.TimelineOpen, "SHARED_SKILL_BODY")
				}
				require.Equal(t, boundary, tl.FreezeSnapshot(), "prompt assembly must not freeze content")
			}
		})
	}
}

func TestSkillPromptSnapshotStaysCompleteDuringFreeze(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(-1)
	require.True(t, tl.PushSkillFileContent(1, "demo-skill", "SKILL.md", "ATOMIC_SKILL_BODY", SkillContentSourceManual, 1, 1, 1))
	var wg sync.WaitGroup
	errors := make(chan int, 4)
	ready := make(chan struct{}, 4)
	start := make(chan struct{})
	for i := 0; i < cap(errors); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			for j := 0; j < 100; j++ {
				view := RenderTimelineFrozenOpen(tl)
				count := strings.Count(view.Open+view.SkillsContext, "ATOMIC_SKILL_BODY")
				if count != 1 {
					errors <- count
					return
				}
			}
		}()
	}
	for i := 0; i < cap(ready); i++ {
		<-ready
	}
	close(start)
	tl.FreezeAll()
	wg.Wait()
	close(errors)
	for count := range errors {
		require.Equal(t, 1, count, "one snapshot must not lose or duplicate content during transfer")
	}
}
