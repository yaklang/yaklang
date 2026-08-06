package browsercrypto

import (
	"context"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/browsertools"
	"github.com/yaklang/yaklang/common/browser"
)

func packetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"url", "headers", "bodyBase64"},
		"properties": map[string]any{
			"method": map[string]any{"type": "string", "maxLength": 32},
			"url":    map[string]any{"type": "string", "format": "uri", "maxLength": 8192},
			"statusCode": map[string]any{
				"type": "integer", "minimum": 100, "maximum": 999,
			},
			"headers": map[string]any{
				"type": "array", "maxItems": 512,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"name", "value"},
					"properties": map[string]any{
						"name":  map[string]any{"type": "string", "maxLength": 512},
						"value": map[string]any{"type": "string", "maxLength": 1000000},
					},
				},
			},
			"bodyBase64": map[string]any{
				"type": "string", "maxLength": 11184820,
			},
		},
	}
}

func buildTools(
	caller browsertools.Caller,
	deviceID string,
	target browsertools.Target,
	capabilityCatalog *browser.ExtensionBridgeCapabilityCatalog,
) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	register := func(
		name string,
		description string,
		usage string,
		timeout time.Duration,
		withTarget bool,
		options ...aitool.ToolOption,
	) error {
		bridgeMethod := "browser." + name
		callback := aitool.WithNoRuntimeCallback(func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			return browsertools.CallCapability(
				ctx,
				caller,
				deviceID,
				target,
				bridgeMethod,
				params,
				timeout,
				withTarget,
			)
		})
		common := []aitool.ToolOption{
			aitool.WithDescription(description),
			aitool.WithUsage(usage),
			aitool.WithKeywords([]string{
				"browser", "frontend cryptography", "plaintext gateway", "evidence",
				"浏览器", "前端加密", "明文网关",
			}),
			callback,
		}
		return factory.RegisterTool(name, append(common, options...)...)
	}

	if err := register(
		"recording.trace.list",
		"List recent business traces, request boundaries, cryptographic-call counts, and inference candidates for the shared page. Returns metadata only.",
		"Start here for a plaintext-gateway investigation. Select the trace closest to the user's operation that contains the target request before inspecting evidence.",
		browsertools.ReadTimeout,
		true,
		aitool.WithIntegerParam(
			"limit",
			aitool.WithParam_Description("Maximum number of traces to return; the extension defaults to 40"),
			aitool.WithParam_Min(1),
			aitool.WithParam_Max(100),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"recording.evidence.inspect",
		"Inspect event order, exact-value fingerprints, call stacks, request-field links, candidates, and replayable functions in one business trace.",
		"Keep includeValues=false unless field semantics cannot be determined from metadata and the grant permits sensitive previews. Use eventId to narrow the result.",
		browsertools.ReadTimeout,
		true,
		aitool.WithStringParam(
			"traceId",
			aitool.WithParam_Description("Trace ID returned by recording.trace.list"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"eventId",
			aitool.WithParam_Description("Optional event ID to inspect within the trace"),
		),
		aitool.WithBoolParam(
			"includeValues",
			aitool.WithParam_Description("Whether to include authorized short-lived value previews; defaults to false"),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"callable.inspect",
		"Inspect a page callable's input slots, output contract, source trace, and request-transaction boundary without executing it.",
		"Before replay, determine whether output.shape is value or envelope. Treat the captured request transaction as authoritative for envelope serialization.",
		browsertools.ReadTimeout,
		true,
		aitool.WithStringParam(
			"callableId",
			aitool.WithParam_Description("Optional callable ID; omit it to list callables for the current document"),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"callable.replay",
		"Replay a registered callable in the live page document. A request-transaction callable intercepts its network boundary instead of sending the test request to the server.",
		"Inspect the callable first and use the smallest recorded sample. Replay success proves executability, not packet-contract correctness.",
		browsertools.ReplayTimeout,
		true,
		aitool.WithStringParam(
			"callableId",
			aitool.WithParam_Description("Callable ID to replay"),
			aitool.WithParam_Required(true),
		),
		aitool.WithRawParam(
			"args",
			map[string]any{
				"type":        "array",
				"maxItems":    64,
				"description": "JSON arguments ordered by the callable's inputSlots",
				"items":       map[string]any{},
			},
			aitool.WithParam_Required(true),
		),
	); err != nil {
		return nil, err
	}

	httpPacketSchema := packetSchema()
	if err := register(
		"packet.compare",
		"Deterministically compare two HTTP packets. structure mode ignores randomized ciphertext values while validating route, serialization, fields, and value types; exact mode compares full values.",
		"Use structure for randomized IVs, nonces, or padding. The comparison detects accidental nested JSON envelopes inside form fields.",
		browsertools.ReadTimeout,
		true,
		aitool.WithRawParam("actual", httpPacketSchema, aitool.WithParam_Required(true)),
		aitool.WithRawParam("expected", httpPacketSchema, aitool.WithParam_Required(true)),
		aitool.WithStringParam(
			"mode",
			aitool.WithParam_Description("Comparison mode: structure or exact; defaults to structure"),
			aitool.WithParam_EnumString("structure", "exact"),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"profile.propose",
		"Use the deterministic compiler to propose a plaintext-gateway Profile from one inference candidate and one callable. The Agent selects evidence and input paths, not serialization topology.",
		"candidateId must come from recording.trace.list and callableId must be inspected first. Supply inputPaths only when their semantics are supported by evidence.",
		browsertools.ReadTimeout,
		true,
		aitool.WithStringParam(
			"candidateId",
			aitool.WithParam_Description("Inference candidate ID"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"callableId",
			aitool.WithParam_Description("Previously inspected page callable ID"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringArrayParam(
			"inputPaths",
			aitool.WithParam_Description("Optional context paths such as body or body.password, ordered by dynamic input slot"),
		),
		aitool.WithStringParam(
			"name",
			aitool.WithParam_Description("Optional plaintext-gateway name"),
			aitool.WithParam_MaxLength(120),
		),
	); err != nil {
		return nil, err
	}

	if err := register(
		"profile.validate",
		"Recompile a Profile deterministically from candidateId, callableId, and inputPaths, execute it, and validate it against captured evidence or an observed browser request. This tool does not accept a model-authored Profile or Pipeline.",
		"Use the same candidateId, callableId, and inputPaths as profile.propose. The extension rebuilds the Profile from evidence before execution; Profile management is a separate reviewed capability.",
		browsertools.ReplayTimeout,
		true,
		aitool.WithStringParam(
			"candidateId",
			aitool.WithParam_Description("Inference candidate ID used by profile.propose"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"callableId",
			aitool.WithParam_Description("Page callable ID used by profile.propose"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringArrayParam(
			"inputPaths",
			aitool.WithParam_Description("Optional context paths used by profile.propose"),
		),
		aitool.WithStringParam(
			"name",
			aitool.WithParam_Description("Optional plaintext-gateway name used by profile.propose"),
			aitool.WithParam_MaxLength(120),
		),
		aitool.WithRawParam("packet", httpPacketSchema, aitool.WithParam_Required(true)),
		aitool.WithRawParam(
			"observed",
			packetSchema(),
			aitool.WithParam_Description("Optional browser request captured from the same business operation"),
		),
		aitool.WithStringParam(
			"comparisonMode",
			aitool.WithParam_Description("Comparison mode: structure or exact; defaults to structure"),
			aitool.WithParam_EnumString("structure", "exact"),
		),
	); err != nil {
		return nil, err
	}

	if err := browsertools.RegisterCapabilityTools(
		factory,
		caller,
		deviceID,
		target,
		capabilityCatalog,
	); err != nil {
		return nil, err
	}

	return factory.Tools(), nil
}
