package rule

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/data/numrange"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/suricata/pcre"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
)

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
	case "http_raw_header", "http.raw_header":
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
	case "http_raw_host", `http.raw_host`:
		return modifier.HTTPHostRaw
	case "http_stat_msg", "http.stat_msg":
		return modifier.HTTPStatMsg
	case "http_stat_code", "http.stat_code":
		return modifier.HTTPStatCode
	case "http_client_body", "http.client_body":
		return modifier.HTTPRequestBody
	case "http_server_body", "http.server_body":
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
	}
	return modifier.Default
}
func mustSoloSingleSetting(ssts []parser.ISingleSettingContext) (bool, string) {
	if len(ssts) != 1 {
		return false, ""
	}
	ctx := ssts[0].(*parser.SingleSettingContext)
	return ctx.Negative() != nil, ctx.Settingcontent().GetText()
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

func (v *RuleSyntaxVisitor) VisitParams(i *parser.ParamsContext, rule *Rule) {
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
			seq := atoi(vStr)
			rule.ContentRuleConfig.TcpConfig.Seq = &seq
		case "ack":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			ack := atoi(vStr)
			rule.ContentRuleConfig.TcpConfig.Ack = &ack
		case "window":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			neg, content := mustSoloSingleSetting(ssts)
			window := atoi(content)
			rule.ContentRuleConfig.TcpConfig.NegativeWindow, rule.ContentRuleConfig.TcpConfig.Window = neg, &window
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
				log.Errorf("parse icmp icode err:%v", err)
				continue
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
				log.Errorf("parse icmp itype err:%v", err)
				continue
			}
			rule.ContentRuleConfig.IcmpConfig.IType = numRange
		case "icmp_id":
			// icmp_id:<number>
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			num := atoi(vStr)
			rule.ContentRuleConfig.IcmpConfig.ICMPId = &num
		case "icmp_seq":
			// icmp_seq:<number>;
			if rule.ContentRuleConfig.IcmpConfig == nil {
				rule.ContentRuleConfig.IcmpConfig = &ICMPLayerRule{}
			}
			num := atoi(vStr)
			rule.ContentRuleConfig.IcmpConfig.ICMPSeq = &num
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
			parsed, err := pcre.ParsePCREStr(contentRule.PCRE)
			if err != nil {
				log.Errorf("parsePCREStr err:%v", err)
			}
			contentRule.PCREParsed = parsed
			contentRule.Modifier = parsed.Modifier()
		case "tcp.mss":
			if rule.ContentRuleConfig.TcpConfig == nil {
				rule.ContentRuleConfig.TcpConfig = &TCPLayerRule{}
			}
			numRange, err := numrange.ParseNumRange(vStr)
			if err != nil {
				log.Errorf("parse tcp.mss err:%v", err)
				continue
			}
			rule.ContentRuleConfig.TcpConfig.TCPMss = numRange
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
			if modifierMapping(key) != modifier.Default {
				contentRule.Modifier = modifierMapping(key)
				break
			}
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
