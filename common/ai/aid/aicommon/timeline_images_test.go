package aicommon

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func timelineImageFixture(t *testing.T) *aimedia.Image {
	t.Helper()
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 17, 13))))
	img, err := aimedia.FromBytes(context.Background(), raw.Bytes(), "image/png")
	require.NoError(t, err)
	store, err := aimedia.NewStore(t.TempDir())
	require.NoError(t, err)
	img, err = store.Save(context.Background(), img)
	require.NoError(t, err)
	return img
}

func TestTimelineImagesPersistForkAndRollbackWithTheirSource(t *testing.T) {
	img := timelineImageFixture(t)
	tl := NewTimeline(nil, nil)
	tl.PushTextWithImages(1, "[attached_image]: first user upload", img)
	tl.PushToolResult(&aitool.ToolResult{ID: 2, Name: "screenshot", Success: true, Images: []*aimedia.Image{img}})
	tl.PushImageObservation(3, "tool:screen-1:0", "[image_observation]: visible dialog")
	before := RenderTimelineFrozenOpen(tl)
	require.Len(t, before.Images, 2)
	require.Equal(t, int64(1), before.Images[0].TimelineItemID)
	require.Equal(t, int64(2), before.Images[1].TimelineItemID)
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	bytes, err := img.Bytes(context.Background())
	require.NoError(t, err)
	require.NotContains(t, raw, base64.StdEncoding.EncodeToString(bytes))
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Len(t, RenderTimelineFrozenOpen(restored).Images, 2)
	require.True(t, restored.HasImageObservation("tool:screen-1:0"))
	fork, err := restored.ForkForTask("child", "image child", nil, nil)
	require.NoError(t, err)
	require.NoError(t, fork.Branch.TruncateAfter(1))
	require.Len(t, RenderTimelineFrozenOpen(fork.Branch).Images, 1)
	require.Len(t, RenderTimelineFrozenOpen(restored).Images, 2)
	require.False(t, fork.Branch.HasImageObservation("tool:screen-1:0"))
}

func TestTimelineImagesFollowRenderedBoundaryAndNeverFollowSummaryMentions(t *testing.T) {
	img := timelineImageFixture(t)
	tl := NewTimeline(nil, nil)
	tl.SetTimelineBucketByteSize(-1)
	tl.PushTextWithImages(1, "[attached_image]: source one", img)
	tl.PushToolResult(&aitool.ToolResult{ID: 2, Name: "screenshot", Success: true, Images: []*aimedia.Image{img}})
	through := int64(1)
	view := RenderTimelineFrozenOpenWithOptions(tl, TimelinePromptOptions{ThroughID: &through})
	require.Len(t, view.Images, 1)
	require.Equal(t, int64(1), view.Images[0].TimelineItemID)
	_, err := tl.ApplyCompressionSelection(&TimelineCompressionSelection{Summary: "Earlier screenshot resource " + img.ID + " showed a dialog."}, "")
	require.NoError(t, err)
	view = RenderTimelineFrozenOpen(tl)
	require.Contains(t, view.PromptText(), img.ID)
	require.Empty(t, view.Images, "mentioning a resource ID in a summary must not reload its bytes")
}

func TestTimelineImagesRespectShrunkAndLightweightPromptViews(t *testing.T) {
	img := timelineImageFixture(t)
	tl := NewTimeline(nil, nil)
	tl.PushTextWithImages(1, "[attached_image]: old screenshot", img)
	shrunk := &aitool.ToolResult{ID: 2, Name: "screenshot", Success: true, Images: []*aimedia.Image{img}, ShrinkResult: "Only a textual summary remains."}
	tl.PushToolResult(shrunk)
	view := RenderTimelineFrozenOpen(tl)
	require.Len(t, view.Images, 1)
	require.Equal(t, int64(1), view.Images[0].TimelineItemID)
	recent := tl.DumpRecentOrdinaryWithImages(10000, false)
	require.Len(t, recent.Images, 1)
	tl.PushText(3, "[observation]: "+strings.Repeat("recent text ", 200))
	recent = tl.DumpRecentOrdinaryWithImages(100, false)
	require.NotEmpty(t, recent.Text)
	require.Empty(t, recent.Images, "an omitted image entry must not survive through the lightweight side channel")
}
