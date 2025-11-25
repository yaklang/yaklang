package httptpl

import (
	"net/http"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"gopkg.in/yaml.v3"
)

// MatchOrExtractResult describes the outcome of MatchOrExtractHTTPFlow.
type MatchOrExtractResult struct {
	IsMatched bool
	Extracted map[string]any
}

type matchOrExtractConfig struct {
	isHTTPS       bool
	vars          map[string]any
	explicitHTTPS bool
}

// MatchOrExtractOption configures MatchOrExtractHTTPFlow behaviour.
type MatchOrExtractOption func(*matchOrExtractConfig)

// MatchOrExtractExports exposes simplified matcher/extractor helpers to yaklang.
var MatchOrExtractExports = map[string]interface{}{
	"MatchOrExtractHTTPFlow": MatchOrExtractHTTPFlow,
	"https":                  MatchOrExtractHTTPS,
	"vars":                   MatchOrExtractVars,
}

// MatchOrExtractHTTPS toggles HTTPS awareness when parsing request URLs.
func MatchOrExtractHTTPS(enable bool) MatchOrExtractOption {
	return func(cfg *matchOrExtractConfig) {
		cfg.isHTTPS = enable
		cfg.explicitHTTPS = true
	}
}

// MatchOrExtractVars injects custom nuclei-dsl variables during matcher execution.
func MatchOrExtractVars(items map[string]any) MatchOrExtractOption {
	return func(cfg *matchOrExtractConfig) {
		if cfg.vars == nil {
			cfg.vars = make(map[string]any)
		}
		for k, v := range items {
			cfg.vars[k] = v
		}
	}
}

// MatchOrExtractHTTPFlow evaluates matchers and extractors (defined in yamlString)
// against a single HTTP request/response pair.
func MatchOrExtractHTTPFlow(req any, rsp any, yamlString string, opts ...MatchOrExtractOption) (*MatchOrExtractResult, error) {
	if strings.TrimSpace(yamlString) == "" {
		return nil, utils.Errorf("yamlString is empty")
	}

	cfg := &matchOrExtractConfig{
		vars: make(map[string]any),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	reqBytes, reqHTTPS, err := normalizeRequestPacket(req)
	if err != nil {
		return nil, err
	}
	rspBytes, derivedReqBytes, rspHTTPS, err := normalizeResponsePacket(rsp)
	if err != nil {
		return nil, err
	}
	if len(rspBytes) == 0 {
		return nil, utils.Errorf("response packet is empty")
	}
	if len(reqBytes) == 0 && len(derivedReqBytes) > 0 {
		reqBytes = derivedReqBytes
	}
	if !cfg.explicitHTTPS {
		cfg.isHTTPS = reqHTTPS || rspHTTPS
	}

	root := &yaml.Node{}
	if err := yaml.Unmarshal([]byte(yamlString), root); err != nil {
		return nil, utils.Errorf("parse yaml failed: %w", err)
	}
	if len(root.Content) == 0 {
		return nil, utils.Errorf("invalid yaml content")
	}
	templateNode, err := resolveTemplateNode(root.Content[0])
	if err != nil {
		return nil, err
	}

	var matcher *YakMatcher
	if node := nodeGetRaw(templateNode, "matchers"); node != nil {
		parsedMatcher, err := generateYakMatcher(templateNode)
		if err != nil {
			return nil, err
		}
		matcher = parsedMatcher
	}

	var extractors []*YakExtractor
	if node := nodeGetRaw(templateNode, "extractors"); node != nil {
		parsedExtractors, err := generateYakExtractors(templateNode)
		if err != nil {
			return nil, err
		}
		extractors = parsedExtractors
	}

	if matcher == nil && len(extractors) == 0 {
		return nil, utils.Errorf("yaml must define at least one matcher or extractor")
	}

	vars := LoadVarFromRawResponseWithRequest(rspBytes, reqBytes, 0, cfg.isHTTPS)
	for k, v := range cfg.vars {
		vars[k] = v
	}

	result := &MatchOrExtractResult{
		Extracted: make(map[string]any),
	}

	if matcher != nil {
		match, err := matcher.Execute(&RespForMatch{
			RawPacket:     rspBytes,
			RequestPacket: reqBytes,
			IsHttps:       cfg.isHTTPS,
		}, vars)
		if err != nil {
			return nil, err
		}
		result.IsMatched = match
	}

	if len(extractors) > 0 {
		prev := make([]map[string]any, 0, len(extractors))
		for _, extractor := range extractors {
			if extractor == nil {
				continue
			}
			extracted, err := extractor.ExecuteWithRequest(rspBytes, reqBytes, cfg.isHTTPS, prev...)
			if err != nil {
				return nil, err
			}
			if extracted != nil {
				for k, v := range extracted {
					result.Extracted[k] = v
				}
				prev = append(prev, extracted)
			}
		}
	}

	return result, nil
}

func resolveTemplateNode(node *yaml.Node) (*yaml.Node, error) {
	if node == nil {
		return nil, utils.Errorf("yaml node is nil")
	}
	if hasMatcherOrExtractor(node) {
		return node, nil
	}
	httpNode := nodeGetFirstRaw(node, "http", "requests")
	if httpNode != nil && httpNode.Kind == yaml.SequenceNode && len(httpNode.Content) > 0 {
		return httpNode.Content[0], nil
	}
	return nil, utils.Errorf("yaml must define matchers or extractors")
}

func hasMatcherOrExtractor(node *yaml.Node) bool {
	return nodeGetRaw(node, "matchers") != nil || nodeGetRaw(node, "extractors") != nil
}

func normalizeRequestPacket(req any) ([]byte, bool, error) {
	if req == nil {
		return nil, false, nil
	}
	switch v := req.(type) {
	case []byte:
		return v, false, nil
	case string:
		return []byte(v), false, nil
	case *http.Request:
		raw, err := utils.DumpHTTPRequest(v, true)
		if err != nil {
			return nil, false, err
		}
		return raw, strings.EqualFold(v.URL.Scheme, "https"), nil
	case *lowhttp.LowhttpResponse:
		return v.RawRequest, v.Https, nil
	default:
		return utils.InterfaceToBytes(v), false, nil
	}
}

func normalizeResponsePacket(rsp any) ([]byte, []byte, bool, error) {
	if rsp == nil {
		return nil, nil, false, nil
	}
	switch v := rsp.(type) {
	case []byte:
		return v, nil, false, nil
	case string:
		return []byte(v), nil, false, nil
	case *http.Response:
		raw, err := utils.DumpHTTPResponse(v, true)
		if err != nil {
			return nil, nil, false, err
		}
		var reqRaw []byte
		if v.Request != nil {
			reqRaw, _ = utils.DumpHTTPRequest(v.Request, true)
		}
		isHTTPS := false
		if v.Request != nil && v.Request.URL != nil {
			isHTTPS = strings.EqualFold(v.Request.URL.Scheme, "https")
		}
		return raw, reqRaw, isHTTPS, nil
	case *lowhttp.LowhttpResponse:
		raw := v.RawPacket
		if len(raw) == 0 {
			raw = v.BareResponse
		}
		if len(raw) == 0 && len(v.MultiResponseInstances) > 0 {
			if dumped, err := utils.DumpHTTPResponse(v.MultiResponseInstances[0], true); err == nil {
				raw = dumped
			}
		}
		return raw, v.RawRequest, v.Https, nil
	default:
		return utils.InterfaceToBytes(v), nil, false, nil
	}
}
