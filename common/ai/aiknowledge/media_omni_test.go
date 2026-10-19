package aiknowledge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestKnowledgeOmniBuildAndAsyncFailure(t *testing.T) {
	db, options, _, _ := fixture(t)
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	var failMedia atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		messages, _ := json.Marshal(body["messages"])
		require.Contains(t, string(messages), "image_url")
		text := "OMNI_READY"
		if !bytes.Contains(messages, []byte("OMNI_READY")) {
			if failMedia.Load() {
				w.WriteHeader(503)
				fmt.Fprint(w, `{"error":{"message":"EXPECTED_OMNI_FAILURE"}}`)
				return
			}
			text = "The recovery code is ORBIT-731."
		}
		w.Header().Set("Content-Type", "text/event-stream")
		payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": text}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
	}))
	t.Cleanup(upstream.Close)
	model := &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", BaseURL: upstream.URL, APIKey: "fixture-key"}, ModelName: "qwen-test-omni"}
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, IntelligentConfigs: []*ypb.AIModelConfig{model}, LightweightConfigs: []*ypb.AIModelConfig{model}, VisionConfigs: []*ypb.AIModelConfig{model}})
	path := filepath.Join(t.TempDir(), "media.png")
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	require.NoError(t, os.WriteFile(path, raw.Bytes(), 0600))
	create(t, options, "media")
	buildOptions := append(append([]Option{}, options...), WithBuildMode("chunks"), WithOmniMode("required"), WithOmniModel("qwen-test-omni"), WithOmniSegmentSeconds(3), WithOmniPayloadBytes(4096))
	result, err := BuildFromFile("media", path, buildOptions...)
	require.NoError(t, err)
	require.Equal(t, 1, result["entries_added"])
	require.Equal(t, "required", result["omni_mode"])
	var entries []schema.KnowledgeBaseEntry
	require.NoError(t, db.Find(&entries).Error)
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].KnowledgeDetails, "ORBIT-731")
	require.Contains(t, entries[0].KnowledgeDetails, path)
	// A failure after the successful probe must propagate through BuildFromFile rather
	// than turning into an empty or partially successful build response.
	failMedia.Store(true)
	_, err = BuildFromFile("media", path, buildOptions...)
	require.ErrorContains(t, err, "EXPECTED_OMNI_FAILURE")
	entries = nil
	require.NoError(t, db.Find(&entries).Error)
	require.Len(t, entries, 1, "a failed extraction must not insert an entry")
}

func TestKnowledgeOmniBuildOptionValidation(t *testing.T) {
	_, options, _, _ := fixture(t)
	create(t, options, "validation")
	for _, invalid := range []Option{
		WithOmniMode("unknown"), WithOmniSegmentSeconds(-1), WithOmniSegmentSeconds(601), WithOmniPayloadBytes(100), WithOmniPayloadBytes(11 * 1024 * 1024),
	} {
		_, err := Build("validation", "plain text", append(append([]Option{}, options...), invalid)...)
		require.Error(t, err)
	}
}
