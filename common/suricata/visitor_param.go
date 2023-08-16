package suricata

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
)

func modifierMapping(str string) Modifier {
	switch str {
	case "file_data", "file.data":
		return FileData
	case "http_content_type", "http.content_type":
		return HTTPContentType
	case "http_content_len", "http.content_len":
		return HTTPContentLen
	case "http_start", "http.start":
		return HTTPStart
	case "http_protocol", "http.protocol":
		return HTTPProtocol
	case "http_header_names", "http.header_names":
		return HTTPHeaderNames
	case "http_request_line", "http.request_line":
		return HTTPRequestLine
	case "http_accept", "http.accept":
		return HTTPAccept
	case "http_accept_enc", "http.accept_enc":
		return HTTPAcceptEnc
	case "http_referer", "http.referer":
		return HTTPReferer
	case "http_connection", "http.connection":
		return HTTPConnection
	case "http_response_line", "http.response_line":
		return HTTPResponseLine
	case "dns_query", "dns.query":
		return DNSQuery
	case "http_header", "http.header":
		return HTTPHeader
	case "http_raw_header", "http.raw_header":
		return HTTPHeaderRaw
	case "http_cookie", "http.cookie":
		return HTTPCookie
	case "http_uri", "http.uri":
		return HTTPUri
	case "http_raw_uri", "http.uri.raw":
		return HTTPUriRaw
	case "http_method", "http.method":
		return HTTPMethod
	case "http_user_agent", "http.user_agent":
		return HTTPUserAgent
	case "http_host", "http.host":
		return HTTPHost
	case "http_raw_host", `http.raw_host`:
		return HTTPHostRaw
	case "http_stat_msg", "http.stat_msg":
		return HTTPStatMsg
	case "http_stat_code", "http.stat_code":
		return HTTPStatCode
	case "http_client_body", "http.client_body":
		return HTTPRequestBody
	case "http_server_body", "http.server_body":
		return HTTPResponseBody
	case "http_server", "http.server":
		return HTTPServer
	case "http_location", "http.location":
		return HTTPLocation
	case "ipv4.hdr", "ipv4_hdr":
		return IPv4HDR
	case "ipv6.hdr", "ipv6_hdr":
		return IPv6HDR
	}
	return Default
}
func mustSoloSingleSetting(ssts []parser.ISingleSettingContext) (bool, string) {
	if len(ssts) != 1 {
		return false, ""
	}
	ctx := ssts[0].(*parser.SingleSettingContext)
	return ctx.Negative() != nil, ctx.Settingcontent().GetText()
}

type MultipleBufferMatching struct {
	last Modifier
}

func (m *MultipleBufferMatching) transfer(modifier Modifier) Modifier {
	switch modifier {
	case DNSQuery, FileData, HTTPHeader:
		m.last = modifier
	case Default:
		modifier = m.last
	default:
		m.last = Default
	}
	return modifier
}

func (r *RuleSyntaxVisitor) VisitParams(i *parser.ParamsContext, rule *Rule) {
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
		if key == "" {
			continue
		}

		var setting *parser.SettingContext
		var ssts []parser.ISingleSettingContext
		var vStr string
		vParams := make(map[string]interface{})

		if st := paramctx.Setting(); st != nil {
			setting = paramctx.Setting().(*parser.SettingContext)
			vStr = setting.GetText()
			ssts = setting.AllSingleSetting()
		}

		switch STATUS {
		case HasNone:
			if modifierMapping(key) != Default {
				STATUS = HasModif
			} else if key == "content" {
				STATUS = HasContent
			} else if key == "pcre" {
				STATUS = SavingPCRE
			} else {
				STATUS = HasNone
			}
		case HasContent:
			if modifierMapping(key) != Default {
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
			if modifierMapping(key) != Default {
				STATUS = ContentModif
			} else if key == "content" {
				STATUS = HasContent
			} else if key == "pcre" {
				STATUS = SavingPCRE
			} else {
				STATUS = HasNone
			}
		case HasModif:
			if modifierMapping(key) != Default {
				contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
				contents = append(contents, contentRule)
				contentRule = new(ContentRule)
				STATUS = HasModif
			} else if key == "content" {
				STATUS = ModifContent
			}
		case ModifContent, ContentModif:
			if modifierMapping(key) != Default {
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
		// payload keyword
		case "file_data", "file.data":
			contentRule.Modifier = FileData
		case "http_content_type", "http.content_type":
			contentRule.Modifier = HTTPContentType
		case "http_content_len", "http.content_len":
			contentRule.Modifier = HTTPContentLen
		case "http_start", "http.start":
			contentRule.Modifier = HTTPStart
		case "http_protocol", "http.protocol":
			contentRule.Modifier = HTTPProtocol
		case "http_header_names", "http.header_names":
			contentRule.Modifier = HTTPHeaderNames
		case "http_request_line", "http.request_line":
			contentRule.Modifier = HTTPRequestLine
		case "http_accept", "http.accept":
			contentRule.Modifier = HTTPAccept
		case "http_accept_enc", "http.accept_enc":
			contentRule.Modifier = HTTPAcceptEnc
		case "http_referer", "http.referer":
			contentRule.Modifier = HTTPReferer
		case "http_connection", "http.connection":
			contentRule.Modifier = HTTPConnection
		case "http_response_line", "http.response_line":
			contentRule.Modifier = HTTPResponseLine
		case "dns_query", "dns.query":
			contentRule.Modifier = DNSQuery
		case "http_header", "http.header":
			contentRule.Modifier = HTTPHeader
		case "http_raw_header", "http.raw_header":
			contentRule.Modifier = HTTPHeaderRaw
		case "http_cookie", "http.cookie":
			contentRule.Modifier = HTTPCookie
		case "http_uri", "http.uri":
			contentRule.Modifier = HTTPUri
		case "http_raw_uri", "http.uri.raw":
			contentRule.Modifier = HTTPUriRaw
		case "http_method", "http.method":
			contentRule.Modifier = HTTPMethod
		case "http_user_agent", "http.user_agent":
			contentRule.Modifier = HTTPUserAgent
		case "http_host", "http.host":
			contentRule.Modifier = HTTPHost
		case "http_raw_host", `http.raw_host`:
			contentRule.Modifier = HTTPHostRaw
		case "http_stat_msg", "http.stat_msg":
			contentRule.Modifier = HTTPStatMsg
		case "http_stat_code", "http.stat_code":
			contentRule.Modifier = HTTPStatCode
		case "http_client_body", "http.client_body":
			contentRule.Modifier = HTTPRequestBody
		case "http_server_body", "http.server_body":
			contentRule.Modifier = HTTPResponseBody
		case "http_server", "http.server":
			contentRule.Modifier = HTTPServer
		case "http_location", "http.location":
			contentRule.Modifier = HTTPLocation
		case "ipv4.hdr", "ipv4_hdr":
			contentRule.Modifier = IPv4HDR
		case "ipv6.hdr", "ipv6_hdr":
			contentRule.Modifier = IPv6HDR
		case "content":
			neg, content := mustSoloSingleSetting(ssts)
			contentRule.Content = []byte(unquoteAndParseHex(content))
			contentRule.Negative = neg
		case "dns.opcode", "dns_opcode":
			neg, content := mustSoloSingleSetting(ssts)
			rule.ContentRuleConfig.DNS = &DNSRule{
				OpcodeNegative: neg,
				Opcode:         atoi(content),
			}
		case "flow":
			if rule.ContentRuleConfig.Flow == nil {
				lvstr := strings.ToLower(vStr)
				rule.ContentRuleConfig.Flow = &FlowRule{
					ToClient:    strings.Contains(lvstr, "to_client"),
					Established: strings.Contains(lvstr, "established"),
					ToServer:    strings.Contains(lvstr, "to_server"),
				}
			}
		case "ttl":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			rule.ContentRuleConfig.IPConfig.TTL = atoi(vStr)
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
			rule.ContentRuleConfig.IPConfig.Id = atoi(vStr)
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
			rule.ContentRuleConfig.IPConfig.FragOffset = vStr
		case "tos":
			if rule.ContentRuleConfig.IPConfig == nil {
				rule.ContentRuleConfig.IPConfig = &IPLayerRule{}
			}
			rule.ContentRuleConfig.IPConfig.Tos = vStr
		case "flags":
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
			rule.ContentRuleConfig.TcpConfig.Seq = atoi(vStr)
		case "ack":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			rule.ContentRuleConfig.TcpConfig.Ack = atoi(vStr)
		case "window":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			neg, content := mustSoloSingleSetting(ssts)
			rule.ContentRuleConfig.TcpConfig.NegativeWindow, rule.ContentRuleConfig.TcpConfig.Window = neg, atoi(content)
		case "threshold":
			config := &ThresholdingConfig{}
			config.Count = atoi(utils.MapGetString(vParams, "count"))
			config.Track = utils.MapGetString(vParams, "track")
			switch utils.MapGetString(vParams, "type") {
			case "both":
				config.ThresholdMode = true
				config.LimitMode = true
			case "threshold":
				config.ThresholdMode = true
			case "limit":
				config.LimitMode = true
			}
			config.Seconds = atoi(utils.MapGetString(vParams, "seconds"))
			rule.ContentRuleConfig.Thresholding = config
		case "udp.hdr":
			if rule.ContentRuleConfig.UdpConfig != nil {
				rule.ContentRuleConfig.UdpConfig = &UDPLayerRule{}
			}
			rule.ContentRuleConfig.UdpConfig.UDPHeader = true
		case "icode":
			/*
				icode:min<>max;
				icode:[<|>]<number>;
			*/
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			rule.ContentRuleConfig.IcmpConfig.ICode = vStr
		case "itype":
			/*
				itype:min<>max;
				itype:[<|>]<number>;
			*/
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			rule.ContentRuleConfig.IcmpConfig.IType = vStr
		case "icmp_id":
			// icmp_id:<number>
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			rule.ContentRuleConfig.IcmpConfig.ICMPId = atoi(vStr)
		case "icmp_seq":
			// icmp_seq:<number>;
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			rule.ContentRuleConfig.IcmpConfig.ICMPSeq = atoi(vStr)

		case "nocase":
			contentRule.Nocase = true
		case "depth":
			contentRule.Depth = atoistar(vStr)
		case "offset":
			contentRule.Offset = atoistar(vStr)
		case "startswith":
			contentRule.StartsWith = true
		case "endswith":
			contentRule.EndsWith = true
		case "distance":
			contentRule.Distance = atoistar(vStr)
		case "within":
			contentRule.Within = atoistar(vStr)
		case "rawbytes":
			contentRule.RawBytes = true
		case "isdataat":
			contentRule.IsDataAt = vStr
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
			contentRule.RPC = unquoteAndParseHex(vStr)
		case "pcre":
			contentRule.PCRE = unquoteString(vStr)
			parsed, err := parsePCREStr(contentRule.PCRE)
			if err != nil {
				log.Errorf("parsePCREStr err:%v", err)
			}
			contentRule.PCREParsed = parsed
		case "fast_pattern":
			contentRule.FastPattern = true
		case "flowbits":
			contentRule.FlowBits = vStr
		case "noalert":
			contentRule.NoAlert = true
		case "base64_decode":
			contentRule.Base64Decode = vStr
		case "base64_data":
			contentRule.Base64Data = true
		case "flowint":
			contentRule.FlowInt = vStr
		case "xbits":
			contentRule.XBits = vStr
		case "app-layer-event":
			contentRule.ExtraFlags = append(contentRule.ExtraFlags, fmt.Sprintf("%v:%v", key, vStr))
		default:
			// fallback
			contentRule.ExtraFlags = append(contentRule.ExtraFlags, fmt.Sprintf("%v:%v", key, vStr))
			if key != "metad" {
				log.Errorf("unknown content rule params: %s\n%v\n\n", key, rule.Raw)
			} else {
				log.Errorf("BAD RULE:\n\n%v\n\n", rule.Raw)
			}
		}
	}

	if STATUS != HasNone {
		contentRule.Modifier = multipleBufferMatching.transfer(contentRule.Modifier)
		contents = append(contents, contentRule)
	}

	rule.ContentRuleConfig.ContentRules = append(rule.ContentRuleConfig.ContentRules, contents...)
}
