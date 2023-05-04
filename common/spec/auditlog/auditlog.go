package auditlog

import (
	"github.com/jinzhu/gorm/dialects/postgres"
)

type Level string

const (
	INFO    Level = "info"
	DEBUG   Level = "debug"
	TRACE         = DEBUG
	WARN    Level = "info"
	WARNING       = WARN
	ERROR   Level = "error"
	FATAL   Level = "fatal"
	PANIC   Level = "panic"
)

type EventSeverity int

const (
	LogNormal     EventSeverity = 1
	LogMiddleLow  EventSeverity = 2
	LogMiddle     EventSeverity = 3
	LogMiddleHigh EventSeverity = 4
	LogHigh       EventSeverity = 5
)

func (e *EventSeverity) String() string {
	switch *e {
	case LogNormal:
		return "info"
	case LogMiddleLow:
		return "low"
	case LogMiddle:
		return "middle"
	case LogMiddleHigh:
		return "high"
	case LogHigh:
		return "alarm"
	}
	return ""
}

type AuditLog struct {
	/*
	   日志本身的属性
	*/
	// Logagent 查询日志的具体时间
	QueryTimestamp int64 `json:"query_timestamp" `

	LogRecordID uint `json:"record_id"`
	//请求的url的唯一ID
	RequestId string `json:"request_id"`

	// 日志记录具体的时间戳
	Timestamp int64 `json:"log_timestamp" gorm:"index"`

	// level
	Level EventSeverity `json:"event_severity"`

	/*
		日志记录的内容 （谁 - 什么时间 - 使用什么/在哪里（发生在那个接口中: url / source ）- 做了一件什么事 - 产生了什么额外数据/影响 ）
	*/

	// 操作用户，可能是用户名也可能是系统名
	OperationUser string `json:"operation_user" gorm:"index"` // 谁
	Organization  string // 这个人的组织
	VpnId         string `json:"vpn_account" gorm:"index"` // 如果通过 VPN 操作产生的日志

	// 发生在那个接口中？
	UrlPath  string `json:"url_path" gorm:"index"` // 针对 URL 产生的日志
	Source   string `json:"log_type" gorm:"index"` // 日志发生在哪个系统中？
	SpotInfo string `json:"spot_info"`             // 发生现场信息 - 哪个模块？哪个文件？哪个函数？

	// 网络与协议相关内容
	// 操作的源、目的网络地址
	DstIP   string `json:"dst_ip"`
	DstPort int    `json:"dst_port"`
	SrcIP   string `json:"src_ip"`
	SrcPort int    `json:"src_port"`

	// http
	HttpMethod          string         `json:"http_method"`
	HttpResponseCode    int            `json:"http_response_code"`
	HttpContentType     string         `json:"http_content_type"`
	HttpContentLength   int            `json:"http_content_length"`
	HttpClientUserAgent string         `json:"http_client_user_agent"`
	HttpHost            string         `json:"http_host"`
	HttpRequestBody     postgres.Jsonb `json:"http_request_body" `

	// 日志的内容
	//Content map[string]interface{} `json:"content" gorm:"type: jsonb"`
	Content postgres.Jsonb `json:"content"  `

	// 这个 ExtraData 代表的是从日志内容中取出的日志内容
	// 假如 content 包含着身份证、手机号等信息，被正则捕获或者分析，提取的数据会结构化后放入 ExtraData 中
	// 或者 JSON 如果被提取出来，也会被提取，放入 extra data 中
	ExtraData postgres.Jsonb `json:"extra" `
	//sso登陆返回的token
	BetaUserToken string `json:"beta_user_token" `
	DeptPath      string `json:"dept_path"` //部门路径
	PsnStatus     string `json:"psnStatus"` //人员状态 1在职 0离职
}

type RpmsPerson struct {
	WorkCity       string      `json:"workCity"`      //工作城市
	PsnStatus      string      `json:"psnStatus"`     //人员状态 1在职 0离职
	IdType         string      `json:"idType"`        //证件类型
	Org            string      `json:"org"`           //组织
	Name           string      `json:"name"`          //员工姓名
	MobileMd5      string      `json:"mobileMd5"`     //电话号码脱敏前的MD5
	Mobile         string      `json:"mobile"`        //联系电话
	JoinDate       interface{} `json:"joinDate"`      //入职日期
	IdMd5          string      `json:"idMd5"`         //证件号码脱敏前MD5值
	IdNo           string      `json:"idNo"`          //证件号码
	Email          string      `json:"email"`         //企业邮箱
	DispatchCorp   string      `json:"dispatchCorp"`  //派遣公司
	DismissingDate interface{} `json:"dimissionDate"` //离职日期
	DeptPath       string      `json:"deptPath"`      //部门路径
	Dept           string      `json:"dept"`          //部门
	DeptLevel1     string      `json:"dept_level1"`   //一级部门
	DeptLevel2     string      `json:"dept_level2"`   //二级部门
	DeptLevel3     string      `json:"dept_level3"`   //三级部门
	Code           string      `json:"code"`          //员工工号
	PsnClass       string      `json:"psnClass"`      //员工类型
}

type SsoLogin struct {
	UserId        string `json:"user_id"`        // 用户的唯一ID
	Email         string `json:"email"`          // sso登陆邮箱账号账号
	LoginIp       string `json:"login_ip"`       // 登陆的ip信息
	TargetSystem  string `json:"target_system"`  // 登陆的目标系统
	DeviceId      string `json:"device_id"`      // 手机登陆的设备ID
	FingerPrint   string `json:"fingerprint"`    //浏览器登陆的设备ID
	LoginCountry  string `json:"login_country"`  // 登陆ip的地理属性
	LoginProvince string `json:"login_province"` //
	LoginCity     string `json:"login_city"`     //

}

type BI struct {
	TracerReportId   string `json:"tracerReportId"`   // 报表id
	TracerReportName string `json:"tracerReportName"` //报表名称
	DateKey          string `json:"datekey"`          //时间 （20200619）
	AreaInfo         string `json:"areaInfo"`         //地区，城市
	AreaName         string `json:"areaName"`         //区域名称
	ClassInfo        string `json:"classInfo"`        //品类
	ClassName        string `json:"className"`        //品类名称
	MmcInfo          string `json:"mmcInfo"`          //商户归属
	MmcName          string `json:"mmcName"`          //商户归属名称
	CustomerType     string `json:"customerType"`     //家庭餐厅code
	CustomerName     string `json:"customerName"`     //家庭/个人、餐厅
}

type Authentication struct {
	UserId        string `json:"user_id"`         // 用户的唯一ID
	TargetUrlPath string `json:"target_url_path"` // 访问的目标url
	TargetSystem  string `json:"target_system"`   // 被访问url所属的系统
	AccessResult  bool   `json:"access_result"`   // 鉴权结果，true=可以访问 false=拒绝访问
	RealIp        string `json:"real_ip"`         // 访问的真实发起IP地址
	ForwardIp     string `json:"forward_ip"`      // 转发服务的IP
}

// 钉钉报告消息，用
type DingReportMsg struct {
	Date          string `json:"date"`
	Name          string `json:"name"`
	DataNum       int    `json:"data_num"`
	TopDeptNum    int    `json:"top_dept_num"`
	WorkCity      int    `json:"work_city"`
	BottomDeptNum int    `json:"bottom_deptNum"`
	MobileNum     int    `json:"mobile_num"`
	OrgNum        int    `json:"org_num"`
	IdNum         int    `json:"id_num"`
	IsWhiteRole   bool   `json:"is_white_role"`
}

type DingReportMsgList []*DingReportMsg

func (p DingReportMsgList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p DingReportMsgList) Len() int      { return len(p) }
func (p DingReportMsgList) Less(i, j int) bool {
	if p[i].IsWhiteRole && p[j].IsWhiteRole {
		return p[i].DataNum > p[j].DataNum
	}
	if p[i].IsWhiteRole {
		return false
	}
	if p[j].IsWhiteRole {
		return true
	}
	return p[i].DataNum > p[j].DataNum
}

type PairKeyStringValueInt struct {
	Key   string
	Value int
}

type PairKeyStringValueIntList []*PairKeyStringValueInt

func (p PairKeyStringValueIntList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p PairKeyStringValueIntList) Len() int           { return len(p) }
func (p PairKeyStringValueIntList) Less(i, j int) bool { return p[i].Value > p[j].Value }
