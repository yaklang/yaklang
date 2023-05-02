package httptpl

import (
	"bufio"
	"fmt"
	"github.com/segmentio/ksuid"
	"gopkg.in/yaml.v3"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
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
var preprocessorRegex = regexp.MustCompile(`{{([a-z0-9_]+)}}`)

// expandPreprocessors expands the pre-processors if any for a template data.
func expandPreprocessors(data string) string {
	foundMap := make(map[string]struct{})
	for _, expression := range preprocessorRegex.FindAllStringSubmatch(string(data), -1) {
		if len(expression) != 2 {
			continue
		}
		value := expression[1]
		if strings.Contains(value, "(") || strings.Contains(value, ")") {
			continue
		}

		if _, ok := foundMap[value]; ok {
			continue
		}
		foundMap[value] = struct{}{}
		if strings.EqualFold(value, "randstr") || strings.HasPrefix(value, "randstr_") {
			data = strings.ReplaceAll(data, expression[0], ksuid.New().String())
		}
	}
	return data
}

const nucleiReverseTag = `"{{interactsh-url}}"`

type NucleiTagData struct {
	IsExpr  bool
	Content string
}

func ParseNucleiTag(raw string) []*NucleiTagData {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Split(bufio.ScanBytes)
	var data []*NucleiTagData
	var last string
	var status = "raw"
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

func CreateYakTemplateFromNucleiTemplateRaw(raw string) (*YakTemplate, error) {
	if strings.Contains(raw, "{{") {
		raw = expandPreprocessors(raw)
	}

	yakTemp := &YakTemplate{}
	for _, interactshTag := range []string{nucleiReverseTag, `{{interactsh}}`, `{{interactsh_url}}`, `interactsh`} {
		if !yakTemp.ReverseConnectionNeed {
			yakTemp.ReverseConnectionNeed = strings.Contains(raw, interactshTag)
		}

		if yakTemp.ReverseConnectionNeed {
			raw = strings.ReplaceAll(raw, nucleiReverseTag, `{{params(reverse_url)}}`)
		}
	}

	var mid = map[string]interface{}{}
	err := yaml.Unmarshal([]byte(raw), &mid)
	if err != nil {
		return nil, utils.Errorf("unmarshal nuclei template failed: %v", err)
	}
	yakTemp.Id = utils.MapGetString(mid, "id")
	info := utils.InterfaceToMapInterface(utils.MapGetRaw(mid, "info"))

	cveInfo := utils.InterfaceToMapInterface(utils.MapGetRaw(info, "classification"))
	yakTemp.Name = utils.MapGetString(info, "name")
	yakTemp.Author = utils.MapGetString(info, "author")
	yakTemp.Severity = utils.MapGetString(info, "severity")
	yakTemp.Description = utils.MapGetString(info, "description")
	yakTemp.Reference = utils.InterfaceToStringSlice(utils.MapGetRaw(info, "reference"))
	yakTemp.Tags = utils.PrettifyListFromStringSplitEx(utils.MapGetString(info, "tags"), ",")
	yakTemp.CVE = utils.MapGetString(cveInfo, "cve-id")

	reqs := utils.MapGetFirstRaw(mid, "requests", "http")
	if reqs == nil || (reqs != nil && reflect.TypeOf(reqs).Kind() != reflect.Slice) {

		if utils.MapGetFirstRaw(mid, "network") != nil {
			return nil, utils.Errorf("nuclei template `network(tcp)` is not supported (*)")
		}

		if utils.MapGetFirstRaw(mid, "headless") != nil {
			return nil, utils.Errorf("nuclei template `headless(crawler)` is not supported (*)")
		}

		return nil, utils.Errorf("nuclei template requests is not slice")
	}

	yakTemp.Variables = generateYakVariables(mid)

	// parse req seqs
	var reqSeq []*YakRequestBulkConfig
	funk.Map(reqs, func(i interface{}) error {
		reqIns := &YakRequestBulkConfig{}

		req := utils.InterfaceToMapInterface(i)
		matcher, err := generateYakMatcher(req)
		if err != nil {
			log.Debugf("extractYakExtractor failed: %v", err)
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
		if reqIns.Matcher == nil && len(reqIns.Extractor) <= 0 {
			log.Error("matcher and extractor are both empty")
			return utils.Error("matcher and extractor are both empty")
		}
		reqIns.EnableRedirect, _ = strconv.ParseBool(utils.InterfaceToString(utils.MapGetFirstRaw(req, "host-redirects", "redirects")))
		reqIns.MaxRedirects = utils.MapGetInt(req, "max-redirects")
		reqIns.CookieInherit = utils.MapGetBool(req, "cookie-reuse")
		reqIns.MaxSize = utils.MapGetInt(req, "max-size")
		reqIns.NoFixContentLength = utils.MapGetBool(req, "unsafe")
		reqIns.AfterRequested = utils.MapGetBool(req, "req-condition")

		if ret := utils.MapGetRaw(req, "raw"); ret != nil {
			reqIns.HTTPRequests = funk.Map(utils.InterfaceToStringSlice(ret), func(i string) *YakHTTPRequestPacket {
				return nucleiRawPacketToYakHTTPRequestPacket(i)
			}).([]*YakHTTPRequestPacket)
		} else {
			method := utils.MapGetString(req, "method")
			paths := utils.InterfaceToStringSlice(utils.MapGetRaw(req, "path"))
			for _, path := range paths {
				var firstLine string = fmt.Sprintf("%v %v HTTP/1.1", method, path)
				if strings.HasPrefix(path, "{{BaseURL}}") {
					if len(path) > 11 && path[11] == '/' {
						firstLine = fmt.Sprintf("%v %v HTTP/1.1", method, strings.ReplaceAll(path, "{{BaseURL}}", "{{params(__path_trim_end_slash__)}}"))
					} else {
						firstLine = fmt.Sprintf("%v %v HTTP/1.1", method, strings.ReplaceAll(path, "{{BaseURL}}", "{{params(__path__)}}"))
					}
				} else if strings.HasPrefix(path, "{{RootURL}}") {
					firstLine = fmt.Sprintf("%v %v HTTP/1.1", method, strings.ReplaceAll(path, "{{RootURL}}", ""))
				}
				firstLine = nucleiFormatRequestTemplate(firstLine)

				// 处理
				var lines []string
				lines = append(lines, firstLine)
				headersRaw := utils.MapGetRaw(req, "headers")
				headers := utils.InterfaceToMapInterface(headersRaw)
				_, hostOk1 := headers["Host"]
				_, hostOk2 := headers["host"]
				if !hostOk1 && !hostOk2 {
					lines = append(lines, "Host: {{params(__hostname__)}}")
				}
				for k, v := range headers {
					lines = append(lines, fmt.Sprintf(`%v: %v`, k, nucleiFormatRequestTemplate(utils.InterfaceToString(v))))
				}
				if len(headers) <= 0 {
					lines = append(lines, `User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:78.0) Gecko/20100101 Firefox/78.0`)
				}
				var rawPacket = strings.Join(lines, "\r\n") + "\r\n\r\n"
				rawPacket += utils.MapGetString(req, "body")
				reqIns.HTTPRequests = append(reqIns.HTTPRequests, &YakHTTPRequestPacket{Request: rawPacket})
			}
		}

		if len(reqIns.HTTPRequests) <= 0 {
			log.Error("http request is empty")
			return utils.Error("http request is empty")
		}
		reqSeq = append(reqSeq, reqIns)
		return nil
	})
	yakTemp.HTTPRequestSequences = reqSeq
	return yakTemp, nil
}

// variableTag
var variableRegexp = regexp.MustCompile(`(?i)\{\{([a-z_][a-z0-9_]*)}}`)
var exprRegexp = regexp.MustCompile(`(?i)\{\{[^}\r\n]+}}`)

func nucleiFormatRequestTemplate(r string) string {
	r = strings.ReplaceAll(r, "{{BaseURL}}", "{{params(__base_url__)}}")
	r = strings.ReplaceAll(r, "{{BaseUrl}}", "{{params(__base_url__)}}")
	r = strings.ReplaceAll(r, "{{RootURL}}", "{{params(__root_url__)}}")
	r = strings.ReplaceAll(r, "{{RootUrl}}", "{{params(__root_url__)}}")
	r = strings.ReplaceAll(r, "{{Hostname}}", "{{params(__hostname__)}}")
	r = strings.ReplaceAll(r, "{{Host}}", "{{params(__host__)}}")
	r = strings.ReplaceAll(r, "{{Port}}", "{{params(__port__)}}")
	r = strings.ReplaceAll(r, "{{Path}}", "{{params(__path__)}}")
	r = strings.ReplaceAll(r, "{{File}}", "{{params(__file__)}}")
	r = strings.ReplaceAll(r, "{{Schema}}", "{{params(__schema__)}}")

	if variableRegexp.MatchString(r) {
		r = variableRegexp.ReplaceAllStringFunc(r, func(s string) string {
			return fmt.Sprintf("{{params(%v)}}", strings.Trim(s, "{}"))
		})
	}

	if exprRegexp.MatchString(r) {
		r = exprRegexp.ReplaceAllStringFunc(r, func(s string) string {
			if strings.HasPrefix(s, "{{params") {
				return s
			}
			return fmt.Sprintf("{{expr:nuclei-dsl(%v)}}", strings.Trim(s, "{}"))
		})
	}

	return r
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
	packet.Request = nucleiFormatRequestTemplate(packet.Request)
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

		switch utils.MapGetString(m, "part") {
		case "body":
			match.Scope = "body"
		case "header":
			match.Scope = "header"
		case "status":
			match.Scope = "status"
		case "raw", "":
			match.Scope = "raw"
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

func generateYakPayloads(req map[string]interface{}) (*YakPayloads, error) {
	data := utils.MapGetMapRaw(req, "payloads")
	if data == nil {
		return nil, nil
	}

	payloads := &YakPayloads{raw: map[string]*YakPayload{}}
	for k, v := range utils.InterfaceToMapInterface(data) {
		if reflect.TypeOf(v).Kind() == reflect.Slice {
			payloads.raw[k] = &YakPayload{
				Data: utils.InterfaceToStringSlice(v),
			}
		} else {
			payload := &YakPayload{
				FromFile: toString(v),
			}
			if utils.GetFirstExistedFile(payload.FromFile) != "" {
				payload.Data = utils.ParseStringToLines(payload.FromFile)
				payloads.raw[k] = payload
			} else {
				err := utils.Errorf("nuclei template payloads file not found: %s", payload.FromFile)
				log.Error(err)
				return nil, err
			}
		}
	}
	return payloads, nil
}

func generateYakVariables(req map[string]interface{}) *YakVariables {
	data := utils.MapGetMapRaw(req, "variables")
	if data == nil {
		return nil
	}
	vars := NewVars()
	for k, v := range utils.InterfaceToMapInterface(data) {
		tags := ParseNucleiTag(toString(v))
		if len(tags) == 0 {
			vars.Set(k, toString(v))
			continue
		}
		if len(tags) == 1 && !tags[0].IsExpr {
			vars.Set(k, tags[0].Content)
			continue
		}
		vars.SetNucleiDSL(k, tags)
	}
	return vars
}
