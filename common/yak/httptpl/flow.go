package httptpl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// flowNode is an AST node for the nuclei flow directive.
// Leaf nodes reference a request sequence by 1-based index.
// Internal nodes combine sub-expressions with && or ||.
type flowNode struct {
	// leaf: seqIndex (1-based, 0 means invalid)
	seqIndex int
	// internal: operator "&&" or "||"
	op    string
	left  *flowNode
	right *flowNode
}

// flowToken represents a single token in the flow expression.
type flowToken struct {
	kind string // "ref", "&&", "||", "(", ")"
	val  string
}

var flowRefRe = regexp.MustCompile(`(?i)^http\s*\(\s*(\d+)\s*\)$`)

// tokenizeFlow splits a flow expression into tokens.
func tokenizeFlow(flow string) ([]flowToken, error) {
	var tokens []flowToken
	i := 0
	for i < len(flow) {
		// skip whitespace
		for i < len(flow) && (flow[i] == ' ' || flow[i] == '\t' || flow[i] == '\n' || flow[i] == '\r') {
			i++
		}
		if i >= len(flow) {
			break
		}
		// operators
		if i+1 < len(flow) && flow[i] == '&' && flow[i+1] == '&' {
			tokens = append(tokens, flowToken{kind: "&&"})
			i += 2
			continue
		}
		if i+1 < len(flow) && flow[i] == '|' && flow[i+1] == '|' {
			tokens = append(tokens, flowToken{kind: "||"})
			i += 2
			continue
		}
		if flow[i] == '(' {
			tokens = append(tokens, flowToken{kind: "("})
			i++
			continue
		}
		if flow[i] == ')' {
			tokens = append(tokens, flowToken{kind: ")"})
			i++
			continue
		}
		// reference: http(N)
		if (flow[i] == 'h' || flow[i] == 'H') && i+4 <= len(flow) {
			// find the closing paren
			end := strings.IndexByte(flow[i:], ')')
			if end == -1 {
				return nil, utils.Errorf("invalid flow expression: unclosed http() reference at position %d", i)
			}
			raw := flow[i : i+end+1]
			m := flowRefRe.FindStringSubmatch(raw)
			if m == nil {
				return nil, utils.Errorf("invalid flow expression: bad http() reference %q", raw)
			}
			tokens = append(tokens, flowToken{kind: "ref", val: m[1]})
			i += end + 1
			continue
		}
		return nil, utils.Errorf("invalid flow expression: unexpected character %q at position %d", string(flow[i]), i)
	}
	return tokens, nil
}

// flowParser is a recursive descent parser for flow expressions.
type flowParser struct {
	tokens []flowToken
	pos    int
}

func (p *flowParser) peek() *flowToken {
	if p.pos < len(p.tokens) {
		return &p.tokens[p.pos]
	}
	return nil
}

func (p *flowParser) consume() *flowToken {
	if p.pos < len(p.tokens) {
		t := &p.tokens[p.pos]
		p.pos++
		return t
	}
	return nil
}

// parseOrExpr := parseAndExpr ( "||" parseAndExpr )*
func (p *flowParser) parseOrExpr() (*flowNode, error) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t == nil || t.kind != "||" {
			break
		}
		p.consume()
		right, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		left = &flowNode{op: "||", left: left, right: right}
	}
	return left, nil
}

// parseAndExpr := parsePrimary ( "&&" parsePrimary )*
func (p *flowParser) parseAndExpr() (*flowNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t == nil || t.kind != "&&" {
			break
		}
		p.consume()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &flowNode{op: "&&", left: left, right: right}
	}
	return left, nil
}

// parsePrimary := "http(" number ")" | "(" parseOrExpr ")"
func (p *flowParser) parsePrimary() (*flowNode, error) {
	t := p.peek()
	if t == nil {
		return nil, utils.Errorf("invalid flow expression: unexpected end of input")
	}
	if t.kind == "(" {
		p.consume()
		node, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		next := p.consume()
		if next == nil || next.kind != ")" {
			return nil, utils.Errorf("invalid flow expression: expected closing parenthesis")
		}
		return node, nil
	}
	if t.kind == "ref" {
		p.consume()
		idx, err := strconv.Atoi(t.val)
		if err != nil {
			return nil, utils.Errorf("invalid flow expression: bad reference index %q", t.val)
		}
		return &flowNode{seqIndex: idx}, nil
	}
	return nil, utils.Errorf("invalid flow expression: unexpected token %q", t.kind)
}

// parseFlow parses a nuclei flow directive into an AST.
func parseFlow(flow string) (*flowNode, error) {
	flow = strings.TrimSpace(flow)
	if flow == "" {
		return nil, nil
	}
	tokens, err := tokenizeFlow(flow)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	p := &flowParser{tokens: tokens}
	node, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return nil, utils.Errorf("invalid flow expression: trailing tokens after position %d", p.pos)
	}
	return node, nil
}

// execFlowResult holds the result of executing a single sequence in a flow.
type execFlowResult struct {
	responses  []*lowhttp.LowhttpResponse
	vulnResult bool   // excludes internal matchers
	flowResult bool   // includes internal matchers (for flow control)
	extracted   map[string]interface{}
	reqCount   int64
}

// evalFlow executes the flow AST with short-circuit evaluation.
// The execute function runs a single request sequence and returns its flow result
// (including internal matchers) and a full execFlowResult.
func evalFlow(node *flowNode, executed map[int]*execFlowResult, execute func(int) (*execFlowResult, error)) bool {
	if node == nil {
		return false
	}
	if node.op == "" {
		// leaf node
		idx := node.seqIndex
		if r, ok := executed[idx]; ok {
			return r.flowResult
		}
		res, err := execute(idx)
		if err != nil || res == nil {
			return false
		}
		executed[idx] = res
		return res.flowResult
	}
	if node.op == "&&" {
		left := evalFlow(node.left, executed, execute)
		if !left {
			return false
		}
		return evalFlow(node.right, executed, execute)
	}
	if node.op == "||" {
		left := evalFlow(node.left, executed, execute)
		if left {
			return true
		}
		return evalFlow(node.right, executed, execute)
	}
	return false
}

// validateFlowNode checks that all referenced indices are within range.
func validateFlowNode(n *flowNode, maxSeqs int) error {
	if n == nil {
		return nil
	}
	if n.op == "" {
		if n.seqIndex < 1 || n.seqIndex > maxSeqs {
			return fmt.Errorf("flow references http(%d) but only %d sequences exist", n.seqIndex, maxSeqs)
		}
		return nil
	}
	if err := validateFlowNode(n.left, maxSeqs); err != nil {
		return err
	}
	return validateFlowNode(n.right, maxSeqs)
}

// execFlowWithUrl executes the template with flow control.
// It returns the total request count and any error.
func (y *YakTemplate) execFlowWithUrl(u string, config *Config, opts []lowhttp.LowhttpOpt) (int, error) {
	ast, err := parseFlow(y.Flow)
	if err != nil {
		log.Errorf("parse flow failed: %v, falling back to concurrent execution", err)
		return y.execConcurrentWithUrl(u, config, opts)
	}
	if ast == nil {
		return y.execConcurrentWithUrl(u, config, opts)
	}

	// Pre-generate all request sequences
	allSeqs := y.GenerateRequestSequences(u, true)
	if len(allSeqs) == 0 {
		return 0, nil
	}

	// validate that all referenced indices are in range
	if err := validateFlowNode(ast, len(allSeqs)); err != nil {
		log.Errorf("flow validation failed: %v, falling back to concurrent execution", err)
		return y.execConcurrentWithUrl(u, config, opts)
	}

	var totalCount int64
	executed := make(map[int]*execFlowResult)

	execute := func(seqIdx int) (*execFlowResult, error) {
		ret := allSeqs[seqIdx-1] // 1-based to 0-based
		session := fmt.Sprintf("flow-%s", uuid.NewString())
		payload := ret.RequestConfig.Payloads.GetData()

		sender := y.makeSequenceSender(config, ret, opts, session)
		rsps, allResult, flowMatched, extracted, reqCount := y.handleRequestSequences(config, ret.RequestConfig, ret.Requests, payload, sender)

		vulnResult := false
		for _, b := range allResult {
			vulnResult = vulnResult || b
		}
		if vulnResult {
			log.Infof("[%v]-[%v] matched (flow http(%d))", y.Name, y.Id, seqIdx)
		}
		totalCount += reqCount
		config.ExecuteResultCallback(y, ret.RequestConfig, rsps, vulnResult, extracted)

		return &execFlowResult{
			responses:  rsps,
			vulnResult: vulnResult,
			flowResult: flowMatched,
			extracted:   extracted,
			reqCount:   reqCount,
		}, nil
	}

	evalFlow(ast, executed, execute)
	return int(totalCount), nil
}

// makeSequenceSender creates a sender function for a single request sequence.
// It is shared between the flow and concurrent execution paths.
func (y *YakTemplate) makeSequenceSender(config *Config, ret *RequestBulk, opts []lowhttp.LowhttpOpt, session string) func(raw []byte, req *requestRaw) (*lowhttp.LowhttpResponse, error) {
	return func(raw []byte, req *requestRaw) (*lowhttp.LowhttpResponse, error) {
		if config.BeforeSendPackage != nil {
			raw = config.BeforeSendPackage(raw, req.IsHttps)
		}

		urlStrIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(raw, req.IsHttps)
		urlStr := ""
		if urlStrIns != nil {
			urlStr = urlStrIns.String()
		}
		if config.mockHTTPRequest != nil {
			var mockResponseRaw []byte
			mocked := utils.NewBool(false)
			config.mockHTTPRequest(req.IsHttps, urlStr, raw, func(rsp interface{}) {
				rspBytes := utils.InterfaceToBytes(rsp)
				fixedRsp, _, _ := lowhttp.FixHTTPResponse(rspBytes)
				if fixedRsp == nil {
					log.Warnf("failed to fix mock response, using original bytes")
					fixedRsp = rspBytes
				}
				mockResponseRaw = fixedRsp
				mocked.Set()
			})
			if mocked.IsSet() {
				remoteAddr := ""
				if urlStrIns != nil {
					remoteAddr = urlStrIns.Host
				}
				return &lowhttp.LowhttpResponse{
					RawPacket:  mockResponseRaw,
					RawRequest: raw,
					Url:        urlStr,
					RemoteAddr: remoteAddr,
					Https:      req.IsHttps,
					Source:     y.Name,
					RuntimeId:  config.RuntimeId,
				}, nil
			}
		}

		packetOpt := opts
		redictTimes := 0
		if ret.RequestConfig.EnableRedirect {
			redictTimes = ret.RequestConfig.MaxRedirects
		}
		packetOpt = append(
			packetOpt,
			lowhttp.WithPacketBytes(raw),
			lowhttp.WithHttps(req.IsHttps),
			lowhttp.WithSource(y.Name),
			lowhttp.WithNoFixContentLength(ret.RequestConfig.NoFixContentLength),
			lowhttp.WithRedirectTimes(redictTimes),
			lowhttp.WithTimeout(req.Timeout),
		)

		if req.Origin.CookieInherit {
			packetOpt = append(packetOpt, lowhttp.WithSession(session))
		}

		if req.OverrideHost != "" {
			packetOpt = append(packetOpt, lowhttp.WithHost(req.OverrideHost))
		}

		if config.Debug && config.DebugRequest {
			fmt.Printf("--------------REQ---------------\n")
			fmt.Println(string(raw))
		}

		utils.Debug(func() {
			log.Info("nuclei lowhttp.Exec! ")
		})
		rsp, err := lowhttp.HTTP(packetOpt...)
		if err != nil {
			return nil, err
		}
		if config.Debug && config.DebugResponse {
			fmt.Printf("--------------RSP---------------\n")
			fmt.Println(string(rsp.RawPacket))
		}
		return rsp, nil
	}
}

// execConcurrentWithUrl is the original concurrent execution path, extracted
// from ExecWithUrl so it can be used as a fallback.
func (y *YakTemplate) execConcurrentWithUrl(u string, config *Config, opts []lowhttp.LowhttpOpt) (int, error) {
	tplConcurrent := config.ConcurrentInTemplates
	var count int64
	swg := utils.NewSizedWaitGroup(tplConcurrent)
	for _, reqSeq := range y.GenerateRequestSequences(u, true) {
		swg.Add()
		go func(ret *RequestBulk, payload map[string][]string) {
			session := uuid.NewString()
			defer swg.Done()
			sender := y.makeSequenceSender(config, ret, opts, session)
			rsps, allResult, _, extracted, reqCount := y.handleRequestSequences(config, ret.RequestConfig, ret.Requests, payload, sender)
			result := false
			for _, b := range allResult {
				result = result || b
			}
			if result {
				log.Infof("[%v]-[%v] matched", y.Name, y.Id)
			}
			atomic.AddInt64(&count, reqCount)
			config.ExecuteResultCallback(y, ret.RequestConfig, rsps, result, extracted)
		}(reqSeq, reqSeq.RequestConfig.Payloads.GetData())
	}
	swg.Wait()
	return int(count), nil
}
