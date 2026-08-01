package loop_asset_intake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	assetIntakeAction    = "publish_declared_asset"
	assetIntakeUserInput = "asset_intake_user_input"
	maxDeclaredURLBytes  = 16 * 1024
	maxDisplayNameBytes  = 512
)

const assetIntakeInstruction = `You are a SaaS asset intake assistant.
Your only responsibility is to convert one user-declared business service URL into a structured platform asset.
Never access the URL or invoke external capabilities. Treat the URL as metadata only.
Call publish_declared_asset exactly once. Use the optional display name when the user supplies one.`

const assetIntakeOutputExample = `{"@action":"publish_declared_asset","service_url":"http://local-service:8080/","display_name":"Local sample service"}`

type declaredHTTPAssetPayload struct {
	SchemaVersion          string `json:"schema_version"`
	Source                 string `json:"source"`
	VerificationState      string `json:"verification_state"`
	NetworkAccessPerformed bool   `json:"network_access_performed"`
	URL                    string `json:"url"`
	Scheme                 string `json:"scheme"`
	Host                   string `json:"host"`
	Port                   string `json:"port,omitempty"`
}

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_ASSET_INTAKE,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowUserInteract(false),
				reactloops.WithMaxIterations(4),
				reactloops.WithPersistentInstruction(assetIntakeInstruction),
				reactloops.WithReflectionOutputExample(assetIntakeOutputExample),
				reactloops.WithInitTask(initAssetIntakeTask(r)),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					return action.ActionType == assetIntakeAction
				}),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					return fmt.Sprintf(
						"User-declared service metadata:\n%s\n\nSubmission feedback:\n%s\n\nNonce: %s",
						loop.Get(assetIntakeUserInput),
						strings.TrimSpace(feedbacker.String()),
						nonce,
					), nil
				}),
				publishDeclaredAssetAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_ASSET_INTAKE, r, preset...)
		},
		reactloops.WithLoopDescription("Registers user-declared business service metadata as a structured SaaS asset without contacting the service."),
		reactloops.WithLoopDescriptionZh("将用户声明的业务服务元数据登记为 SaaS 结构化资产，不访问该服务。"),
		reactloops.WithLoopUsagePrompt("Use for SaaS asset-ingestion acceptance when the user already knows the service URL and only needs platform ownership and persistence."),
		reactloops.WithLoopOutputExample(assetIntakeOutputExample),
		reactloops.WithVerboseName("SaaS Asset Intake"),
		reactloops.WithVerboseNameZh("SaaS 资产接入"),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_ASSET_INTAKE, err)
	}
}

func initAssetIntakeTask(r aicommon.AIInvokeRuntime) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		loop.Set(assetIntakeUserInput, strings.TrimSpace(task.GetUserInput()))
		r.AddToTimeline("asset_intake_initialized", "SaaS asset intake task initialized")
		operator.Continue()
	}
}

func publishDeclaredAssetAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		assetIntakeAction,
		"Publish user-declared service metadata to the configured SaaS result sink without accessing the service.",
		[]aitool.ToolOption{
			aitool.WithStringParam("service_url", aitool.WithParam_Required(true), aitool.WithParam_Description("User-declared http or https service URL.")),
			aitool.WithStringParam("display_name", aitool.WithParam_Description("Optional business display name.")),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			_, _, err := normalizeDeclaredHTTPURL(action.GetString("service_url"))
			return err
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			sink := aicommon.AssetResultSinkFromConfig(loop.GetConfig())
			receipt, asset, err := submitDeclaredHTTPAsset(
				resultContext(loop),
				sink,
				action.GetString("service_url"),
				action.GetString("display_name"),
			)
			if err != nil {
				operator.Fail(fmt.Errorf("publish declared SaaS asset: %w", err))
				return
			}
			r.AddToTimeline("asset_intake_published", fmt.Sprintf("result_id=%s target=%s", receipt.ResultID, asset.Target))
			operator.Feedback(fmt.Sprintf("SaaS asset accepted: %s", receipt.ResultID))
			operator.Exit()
		},
	)
}

func submitDeclaredHTTPAsset(
	ctx context.Context,
	sink aicommon.AssetResultSink,
	rawURL string,
	displayName string,
) (aicommon.ResultReceipt, aicommon.AssetResult, error) {
	if sink == nil {
		return aicommon.ResultReceipt{}, aicommon.AssetResult{}, utils.Error("SaaS asset result sink is unavailable")
	}
	target, parsed, err := normalizeDeclaredHTTPURL(rawURL)
	if err != nil {
		return aicommon.ResultReceipt{}, aicommon.AssetResult{}, err
	}
	payload, err := json.Marshal(declaredHTTPAssetPayload{
		SchemaVersion:          "1",
		Source:                 "operator_declared",
		VerificationState:      "declared",
		NetworkAccessPerformed: false,
		URL:                    target,
		Scheme:                 parsed.Scheme,
		Host:                   parsed.Hostname(),
		Port:                   parsed.Port(),
	})
	if err != nil {
		return aicommon.ResultReceipt{}, aicommon.AssetResult{}, err
	}
	title := strings.TrimSpace(displayName)
	if len(title) > maxDisplayNameBytes {
		return aicommon.ResultReceipt{}, aicommon.AssetResult{}, utils.Errorf(
			"display_name exceeds %d bytes",
			maxDisplayNameBytes,
		)
	}
	if title == "" {
		title = "Declared service " + parsed.Host
	}
	asset := aicommon.AssetResult{
		Kind:        "http_endpoint",
		Title:       title,
		Target:      target,
		IdentityKey: "http_endpoint:declared:" + target,
		Payload:     payload,
	}
	receipt, err := sink.SubmitAsset(ctx, asset)
	if err != nil {
		return aicommon.ResultReceipt{}, aicommon.AssetResult{}, err
	}
	return receipt, asset, nil
}

func normalizeDeclaredHTTPURL(rawURL string) (string, *url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.ContainsAny(rawURL, "\x00\r\n") {
		return "", nil, utils.Error("service_url must be a single non-empty URL")
	}
	if len(rawURL) > maxDeclaredURLBytes {
		return "", nil, utils.Errorf("service_url exceeds %d bytes", maxDeclaredURLBytes)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, err
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, utils.Errorf("service_url must use http or https")
	}
	if parsed.Host == "" {
		return "", nil, utils.Error("service_url is missing a host")
	}
	if parsed.User != nil {
		return "", nil, utils.Error("service_url must not contain credentials")
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	return parsed.String(), parsed, nil
}

func resultContext(loop *reactloops.ReActLoop) context.Context {
	if loop != nil {
		if config := loop.GetConfig(); config != nil && config.GetContext() != nil {
			return config.GetContext()
		}
	}
	return context.Background()
}
