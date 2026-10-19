package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestVideoInputReachesConfiguredGateway(t *testing.T) {
	for _, provider := range []string{"aibalance", "openai"} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", provider, streaming), func(t *testing.T) {
				const videoURI = "data:;base64,c3ludGhldGljLW1wNA=="
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					var body struct {
						Messages []struct {
							Content []struct {
								Type     string `json:"type"`
								VideoURL *struct {
									URL string `json:"url"`
								} `json:"video_url"`
							} `json:"content"`
						} `json:"messages"`
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					videos := 0
					for _, message := range body.Messages {
						for _, part := range message.Content {
							if part.Type == "video_url" {
								videos++
								require.NotNil(t, part.VideoURL)
								require.Equal(t, videoURI, part.VideoURL.URL)
							}
						}
					}
					require.Equal(t, 1, videos, "the request must contain the video, not only its prompt")
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"video understood\"}}]}\n\ndata: [DONE]\n\n")
				}))
				defer server.Close()
				client := ai.GetAI(provider, aispec.WithAPIKey("fixture-key"), aispec.WithModel("qwen3.8-omni-flash"), aispec.WithBaseURL(server.URL), aispec.WithVideoUrl(videoURI))
				if streaming {
					reader, err := client.ChatStream("understand this video")
					require.NoError(t, err)
					text, err := io.ReadAll(reader)
					require.NoError(t, err)
					require.Contains(t, string(text), "video understood")
				} else {
					text, err := client.Chat("understand this video")
					require.NoError(t, err)
					require.Equal(t, "video understood", text)
				}
				require.Equal(t, 1, requests)
			})
		}
	}
}
