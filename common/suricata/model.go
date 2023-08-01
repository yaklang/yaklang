package suricata

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/regen"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Modifier uint32

const (
	Default Modifier = iota

	// http req
	HTTPUri
	HTTPUriRaw
	HTTPMethod
	HTTPRequestLine
	HTTPRequestBody
	HTTPUserAgent
	HTTPHost
	HTTPHostRaw
	HTTPAccept
	HTTPAcceptLang
	HTTPAcceptEnc
	HTTPReferer

	// http resp
	HTTPStatMsg
	HTTPStatCode
	HTTPResponseLine
	HTTPResponseBody
	HTTPServer
	HTTPLocation

	// http common
	HTTPHeader
	HTTPHeaderRaw
	HTTPCookie
	HTTPConnection
	FileData
	HTTPContentType
	HTTPContentLen
	HTTPStart
	HTTPProtocol
	HTTPHeaderNames

	// DNS
	DNSQuery

	// IP
	IPv4HDR
	IPv6HDR
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

func (r *AddressRule) GetLocalIPAddress() string {
	return utils.GetLocalIPAddress()
}

func (r *PortRule) GetHighPort() uint32 {
	return uint32(55000 + rand.Intn(65535-55000))
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

type ContentRuleConfig struct {
	Flow *FlowRule

	Thresholding *ThresholdingConfig

	/* DNS Config*/
	DNS *DNSRule

	/* HTTP Config */
	HTTPConfig *HTTPConfig

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
	Nocase     bool // case insensitive
	Depth      *int
	Offset     *int
	StartsWith bool
	EndsWith   bool
	Distance   *int
	Within     *int
	// no effect
	RawBytes bool
	IsDataAt string
	BSize    string
	DSize    string
	// won't support
	ByteTest string
	// won't support
	ByteMath string
	// won't support
	ByteJump string
	// won't support
	ByteExtract string
	// won't support
	RPC string // sunrpc call
	// won't support
	Replace []byte
	PCRE    string

	Modifier Modifier
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
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPUri})
				case 'P', 'Q': // 这个都是针对 body 的，没有啥限制
					rules = append(rules, &ContentRule{Content: []byte(result)})
				case 'H':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPHeader})
				case 'D':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPHeaderRaw})
				case 'M':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPMethod})
				case 'V':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPUserAgent})
				case 'W':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPHost})
				case 'C':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPCookie})
				case 'S':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPStatCode})
				case 'Y':
					rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPStatMsg})
				default:
					once.Do(func() {
						// 如果无法检测的话，针对 URI 这些将是默认选项
						rules = append(rules, &ContentRule{Content: []byte(result)})
						if len(result) < 100 {
							rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPUri})
							rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPHeader})
							rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPUserAgent})
							rules = append(rules, &ContentRule{Content: []byte(result), Modifier: HTTPCookie})
						}
					})

				}

				if current >= count {
					return rules
				}
			}
		}
		return rules
	case <-ctx.Done():
		log.Warn("PCRE生成超时 rule:" + c.PCRE)
	}
	return nil
}
