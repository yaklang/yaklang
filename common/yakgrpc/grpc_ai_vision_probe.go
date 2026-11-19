package yakgrpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	imagedraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s *Server) ProbeModelVision(ctx context.Context, req *ypb.ProbeModelVisionRequest) (*ypb.ProbeModelVisionResponse, error) {
	return probeModelVision(ctx, req, ai.Chat)
}

func probeModelVision(ctx context.Context, req *ypb.ProbeModelVisionRequest, chat func(string, ...aispec.AIConfigOption) (string, error)) (*ypb.ProbeModelVisionResponse, error) {
	if req.GetConfig() == nil || strings.TrimSpace(req.GetModel()) == "" || !ai.HaveAI(req.GetConfig().GetType()) {
		return nil, grpcstatus.Error(codes.InvalidArgument, "a supported provider and model are required")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	result := &ypb.ProbeModelVisionResponse{}
	var err error
	result.SupportsImageInput, result.ImageInputStatus, err = probeVisionRequest(ctx, req, chat, false)
	if err != nil {
		return nil, err
	}
	if result.SupportsImageInput != nil && !result.GetSupportsImageInput() {
		result.SupportsImageToolCalling = proto.Bool(false)
		result.ImageToolCallingStatus = "image_unsupported"
		return result, nil
	}
	if result.SupportsImageInput == nil || ctx.Err() != nil {
		result.ImageToolCallingStatus = "unverified"
		return result, nil
	}
	result.SupportsImageToolCalling, result.ImageToolCallingStatus, err = probeVisionRequest(ctx, req, chat, true)
	if err != nil {
		return nil, err
	}
	return result, nil
}

const visionProbeTool = "report_fixture_code"

func probeVisionRequest(ctx context.Context, req *ypb.ProbeModelVisionRequest, chat func(string, ...aispec.AIConfigOption) (string, error), tools bool) (*bool, string, error) {
	number, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return nil, "", err
	}
	code := fmt.Sprint(100000 + number.Int64())
	fixture, err := renderVisionProbeImage(code)
	if err != nil {
		return nil, "", err
	}

	var mu sync.Mutex
	var statusCode int
	var providerError string
	var output, arguments strings.Builder
	var toolID, toolName string
	prompt := "Read the six digits printed in the attached image. Reply with only those digits."
	options := aispec.BuildOptionsFromConfig(&ypb.AIModelConfig{Provider: req.GetConfig(), ModelName: req.GetModel()})
	options = append(options, aispec.WithContext(ctx), aispec.WithTimeout(12),
		aispec.WithDisableProviderFallback(true), aispec.WithMaxTokens(1024),
		aispec.WithEnableThinking(false), aispec.WithImageRaw(fixture),
		aispec.WithStreamHandler(func(r io.Reader) {
			text, _ := io.ReadAll(io.LimitReader(r, 8192))
			_, _ = io.Copy(io.Discard, r)
			mu.Lock()
			defer mu.Unlock()
			output.Write(text)
		}),
		aispec.WithReasonStreamHandler(func(r io.Reader) { _, _ = io.Copy(io.Discard, r) }),
		aispec.WithRawHTTPRequestResponseCallback(func(_ []byte, header, body []byte, _ *aispec.ChatUsage) {
			mu.Lock()
			defer mu.Unlock()
			statusCode = lowhttp.GetStatusCodeFromResponse(header)
			if statusCode >= 400 {
				providerError = string(body)
			}
		}),
	)
	if tools {
		prompt = "Read the six digits printed in the attached image. Call report_fixture_code with those digits as code."
		options = append(options, aispec.WithToolChoice("auto"),
			aispec.WithTools([]aispec.Tool{{Type: "function", Function: aispec.ToolFunction{
				Name: visionProbeTool, Description: "Report the digits printed in the synthetic test image",
				Parameters: map[string]any{
					"type": "object", "properties": map[string]any{"code": map[string]any{"type": "string"}},
					"required": []string{"code"}, "additionalProperties": false,
				},
			}}}),
			aispec.WithToolCallCallback(func(calls []*aispec.ToolCall) {
				mu.Lock()
				defer mu.Unlock()
				for _, call := range calls {
					if call == nil || call.Index != 0 {
						continue
					}
					if call.ID != "" {
						toolID = call.ID
					}
					if call.Function.Name != "" {
						toolName = call.Function.Name
					}
					if arguments.Len() < 8192 {
						arguments.WriteString(call.Function.Arguments)
					}
				}
			}),
		)
	}
	result, err := chat(prompt, options...)
	mu.Lock()
	defer mu.Unlock()
	if ctx.Err() != nil {
		return nil, "timeout", nil
	}
	if statusCode == 429 {
		return nil, "rate_limited", nil
	}
	if statusCode == 400 || statusCode == 422 {
		upstream := fmt.Errorf("HTTP %d: %s", statusCode, providerError)
		if aispec.IsUnsupportedImageInput(upstream) {
			return proto.Bool(false), "image_unsupported", nil
		}
		message := strings.ToLower(providerError)
		if tools && (strings.Contains(message, "does not support tools") || strings.Contains(message, "does not support tool calling") || strings.Contains(message, "tools are not supported")) {
			return proto.Bool(false), "tools_unsupported", nil
		}
	}
	if aispec.IsUnsupportedImageInput(err) {
		return proto.Bool(false), "image_unsupported", nil
	}
	if err != nil || statusCode >= 400 {
		return nil, "request_failed", nil
	}
	if tools {
		var params struct {
			Code string `json:"code"`
		}
		if toolID != "" && toolName == visionProbeTool && json.Unmarshal([]byte(arguments.String()), &params) == nil && params.Code == code {
			return proto.Bool(true), "verified", nil
		}
	} else {
		if output.Len() > 0 {
			result = output.String()
		}
		if strings.TrimSpace(result) == code {
			return proto.Bool(true), "verified", nil
		}
	}
	return nil, "unexpected_output", nil
}

func renderVisionProbeImage(code string) ([]byte, error) {
	glyphs := image.NewRGBA(image.Rect(0, 0, 60, 22))
	draw.Draw(glyphs, glyphs.Bounds(), image.White, image.Point{}, draw.Src)
	label := font.Drawer{Dst: glyphs, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(9, 16)}
	label.DrawString(code)
	frame := image.NewRGBA(image.Rect(0, 0, 420, 154))
	imagedraw.NearestNeighbor.Scale(frame, frame.Bounds(), glyphs, glyphs.Bounds(), draw.Src, nil)
	var pngBytes bytes.Buffer
	err := png.Encode(&pngBytes, frame)
	return pngBytes.Bytes(), err
}
