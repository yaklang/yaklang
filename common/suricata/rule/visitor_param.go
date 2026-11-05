package rule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/numrange"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/suricata/pcre"
)

var flowTokens = map[string]struct{}{
	"to_server": {}, "from_client": {}, "to_client": {}, "from_server": {},
	"established": {}, "not_established": {}, "stateless": {}, "only_stream": {}, "no_stream": {},
}

func parseCommaParts(raw string) []string {
	pieces := strings.Split(raw, ",")
	parts := make([]string, 0, len(pieces))
	for _, piece := range pieces {
		parts = append(parts, strings.TrimSpace(piece))
	}
	return parts
}

func parseFlowRule(parts []string) (*FlowRule, error) {
	flow := &FlowRule{}
	seen := make(map[string]bool)
	for _, raw := range parts {
		token := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := flowTokens[token]; !ok {
			return nil, fmt.Errorf("unknown flow token %q", raw)
		}
		if seen[token] {
			return nil, fmt.Errorf("duplicate flow token %q", token)
		}
		seen[token] = true
		flow.Tokens = append(flow.Tokens, token)
		switch token {
		case "to_server", "from_client":
			flow.ToServer = true
		case "to_client", "from_server":
			flow.ToClient = true
		case "established":
			flow.Established = true
		case "not_established":
			flow.NotEstablished = true
		case "stateless":
			flow.Stateless = true
		case "only_stream":
			flow.OnlyStream = true
		case "no_stream":
			flow.NoStream = true
		}
	}
	if flow.ToServer && flow.ToClient {
		return nil, fmt.Errorf("flow direction tokens conflict")
	}
	if flow.Established && flow.NotEstablished {
		return nil, fmt.Errorf("flow established and not_established conflict")
	}
	if flow.Stateless && (flow.Established || flow.NotEstablished || flow.OnlyStream) {
		return nil, fmt.Errorf("flow stateless conflicts with connection or stream state")
	}
	if flow.OnlyStream && flow.NoStream {
		return nil, fmt.Errorf("flow only_stream and no_stream conflict")
	}
	return flow, nil
}

func parseFlowBits(raw string, parts []string) (*FlowBitsOption, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("flowbits requires an operation")
	}
	operation := FlowBitsOperation(strings.ToLower(parts[0]))
	option := &FlowBitsOption{Raw: raw, Operation: operation}
	switch operation {
	case FlowBitsNoAlert:
		if len(parts) != 1 {
			return nil, fmt.Errorf("flowbits noalert does not accept a name")
		}
	case FlowBitsSet, FlowBitsUnset, FlowBitsToggle, FlowBitsIsSet, FlowBitsIsNotSet:
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("flowbits %s requires exactly one name", operation)
		}
		option.Name = strings.TrimSpace(parts[1])
	default:
		return nil, fmt.Errorf("unknown flowbits operation %q", parts[0])
	}
	return option, nil
}

func parseStateValueOption(keyword, raw string, parts []string) (*StateValueOption, error) {
	if len(parts) < 2 {
		return nil, fmt.Errorf("%s requires operation and name", keyword)
	}
	option := &StateValueOption{Raw: raw}
	if keyword == "flowint" {
		option.Name = strings.TrimSpace(parts[0])
		option.Operation = strings.ToLower(strings.TrimSpace(parts[1]))
	} else {
		option.Operation = strings.ToLower(strings.TrimSpace(parts[0]))
		option.Name = strings.TrimSpace(parts[1])
	}
	for i := 2; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])
		fields := strings.Fields(part)
		if len(fields) == 2 {
			switch strings.ToLower(fields[0]) {
			case "track":
				option.Track = strings.ToLower(fields[1])
				continue
			case "expire":
				expire, err := strconv.Atoi(fields[1])
				if err != nil || expire < 0 {
					return nil, fmt.Errorf("invalid %s expire %q", keyword, fields[1])
				}
				option.Expire = &expire
				continue
			}
		}
		option.Arguments = append(option.Arguments, part)
	}
	return option, nil
}

func parseStateMetric(keyword, raw string, parts []string) (*StateMetricRule, error) {
	metric := &StateMetricRule{Raw: raw}
	value := ""
	if keyword == "stream_size" {
		if len(parts) != 2 && len(parts) != 3 {
			return nil, fmt.Errorf("stream_size requires direction and range")
		}
		metric.Direction = strings.ToLower(parts[0])
		switch metric.Direction {
		case "client", "server", "both", "either":
		default:
			return nil, fmt.Errorf("invalid stream_size direction %q", parts[0])
		}
		value = strings.Join(parts[1:], "")
	} else {
		switch len(parts) {
		case 1:
			value = parts[0]
		case 2:
			if parts[0] == "<" || parts[0] == ">" || parts[0] == "!" {
				value = strings.Join(parts, "")
			} else {
				metric.Direction = strings.ToLower(parts[0])
				value = parts[1]
			}
		case 3:
			metric.Direction = strings.ToLower(parts[0])
			value = strings.Join(parts[1:], "")
		default:
			return nil, fmt.Errorf("%s requires a range with optional direction", keyword)
		}
	}
	numericRange, err := numrange.ParseNumRange(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("parse %s range %q: %w", keyword, value, err)
	}
	metric.Range = numericRange
	return metric, nil
}

func dnsRRTypeNumber(raw string) string {
	if value, ok := map[string]string{
		"a": "1", "ns": "2", "cname": "5", "soa": "6", "ptr": "12", "mx": "15", "txt": "16",
		"aaaa": "28", "srv": "33", "opt": "41", "rrsig": "46", "dnskey": "48", "svcb": "64", "https": "65",
	}[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return value
	}
	return raw
}

func modifierMapping(str string) modifier.Modifier {
	switch str {
	case "file_data", "file.data":
		return modifier.FileData
	case "http_content_type", "http.content_type":
		return modifier.HTTPContentType
	case "http_content_len", "http.content_len":
		return modifier.HTTPContentLen
	case "http_start", "http.start":
		return modifier.HTTPStart
	case "http_protocol", "http.protocol":
		return modifier.HTTPProtocol
	case "http_header_names", "http.header_names":
		return modifier.HTTPHeaderNames
	case "http_request_line", "http.request_line":
		return modifier.HTTPRequestLine
	case "http_accept", "http.accept":
		return modifier.HTTPAccept
	case "http_accept_enc", "http.accept_enc":
		return modifier.HTTPAcceptEnc
	case "http_accept_lang", "http.accept_lang":
		return modifier.HTTPAcceptLang
	case "http_referer", "http.referer":
		return modifier.HTTPReferer
	case "http_connection", "http.connection":
		return modifier.HTTPConnection
	case "http_response_line", "http.response_line":
		return modifier.HTTPResponseLine
	case "dns_query", "dns.query":
		return modifier.DNSQuery
	case "http_header", "http.header":
		return modifier.HTTPHeader
	case "http_raw_header", "http.raw_header", "http.header.raw":
		return modifier.HTTPHeaderRaw
	case "http_cookie", "http.cookie":
		return modifier.HTTPCookie
	case "http_uri", "http.uri":
		return modifier.HTTPUri
	case "http_raw_uri", "http.uri.raw":
		return modifier.HTTPUriRaw
	case "http_method", "http.method":
		return modifier.HTTPMethod
	case "http_user_agent", "http.user_agent":
		return modifier.HTTPUserAgent
	case "http_host", "http.host":
		return modifier.HTTPHost
	case "http_raw_host", "http.raw_host", "http.host.raw":
		return modifier.HTTPHostRaw
	case "http_stat_msg", "http.stat_msg":
		return modifier.HTTPStatMsg
	case "http_stat_code", "http.stat_code":
		return modifier.HTTPStatCode
	case "http_client_body", "http.client_body", "http_request_body", "http.request_body":
		return modifier.HTTPRequestBody
	case "http_server_body", "http.server_body", "http.response_body":
		return modifier.HTTPResponseBody
	case "http_server", "http.server":
		return modifier.HTTPServer
	case "http_location", "http.location":
		return modifier.HTTPLocation
	case "ipv4.hdr", "ipv4_hdr":
		return modifier.IPv4HDR
	case "ipv6.hdr", "ipv6_hdr":
		return modifier.IPv6HDR
	case "tcp.hdr", "tcp_hdr":
		return modifier.TCPHDR
	case "ja3.hash":
		return modifier.JA3Hash
	case "ja3s.hash":
		return modifier.JA3SHash
	case "ja3.string":
		return modifier.JA3String
	case "ja3s.string":
		return modifier.JA3SString
	case "tls.cert_subject":
		return modifier.TLSCertSubject
	}
	return modifier.Default
}
func mustSoloSingleSetting(ssts []parser.ISingleSettingContext) (bool, string) {
	if len(ssts) != 1 {
		return false, ""
	}
	ctx := ssts[0].(*parser.SingleSettingContext)
	data := ctx.GetText()
	return ctx.Negative() != nil, data
}

type MultipleBufferMatching struct {
	last modifier.Modifier
}

func (m *MultipleBufferMatching) transfer(mdf modifier.Modifier) modifier.Modifier {
	switch mdf {
	case modifier.DNSQuery, modifier.FileData, modifier.HTTPHeader:
		m.last = mdf
	case modifier.Default:
		mdf = m.last
	default:
		m.last = modifier.Default
	}
	return mdf
}

func (v *RuleSyntaxVisitor) VisitParams(i *parser.ParamsContext, rule *Rule) (parseError error) {
	const (
		HasNone = iota
		HasContent
		HasModif
		ModifContent
		ContentModif
		SavingPCRE
	)
	STATUS := HasNone
	var contents []*ContentRule
	multipleBufferMatching := new(MultipleBufferMatching)
	contentRule := new(ContentRule)

	params := i.AllParam()
	for i := 0; i < len(params); i++ {
		if params[i] == nil {
			continue
		}
		paramctx := params[i].(*parser.ParamContext)
		key := paramctx.Keyword().GetText()
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		var setting *parser.SettingContext
		var ssts []parser.ISingleSettingContext
		var vStr string

		if st := paramctx.Setting(); st != nil {
			setting = paramctx.Setting().(*parser.SettingContext)
			vStr = strings.TrimSpace(setting.GetText())
			ssts = setting.AllSingleSetting()
		}
		if rule.SettingMap == nil {
			rule.SettingMap = make(map[string]string)
		}
		rule.SettingMap[key] = vStr
		switch STATUS {
		case HasNone:
			if modifierMapping(key) != modifier.Default {
				STATUS = HasModif
			} else if key == "content" {
				STATUS = HasContent
			} else if key == "pcre" {
				STATUS = SavingPCRE
			} else {
				STATUS = HasNone
			}
		case HasContent:
			if modifierMapping(key) != modifier.Default {
				STATUS = ContentModif
			} else if key == "content" {
				contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
				contents = append(contents, contentRule)
				contentRule = new(ContentRule)
				STATUS = HasContent
			} else if key == "pcre" {
				contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
				contents = append(contents, contentRule)
				contentRule = new(ContentRule)
				STATUS = SavingPCRE
			} else {
				STATUS = HasContent
			}
		case SavingPCRE:
			contents = append(contents, contentRule)
			contentRule = new(ContentRule)
			if modifierMapping(key) != modifier.Default {
				STATUS = ContentModif
			} else if key == "content" {
				STATUS = HasContent
			} else if key == "pcre" {
				STATUS = SavingPCRE
			} else {
				STATUS = HasNone
			}
		case HasModif:
			if modifierMapping(key) != modifier.Default {
				contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
				contents = append(contents, contentRule)
				contentRule = new(ContentRule)
				STATUS = HasModif
			} else if key == "content" {
				STATUS = ModifContent
			} else if key == "pcre" {
				STATUS = SavingPCRE
			}
		case ModifContent, ContentModif:
			if modifierMapping(key) != modifier.Default {
				contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
				contents = append(contents, contentRule)
				contentRule = new(ContentRule)
				STATUS = HasModif
			} else if key == "content" {
				contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
				contents = append(contents, contentRule)
				contentRule = new(ContentRule)
				STATUS = HasContent
			} else if key == "pcre" {
				contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
				contents = append(contents, contentRule)
				contentRule = new(ContentRule)
				STATUS = SavingPCRE
			}
		}

		log.Debugf("key: %#v value: %#v", key, vStr)

		switch key {
		// meta keywords
		case "sid":
			rule.Sid, _ = strconv.Atoi(vStr)
		case "rev":
			rule.Rev, _ = strconv.Atoi(vStr)
		case "gid":
			rule.Gid, _ = strconv.Atoi(vStr)
		case "classtype":
			rule.ClassType = vStr
		case "reference":
			if rule.Reference == nil {
				rule.Reference = map[string]string{}
			}
			if len(ssts) == 2 {
				rule.Reference[ssts[0].GetText()] = ssts[1].GetText()
			} else {
				rule.Reference[vStr] = ""
			}
		case "msg":
			rule.Message, _ = strconv.Unquote(vStr)
		case "priority":
			rule.Priority, _ = strconv.Atoi(vStr)
		case "metadata":
			for _, v := range ssts {
				rule.Metadata = append(rule.Metadata, v.GetText())
			}
		case "content":
			neg, content := mustSoloSingleSetting(ssts)
			contentRule.Content = []byte(unquoteAndParseHex(content))
			contentRule.Negative = neg
		case "dns.opcode", "dns_opcode":
			neg, content := mustSoloSingleSetting(ssts)
			if rule.ContentRuleConfig.DNS == nil {
				rule.ContentRuleConfig.DNS = &DNSRule{}
			}
			opcode, err := strconv.Atoi(content)
			if err != nil {
				v.Errorf("parse dns opcode %q: %v", content, err)
				parseError = fmt.Errorf("parse dns opcode %q: %w", content, err)
				return
			}
			rule.ContentRuleConfig.DNS.OpcodeNegative = neg
			rule.ContentRuleConfig.DNS.OpcodeSet = true
			rule.ContentRuleConfig.DNS.Opcode = opcode
		case "dns.rcode", "dns_rcode":
			if rule.ContentRuleConfig.DNS == nil {
				rule.ContentRuleConfig.DNS = &DNSRule{}
			}
			rcode, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse dns rcode %q: %v", vStr, err)
				parseError = fmt.Errorf("parse dns rcode %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.DNS.RCode = rcode
		case "dns.rrtype", "dns_rrtype":
			if rule.ContentRuleConfig.DNS == nil {
				rule.ContentRuleConfig.DNS = &DNSRule{}
			}
			rrtypeValue := dnsRRTypeNumber(vStr)
			rrtype, err := numrange.ParseNumRange(rrtypeValue)
			if err != nil {
				v.Errorf("parse dns rrtype %q: %v", vStr, err)
				parseError = fmt.Errorf("parse dns rrtype %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.DNS.RRType = rrtype
		case "flow":
			flow, err := parseFlowRule(parseCommaParts(vStr))
			if err != nil {
				v.Errorf("parse flow %q: %v", vStr, err)
				parseError = fmt.Errorf("parse flow %q: %w", vStr, err)
				return
			}
			if rule.ContentRuleConfig.Flow != nil {
				v.Errorf("parse flow %q: duplicate flow option", vStr)
				parseError = fmt.Errorf("parse flow %q: duplicate flow option", vStr)
				return
			}
			rule.ContentRuleConfig.Flow = flow
			rule.StateOptions = append(rule.StateOptions, StateOption{Keyword: key, Raw: vStr, Flow: flow})
		case "ttl":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			ttl, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse ttl %q: %v", vStr, err)
				parseError = fmt.Errorf("parse ttl %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IPConfig.TTL = ttl
		case "sameip":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			rule.ContentRuleConfig.IPConfig.Sameip = true
		case "ipopts":
			/*
				rr	Record Route
				eol	End of List
				nop	No Op
				ts	Time Stamp
				sec	IP Security
				esec	IP Extended Security
				lsrr	Loose Source Routing
				ssrr	Strict Source Routing
				satid	Stream Identifier
				any	any IP options are set
			*/
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			rule.ContentRuleConfig.IPConfig.IPOpts = vStr
		case "ip_proto":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			rule.ContentRuleConfig.IPConfig.IPProto = vStr //number or name
		case "id":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			id, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse id %q: %v", vStr, err)
				parseError = fmt.Errorf("parse id %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IPConfig.Id = id
		case "geoip":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			rule.ContentRuleConfig.IPConfig.Geoip = vStr
		case "fragbits":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			rule.ContentRuleConfig.IPConfig.FragBits = vStr
		case "fragoffset":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			fragOffset, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse fragoffset %q: %v", vStr, err)
				parseError = fmt.Errorf("parse fragoffset %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IPConfig.FragOffset = fragOffset
		case "tos":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			tos, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse tos %q: %v", vStr, err)
				parseError = fmt.Errorf("parse tos %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IPConfig.Tos = tos
		case "flags", "tcp.flags":
			/*
				S: 匹配TCP SYN标志位
				F: 匹配TCP FIN标志位
				R: 匹配TCP RST标志位
				P: 匹配TCP PUSH标志位
				U: 匹配TCP URG标志位
				E: 匹配TCP ECE标志位
				C: 匹配TCP CWR标志位
			*/
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			rule.ContentRuleConfig.TcpConfig.Flags = vStr
		case "seq":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			seq, err := strconv.Atoi(vStr)
			if err != nil {
				v.Errorf("parse seq %q: %v", vStr, err)
				parseError = fmt.Errorf("parse seq %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.TcpConfig.Seq = &seq
		case "ack":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			ack, err := strconv.Atoi(vStr)
			if err != nil {
				v.Errorf("parse ack %q: %v", vStr, err)
				parseError = fmt.Errorf("parse ack %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.TcpConfig.Ack = &ack
		case "window":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			neg, content := mustSoloSingleSetting(ssts)
			window, err := strconv.Atoi(content)
			if err != nil {
				v.Errorf("parse window %q: %v", content, err)
				parseError = fmt.Errorf("parse window %q: %w", content, err)
				return
			}
			rule.ContentRuleConfig.TcpConfig.NegativeWindow, rule.ContentRuleConfig.TcpConfig.Window = neg, &window
		case "threshold", "detection_filter":
			config, err := parseThresholding(key, vStr, parseCommaParts(vStr))
			if err != nil {
				v.Errorf("parse %s %q: %v", key, vStr, err)
				parseError = fmt.Errorf("parse %s %q: %w", key, vStr, err)
				return
			}
			rule.ContentRuleConfig.Thresholding = config
			rule.StateOptions = append(rule.StateOptions, StateOption{Keyword: key, Raw: vStr, Threshold: config})
		case "stream_size", "flow.age", "flow.pkts", "flow.bytes":
			metric, err := parseStateMetric(key, vStr, parseCommaParts(vStr))
			if err != nil {
				v.Errorf("parse %s %q: %v", key, vStr, err)
				parseError = fmt.Errorf("parse %s %q: %w", key, vStr, err)
				return
			}
			switch key {
			case "stream_size":
				rule.ContentRuleConfig.StreamSize = metric
			case "flow.age":
				rule.ContentRuleConfig.FlowAge = metric
			case "flow.pkts":
				rule.ContentRuleConfig.FlowPkts = metric
			case "flow.bytes":
				rule.ContentRuleConfig.FlowBytes = metric
			}
			rule.StateOptions = append(rule.StateOptions, StateOption{Keyword: key, Raw: vStr, Metric: metric})
		case "icode":
			/*
				icode:min<>max;
				icode:[<|>]<number>;
			*/
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			numRange, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse icmp icode %q: %v", vStr, err)
				parseError = fmt.Errorf("parse icmp icode %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IcmpConfig.ICode = numRange
		case "itype":
			/*
				itype:min<>max;
				itype:[<|>]<number>;
			*/
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			numRange, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse icmp itype %q: %v", vStr, err)
				parseError = fmt.Errorf("parse icmp itype %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IcmpConfig.IType = numRange
		case "icmp_id":
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			num, err := strconv.Atoi(vStr)
			if err != nil {
				v.Errorf("parse icmp_id %q: %v", vStr, err)
				parseError = fmt.Errorf("parse icmp_id %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IcmpConfig.ICMPId = &num
		case "icmp_seq":
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			num, err := strconv.Atoi(vStr)
			if err != nil {
				v.Errorf("parse icmp_seq %q: %v", vStr, err)
				parseError = fmt.Errorf("parse icmp_seq %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.IcmpConfig.ICMPSeq = &num
		case "urilen":
			if rule.ContentRuleConfig.HTTPConfig == nil {
				rule.ContentRuleConfig.HTTPConfig = &HTTPConfig{}
			}
			numRange, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse urilen %q: %v", vStr, err)
				parseError = fmt.Errorf("parse urilen %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.HTTPConfig.Urilen = numRange
		case "nocase":
			contentRule.Nocase = true
		case "depth":
			value, err := parseOptionalInt(vStr)
			if err != nil {
				v.Errorf("parse depth %q: %v", vStr, err)
				parseError = fmt.Errorf("parse depth %q: %w", vStr, err)
				return
			}
			contentRule.Depth = value
		case "offset":
			value, err := parseOptionalInt(vStr)
			if err != nil {
				v.Errorf("parse offset %q: %v", vStr, err)
				parseError = fmt.Errorf("parse offset %q: %w", vStr, err)
				return
			}
			contentRule.Offset = value
		case "startswith":
			contentRule.StartsWith = true
		case "endswith":
			contentRule.EndsWith = true
		case "distance":
			value, err := parseOptionalInt(vStr)
			if err != nil {
				v.Errorf("parse distance %q: %v", vStr, err)
				parseError = fmt.Errorf("parse distance %q: %w", vStr, err)
				return
			}
			contentRule.Distance = value
		case "within":
			value, err := parseOptionalInt(vStr)
			if err != nil {
				v.Errorf("parse within %q: %v", vStr, err)
				parseError = fmt.Errorf("parse within %q: %w", vStr, err)
				return
			}
			contentRule.Within = value
		case "rawbytes":
			contentRule.RawBytes = true
		case "isdataat":
			contentRule.IsDataAt = vStr
			if STATUS == HasNone {
				STATUS = HasContent
			}
		case "bsize":
			contentRule.BSize = vStr
		case "dsize":
			contentRule.DSize = vStr
		case "byte_test":
			contentRule.ByteTest = vStr
		case "byte_math":
			contentRule.ByteMath = vStr
		case "byte_extract":
			contentRule.ByteExtract = vStr
		case "byte_jump":
			contentRule.ByteJump = vStr
		case "rpc":
			contentRule.RPC = vStr
		case "replace":
			contentRule.Replace = []byte(unquoteAndParseHex(vStr))
		case "pcre":
			contentRule.PCRE = unquoteString(vStr)
			parsed, err := pcre.ParsePCREStr(contentRule.PCRE)
			if err != nil {
				v.Errorf("parsePCREStr err:%v", err)
				parseError = fmt.Errorf("parsePCREStr err:%v", err)
				return
			}
			contentRule.PCREParsed = parsed
			if parsed.Modifier() != modifier.Default {
				contentRule.Modifier = parsed.Modifier()
			}
		case "tcp.mss":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			numRange, err := numrange.ParseNumRange(vStr)
			if err != nil {
				v.Errorf("parse tcp.mss %q: %v", vStr, err)
				parseError = fmt.Errorf("parse tcp.mss %q: %w", vStr, err)
				return
			}
			rule.ContentRuleConfig.TcpConfig.TCPMss = numRange
		case "fast_pattern":
			contentRule.FastPattern = true
		case "flowbits":
			flowBits, err := parseFlowBits(vStr, parseCommaParts(vStr))
			if err != nil {
				v.Errorf("parse flowbits %q: %v", vStr, err)
				parseError = fmt.Errorf("parse flowbits %q: %w", vStr, err)
				return
			}
			contentRule.FlowBits = vStr
			if flowBits.Operation == FlowBitsNoAlert {
				contentRule.NoAlert = true
			}
			rule.StateOptions = append(rule.StateOptions, StateOption{Keyword: key, Raw: vStr, FlowBits: flowBits})
		case "noalert":
			contentRule.NoAlert = true
			rule.StateOptions = append(rule.StateOptions, StateOption{Keyword: key, NoAlert: true})
		case "base64_decode":
			contentRule.Base64Decode = vStr
		case "base64_data":
			contentRule.Base64Data = true
		case "flowint":
			flowInt, err := parseStateValueOption(key, vStr, parseCommaParts(vStr))
			if err != nil {
				v.Errorf("parse flowint %q: %v", vStr, err)
				parseError = fmt.Errorf("parse flowint %q: %w", vStr, err)
				return
			}
			contentRule.FlowInt = vStr
			rule.StateOptions = append(rule.StateOptions, StateOption{Keyword: key, Raw: vStr, FlowInt: flowInt})
		case "xbits":
			xBits, err := parseStateValueOption(key, vStr, parseCommaParts(vStr))
			if err != nil {
				v.Errorf("parse xbits %q: %v", vStr, err)
				parseError = fmt.Errorf("parse xbits %q: %w", vStr, err)
				return
			}
			contentRule.XBits = vStr
			rule.StateOptions = append(rule.StateOptions, StateOption{Keyword: key, Raw: vStr, XBits: xBits})
		case "app-layer-event":
			contentRule.ExtraFlags = append(contentRule.ExtraFlags, fmt.Sprintf("%v:%v", key, vStr))
		default:
			if modifierMapping(key) != modifier.Default {
				contentRule.Modifier = modifierMapping(key)
				break
			}
			// fallback
			contentRule.ExtraFlags = append(contentRule.ExtraFlags, fmt.Sprintf("%v:%v", key, vStr))
			if key != "metad" {
				log.Debugf("unknown content rule params: %s\n%v\n\n", key, rule.Raw)
			} else {
				log.Warnf("BAD RULE:\n\n%v\n\n", rule.Raw)
			}
		}
	}

	if STATUS != HasNone {
		contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
		contents = append(contents, contentRule)
	}

	rule.ContentRuleConfig.ContentRules = append(rule.ContentRuleConfig.ContentRules, contents...)
	return
}
