package aicommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestMainModelImageWireChatAndResponses(t *testing.T) {
	for _, api := range []string{"chat_completions", "responses"} {
		t.Run(api, func(t *testing.T) {
			captured := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				captured <- body
				w.Header().Set("Content-Type", "text/event-stream")
				if api == "responses" {
					fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"seen\",\"item_id\":\"m1\",\"output_index\":0}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":400,\"output_tokens\":1}}}\n\n")
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"seen\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":400,\"completion_tokens\":1,\"total_tokens\":401}}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			cb, err := LoadAIService("openai", aispec.WithBaseURL(server.URL), aispec.WithAPIType(api), aispec.WithAPIKey("local-test-key"), aispec.WithModel("main-multimodal"))
			require.NoError(t, err)
			cfg := NewConfig(context.Background(), WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true), WithAICallback(cb), WithAIAutoRetry(1))
			var imageBytes bytes.Buffer
			require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 8, 8))))
			prompt := aiprojection.CreateTemplate("<|AI_CACHE_SYSTEM_high-static|>\nInspect visible evidence.\n<|AI_CACHE_SYSTEM_END_high-static|>\n"+
				"<|PROMPT_SECTION_dynamic_fixture|>\nCurrent turn: inspect image 1 after clicking Save\n<|PROMPT_SECTION_dynamic_END_fixture|>") +
				"\n<image_attachments>\nAttached image 1: timeline_item=42 image_id=fixture-screen\n</image_attachments>"
			req := NewAIRequest(prompt, WithAIRequest_DetachCheckpoint(), WithAIRequest_ImageData(&ImageData{Data: imageBytes.Bytes()}), WithAIRequest_ExtraSpecOpts(func(c *aispec.AIConfig) { c.PreserveImageModel = true }))
			resp, err := cfg.CallAI(req)
			require.NoError(t, err)
			output, err := io.ReadAll(resp.GetOutputStreamReader("wire", true, cfg.GetEmitter()))
			require.NoError(t, err)
			require.Contains(t, string(output), "seen")
			wire := <-captured
			require.Equal(t, "main-multimodal", wire["model"])
			raw, _ := json.Marshal(wire)
			require.Contains(t, string(raw), "data:image/png;base64,")
			require.Contains(t, string(raw), "after clicking Save")
			require.Contains(t, string(raw), "timeline_item=42")
			messageKey := "messages"
			if api == "responses" {
				messageKey = "input"
			}
			messages, ok := wire[messageKey].([]any)
			require.True(t, ok)
			associated := false
			for _, message := range messages {
				value, _ := message.(map[string]any)
				content, _ := json.Marshal(value["content"])
				if bytes.Contains(content, []byte("data:image/png;base64,")) {
					require.Equal(t, "user", value["role"])
					require.Contains(t, string(content), "timeline_item=42", "projection must keep the manifest with its image attachment")
					associated = true
				}
			}
			require.True(t, associated)
			if api == "responses" {
				require.Contains(t, string(raw), "input_image")
			} else {
				require.Contains(t, string(raw), "image_url")
			}
		})
	}
}
