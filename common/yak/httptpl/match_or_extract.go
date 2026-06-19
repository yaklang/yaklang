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

// MatchOrExtractHTTPS 是一个配置选项，用于在解析请求 URL 时声明是否按 HTTPS 处理
// 参数:
//   - enable: 是否按 HTTPS 处理
//
// 返回值:
//   - 一个配置选项，作为可变参数传入 httptpl.MatchOrExtractHTTPFlow
//
// Example:
// ```
// // 声明按 HTTPS 处理后再执行匹配
// result = httptpl.MatchOrExtractHTTPFlow(req, rsp, yamlRule, httptpl.https(true))~
// println(result.IsMatched)
// ```
func MatchOrExtractHTTPS(enable bool) MatchOrExtractOption {
	return func(cfg *matchOrExtractConfig) {
		cfg.isHTTPS = enable
		cfg.explicitHTTPS = true
	}
}

// MatchOrExtractVars 是一个配置选项，用于在匹配/提取执行期间注入自定义的 nuclei-dsl 变量
// 参数:
//   - items: 要注入的变量键值对
//
// 返回值:
//   - 一个配置选项，作为可变参数传入 httptpl.MatchOrExtractHTTPFlow
//
// Example:
// ```
// // 注入自定义变量供 matcher/extractor 使用
// result = httptpl.MatchOrExtractHTTPFlow(req, rsp, yamlRule, httptpl.vars({"flag": "abc"}))~
// println(result.IsMatched)
// ```
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

// MatchOrExtractHTTPFlow 针对单个 HTTP 请求/响应对，执行 yamlString 中定义的 nuclei 风格 matcher 与 extractor
// 参数:
//   - req: 请求报文（字符串或字节数组），可为空（将尝试从响应推导）
//   - rsp: 响应报文（字符串或字节数组）
//   - yamlString: 定义 matchers/extractors 的 nuclei 风格 YAML 字符串
//   - opts: 可选配置，例如 httptpl.https、httptpl.vars
//
// 返回值:
//   - 匹配/提取结果，包含是否命中(IsMatched)与提取到的变量(Extracted)
//   - 错误信息，规则为空或解析失败时返回非空
//
// Example:
// ```
// rsp = "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<title>Example Domain</title>"
// rule = `matchers:
//   - type: word
//     part: body
//     words:
//   - "Example Domain"`
//
// result = httptpl.MatchOrExtractHTTPFlow("", rsp, rule)~
// println(result.IsMatched)   // OUT: true
// assert result.IsMatched == true, "word matcher should match the response body"
// ```
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
