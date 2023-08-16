package suricata

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
)

func mustSoloSingleSetting(ssts []parser.ISingleSettingContext) (bool, string) {
	if len(ssts) != 1 {
		return false, ""
	}
	ctx := ssts[0].(*parser.SingleSettingContext)
	return ctx.Negative() != nil, ctx.Settingcontent().GetText()
}

func (r *RuleSyntaxVisitor) VisitParams(i *parser.ParamsContext, rule *Rule) {
	var contents []*ContentRule
	var MultipleBufferMatching Modifier
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

		var set = true

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
			set = setIfNotZero(&contentRule.Modifier, FileData)
		case "http_content_type", "http.content_type":
			set = setIfNotZero(&contentRule.Modifier, HTTPContentType)
		case "http_content_len", "http.content_len":
			set = setIfNotZero(&contentRule.Modifier, HTTPContentLen)
		case "http_start", "http.start":
			set = setIfNotZero(&contentRule.Modifier, HTTPStart)
		case "http_protocol", "http.protocol":
			set = setIfNotZero(&contentRule.Modifier, HTTPProtocol)
		case "http_header_names", "http.header_names":
			set = setIfNotZero(&contentRule.Modifier, HTTPHeaderNames)
		case "http_request_line", "http.request_line":
			set = setIfNotZero(&contentRule.Modifier, HTTPRequestLine)
		case "http_accept", "http.accept":
			set = setIfNotZero(&contentRule.Modifier, HTTPAccept)
		case "http_accept_enc", "http.accept_enc":
			set = setIfNotZero(&contentRule.Modifier, HTTPAcceptEnc)
		case "http_referer", "http.referer":
			set = setIfNotZero(&contentRule.Modifier, HTTPReferer)
		case "http_connection", "http.connection":
			set = setIfNotZero(&contentRule.Modifier, HTTPConnection)
		case "http_response_line", "http.response_line":
			set = setIfNotZero(&contentRule.Modifier, HTTPResponseLine)
		case "dns_query", "dns.query":
			set = setIfNotZero(&contentRule.Modifier, DNSQuery)
		case "http_header", "http.header":
			set = setIfNotZero(&contentRule.Modifier, HTTPHeader)
		case "http_raw_header", "http.raw_header":
			set = setIfNotZero(&contentRule.Modifier, HTTPHeaderRaw)
		case "http_cookie", "http.cookie":
			set = setIfNotZero(&contentRule.Modifier, HTTPCookie)
		case "http_uri", "http.uri":
			set = setIfNotZero(&contentRule.Modifier, HTTPUri)
		case "http_raw_uri", "http.uri.raw":
			set = setIfNotZero(&contentRule.Modifier, HTTPUriRaw)
		case "http_method", "http.method":
			set = setIfNotZero(&contentRule.Modifier, HTTPMethod)
		case "http_user_agent", "http.user_agent":
			set = setIfNotZero(&contentRule.Modifier, HTTPUserAgent)
		case "http_host", "http.host":
			set = setIfNotZero(&contentRule.Modifier, HTTPHost)
		case "http_raw_host", `http.raw_host`:
			set = setIfNotZero(&contentRule.Modifier, HTTPHostRaw)
		case "http_stat_msg", "http.stat_msg":
			set = setIfNotZero(&contentRule.Modifier, HTTPStatMsg)
		case "http_stat_code", "http.stat_code":
			set = setIfNotZero(&contentRule.Modifier, HTTPStatCode)
		case "http_client_body", "http.client_body":
			set = setIfNotZero(&contentRule.Modifier, HTTPRequestBody)
		case "http_server_body", "http.server_body":
			set = setIfNotZero(&contentRule.Modifier, HTTPResponseBody)
		case "http_server", "http.server":
			set = setIfNotZero(&contentRule.Modifier, HTTPServer)
		case "http_location", "http.location":
			set = setIfNotZero(&contentRule.Modifier, HTTPLocation)
		case "ipv4.hdr", "ipv4_hdr":
			set = setIfNotZero(&contentRule.Modifier, IPv4HDR)
		case "ipv6.hdr", "ipv6_hdr":
			set = setIfNotZero(&contentRule.Modifier, IPv6HDR)
		case "content":
			neg, content := mustSoloSingleSetting(ssts)
			if contentRule.Content == nil {
				contentRule.Content = []byte(unquoteAndParseHex(content))
				contentRule.Negative = neg
			} else {
				set = false
			}
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

		// pcre is individual
		if contentRule.PCRE != "" {
			pcre := contentRule.PCRE
			contentRule.PCRE = ""
			contents = append(contents, contentRule, &ContentRule{
				PCRE: pcre,
			})
			contentRule = new(ContentRule)
			continue
		}

		// conflict, save current and turn to new empty rule
		if !set || i == len(params)-1 {
			// MultipleBufferMatching
			switch contentRule.Modifier {
			case DNSQuery, FileData, HTTPHeader:
				MultipleBufferMatching = contentRule.Modifier
			case Default:
				contentRule.Modifier = MultipleBufferMatching
			default:
				MultipleBufferMatching = Default
			}

		}

		// save current
		if !set || i == len(params)-1 && len(contentRule.Content) != 0 {
			contents = append(contents, contentRule)
			contentRule = new(ContentRule)
		}

		if !set {
			i--
		}

		set = true
	}

	rule.ContentRuleConfig.ContentRules = append(rule.ContentRuleConfig.ContentRules, contents...)
}
