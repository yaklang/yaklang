package rule

import (
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/pcre"
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
	/* Just No Config */

	/* ICMP */
	IcmpConfig *ICMPLayerRule

	/* Payload Match */
	ContentRules []*ContentRule

	// PrefilterRule is a contentRuleConfig with no more than single config.
	// not implement yet
	PrefilterRule *ContentRuleConfig
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
	Replace     []byte
	PCRE        string
	PCREParsed  *pcre.PCRE
	FastPattern bool

	// e.g set,bihinder3
	FlowBits     string
	FlowInt      string
	XBits        string
	NoAlert      bool
	Base64Decode string
	Base64Data   bool

	ExtraFlags []string

	Modifier modifier.Modifier
}
