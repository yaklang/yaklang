package httptpl

import (
	"bufio"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/go-funk"
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

	//if strings.Contains(tplRaw, "{{") {
	//	tplRaw = ExpandPreprocessor(tplRaw)
	//}
	yakTemp := &YakTemplate{}
	for _, v := range []string{`{{interactsh-url}}`, `{{interactsh}}`, `{{interactsh_url}}`} {
		if strings.Contains(tplRaw, v) {
			yakTemp.ReverseConnectionNeed = true
		}
	}
	mid := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(tplRaw), &mid)
	if err != nil {
		return nil, utils.Errorf("unmarshal nuclei template failed: %v", err)
	}
	yakTemp.Id = utils.MapGetString(mid, "id")
	info := utils.InterfaceToMapInterface(utils.MapGetRaw(mid, "info"))

	yakTemp.SelfContained = utils.MapGetBool(mid, "self-contained")
	cveInfo := utils.InterfaceToMapInterface(utils.MapGetRaw(info, "classification"))
	yakTemp.Name = utils.MapGetString(info, "name")
	yakTemp.Author = utils.MapGetString(info, "author")
	yakTemp.Severity = utils.MapGetString(info, "severity")
	yakTemp.Description = utils.MapGetString(info, "description")
	yakTemp.Reference = utils.InterfaceToStringSlice(utils.MapGetRaw(info, "reference"))
	yakTemp.Tags = utils.PrettifyListFromStringSplitEx(utils.MapGetString(info, "tags"), ",")
	yakitInfo := utils.InterfaceToMapInterface(utils.MapGetRaw(info, "yakit-info"))
	if yakitInfo != nil {
		yakTemp.Sign = utils.MapGetString(yakitInfo, "sign")
	}
	yakTemp.CVE = utils.MapGetString(cveInfo, "cve-id")

	reqs := utils.MapGetFirstRaw(mid, "requests", "http")
	if reqs == nil || (reqs != nil && reflect.TypeOf(reqs).Kind() != reflect.Slice) {
		if ret := utils.MapGetFirstRaw(mid, "network", "tcp"); ret != nil {
			if reflect.TypeOf(ret).Kind() != reflect.Slice {
				return nil, utils.Error("nuclei template `network` is not slice")
			}
			// network means tcp packets...
			yakTemp.TCPRequestSequences, err = parseNetworkBulk(utils.InterfaceToSliceInterface(ret), yakTemp.ReverseConnectionNeed)
			if err != nil {
				return nil, utils.Errorf("parse network bulk failed: %v", err)
			}

			return yakTemp, nil
		} else if utils.MapGetFirstRaw(mid, "workflows") != nil {
			return nil, utils.Error("yakit nuclei cannot support workflows now~")
		} else if utils.MapGetFirstRaw(mid, "headless") != nil {
			return nil, utils.Errorf("nuclei template `headless(crawler)` is not supported (*)")
		} else {
			log.Warnf("-----------------NUCLEI FORMATTER CANNOT FIX--------------------")
			fmt.Println(tplRaw)
			return nil, utils.Errorf("nuclei template requests is not slice")
		}
	}

	yakTemp.Variables = generateYakVariables(mid)

	// parse req seqs
	var reqSeq []*YakRequestBulkConfig
	hasMatcherOrExtractor := false
	extractConfig := func(config *RequestConfig, data map[string]interface{}) {
		config.IsHTTPS = utils.MapGetBool(data, "is-https")
		config.IsGmTLS = utils.MapGetBool(data, "is-gmtls")
		config.Host = utils.MapGetString(data, "host")
		config.Proxy = utils.MapGetString(data, "proxy")
		config.NoSystemProxy = utils.MapGetBool(data, "no-system-proxy")
		config.ForceFuzz = utils.MapGetBool(data, "force-fuzz")
		config.RequestTimeout = utils.MapGetFloat64(data, "request-timeout")
		config.RepeatTimes = utils.MapGetInt64(data, "repeat-times")
		config.Concurrent = utils.MapGetInt64(data, "concurrent")
		config.DelayMinSeconds = utils.MapGetFloat64(data, "delay-min-seconds")
		config.DelayMaxSeconds = utils.MapGetFloat64(data, "delay-max-seconds")
		config.MaxRetryTimes = utils.MapGetInt64(data, "max-retry-times")
		config.RetryInStatusCode = utils.MapGetString(data, "retry-in-status-code")
		config.RetryNotInStatusCode = utils.MapGetString(data, "retry-not-in-status-code")
		config.MaxRedirects = utils.MapGetInt(data, "max-redirects")
		config.JsEnableRedirect = utils.MapGetBool(data, "js-enable-redirect")
		config.JsMaxRedirects = utils.MapGetInt(data, "js-max-redirect")
		config.EnableRedirect = utils.MapGetBool(data, "enable-redirect")
		config.MaxRedirects = utils.MapGetInt(data, "max-redirects")
		config.DNSServers = utils.MapGetStringSlice(data, "dns-servers")
		ietcHosts := utils.MapGetRaw(data, "etc-hosts")
		if etcHosts, ok := ietcHosts.(map[string]interface{}); ok {
			hosts := make(map[string]string)
			for k, v := range etcHosts {
				hosts[k] = utils.InterfaceToString(v)
			}
			config.EtcHosts = hosts
		}
		vars := utils.MapGetRaw(data, "variables")
		config.Variables = NewVars()
		for k, v := range utils.InterfaceToMapInterface(vars) {
			config.Variables.AutoSet(k, utils.InterfaceToString(v))
		}
	}
	_ = extractConfig
	for _, i := range utils.InterfaceToSliceInterface(reqs) {
		reqIns := &YakRequestBulkConfig{
			Headers: map[string]string{},
		}
		// extractConfig(&reqIns.RequestConfig, utils.InterfaceToMapInterface(i))
		req := utils.InterfaceToMapInterface(i)
		matcher, err := generateYakMatcher(req)
		if err != nil {
			log.Debugf("extractYakExtractor failed: %v", err)
		} else if matcher != nil {
			hasMatcherOrExtractor = true
			if yakTemp != nil {
				matcher.TemplateName = yakTemp.Name
			}
		}

		payloads, err := generateYakPayloads(req)
		if err != nil {
			log.Debugf("extractYakPayloads failed: %v", err)
		}
		reqIns.Payloads = payloads
		switch strings.ToLower(strings.TrimSpace(utils.MapGetString(req, "attack"))) {
		case "pitchfork":
			reqIns.AttackMode = "sync"
		default:
			reqIns.AttackMode = "cartesian-product"
		}

		reqIns.Matcher = matcher
		extractors, err := generateYakExtractors(req)
		if err != nil {
			log.Errorf("extractYakExtractor failed: %v", err)
		}
		reqIns.Extractor = extractors
		if len(reqIns.Extractor) != 0 {
			hasMatcherOrExtractor = true
		}
		reqIns.StopAtFirstMatch = utils.MapGetBool(req, "stop-at-first-match")
		reqIns.CookieInherit = utils.MapGetBool(req, "cookie-reuse")
		reqIns.MaxSize = utils.MapGetInt(req, "max-size")
		reqIns.NoFixContentLength = utils.MapGetBool(req, "unsafe")
		reqIns.AfterRequested = utils.MapGetBool(req, "req-condition")
		reqIns.AttackMode = utils.MapGetString(req, "attack-mode")
		reqIns.InheritVariables = utils.MapGetBool(req, "inherit-variables")
		// reqIns.HotPatchCode = utils.MapGetString(req, "hot-patch-code")

		reqIns.EnableRedirect, _ = strconv.ParseBool(utils.InterfaceToString(utils.MapGetFirstRaw(req, "host-redirects", "redirects")))
		reqIns.MaxRedirects = utils.MapGetInt(req, "max-redirects")

		if ret := utils.MapGetRaw(req, "raw"); ret != nil {
			reqIns.HTTPRequests = funk.Map(utils.InterfaceToStringSlice(ret), func(i string) *YakHTTPRequestPacket {
				return nucleiRawPacketToYakHTTPRequestPacket(i)
			}).([]*YakHTTPRequestPacket)
		} else {
			reqIns.Method = utils.MapGetString(req, "method")
			reqIns.Paths = utils.InterfaceToStringSlice(utils.MapGetRaw(req, "path"))
			for k, v := range utils.InterfaceToMapInterface(utils.MapGetRaw(req, "headers")) {
				reqIns.Headers[k] = toString(v)
			}
			reqIns.Body = utils.MapGetString(req, "body")
		}

		if len(reqIns.HTTPRequests) <= 0 && len(reqIns.Paths) == 0 {
			log.Error("http request is empty")
			return nil, utils.Error("http request is empty")
		}
		reqSeq = append(reqSeq, reqIns)
	}
	_ = hasMatcherOrExtractor
	//if !hasMatcherOrExtractor {
	//	return nil, utils.Error("matcher and extractor are both empty")
	//}
	yakTemp.HTTPRequestSequences = reqSeq
	// extractConfig(&yakTemp.RequestConfig, mid)
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
						MatcherType: "status",
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
						MatcherType: "dsl",
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

func generateYakExtractors(req map[string]interface{}) ([]*YakExtractor, error) {
	extractorsRaw := utils.MapGetRaw(req, "extractors")
	if extractorsRaw == nil {
		return nil, nil
	}
	if reflect.TypeOf(extractorsRaw).Kind() != reflect.Slice {
		return nil, utils.Errorf("nuclei template extractors is not slice")
	}

	var extractors []*YakExtractor
	funk.Map(extractorsRaw, func(i interface{}) error {
		ext := &YakExtractor{}
		m := utils.InterfaceToMapInterface(i)
		ext.Name = utils.MapGetString(m, "name")
		ext.Scope = utils.MapGetString(m, "scope")
		ext.Id = utils.MapGetInt(m, "id")

		switch utils.MapGetString(m, "type") {
		case "regex":
			ext.Type = "regex"
			ext.Groups = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "regex"))
			ext.RegexpMatchGroup = []int{utils.MapGetInt(m, "group")}
		case "kval":
			ext.Type = "key-value"
			ext.Groups = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "kval"))
		case "json":
			ext.Type = "json"
			ext.Groups = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "json"))
		case "xpath":
			ext.Type = "xpath"
			ext.Groups = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "xpath"))
			ext.XPathAttribute = utils.MapGetString(m, "attribute")
		case "dsl":
			ext.Type = "dsl"
			ext.Groups = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "dsl"))
		default:
			log.Errorf("extractYakExtractor failed: %v", utils.Errorf("nuclei template extractors type is not supported"))
			return utils.Errorf("nuclei template extractors type is not supported")
		}
		extractors = append(extractors, ext)
		return nil
	})
	return extractors, nil
}

func generateYakMatcher(req map[string]interface{}) (*YakMatcher, error) {
	matchersRaw := utils.MapGetRaw(req, "matchers")
	if matchersRaw == nil {
		return nil, utils.Errorf("nuclei template matchers is nil")
	}
	if reflect.TypeOf(matchersRaw).Kind() != reflect.Slice {
		return nil, utils.Errorf("nuclei template matchers is not slice")
	}
	var matchers []*YakMatcher
	funk.Map(matchersRaw, func(i interface{}) error {
		match := &YakMatcher{
			MatcherType: "",
			ExprType:    "",
			Scope:       "",
			Condition:   "",
			Group:       nil,
		}
		m := utils.InterfaceToMapInterface(i)
		match.Negative = utils.MapGetBool(m, "negative")
		match.Condition = utils.MapGetString(m, "condition")
		match.Id = utils.MapGetInt(m, "id")

		switch utils.MapGetString(m, "part") {
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
		}

		switch utils.MapGetString(m, "type") {
		case "word":
			match.MatcherType = "word"
			match.GroupEncoding = utils.InterfaceToString(utils.MapGetRaw(m, "encoding"))
			match.Group = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "words"))
		case "status":
			match.MatcherType = "status_code"
			match.Group = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "status"))
		case "size":
			match.MatcherType = "content_length"
			match.Group = utils.InterfaceToStringSlice(utils.MapGetFirstRaw(m, "size", "sizes", "content-length"))
		case "binary":
			match.MatcherType = "binary"
			match.Group = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "binary"))
		case "regex":
			match.MatcherType = "regex"
			match.Group = utils.InterfaceToStringSlice(utils.MapGetFirstRaw(m, "regex", "regexp"))
		case "dsl":
			match.MatcherType = "expr"
			match.ExprType = "nuclei-dsl"
			match.Group = utils.InterfaceToStringSlice(utils.MapGetRaw(m, "dsl"))
		default:
			log.Errorf("parse nuclei template matcher type failed: %v", utils.MapGetString(m, "type"))
			return nil
		}

		matchers = append(matchers, match)
		return nil
	})

	var matchInstance *YakMatcher
	if len(matchers) > 1 {
		matchInstance = &YakMatcher{
			SubMatcherCondition: utils.MapGetString(req, "matchers-condition"),
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

func NewYakPayloads(data map[string]any) (*YakPayloads, error) {
	payloads := &YakPayloads{raw: map[string]*YakPayload{}}
	return payloads, payloads.AddPayloads(data)
}

func generateYakPayloads(req map[string]interface{}) (*YakPayloads, error) {
	data := utils.MapGetMapRaw(req, "payloads")
	if data == nil {
		return nil, nil
	}
	return NewYakPayloads(utils.InterfaceToMapInterface(data))
}

func generateYakVariables(req map[string]interface{}) *YakVariables {
	data := utils.MapGetMapRaw(req, "variables")
	vars := NewVars()
	if data == nil {
		return vars
	}
	for k, v := range utils.InterfaceToMapInterface(data) {
		//tags := ParseNucleiTag(toString(v))
		//if len(tags) == 0 {
		//	vars.Set(k, toString(v))
		//	continue
		//}
		//if len(tags) == 1 && !tags[0].IsExpr {
		//	vars.Set(k, tags[0].Content)
		//	continue
		//}
		vars.SetNucleiDSL(k, toString(v))
	}
	return vars
}
