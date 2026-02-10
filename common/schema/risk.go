package schema

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/kataras/pio"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Risk struct {
	gorm.Model

	Hash string `json:"hash"`

	// essential
	IP        string `json:"ip"`
	IPInteger int64  `json:"ip_integer"`

	// extraTargets
	Url  string `json:"url"`
	Port int    `json:"port"`
	Host string `json:"host"`

	//
	Title           string `json:"title"`
	TitleVerbose    string `json:"title_verbose"`
	Description     string `json:"description"`
	Solution        string `json:"solution"`
	RiskType        string `json:"risk_type"`
	RiskTypeVerbose string `json:"risk_verbose"`
	Parameter       string `json:"parameter"`
	Payload         string `json:"payload"`
	Details         string `json:"details"`
	Severity        string `json:"severity"`

	// 来源于哪个插件？
	FromYakScript string `json:"from_yak_script"`
	YakScriptUUID string `json:"yak_script_uuid"`

	// 等待验证中？
	WaitingVerified bool `json:"waiting_verified"`
	// 用于验证的 ReverseToken
	ReverseToken string `json:"reverse_token"`

	// 设置运行时 ID 为了关联具体漏洞
	RuntimeId      string `json:"runtime_id"`
	QuotedRequest  string `json:"quoted_request"`
	QuotedResponse string `json:"quoted_response"`

	PacketPairs PacketPairList `json:"packet_pairs" gorm:"type:text"`
	// 潜在威胁：用于输出合规性质的漏洞内容
	IsPotential bool `json:"is_potential"`

	CVE                 string `json:"cve"`
	IsRead              bool   `json:"is_read"`
	Ignore              bool   `json:"ignore"`
	UploadOnline        bool   `json:"upload_online"`
	TaskName            string `json:"task_name"`
	CveAccessVector     string `json:"cve_access_vector"`
	CveAccessComplexity string `json:"cve_access_complexity"`
	Tags                string `json:"tags"`

	// SyntaxFlow
	ResultID    uint   `json:"result_id"`
	Variable    string `json:"variable"`
	ProgramName string `json:"program_name"`
}

type PacketPair struct {
	Request  []byte `json:"request"`
	Response []byte `json:"response"`
}

// PacketPairList 是 PacketPair 的集合，用于 JSON 持久化到数据库
type PacketPairList []*PacketPair

// Value 实现 driver.Valuer，将 PacketPairList 序列化为 JSON 存入数据库
func (p PacketPairList) Value() (driver.Value, error) {
	if p == nil {
		return "[]", nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

// Scan 实现 sql.Scanner，从数据库中的 JSON 反序列化为 PacketPairList
func (p *PacketPairList) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported type for PacketPairList Scan: %T", value)
	}

	if len(data) == 0 {
		*p = nil
		return nil
	}

	var list []*PacketPair
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*p = list
	return nil
}

func (p *Risk) ColorizedShow() {
	buf := bytes.NewBufferString("")
	buf.WriteString(pio.Red("========RISK: " + p.Title + "========"))
	buf.WriteByte('\n')
	buf.WriteString(pio.Red("    TYPE: " + p.RiskType + "(" + p.RiskTypeVerbose + ")"))
	buf.WriteByte('\n')
	buf.WriteString(pio.Red("    Target: " + p.Url + " (" + p.IP + ":" + fmt.Sprint(p.Port) + ")"))
	buf.WriteByte('\n')
	buf.WriteString(pio.Red("    REQUEST:"))
	buf.WriteByte('\n')
	requsetRaw, _ := strconv.Unquote(p.QuotedRequest)
	if len(requsetRaw) > 0 {
		buf.WriteString(pio.Yellow(string(requsetRaw)))
	}
	buf.WriteByte('\n')
	buf.WriteString(pio.Red(`========================================`))
	buf.WriteByte('\n')
	fmt.Println(buf.String())
}

func (p *Risk) ToGRPCModel() *ypb.Risk {
	details, _ := strconv.Unquote(p.Details)
	if details == "" {
		details = p.Details
	}

	var request []byte
	var response []byte

	reqRaw, _ := strconv.Unquote(p.QuotedRequest)
	if reqRaw != "" {
		request = []byte(reqRaw)
	} else {
		request = []byte(p.QuotedRequest)
	}

	rspRaw, _ := strconv.Unquote(p.QuotedResponse)
	if rspRaw != "" {
		response = []byte(rspRaw)
	} else {
		response = []byte(p.QuotedResponse)
	}

	var packetPairs []*ypb.PacketPair
	for _, pp := range p.PacketPairs {
		if pp == nil {
			continue
		}
		packetPairs = append(packetPairs, &ypb.PacketPair{
			Request:  pp.Request,
			Response: pp.Response,
		})
	}

	return &ypb.Risk{
		Hash:            utils.EscapeInvalidUTF8Byte([]byte(p.Hash)),
		IP:              utils.EscapeInvalidUTF8Byte([]byte(p.IP)),
		Url:             utils.EscapeInvalidUTF8Byte([]byte(p.Url)),
		Port:            int32(p.Port),
		Host:            utils.EscapeInvalidUTF8Byte([]byte(p.Host)),
		Title:           utils.EscapeInvalidUTF8Byte([]byte(p.Title)),
		TitleVerbose:    utils.EscapeInvalidUTF8Byte([]byte(p.TitleVerbose)),
		Description:     utils.EscapeInvalidUTF8Byte([]byte(p.Description)),
		Solution:        utils.EscapeInvalidUTF8Byte([]byte(p.Solution)),
		RiskType:        utils.EscapeInvalidUTF8Byte([]byte(p.RiskType)),
		RiskTypeVerbose: utils.EscapeInvalidUTF8Byte([]byte(p.RiskTypeVerbose)),
		Parameter:       utils.EscapeInvalidUTF8Byte([]byte(p.Parameter)),
		Payload:         utils.EscapeInvalidUTF8Byte([]byte(p.Payload)),
		Details:         details,
		FromYakScript:   utils.EscapeInvalidUTF8Byte([]byte(p.FromYakScript)),
		WaitingVerified: p.WaitingVerified,
		ReverseToken:    utils.EscapeInvalidUTF8Byte([]byte(p.ReverseToken)),
		Id:              int64(p.ID),
		CreatedAt:       p.CreatedAt.Unix(),
		UpdatedAt:       p.UpdatedAt.Unix(),
		Severity:        utils.EscapeInvalidUTF8Byte([]byte(p.Severity)),

		Request:     request,
		Response:    response,
		PacketPairs: packetPairs,

		RuntimeId: utils.EscapeInvalidUTF8Byte([]byte(p.RuntimeId)),
		CVE:       utils.EscapeInvalidUTF8Byte([]byte(p.CVE)),
		TaskName:  utils.EscapeInvalidUTF8Byte([]byte(p.TaskName)),
		Tags:      p.Tags,
		IsRead:    p.IsRead,

		YakScriptUUID: p.YakScriptUUID,
		// for syntaxflow risk
		ResultID:           uint64(p.ResultID),
		ProgramName:        p.ProgramName,
		SyntaxFlowVariable: p.Variable,
		IsPotential:        p.IsPotential,
	}
}

func (p *Risk) BeforeSave() error {
	if p.Hash == "" {
		p.Hash = uuid.New().String()
	}

	p.RiskType = strings.ReplaceAll(p.RiskType, "|", "_")
	p.Severity = strings.ReplaceAll(p.Severity, "|", "_")

	if p.IPInteger <= 0 && p.IP != "" {
		p.IPInteger, _ = utils.IPv4ToUint64(p.IP)
	}

	if p.Severity == "" {
		p.Severity = "info"
	}

	if p.RiskType == "" {
		p.RiskType = "info"
	}

	if p.RiskTypeVerbose == "" {
		p.RiskTypeVerbose = "信息"
	}

	return nil
}

func (r *Risk) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call("risk", "create")
	return nil
}

func (r *Risk) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call("risk", "update")
	return nil
}

func (r *Risk) AfterDelete(tx *gorm.DB) (err error) {
	broadcastData.Call("risk", "delete")
	return nil
}
