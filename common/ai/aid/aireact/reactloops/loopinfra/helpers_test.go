package loopinfra

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoopInfraExtractLineRange(t *testing.T) {
	content := "a\nb\nc\nd"
	assert.Equal(t, "b", loopInfraExtractLineRange(content, 2, 2))
	assert.Equal(t, "b\nc", loopInfraExtractLineRange(content, 2, 3))
	assert.Equal(t, "", loopInfraExtractLineRange(content, 10, 11))
}

func TestLoopInfraFormatSegmentDiff_UnifiedDiff(t *testing.T) {
	diff := loopInfraFormatSegmentDiff("old\nline", "new\nline")
	require.NotEmpty(t, diff)
	assert.Contains(t, diff, "--- a/content")
	assert.Contains(t, diff, "+++ b/content")
	assert.Contains(t, diff, "-old")
	assert.Contains(t, diff, "+new")
	assert.Contains(t, diff, " line") // unchanged context line
}

func TestLoopInfraFormatSegmentDiff_SingleLineReplace(t *testing.T) {
	diff := loopInfraFormatSegmentDiff("b", "B")
	require.NotEmpty(t, diff)
	assert.Contains(t, diff, "-b")
	assert.Contains(t, diff, "+B")
}

func TestLoopInfraFormatSegmentDiff_InsertOnly(t *testing.T) {
	diff := loopInfraFormatSegmentDiff("", "b")
	require.NotEmpty(t, diff)
	assert.Contains(t, diff, "+b")
}

func TestLoopInfraFormatSegmentDiff_DeleteOnly(t *testing.T) {
	diff := loopInfraFormatSegmentDiff("b", "")
	require.NotEmpty(t, diff)
	assert.Contains(t, diff, "-b")
}

func TestLoopInfraFormatSegmentDiff_IdenticalEmpty(t *testing.T) {
	assert.Empty(t, loopInfraFormatSegmentDiff("same", "same"))
}

func TestLoopInfraFormatFileOpTimeline_Modify(t *testing.T) {
	body := loopInfraFormatFileOpTimeline(loopInfraFileOpTimeline{
		Op:         "modify",
		Filename:   "/tmp/test.yak",
		OldSegment: "old",
		NewSegment: "new",
		StartLine:  2,
		EndLine:    2,
	})
	assert.Contains(t, body, "File: /tmp/test.yak")
	assert.Contains(t, body, "Operation: modify lines 2-2")
	assert.Contains(t, body, "-old")
	assert.Contains(t, body, "+new")
}

func TestLoopInfraFormatFileOpTimeline_Insert(t *testing.T) {
	body := loopInfraFormatFileOpTimeline(loopInfraFileOpTimeline{
		Op:         "insert",
		Filename:   "/tmp/test.yak",
		NewSegment: "println(\"x\")",
		InsertLine: 3,
	})
	assert.Contains(t, body, "Operation: insert at line 3")
	assert.Contains(t, body, `+println("x")`)
}

func TestLoopInfraFormatFileOpTimeline_Delete(t *testing.T) {
	body := loopInfraFormatFileOpTimeline(loopInfraFileOpTimeline{
		Op:         "delete",
		Filename:   "/tmp/test.yak",
		OldSegment: "dead code",
		StartLine:  5,
		EndLine:    5,
	})
	assert.Contains(t, body, "Operation: delete line 5")
	assert.Contains(t, body, "-dead code")
}

func TestLoopInfraFormatFileOpTimeline_Write(t *testing.T) {
	body := loopInfraFormatFileOpTimeline(loopInfraFileOpTimeline{
		Op:         "write",
		Filename:   "/tmp/out.yak",
		NewSegment: "println(\"hi\")",
	})
	assert.Contains(t, body, "Operation: write")
	assert.True(t, strings.Contains(body, "println"))
}

func TestLoopInfraFormatFileOpTimeline_DeferredNote(t *testing.T) {
	body := loopInfraFormatFileOpTimeline(loopInfraFileOpTimeline{
		Op:       "write",
		Filename: "/tmp/out.yak",
		Deferred: true,
	})
	assert.Contains(t, body, "disk write deferred for frontend review")
}
