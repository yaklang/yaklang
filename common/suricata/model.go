package suricata

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/regen"
)

type Rule struct {
	Raw                string       `json:"raw"`
	Message            string       `json:"message"`
	Action             string       `json:"action"`
	Protocol           string       `json:"protocol"`
	SourceAddress      *AddressRule `json:"source_address"`
	DestinationAddress *AddressRule `json:"destination_address"`
	SourcePort         *PortRule    `json:"source_port"`
	DestinationPort    *PortRule    `json:"destination_port"`

	Sid       int
	Rev       int
	Gid       int
	ClassType string
	Reference map[string]string
	Priority  int
	Metadata  []string
	Target    string // src_ip/dest_ip

	ContentRuleConfig *ContentRuleConfig
}

func (r *PortRule) GetHighPort() uint32 {
	return uint32(55000 + rand.Intn(65535-55000))
}

func (r *AddressRule) GetLocalIPAddress() string {
	return utils.GetLocalIPAddress()
}

func (r *PortRule) GetAvailablePort() uint32 {
	if r.Any {
		return 80
	}

	if strings.Contains(strings.ToLower(r.Env), "ssh") {
		return 22
	} else if r.Env != "" {
		return r.GetHighPort()
	}

	if len(r.Ports) > 0 && !r.Negative {
		return uint32(r.Ports[rand.Intn(len(r.Ports))])
	}

	var count int = 1000
	for len(r.Ports) > 0 && r.Negative && count <= 30000 {
		matched := false
		for _, p := range r.Ports {
			if p == count {
				matched = true
				break
			}
		}
		if matched {
			return uint32(count)
		}
		count++
	}

	return r.Rules[rand.Intn(len(r.Rules))].GetAvailablePort()
}

func (r *Rule) init() {
	r.ContentRuleConfig = &ContentRuleConfig{
		Thresholding:       &ThresholdingConfig{},
		HttpBaseSticky:     &HttpBaseStickyRule{},
		HttpRequestSticky:  &HttpRequestStickyRule{},
		HttpResponseSticky: &HttpResponseStickyRule{},
		IPConfig:           &IPLayerRule{},
		TcpConfig:          &TCPLayerRule{},
		UdpConfig:          &UDPLayerRule{},
		IcmpConfig:         &ICMPLayerRule{},
		ContentRules:       nil,
		DNS: &DNSRule{
			OpcodeNegative: false,
			Opcode:         0,
			DNSQuery:       false,
		},
	}
}

type ContentRuleConfig struct {
	Flow *FlowRule

	Thresholding *ThresholdingConfig

	/* DNS Config*/
	DNS *DNSRule

	/* HTTP Config */
	HttpBaseSticky     *HttpBaseStickyRule
	HttpRequestSticky  *HttpRequestStickyRule
	HttpResponseSticky *HttpResponseStickyRule

	/* IP */
	IPConfig *IPLayerRule

	/* TCP */
	TcpConfig *TCPLayerRule

	/* UDP */
	UdpConfig *UDPLayerRule

	/* ICMP */
	IcmpConfig *ICMPLayerRule

	/* Payload Match */
	ContentRules []*ContentRule
}

type FlowRule struct {
	ToClient    bool
	Established bool
	ToServer    bool
}

type ContentRule struct {
	Negative bool
	Content  []byte

	// payload config
	Nocase      bool // case insensitive
	Depth       int
	Offset      int
	StartsWith  bool
	EndsWith    bool
	Distance    int
	Within      int
	RawBytes    bool
	IsDataSet   int
	BSize       string
	DSize       string
	ByteTest    string
	ByteMath    string
	ByteJump    string
	ByteExtract string
	RPC         string // sunrpc call
	Replace     []byte
	PCRE        string

	HttpBaseModifier     *HttpBaseModifierRule
	HttpResponseModifier *HttpResponseModifierRule
	HttpRequestModifier  *HttpRequestModifierRule
}

func fixRules(rules []*ContentRule) []*ContentRule {
	for _, r := range rules {
		if r.HttpRequestModifier == nil {
			r.HttpRequestModifier = &HttpRequestModifierRule{}
		}
		if r.HttpResponseModifier == nil {
			r.HttpResponseModifier = &HttpResponseModifierRule{}
		}
		if r.HttpBaseModifier == nil {
			r.HttpBaseModifier = &HttpBaseModifierRule{}
		}
	}
	return rules
}

func (c *ContentRule) PCREStringGenerator(count int) []*ContentRule {
	if c.PCRE == "" {
		return nil
	}
	r := c.PCRE
	if strings.HasPrefix(c.PCRE, `"`) && strings.HasSuffix(c.PCRE, `"`) {
		var res, _ = strconv.Unquote(r)
		if res != "" {
			r = res
		}
	}
	re := r
	r = strings.Trim(r, `"/IPQHDMCSYRBOVW`)
	resultsCh := make(chan []string, 1)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		results, err := regen.GenerateOne(r)
		if err != nil {
			errCh <- fmt.Errorf("fetch regexp[%s] failed: %s", r, err)
			return
		}
		resultsCh <- results
	}()
	select {
	case err := <-errCh:
		if err != nil {
			log.Errorf("fetch regexp[%s] failed: %s", r, err)
		}
	case results := <-resultsCh:
		if results == nil {
			return nil
		}
		var flags []byte
		if ret := strings.LastIndexByte(re, '/'); ret > 0 {
			flags = []byte(re[ret+1:])
		}
		var rules []*ContentRule

		current := 0
		var once sync.Once
		for _, result := range results {
			for _, flag := range flags {
				current++
				switch flag {
				case 'I':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpRequestModifier: &HttpRequestModifierRule{HttpUri: true}})
				case 'P', 'Q': // 这个都是针对 body 的，没有啥限制
					rules = append(rules, &ContentRule{Content: []byte(result)})
				case 'H':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpBaseModifier: &HttpBaseModifierRule{HttpHeader: true}})
				case 'D':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpBaseModifier: &HttpBaseModifierRule{HttpRawHeader: true}})
				case 'M':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpRequestModifier: &HttpRequestModifierRule{HttpMethod: true}})
				case 'V':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpRequestModifier: &HttpRequestModifierRule{HttpUserAgent: true}})
				case 'W':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpRequestModifier: &HttpRequestModifierRule{HttpHost: true}})
				case 'C':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpBaseModifier: &HttpBaseModifierRule{HttpCookie: true}})
				case 'S':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpResponseModifier: &HttpResponseModifierRule{HttpStatCode: true}})
				case 'Y':
					rules = append(rules, &ContentRule{Content: []byte(result), HttpResponseModifier: &HttpResponseModifierRule{HttpStatMsg: true}})
				default:
					once.Do(func() {
						// 如果无法检测的话，针对 URI 这些将是默认选项
						rules = append(rules, &ContentRule{Content: []byte(result)})
						if len(result) < 100 {
							rules = append(rules, &ContentRule{Content: []byte(result), HttpRequestModifier: &HttpRequestModifierRule{HttpUri: true}})
							rules = append(rules, &ContentRule{Content: []byte(result), HttpBaseModifier: &HttpBaseModifierRule{HttpHeader: true}})
							rules = append(rules, &ContentRule{Content: []byte(result), HttpRequestModifier: &HttpRequestModifierRule{HttpUserAgent: true}})
							rules = append(rules, &ContentRule{Content: []byte(result), HttpBaseModifier: &HttpBaseModifierRule{HttpCookie: true}})
						}
					})

				}

				if current >= count {
					return fixRules(rules)
				}
			}
		}
		return fixRules(rules)
	case <-ctx.Done():
		log.Warn("PCRE生成超时 rule:" + c.PCRE)
	}
	return nil
}
