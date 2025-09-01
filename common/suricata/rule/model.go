package rule

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/pcre"
	"github.com/yaklang/yaklang/common/utils"
)

// Rule is a suricata rule
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

	SettingMap map[string]string
	Sid        int
	Rev        int
	Gid        int
	ClassType  string
	Reference  map[string]string
	Priority   int
	Metadata   []string
	Target     string // src_ip/dest_ip

	ContentRuleConfig *ContentRuleConfig

	RuleUpdatedAt      string `json:"update_at"`
	RuleCreatedAt      string `json:"created_at"`
	Deployment         string `json:"deployment"`
	SignatureSeverity  string `json:"signature_severity"`
	AttackTarget       string `json:"attack_target"`
	FormerCategory     string `json:"former_category"`
	AffectedProduct    string `json:"affected_product"`
	Tag                string `json:"tag"`
	PerformanceImpact  string `json:"performance_impact"`
	MalwareFamily      string `json:"malware_family"`
	MitreTechniqueID   string `json:"mitre_technique_id"`
	MitreTacticID      string `json:"mitre_tactic_id"`
	MitreTechniqueName string `json:"mitre_technique_name"`
	MitreTacticName    string `json:"mitre_tactic_name"`
	Confidence         string `json:"confidence"`
	ReviewedAt         string `json:"reviewed_at"`
	CVE                string `json:"cve"`
}

func (r *Rule) AIDecoration(opts ...aispec.AIConfigOption) {
	if r.MessageChinese == "" {
		msg := r.Message
		if strings.HasPrefix(r.Message, "ET ") {
			msg = r.Message[3:]
		}
		result, err := ai.Chat(`我在翻译网络安全领域的规则的名称(Suricata)，帮我翻译下规则的内容，输入在当前消息的 json 中

{"input": `+strconv.Quote(msg)+`}

把结果放在 json 中, json 的 key 为 result, 以方便我提取，翻译过程中尽量使用网络安全术语，注重可读性，不要太晦涩。注意，我有一些翻译偏好，希望能遵守：

Hash 是一个专有名词，不要翻译；"可能" 使用 "潜在" 代替；按习惯来说你认为是产品名或专有名字可以不翻译;糟糕/恶劣声誉等词汇，使用 "恶意黑名单" 代替；
Poor Reputation 翻译为"恶意"。Observed 翻译为 "检测到"。 "CINS Active"是专有名词 

`, opts...)
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

	/* TLS */
	TLSConfig *TLSRule

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
