package aicommon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
)

func imageArtifactBundle(t *testing.T) *toolCallArtifactBundle {
	t.Helper()
	dir := t.TempDir()
	b := &toolCallArtifactBundle{
		dir: dir, reportPath: filepath.Join(dir, "report.md"),
		combinedPath: filepath.Join(dir, "combined_output.txt"),
		stdoutPath:   filepath.Join(dir, "stdout.txt"), stderrPath: filepath.Join(dir, "stderr.txt"),
		manifestPath: filepath.Join(dir, "manifest.json"),
		preview:      newBoundedHeadTailBuffer(toolCapturePreviewBytes),
	}
	var err error
	b.combined, err = os.Create(b.combinedPath)
	require.NoError(t, err)
	b.stdout, err = os.Create(b.stdoutPath)
	require.NoError(t, err)
	b.stderr, err = os.Create(b.stderrPath)
	require.NoError(t, err)
	t.Cleanup(b.closeStreams)
	return b
}

func TestToolImagesSurviveArtifactShrinkingAndCheckpointJSON(t *testing.T) {
	ctx := context.Background()
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 17, 13))))
	media, err := aitool.ImageResult(ctx, raw.Bytes(), "image/png", strings.Repeat("large output\n", 20_000))
	require.NoError(t, err)
	tool, err := aitool.New("image-artifact", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return media, nil }))
	require.NoError(t, err)
	result, err := tool.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	result.ToolCallID = "capture-1"
	b := imageArtifactBundle(t)
	caller := &ToolCaller{ctx: ctx, emitter: NewEmitter("image-artifact", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) { return event, nil })}
	require.NoError(t, b.finalize(caller, tool, result.ToolCallID, "", nil, result))
	require.IsType(t, "", result.Data)
	require.LessOrEqual(t, ytoken.CalcTokenCount(result.String()), ToolResultTokenLimit)
	require.Len(t, result.Images, 1)
	require.FileExists(t, result.Images[0].Path)
	require.Equal(t, "capture-1", result.Images[0].Source.ToolCallID)
	require.Equal(t, "image-artifact", result.Images[0].Source.ToolName)
	require.NotEmpty(t, result.Images[0].Source.ObservedAt)

	checkpoint, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(checkpoint), base64.StdEncoding.EncodeToString(raw.Bytes()))
	var restored aitool.ToolResult
	require.NoError(t, json.Unmarshal(checkpoint, &restored))
	data, err := restored.Images[0].Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, raw.Bytes(), data)
	manifestData, err := os.ReadFile(b.manifestPath)
	require.NoError(t, err)
	var manifest toolCallArtifactManifest
	require.NoError(t, json.Unmarshal(manifestData, &manifest))
	require.Len(t, manifest.Images, 1)
	require.Equal(t, restored.Images[0].ID, manifest.Images[0].ID)

	// Checkpoint replay must keep the old immutable image file, even when the
	// new invocation's unused artifact staging directory is discarded.
	replay := imageArtifactBundle(t)
	require.NoError(t, replay.finalize(caller, tool, "capture-1", "", nil, &restored))
	data, err = restored.Images[0].Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, raw.Bytes(), data)
}

func TestToolImagePersistenceFailureIsExplicit(t *testing.T) {
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	media, err := aitool.ImageResult(context.Background(), raw.Bytes(), "image/png", "captured")
	require.NoError(t, err)
	tool, err := aitool.New("image-failure", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return media, nil }))
	require.NoError(t, err)
	result, err := tool.InvokeWithParams(map[string]any{})
	require.NoError(t, err)
	b := &toolCallArtifactBundle{prepare: fmt.Errorf("disk unavailable"), preview: newBoundedHeadTailBuffer(toolCapturePreviewBytes)}
	err = b.finalize(&ToolCaller{}, tool, "failed-capture", "", nil, result)
	require.ErrorContains(t, err, "disk unavailable")
	require.False(t, result.Success)
	require.Contains(t, result.Error, "failed to preserve tool images")
	require.Empty(t, result.Images)
}

func TestToolImageRefsAreRemovedIfBundleCommitFails(t *testing.T) {
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	media, err := aitool.ImageResult(context.Background(), raw.Bytes(), "image/png", "captured")
	require.NoError(t, err)
	tool, err := aitool.New("image-commit-failure", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return media, nil }))
	require.NoError(t, err)
	for _, file := range []string{"manifest", "report"} {
		t.Run(file, func(t *testing.T) {
			result, err := tool.InvokeWithParams(map[string]any{})
			require.NoError(t, err)
			b := imageArtifactBundle(t)
			if file == "manifest" {
				b.manifestPath = b.dir // Writing a file over a directory must fail.
			} else {
				b.reportPath = b.dir
			}
			err = b.finalize(&ToolCaller{}, tool, "capture", "", nil, result)
			require.ErrorContains(t, err, "failed to preserve tool images")
			require.False(t, result.Success)
			require.Empty(t, result.Images, "rollback must not leave references to removed files")
			require.Contains(t, result.Data.(string), "artifact_persist_failed")
		})
	}
}
