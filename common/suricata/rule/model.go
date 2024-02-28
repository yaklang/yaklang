package rule

import (
	"fmt"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openai"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/pcre"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
)

type Rule struct {
	Raw                string       `json:"raw"`
	Message            string       `json:"message"`
	MessageChinese     string       `json:"message_chinese"`
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

func (r *Rule) AIDecoration(opts ...openai.ConfigOption) {
	client := openai.NewOpenAIClient(opts...)
	fmt.Println(string(r.Raw))
	if r.MessageChinese == "" {
		msg := r.Message
		if strings.HasPrefix(r.Message, "ET ") {
			msg = r.Message[3:]
		}
		result, err := client.Chat(`我在翻译网络安全领域的规则的名称(Suricata)，帮我翻译下规则的内容，输入在当前消息的 json 中

{"input": ` + strconv.Quote(msg) + `}

把结果放在 json 中, json 的 key 为 result, 以方便我提取，翻译过程中尽量使用网络安全术语，注重可读性，不要太晦涩。注意，我有一些翻译偏好，希望能遵守：

Hash 是一个专有名词，不要翻译；“可能” 使用 “潜在” 代替；按习惯来说你认为是产品名或专有名字可以不翻译;糟糕/恶劣声誉等词汇，使用 “恶意黑名单” 代替；
Poor Reputation 翻译为“恶意”。Observed 翻译为 “检测到”

`)
		if err != nil {
			log.Warnf("openai failed: %s", err)
		} else {
			for _, i := range jsonextractor.ExtractStandardJSON(result) {
				m := utils.ParseStringToGeneralMap(i)
				r.MessageChinese = utils.MapGetString(m, "result")
				if utils.MatchAnyOfSubString(r.MessageChinese, "翻译", "对应内容", "内容为") {
					r.MessageChinese = ""
				} else {
					fmt.Println("translate: " + msg + " to " + r.MessageChinese)
				}
			}
		}
	}
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
