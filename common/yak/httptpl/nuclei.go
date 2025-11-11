package httptpl

import (
	"bufio"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"gopkg.in/yaml.v3"
)

// vars in http
// https://nuclei.projectdiscovery.io/templating-guide/protocols/http/
// {{BaseURL}} - This will replace on runtime in the request by the input URL as specified in the target file.
// {{RootURL}} - This will replace on runtime in the request by the root URL as specified in the target file.
// {{Hostname}} - Hostname variable is replaced by the hostname including port of the target on runtime.
// {{Host}} - This will replace on runtime in the request by the input host as specified in the target file.
// {{Port}} - This will replace on runtime in the request by the input port as specified in the target file.
// {{Path}} - This will replace on runtime in the request by the input path as specified in the target file.
// {{File}} - This will replace on runtime in the request by the input filename as specified in the target file.
// {{Scheme}} - This will replace on runtime in the request by protocol scheme as specified in the target file.

type NucleiTagData struct {
	IsExpr  bool
	Content string
}

func _ParseNucleiTag(raw string) []*NucleiTagData {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Split(bufio.ScanBytes)
	var data []*NucleiTagData
	var last string
	status := "raw"
	var currentTagContent string
	var currentContent string
	handle := func(s string) {
		switch status {
		case "raw":
			if currentTagContent != "" {
				data = append(data, &NucleiTagData{
					IsExpr:  true,
					Content: currentTagContent,
				})
				currentTagContent = ""
			}

			currentContent += s
			if s == "{" && last == "{" {
				status = "open"
				if len(currentContent) >= 2 {
					currentContent = currentContent[:len(currentContent)-2]
				}
				return
			}
		case "open":
			if currentContent != "" {
				data = append(data, &NucleiTagData{
					Content: currentContent,
				})
				currentContent = ""
			}
			currentTagContent += s

			if s == "}" && last == "}" {
				status = "raw"
				currentTagContent = strings.TrimRight(currentTagContent, "}")
				return
			}
		}
	}
	for scanner.Scan() {
		handle(scanner.Text())
		last = scanner.Text()
	}

	if currentTagContent != "" {
		data = append(data, &NucleiTagData{
			IsExpr:  true,
			Content: currentTagContent,
		})
	}

	if currentContent != "" {
		data = append(data, &NucleiTagData{
			Content: currentContent,
		})
	}

	return data
}

func CreateYakTemplateFromYakScript(s *schema.YakScript) (*YakTemplate, error) {
	tpl, err := CreateYakTemplateFromNucleiTemplateRaw(s.Content)
	if err != nil {
		return nil, err
	}
	tpl.UUID = s.Uuid
	tpl.ScriptName = s.ScriptName
	return tpl, nil
}

func CreateYakTemplateFromNucleiTemplateRaw(tplRaw string) (*YakTemplate, error) {
	// 渲染randstr
	randStrMap := new(sync.Map)
	randStrVarGenerator := func(varName string) string {
		if randStrMap == nil {
		}

		if value, ok := randStrMap.Load(varName); ok {
			return value.(string)
		} else {
			value := uuid.NewString()
			randStrMap.Store(varName, value)
			return value
		}
	}

	pattern := regexp.MustCompile(`\{\{randstr(_\d+)?\}\}`)
	tplRaw = pattern.ReplaceAllStringFunc(tplRaw, func(raw string) string {
		return randStrVarGenerator(raw)
	})

	yakTemp := &YakTemplate{}
	for _, v := range []string{`{{interactsh-url}}`, `{{interactsh}}`, `{{interactsh_url}}`} {
		if strings.Contains(tplRaw, v) {
			yakTemp.ReverseConnectionNeed = true
		}
	}
	rootNode := &yaml.Node{}
	err := yaml.Unmarshal([]byte(tplRaw), rootNode)
	if err != nil {
		return nil, utils.Errorf("unmarshal nuclei template failed: %v", err)
	}
	yakTemp.Id = nodeGetString(rootNode, "id")
	info := nodeGetRaw(rootNode, "info")

	yakTemp.SelfContained = nodeGetBool(rootNode, "self-contained")
	cveInfo := nodeGetRaw(info, "classification")
	yakTemp.Name = nodeGetString(info, "name")
	yakTemp.Author = nodeGetString(info, "author")
	yakTemp.Severity = nodeGetString(info, "severity")
	yakTemp.Description = nodeGetString(info, "description")
	yakTemp.Reference = nodeGetStringSlice(info, "reference")
	yakTemp.Tags = utils.PrettifyListFromStringSplitEx(nodeGetString(info, "tags"), ",")
	yakitInfo := nodeGetRaw(info, "yakit-info")
	if yakitInfo != nil {
		yakTemp.Sign = nodeGetString(yakitInfo, "sign")
	}
	yakTemp.CVE = nodeGetString(cveInfo, "cve-id")

	yakTemp.Variables = generateYakVariables(rootNode)

	reqs := nodeGetFirstRaw(rootNode, "requests", "http")
	if reqs == nil || reqs.Kind != yaml.SequenceNode {
		if networkNode := nodeGetFirstRaw(rootNode, "network", "tcp"); networkNode != nil {
			if networkNode.Kind != yaml.SequenceNode {
				return nil, utils.Error("nuclei template network is not slice")
			}
			// network means tcp packets...
			yakTemp.TCPRequestSequences, err = parseNetworkBulk(networkNode.Content, yakTemp.ReverseConnectionNeed)
			if err != nil {
				return nil, utils.Errorf("parse network bulk failed: %v", err)
			}

			return yakTemp, nil
		} else if nodeGetFirstRaw(rootNode, "workflows") != nil {
			return nil, utils.Error("yakit nuclei cannot support workflows now~")
		} else if nodeGetFirstRaw(rootNode, "headless") != nil {
			return nil, utils.Errorf("nuclei template `headless(crawler)` is not supported (*)")
		} else {
			// log.Warnf("-----------------NUCLEI FORMATTER CANNOT FIX--------------------")
			// fmt.Println(tplRaw)
			return nil, utils.Errorf("nuclei template unsupported: %s[%s]", yakTemp.Id, yakTemp.Name)
		}
	}

	// parse req seqs
	var reqSeq []*YakRequestBulkConfig
	hasMatcherOrExtractor := false
	extractConfig := func(config *RequestConfig, data *yaml.Node) {
		config.IsHTTPS = nodeGetBool(data, "is-https")
		config.IsGmTLS = nodeGetBool(data, "is-gmtls")
		config.Host = nodeGetString(data, "host")
		config.Proxy = nodeGetString(data, "proxy")
		config.NoSystemProxy = nodeGetBool(data, "no-system-proxy")
		config.ForceFuzz = nodeGetBool(data, "force-fuzz")
		config.RequestTimeout = nodeGetFloat64(data, "request-timeout")
		config.RepeatTimes = nodeGetInt64(data, "repeat-times")
		config.Concurrent = nodeGetInt64(data, "concurrent")
		config.DelayMinSeconds = nodeGetFloat64(data, "delay-min-seconds")
		config.DelayMaxSeconds = nodeGetFloat64(data, "delay-max-seconds")
		config.MaxRetryTimes = nodeGetInt64(data, "max-retry-times")
		config.RetryInStatusCode = nodeGetString(data, "retry-in-status-code")
		config.RetryNotInStatusCode = nodeGetString(data, "retry-not-in-status-code")
		config.MaxRedirects = int(nodeGetInt64(data, "max-redirects"))
		config.JsEnableRedirect = nodeGetBool(data, "js-enable-redirect")
		config.JsMaxRedirects = int(nodeGetInt64(data, "js-max-redirect"))
		config.EnableRedirect = nodeGetBool(data, "enable-redirect")
		config.MaxRedirects = int(nodeGetInt64(data, "max-redirects"))
		config.DNSServers = nodeGetStringSlice(data, "dns-servers")
		etcHosts := nodeGetRaw(data, "etc-hosts")
		if etcHosts.Kind == yaml.MappingNode {
			hosts := make(map[string]string)
			mappingNodeForEach(etcHosts, func(key string, node *yaml.Node) error {
				hosts[key] = node.Value
				return nil
			})
			config.EtcHosts = hosts
		}
		vars := nodeGetRaw(data, "variables")
		config.Variables = NewVars()
		mappingNodeForEach(vars, func(key string, node *yaml.Node) error {
			config.Variables.AutoSet(key, node.Value)
			return nil
		})
	}
	_ = extractConfig
	for _, node := range reqs.Content {

		reqIns := &YakRequestBulkConfig{
			Headers: map[string]string{},
		}
		matcher, err := generateYakMatcher(node)
		if err != nil {
			log.Debugf("extractYakExtractor failed: %v", err)
		} else if matcher != nil {
			hasMatcherOrExtractor = true
			if yakTemp != nil {
				matcher.TemplateName = yakTemp.Name
			}
		}

		payloads, err := generateYakPayloads(node)
		if err != nil {
			log.Debugf("extractYakPayloads failed: %v", err)
		}
		reqIns.Payloads = payloads
		switch strings.ToLower(strings.TrimSpace(nodeGetString(node, "attack"))) {
		case "pitchfork":
			reqIns.AttackMode = "sync"
		default:
			reqIns.AttackMode = "cartesian-product"
		}

		reqIns.Matcher = matcher
		extractors, err := generateYakExtractors(node)
		if err != nil {
			log.Errorf("extractYakExtractor failed: %v", err)
		}
		reqIns.Extractor = extractors
		if len(reqIns.Extractor) != 0 {
			hasMatcherOrExtractor = true
		}
		reqIns.StopAtFirstMatch = nodeGetBool(node, "stop-at-first-match")
		reqIns.CookieInherit = !nodeGetBool(node, "disable-cookie")
		reqIns.MaxSize = int(nodeGetInt64(node, "max-size"))
		reqIns.NoFixContentLength = nodeGetBool(node, "unsafe")
		reqIns.AfterRequested = nodeGetBool(node, "req-condition")
		reqIns.AttackMode = nodeGetString(node, "attack-mode")
		reqIns.InheritVariables = nodeGetBool(node, "inherit-variables")
		// reqIns.HotPatchCode = nodeGetString(req, "hot-patch-code")

		reqIns.EnableRedirect = nodeToBool(nodeGetFirstRaw(node, "host-redirects", "redirects"))
		reqIns.MaxRedirects = int(nodeGetInt64(node, "max-redirects"))

		if rawNode := nodeGetRaw(node, "raw"); rawNode != nil {
			raws := nodeToStringSlice(rawNode)
			reqIns.HTTPRequests = lo.Map(raws, func(i string, _ int) *YakHTTPRequestPacket {
				return nucleiRawPacketToYakHTTPRequestPacket(i)
			})
		} else {
			reqIns.Method = nodeGetString(node, "method")
			reqIns.Paths = nodeGetStringSlice(node, "path")
			mappingNodeForEach(nodeGetRaw(node, "headers"), func(key string, value *yaml.Node) error {
				reqIns.Headers[key] = value.Value
				return nil
			})
			reqIns.Body = nodeGetString(node, "body")
		}

		if len(reqIns.HTTPRequests) <= 0 && len(reqIns.Paths) == 0 {
			log.Error("http request is empty")
			return nil, utils.Error("http request is empty")
		}
		reqSeq = append(reqSeq, reqIns)
	}

	_ = hasMatcherOrExtractor
	yakTemp.HTTPRequestSequences = reqSeq
	if yakTemp.NoMatcherAndExtractor() {
		for _, i := range yakTemp.HTTPRequestSequences {
			if i.Matcher == nil {
				i.Matcher = &YakMatcher{
					SubMatcherCondition: "and",
				}
			}
			if len(i.Matcher.SubMatchers) <= 0 {
				i.Matcher.SubMatchers = []*YakMatcher{
					{
						MatcherType: MATCHER_TYPE_STATUS_CODE,
						Group:       []string{"200"},
					},
				}
			}
		}
		for _, i := range yakTemp.TCPRequestSequences {
			if i.Matcher == nil {
				i.Matcher = &YakMatcher{
					SubMatcherCondition: "and",
				}
			}
			if len(i.Matcher.SubMatchers) <= 0 {
				i.Matcher.SubMatchers = []*YakMatcher{
					{
						MatcherType: MATCHER_TYPE_EXPR,
						Group:       []string{"true"},
					},
				}
			}
		}
		yakTemp.Sign = yakTemp.SignMainParams()
	}
	return yakTemp, nil
}

func nucleiRawPacketToYakHTTPRequestPacket(i string) *YakHTTPRequestPacket {
	packet := &YakHTTPRequestPacket{}
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(i))
	startWithAt := strings.HasPrefix(strings.TrimSpace(i), "@")
	for scanner.Scan() {
		if !startWithAt {
			lines = append(lines, scanner.Text())
			continue
		} else {
			line := scanner.Text()
			if strings.HasPrefix(line, "@") {
				if len(line) <= 1 {
					continue
				}
				line = strings.TrimSpace(line[1:])
				k, v := lowhttp.SplitHTTPHeader(line)
				switch strings.ToLower(k) {
				case "tls-sni":
					packet.SNI = v
				case "timeout":
					packet.Timeout, _ = time.ParseDuration(v)
				case "host":
					packet.OverrideHost = v
				}
			} else {
				startWithAt = false
				lines = append(lines, line)
			}
		}
	}
	packet.Request = strings.Join(lines, "\r\n")
	if strings.HasSuffix(packet.Request, "\r\n") {
		packet.Request += "\r\n"
	}
	return packet
}

func generateYakExtractors(rootNode *yaml.Node) ([]*YakExtractor, error) {
	extractorsNode := nodeGetRaw(rootNode, "extractors")
	if extractorsNode == nil {
		return nil, nil
	}

	var extractors []*YakExtractor
	sequenceNodeForEach(extractorsNode, func(node *yaml.Node) error {
		ext := &YakExtractor{}
		ext.Name = nodeGetString(node, "name")
		// Support both 'scope' (preferred) and 'part' (Nuclei compatible)
		scope := nodeGetString(node, "scope")
		if scope == "" {
			scope = nodeGetString(node, "part")
		}
		ext.Scope = scope
		ext.Id = int(nodeGetInt64(node, "id"))
		typ := nodeGetString(node, "type")
		switch typ {
		case "regex":
			ext.Type = "regex"
			ext.Groups = nodeGetStringSliceFallback(node, "regex")
			ext.RegexpMatchGroup = []int{int(nodeGetInt64(node, "group"))}
		case "kval":
			ext.Type = "key-value"
			ext.Groups = nodeGetStringSliceFallback(node, "kval")
		case "json":
			ext.Type = "json"
			ext.Groups = nodeGetStringSliceFallback(node, "json")
		case "xpath":
			ext.Type = "xpath"
			ext.Groups = nodeGetStringSliceFallback(node, "xpath")
			ext.XPathAttribute = nodeGetString(node, "attribute")
		case "dsl":
			ext.Type = "dsl"
			ext.Groups = nodeGetStringSliceFallback(node, "dsl")
		default:
			log.Errorf("extractYakExtractor failed: %v", utils.Errorf("nuclei template extractors type is not supported"))
			return utils.Errorf("nuclei template extractors type is not supported")
		}
		extractors = append(extractors, ext)
		return nil
	})
	return extractors, nil
}

func generateYakMatcher(rootNode *yaml.Node) (*YakMatcher, error) {
	matchersNode := nodeGetRaw(rootNode, "matchers")
	if matchersNode == nil {
		return nil, utils.Errorf("nuclei template matchers is nil")
	}
	if matchersNode.Kind != yaml.SequenceNode {
		return nil, utils.Errorf("nuclei template matchers is not slice")
	}
	var matchers []*YakMatcher
	sequenceNodeForEach(matchersNode, func(node *yaml.Node) error {
		match := &YakMatcher{
			MatcherType: "",
			ExprType:    "",
			Scope:       "",
			Condition:   "",
			Group:       nil,
		}
		match.Negative = nodeGetBool(node, "negative")
		match.Condition = nodeGetString(node, "condition")
		match.Id = int(nodeGetFloat64(node, "id"))

		// Support both 'scope' (preferred) and 'part' (Nuclei compatible)
		scopeStr := nodeGetString(node, "scope")
		if scopeStr == "" {
			scopeStr = nodeGetString(node, "part")
		}
		
		switch scopeStr {
		case "body":
			match.Scope = "body"
		case "header":
			match.Scope = "header"
		case "status":
			match.Scope = "status"
		case "raw", "":
			match.Scope = "raw"
		case "interactsh_protocol", "oob_protocol":
			match.Scope = "interactsh_protocol"
		case "request_header":
			match.Scope = "request_header"
		case "request_body":
			match.Scope = "request_body"
		case "request_raw":
			match.Scope = "request_raw"
		case "request_url":
			match.Scope = "request_url"
		default:
			// If not recognized, use as-is (for future extensions)
			if scopeStr != "" {
				match.Scope = scopeStr
			} else {
				match.Scope = "raw"
			}
		}
		typ := nodeGetString(node, "type")
		switch typ {
		case "word":
			match.MatcherType = "word"
			match.GroupEncoding = nodeGetString(node, "encoding")
			match.Group = nodeGetStringSliceFallback(node, "words")
		case "status":
			match.MatcherType = "status_code"
			match.Group = nodeGetStringSliceFallback(node, "status")
		case "size":
			match.MatcherType = "content_length"
			match.Group = nodeGetStringSliceFallback(node, "size", "sizes", "content-length")
		case "binary":
			match.MatcherType = "binary"
			match.Group = nodeGetStringSliceFallback(node, "binary")
		case "regex":
			match.MatcherType = "regex"
			match.Group = nodeGetStringSliceFallback(node, "regex", "regexp")
		case "dsl":
			match.MatcherType = "expr"
			match.ExprType = "nuclei-dsl"
			match.Group = nodeGetStringSliceFallback(node, "dsl")
		default:
			log.Errorf("parse nuclei template matcher type failed: %v", typ)
			return nil
		}

		matchers = append(matchers, match)
		return nil
	})

	var matchInstance *YakMatcher
	if len(matchers) > 1 {
		matchInstance = &YakMatcher{
			SubMatcherCondition: nodeGetString(rootNode, "matchers-condition"),
			SubMatchers:         matchers,
		}
	} else if len(matchers) == 1 {
		matchInstance = matchers[0]
	} else {
		log.Errorf("parse nuclei template matcher failed: %v", matchers)
		return nil, utils.Errorf("parse nuclei template matcher failed: %v", matchers)
	}
	return matchInstance, nil
}

func generateYakPayloads(node *yaml.Node) (*YakPayloads, error) {
	subNode := nodeGetRaw(node, "payloads")
	payloads, _ := NewYakPayloads(nil)
	if subNode == nil {
		return payloads, nil
	}
	if subNode.Kind != yaml.MappingNode {
		return nil, utils.Errorf("nuclei template payloads is not map")
	}
	payloadDatas := make(map[string]any, len(subNode.Content)/2)
	mappingNodeForEach(subNode, func(key string, value *yaml.Node) error {
		payloadDatas[key] = nodeToStringSlice(value)
		return nil
	})
	payloads.AddPayloads(payloadDatas)
	return payloads, nil
}

func NewYakPayloads(data map[string]any) (*YakPayloads, error) {
	payloads := &YakPayloads{raw: map[string]*YakPayload{}}
	if data == nil {
		return payloads, nil
	}
	return payloads, payloads.AddPayloads(data)
}

func generateYakVariables(node *yaml.Node) *YakVariables {
	subNode := nodeGetRaw(node, "variables")
	vars := NewVars()
	if subNode == nil {
		return vars
	}
	mappingNodeForEach(subNode, func(key string, valueNode *yaml.Node) error {
		vars.AutoSet(key, valueNode.Value)
		return nil
	})
	return vars
}
