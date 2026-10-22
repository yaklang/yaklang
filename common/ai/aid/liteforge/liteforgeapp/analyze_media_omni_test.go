package liteforgeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func omniTestImage(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 7), B: uint8(x ^ y), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	path := filepath.Join(t.TempDir(), "knowledge.png")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0600))
	return path
}

func omniTestProvider(t *testing.T, status func(int32) int, inspect func(map[string]any)) (AnalysisOption, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		require.Equal(t, "omni-test", r.Header.Get("X-Knowledge-Test"))
		require.Equal(t, "qwen-test-omni", body["model"])
		n := calls.Add(1)
		if code := status(n); code != http.StatusOK {
			w.WriteHeader(code)
			fmt.Fprint(w, `{"error":{"message":"Omni unavailable"}}`)
			return
		}
		if inspect != nil {
			inspect(body)
		}
		answer := "OMNI_READY"
		if n > 1 {
			answer = "# Redhaze recovery\nThe recovery code is ORBIT-731. Preserve this code in the knowledge base."
		}
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": answer}}}})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": answer}}}})
		}
	}))
	t.Cleanup(server.Close)
	return WithOmniMediaAIOptions(aispec.WithType("openai"), aispec.WithBaseURL(server.URL), aispec.WithAPIKey("fixture-key"),
		aispec.WithModel("qwen-test-omni"), aispec.WithExtraHeader(map[string]string{"X-Knowledge-Test": "omni-test"}), aispec.WithProxy("")), calls
}

func omniTestLegacy(calls *atomic.Int32) AnalysisOption {
	return WithVisionAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls.Add(1)
		rsp := c.NewAIResponse()
		rsp.EmitOutputStream(strings.NewReader(`{"@action":"object","cumulative_summary":"Legacy image knowledge","text_elements":[{"id":"code","text":"LEGACY-731","role":"paragraph","bounding_box":[0,0,1,1]}]}`))
		rsp.Close()
		return rsp, nil
	})
}

func TestOmniMediaLocalProviderCredentials(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })

	for _, modelName := range []string{DefaultKnowledgeOmniModel, "custom-omni"} {
		t.Run(modelName, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "/v1/chat/completions", r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer configured-user-key" {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
					return
				}
				require.Equal(t, "local-provider", r.Header.Get("X-Omni-Test"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, modelName, body["model"])
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"OMNI_READY"}}]}`)
			}))
			t.Cleanup(server.Close)
			builtin := yakit.DefaultAIBalanceProviderConfig()
			builtin.Domain, builtin.BaseURL = "", server.URL+"/v1"
			consts.SetTieredAIConfig(&consts.TieredAIConfig{
				VisionConfigs:      []*ypb.AIModelConfig{{Provider: builtin, ModelName: "memfit-vision-free"}},
				LightweightConfigs: []*ypb.AIModelConfig{{Provider: builtin, ModelName: "memfit-light-free"}},
				IntelligentConfigs: []*ypb.AIModelConfig{{ModelName: "deepseek-v4.1-flash", Provider: &ypb.ThirdPartyApplicationConfig{
					Type: "aibalance", APIKey: "configured-user-key", BaseURL: server.URL + "/v1",
					Headers: []*ypb.KVPair{{Key: "X-Omni-Test", Value: "local-provider"}},
				}}},
			})
			info, err := CheckOmniAvailable(WithVideoOmniModel(modelName))
			require.NoError(t, err)
			require.Equal(t, &OmniModelInfo{Provider: "aibalance", Model: modelName}, info)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestOmniMediaBuiltinFreeCredentials(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	aiconfig.EnsureConfigLoaded()
	previous := consts.GetTieredAIConfig()
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "Bearer explicit-user-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"OMNI_READY"}}]}`)
	}))
	t.Cleanup(server.Close)
	builtin := yakit.DefaultAIBalanceProviderConfig()
	builtin.Domain, builtin.BaseURL = "", server.URL+"/v1"
	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		VisionConfigs: []*ypb.AIModelConfig{{Provider: builtin, ModelName: "memfit-vision-free"}},
	})
	_, err := CheckOmniAvailable()
	require.ErrorContains(t, err, "requires a configured AIBalance API key")
	require.Zero(t, calls.Load(), "do not send known incompatible free credentials to Omni")
	info, err := CheckOmniAvailable(WithOmniMediaAIOptions(aispec.WithAPIKey("explicit-user-key")))
	require.NoError(t, err)
	require.Equal(t, DefaultKnowledgeOmniModel, info.Model)
	require.EqualValues(t, 1, calls.Load(), "explicit credentials must retain the configured endpoint")
}

func TestOmniMediaImageAndTextRouting(t *testing.T) {
	provider, calls := omniTestProvider(t, func(int32) int { return 200 }, func(body map[string]any) {
		encoded, err := json.Marshal(body["messages"])
		require.NoError(t, err)
		require.Contains(t, string(encoded), "image_url")
	})
	path := omniTestImage(t)
	ch, err := AnalyzeFile(path, provider, WithOmniMediaMode("required"))
	require.NoError(t, err)
	var results []AnalysisResult
	for result := range ch {
		results = append(results, result)
	}
	require.Len(t, results, 1)
	r := results[0].(*OmniMediaAnalysisResult)
	require.Equal(t, "qwen-test-omni", r.Model)
	require.Equal(t, path, r.Source)
	require.Contains(t, r.Dump(), "ORBIT-731")
	require.False(t, r.Fallback)
	require.EqualValues(t, 2, calls.Load(), "probe once, then analyze the image")
	text := filepath.Join(t.TempDir(), "note.txt")
	require.NoError(t, os.WriteFile(text, []byte("plain text stays on the text path"), 0600))
	ch, err = AnalyzeFile(text, provider)
	require.NoError(t, err)
	for result := range ch {
		require.IsType(t, &TextAnalysisResult{}, result)
	}
	require.EqualValues(t, 2, calls.Load(), "text must not probe or invoke Omni")
}

func TestOmniMediaFallbackAndRequired(t *testing.T) {
	for _, test := range []struct {
		name       string
		mode       string
		probeFails bool
		wantLegacy int32
	}{
		{"unavailable", "auto", true, 1},
		{"request_failure", "auto", false, 1},
		{"required_unavailable", "required", true, 0},
		{"required_request_failure", "required", false, 0},
		{"disabled", "disabled", true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider, _ := omniTestProvider(t, func(n int32) int {
				if test.probeFails || n > 1 {
					return 503
				}
				return 200
			}, nil)
			var legacyCalls atomic.Int32
			var errors []error
			ch, err := AnalyzeFile(omniTestImage(t), provider, WithOmniMediaMode(test.mode), omniTestLegacy(&legacyCalls),
				WithAnalyzeErrorHandler(func(err error) { errors = append(errors, err) }), aicommon.WithDisableCreateDBRuntime(true))
			if test.mode == "required" && test.probeFails {
				require.ErrorContains(t, err, "probe failed")
				return
			}
			require.NoError(t, err)
			var results []AnalysisResult
			for r := range ch {
				results = append(results, r)
			}
			require.Equal(t, test.wantLegacy, legacyCalls.Load())
			if test.mode == "required" {
				require.Empty(t, results)
				require.NotEmpty(t, errors)
			} else {
				require.Len(t, results, 1)
				require.Contains(t, results[0].Dump(), "LEGACY-731")
				require.Empty(t, errors, "successful fallback must not fail the build")
			}
		})
	}
}

func TestOmniMediaImagePayloadLimit(t *testing.T) {
	raw, err := os.ReadFile(omniTestImage(t))
	require.NoError(t, err)
	compressed, err := boundedOmniImage(raw, 6000)
	require.NoError(t, err)
	require.LessOrEqual(t, len("data:image/jpeg;base64,")+len(base64.StdEncoding.EncodeToString(compressed)), 6000)
	_, err = boundedOmniImage(raw, 10)
	require.Error(t, err)
}

func TestOmniMediaVideoSegments(t *testing.T) {
	ffmpeg := consts.GetFfmpegPath()
	if ffmpeg == "" {
		ffmpeg = os.Getenv("YAK_OMNI_TEST_FFMPEG")
	}
	if ffmpeg == "" {
		ffmpeg, _ = exec.LookPath("ffmpeg")
	}
	if ffmpeg == "" {
		t.Skip("ffmpeg is required")
	}
	t.Setenv("PATH", filepath.Dir(ffmpeg)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "knowledge.mp4")
	data, err := exec.CommandContext(ctx, ffmpeg, "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=16000", "-t", "5", "-c:v", "libx264", "-c:a", "aac", path).CombinedOutput()
	require.NoError(t, err, string(data))
	provider, calls := omniTestProvider(t, func(int32) int { return 200 }, func(body map[string]any) {
		messages, _ := json.Marshal(body["messages"])
		if !bytes.Contains(messages, []byte("OMNI_READY")) {
			require.Contains(t, string(messages), "video_url", "the gateway must transmit the video")
		}
	})
	var errors []error
	ch, err := AnalyzeFile(path, provider, WithOmniMediaMode("required"), WithAnalyzeContext(ctx), WithVideoOmniSegmentSeconds(2), WithVideoOmniTargetFPS(2),
		WithAnalyzeErrorHandler(func(err error) { errors = append(errors, err) }))
	require.NoError(t, err)
	var clips []*OmniMediaAnalysisResult
	for r := range ch {
		clips = append(clips, r.(*OmniMediaAnalysisResult))
	}
	require.Empty(t, errors)
	require.Len(t, clips, 3, "keyframes must bound the two-second clips")
	require.EqualValues(t, 4, calls.Load())
	for i, clip := range clips {
		require.Equal(t, path, clip.Source)
		require.InDelta(t, float64(i*2), clip.StartTime.Seconds(), 0.2)
		require.LessOrEqual(t, (clip.EndTime - clip.StartTime).Seconds(), 2.2)
	}
	require.InDelta(t, 5, clips[2].EndTime.Seconds(), 0.2)
	t.Run("bounded payload with audio", func(t *testing.T) {
		const limit = 40 * 1024
		provider, calls := omniTestProvider(t, func(int32) int { return 200 }, func(body map[string]any) {
			seenVideo := false
			for _, message := range body["messages"].([]any) {
				content, _ := message.(map[string]any)["content"].([]any)
				for _, part := range content {
					video, ok := part.(map[string]any)["video_url"].(map[string]any)
					if !ok {
						continue
					}
					uri := video["url"].(string)
					seenVideo = true
					require.LessOrEqual(t, len(uri), limit)
					raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:;base64,"))
					require.NoError(t, err)
					require.Contains(t, string(raw), "soun", "keep the audio track")
					require.Contains(t, string(raw), "vide", "keep the video track")
				}
			}
			messages, _ := json.Marshal(body["messages"])
			if !bytes.Contains(messages, []byte("OMNI_READY")) {
				require.True(t, seenVideo, "a video request must actually carry a video")
			}
		})
		var errors []error
		ch, err := AnalyzeFile(path, provider, WithAnalyzeContext(ctx), WithOmniMediaMode("required"), WithVideoOmniSegmentSeconds(5), WithVideoOmniMaxBase64Bytes(limit),
			WithAnalyzeErrorHandler(func(err error) { errors = append(errors, err) }))
		require.NoError(t, err)
		count := 0
		for range ch {
			count++
		}
		require.Empty(t, errors)
		require.Greater(t, count, 1, "oversized clips must be subdivided")
		require.EqualValues(t, count+1, calls.Load())
	})
}

func omniTestPDF(t *testing.T) string {
	t.Helper()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 4 0 R >>",
		"", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R >>", "",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	for page, index := range []int{3, 5} {
		// The first page exercises the page2img quality setting which can produce
		// JPEG short Huffman data; ffmpeg normalization must preserve that page.
		stream := "BT /F1 18 Tf 50 700 Td (Redhaze recovery code: ORBIT-731) Tj 0 -40 Td (Stop worker, restore snapshot, restart worker.) Tj ET"
		if page == 1 {
			stream = "BT /F1 18 Tf 50 700 Td (Verification: ORBIT-731) Tj 0 -40 Td (Check service health after recovery.) Tj ET"
		}
		objects[index] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	path := filepath.Join(t.TempDir(), "recovery.pdf")
	require.NoError(t, os.WriteFile(path, pdf.Bytes(), 0600))
	return path
}

func TestOmniMediaPDFPages(t *testing.T) {
	if dir := os.Getenv("YAK_OMNI_TEST_TOOLS"); dir != "" {
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if consts.GetPage2ImgBinaryPath() == "" {
		if _, err := exec.LookPath("page2img"); err != nil {
			t.Skip("page2img is required")
		}
	}
	provider, calls := omniTestProvider(t, func(int32) int { return 200 }, nil)
	var errors []error
	ch, err := AnalyzeFile(omniTestPDF(t), provider, WithOmniMediaMode("required"), WithAnalyzeErrorHandler(func(err error) { errors = append(errors, err) }))
	require.NoError(t, err)
	var pages []int
	for item := range ch {
		result := item.(*OmniMediaAnalysisResult)
		pages = append(pages, result.Page)
		require.Contains(t, result.Dump(), fmt.Sprintf("Page: %d", result.Page))
	}
	require.Empty(t, errors)
	require.ElementsMatch(t, []int{1, 2}, pages)
	require.EqualValues(t, 3, calls.Load(), "probe once for the whole PDF")
}

func TestOmniMediaCancelledProbeDoesNotFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider, _ := omniTestProvider(t, func(int32) int { return 200 }, nil)
	var legacy atomic.Int32
	ch, err := AnalyzeFile(omniTestImage(t), provider, WithAnalyzeContext(ctx), omniTestLegacy(&legacy))
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, ch)
	require.Zero(t, legacy.Load())
}

func TestOmniMediaTransientRetryBudget(t *testing.T) {
	for _, firstErr := range []error{io.ErrUnexpectedEOF, fmt.Errorf("401 invalid api key"), context.Canceled} {
		t.Run(firstErr.Error(), func(t *testing.T) {
			calls := 0
			var deadline time.Time
			s := &omniMediaSession{config: NewAnalysisConfig(), video: NewDefaultVideoOmniConfig(), info: OmniModelInfo{Model: "fixture"}}
			s.video.Timeout = time.Second
			s.chat = func(_ string, options ...aispec.AIConfigOption) (string, error) {
				cfg := aispec.NewDefaultAIConfig(options...)
				require.EqualValues(t, 4096, *cfg.MaxTokens)
				current, ok := cfg.Context.Deadline()
				require.True(t, ok)
				calls++
				if calls == 1 {
					deadline = current
					return "discard partial output", firstErr
				}
				require.Equal(t, deadline, current, "retry must not reset the timeout budget")
				cfg.StreamHandler(strings.NewReader("ORBIT-731"))
				return "ORBIT-731", nil
			}
			text, err := s.call("video range=20s~40s", "extract", aispec.WithVideoUrl("fixture"))
			if firstErr == io.ErrUnexpectedEOF {
				require.NoError(t, err)
				require.Equal(t, "ORBIT-731", text)
				require.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestOmniMediaVideoFallbackFrameExtraction(t *testing.T) {
	ffmpeg := consts.GetFfmpegPath()
	if ffmpeg == "" {
		ffmpeg = os.Getenv("YAK_OMNI_TEST_FFMPEG")
	}
	if ffmpeg == "" {
		ffmpeg, _ = exec.LookPath("ffmpeg")
	}
	if ffmpeg == "" {
		t.Skip("ffmpeg is required")
	}
	t.Setenv("PATH", filepath.Dir(ffmpeg)+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(t.TempDir(), "fallback.mp4")
	data, err := exec.Command(ffmpeg, "-y", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=5", "-t", "4", "-c:v", "libx264", path).CombinedOutput()
	require.NoError(t, err, string(data))
	provider, _ := omniTestProvider(t, func(n int32) int {
		if n == 1 {
			return 200
		}
		return 503
	}, nil)
	var legacyCalls atomic.Int32
	var failures []error
	ch, err := AnalyzeFile(path, provider, omniTestLegacy(&legacyCalls), aicommon.WithDisableCreateDBRuntime(true), WithAnalyzeErrorHandler(func(err error) { failures = append(failures, err) }))
	require.NoError(t, err)
	count := 0
	for item := range ch {
		count++
		require.True(t, item.(*OmniMediaAnalysisResult).Fallback)
	}
	require.Empty(t, failures)
	require.Positive(t, count, "fallback must produce frames even when Windows timestamp filters crash")
	require.Positive(t, legacyCalls.Load(), "original vision callback must be preserved by the video fallback")
}
