package suricata

import (
	"fmt"
	"strconv"
	"strings"
	"github.com/yaklang/yaklang/common/suricata/parser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func atoi(i string) int {
	parsed, _ := strconv.Atoi(i)
	return parsed
}

func (r *RuleSyntaxVisitor) VisitParams(i *parser.ParamsContext, rule *Rule) {
	var contents []*ContentRule
	var contentRule *ContentRule
	for _, param := range i.AllParam() {
		if param == nil {
			continue
		}
		key, v := r.VisitParam(param.(*parser.ParamContext))
		key = strings.Trim(key, `" :`)
		if key == "" {
			continue
		}

		vStr := fmt.Sprint(v)
		vParams := make(map[string]interface{})
		for _, v := range utils.PrettifyListFromStringSplitEx(vStr, ",") {
			ret := strings.SplitN(v, " ", 2)
			if len(ret) == 1 {
				vParams[ret[0]] = true
			} else if len(ret) == 2 {
				vParams[strings.TrimSpace(ret[0])] = strings.TrimSpace(ret[1])
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
			results := strings.SplitN(vStr, ",", 1)
			if rule.Reference == nil {
				rule.Reference = map[string]string{}
			}
			if len(results) == 2 {
				rule.Reference[results[0]] = results[1]
			} else {
				rule.Reference[vStr] = ""
			}
		case "msg":
			rule.Message = fmt.Sprint(v)
		case "priority":
			rule.Priority, _ = strconv.Atoi(vStr)
		case "metadata":
			rule.Metadata = utils.PrettifyListFromStringSplited(vStr, ",")

		case "file_data", "file.data":
			rule.ContentRuleConfig.HttpBaseSticky.FileData = true
		case "http_content_type", "http.content_type":
			rule.ContentRuleConfig.HttpBaseSticky.HttpContentType = true
		case "http_content_len", "http.content_len":
			rule.ContentRuleConfig.HttpBaseSticky.HttpContentLength = true
		case "http_start", "http.start":
			rule.ContentRuleConfig.HttpBaseSticky.HttpStart = true
		case "http_protocol", "http.protocol":
			rule.ContentRuleConfig.HttpBaseSticky.HttpProtocol = true
		case "http_header_names", "http.header_names":
			rule.ContentRuleConfig.HttpBaseSticky.HttpHeaderNames = true
		case "http_request_line", "http.request_line":
			rule.ContentRuleConfig.HttpRequestSticky.HttpRequestLine = true
		case "http_accept", "http.accept":
			rule.ContentRuleConfig.HttpRequestSticky.HttpAccept = true
		case "http_accept_enc", "http.accept_enc":
			rule.ContentRuleConfig.HttpRequestSticky.HttpAcceptEnc = true
		case "http_referer", "http.referer":
			rule.ContentRuleConfig.HttpRequestSticky.HttpReferer = true
		case "http_connection", "http.connection":
			rule.ContentRuleConfig.HttpRequestSticky.HttpConnection = true
		case "http_response_line", "http.response_line":
			rule.ContentRuleConfig.HttpResponseSticky.HttpResponseLine = true
		case "content": /* start to handle payload */
			// content start
			if contentRule != nil {
				contents = append(contents, contentRule)
			}
			contentRule = &ContentRule{
				HttpBaseModifier:     &HttpBaseModifierRule{},
				HttpResponseModifier: &HttpResponseModifierRule{},
				HttpRequestModifier:  &HttpRequestModifierRule{},
			}
			neg := strings.HasPrefix(vStr, `!"`)
			if neg {
				contentRule.Negative = true
				vStr = strings.TrimPrefix(vStr, `!`)
			}
			contentRule.Content = []byte(UnquoteString(vStr))

		case "dns.opcode", "dns_opcode":
			config := rule.ContentRuleConfig.DNS
			if strings.HasPrefix(vStr, "!") {
				config.OpcodeNegative = true
			}
			config.Opcode = atoi(strings.Trim(vStr, "!"))
		case "dns_query", "dns.query":
			rule.ContentRuleConfig.DNS.DNSQuery = true
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
			rule.ContentRuleConfig.IPConfig.TTL = atoi(vStr)
		case "sameip":
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
			rule.ContentRuleConfig.IPConfig.IPOpts = vStr
		case "ip_proto":
			rule.ContentRuleConfig.IPConfig.IPProto = vStr //number or name
		case "ipv4.hdr", "ipv4_hdr":
			rule.ContentRuleConfig.IPConfig.IPv4Header = true
		case "ipv6.hdr", "ipv6_hdr":
			rule.ContentRuleConfig.IPConfig.IPv6Header = true
		case "id":
			rule.ContentRuleConfig.IPConfig.Id = atoi(vStr)
		case "geoip":
			rule.ContentRuleConfig.IPConfig.Geoip = vStr
		case "fragbits":
			rule.ContentRuleConfig.IPConfig.FragBits = vStr
		case "fragoffset":
			rule.ContentRuleConfig.IPConfig.FragOffset = vStr
		case "tos":
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
			rule.ContentRuleConfig.TcpConfig.Flags = vStr
		case "seq":
			rule.ContentRuleConfig.TcpConfig.Seq = atoi(vStr)
		case "ack":
			rule.ContentRuleConfig.TcpConfig.Ack = atoi(vStr)
		case "window":
			if strings.HasPrefix(vStr, "!") {
				rule.ContentRuleConfig.TcpConfig.NegativeWindow = true
				rule.ContentRuleConfig.TcpConfig.Window = atoi(vStr[1:])
			} else {
				rule.ContentRuleConfig.TcpConfig.NegativeWindow = false
				rule.ContentRuleConfig.TcpConfig.Window = atoi(vStr)
			}
		case "threshold":
			config := rule.ContentRuleConfig.Thresholding
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

		case "udp.hdr":
			rule.ContentRuleConfig.UdpConfig.UDPHeader = true
		case "icode":
			/*
				icode:min<>max;
				icode:[<|>]<number>;
			*/
			rule.ContentRuleConfig.IcmpConfig.ICode = vStr
		case "itype":
			/*
				itype:min<>max;
				itype:[<|>]<number>;
			*/
			rule.ContentRuleConfig.IcmpConfig.IType = vStr
		case "icmp_id":
			// icmp_id:<number>
			rule.ContentRuleConfig.IcmpConfig.ICMPId = atoi(vStr)
		case "icmp_seq":
			// icmp_seq:<number>;
			rule.ContentRuleConfig.IcmpConfig.ICMPSeq = atoi(vStr)
		}

		if contentRule == nil {
			continue
		}

		switch key {
		case "http_header", "http.header":
			contentRule.HttpBaseModifier.HttpHeader = true
		case "http_raw_header", "http.raw_header":
			contentRule.HttpBaseModifier.HttpRawHeader = true
		case "http_cookie", "http.cookie":
			contentRule.HttpBaseModifier.HttpCookie = true
		case "http_uri", "http.uri":
			contentRule.HttpRequestModifier.HttpUri = true
		case "http_raw_uri", "http.raw_uri":
			contentRule.HttpRequestModifier.HttpRawUri = true
		case "http_method", "http.method":
			contentRule.HttpRequestModifier.HttpMethod = true
		case "http_user_agent", "http.user_agent":
			contentRule.HttpRequestModifier.HttpUserAgent = true
		case "http_host", "http.host":
			contentRule.HttpRequestModifier.HttpHost = true
		case "http_raw_host", `http.raw_host`:
			contentRule.HttpRequestModifier.HttpRawHost = true
		case "http_stat_msg", "http.stat_msg":
			contentRule.HttpResponseModifier.HttpStatMsg = true
		case "http_stat_code", "http.stat_code":
			contentRule.HttpResponseModifier.HttpStatCode = true
		case "http_server_body", "http.server_body":
			contentRule.HttpResponseModifier.HttpServerBody = true
		case "http_server", "http.server":
			contentRule.HttpResponseModifier.HttpServer = true
		case "http_location", "http.location":
			contentRule.HttpResponseModifier.HttpLocation = true
		case "nocase":
			contentRule.Nocase = true
		case "depth":
			contentRule.Depth = atoi(vStr)
		case "offset":
			contentRule.Offset = atoi(vStr)
		case "startswith":
			contentRule.StartsWith = true
		case "endswith":
			contentRule.EndsWith = true
		case "distance":
			contentRule.Distance = atoi(vStr)
		case "within":
			contentRule.Within = atoi(vStr)
		case "rawbytes":
			contentRule.RawBytes = true
		case "isdataset":
			contentRule.IsDataSet = atoi(vStr)
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
			contentRule.RPC = UnquoteString(vStr)
		case "pcre":
			contentRule.PCRE = vStr
		}
	}
	if contentRule != nil {
		contents = append(contents, contentRule)
	}
	rule.ContentRuleConfig.ContentRules = append(rule.ContentRuleConfig.ContentRules, contents...)
}

func (r *RuleSyntaxVisitor) VisitParam(i *parser.ParamContext) (string, interface{}) {
	var value interface{}
	raw := strings.TrimSpace(i.GetText())
	key, valueRaw := lowhttp.SplitHTTPHeader(raw)
	if valueRaw == "" {
		value = true
	} else {
		value = valueRaw
	}
	return key, value
}
