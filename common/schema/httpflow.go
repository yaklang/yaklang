package schema

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

const COLORPREFIX = "YAKIT_COLOR_"

func yakitColor(i string) string {
	return COLORPREFIX + i
}

var (
	HTTPFlow_SourceType_MITM    = "mitm"
	HTTPFlow_SourceType_SCAN    = "scan"
	HTTPFlow_SourceType_CRAWLER = "basic-crawler"
	HTTPFlow_SourceType_HAR     = "har"
)

type HTTPFlow struct {
	gorm.Model

	HiddenIndex        string `gorm:"index" json:"hidden_index,omitempty"`
	NoFixContentLength bool   `json:"no_fix_content_length" json:"no_fix_content_length,omitempty"`
	Hash               string `gorm:"unique_index" json:"unique_index,omitempty"`
	IsHTTPS            bool   `json:"is_https,omitempty"`
	Url                string `gorm:"index" json:"url,omitempty"`
	Path               string `json:"path,omitempty"`
	Method             string `json:"method,omitempty"`
	RequestLength      int64  `json:"request_length,omitempty"`
	BodyLength         int64  `json:"body_length,omitempty"`
	ContentType        string `json:"content_type,omitempty"`
	StatusCode         int64  `json:"status_code,omitempty"`
	SourceType         string `json:"source_type,omitempty"`
	Request            string `json:"request,omitempty"`
	Response           string `json:"response,omitempty"`
	Duration           int64  `json:"duration,omitempty"`
	GetParamsTotal     int    `json:"get_params_total,omitempty"`
	PostParamsTotal    int    `json:"post_params_total,omitempty"`
	CookieParamsTotal  int    `json:"cookie_params_total,omitempty"`
	IPAddress          string `json:"ip_address,omitempty"`
	RemoteAddr         string `json:"remote_addr,omitempty"`
	IPInteger          int    `json:"ip_integer,omitempty"`
	Tags               string `json:"tags,omitempty"` // 用来打标！
	Payload            string `json:"payload,omitempty"`

	// Websocket 相关字段
	IsWebsocket bool `json:"is_websocket,omitempty"`
	// 用来计算 websocket hash, 每次连接都不一样，一般来说，内部对象 req 指针足够了
	WebsocketHash string `json:"websocket_hash,omitempty"`

	RuntimeId   string         `json:"runtime_id,omitempty" gorm:"index"`
	FromPlugin  string         `json:"from_plugin,omitempty"`
	ProcessName sql.NullString `json:"process_name,omitempty"`

	// friendly for gorm build instance, not for store
	// 这两个字段不参与数据库存储，但是在序列化的时候，会被覆盖
	// 主要用来标记用户的 Request 和 Response 是否超大
	IsRequestOversize  bool `gorm:"-" json:"is_request_oversize,omitempty"`
	IsResponseOversize bool `gorm:"-" json:"is_response_oversize,omitempty"`

	IsReadTooSlowResponse      bool   `json:"is_read_too_slow_response,omitempty"`
	IsTooLargeResponse         bool   `json:"is_too_large_response,omitempty"`
	TooLargeResponseHeaderFile string `json:"too_large_response_header_file,omitempty"`
	TooLargeResponseBodyFile   string `json:"too_large_response_body_file,omitempty"`
	// 同步到企业端
	UploadOnline bool   `json:"upload_online,omitempty"`
	Host         string `json:"host,omitempty"`
}

func (f *HTTPFlow) GetRequest() string {
	unquoted, err := strconv.Unquote(f.Request)
	if err != nil {
		return ""
	}
	return unquoted
}

func (f *HTTPFlow) GetResponse() string {
	unquoted, err := strconv.Unquote(f.Response)
	if err != nil {
		return ""
	}
	return unquoted
}

func (f *HTTPFlow) SetRequest(req string) {
	f.Request = strconv.Quote(req)
}

func (f *HTTPFlow) SetResponse(rsp string) {
	f.Response = strconv.Quote(rsp)
}

// 颜色与 Tag API
func (f *HTTPFlow) AddTag(appendTags ...string) {
	existed := utils.PrettifyListFromStringSplited(f.Tags, "|")
	existedCount := len(existed)
	extLen := len(appendTags)
	tags := make([]string, existedCount+extLen)
	copy(tags, existed)
	for i := 0; i < extLen; i++ {
		tags[i+existedCount] = appendTags[i]
	}
	f.Tags = strings.Join(utils.RemoveRepeatStringSlice(tags), "|")
}

func (f *HTTPFlow) AddTagToFirst(appendTags ...string) {
	existed := utils.PrettifyListFromStringSplited(f.Tags, "|")
	f.Tags = strings.Join(utils.RemoveRepeatStringSlice(append(appendTags, existed...)), "|")
}

func (f *HTTPFlow) HasColor(color string) bool {
	return utils.StringArrayContains(utils.PrettifyListFromStringSplited(f.Tags, "|"), color)
}

var (
	FLOW_COLOR_RED    = yakitColor("RED")
	FLOW_COLOR_GREEN  = yakitColor("GREEN")
	FLOW_COLOR_BLUE   = yakitColor("BLUE")
	FLOW_COLOR_YELLOW = yakitColor("YELLOW")
	FLOW_COLOR_ORANGE = yakitColor("ORANGE")
	FLOW_COLOR_PURPLE = yakitColor("PURPLE")
	FLOW_COLOR_CYAN   = yakitColor("CYAN")
	FLOW_COLOR_GREY   = yakitColor("GREY")
)

func (f *HTTPFlow) Red() {
	if f.HasColor(FLOW_COLOR_RED) {
		return
	}
	f.AddTag(FLOW_COLOR_RED)
}

func (f *HTTPFlow) Green() {
	if f.HasColor(FLOW_COLOR_GREEN) {
		return
	}
	f.AddTag(FLOW_COLOR_GREEN)
}

func (f *HTTPFlow) Blue() {
	if f.HasColor(FLOW_COLOR_BLUE) {
		return
	}
	f.AddTag(FLOW_COLOR_BLUE)
}

func (f *HTTPFlow) Yellow() {
	if f.HasColor(FLOW_COLOR_YELLOW) {
		return
	}
	f.AddTag(FLOW_COLOR_YELLOW)
}

func (f *HTTPFlow) Orange() {
	if f.HasColor(FLOW_COLOR_ORANGE) {
		return
	}
	f.AddTag(FLOW_COLOR_ORANGE)
}

func (f *HTTPFlow) Purple() {
	if f.HasColor(FLOW_COLOR_PURPLE) {
		return
	}
	f.AddTag(FLOW_COLOR_PURPLE)
}

func (f *HTTPFlow) Cyan() {
	if f.HasColor(FLOW_COLOR_CYAN) {
		return
	}
	f.AddTag(FLOW_COLOR_CYAN)
}

func (f *HTTPFlow) Grey() {
	if f.HasColor(FLOW_COLOR_GREY) {
		return
	}
	f.AddTag(FLOW_COLOR_GREY)
}

func (f *HTTPFlow) ColorSharp(rgbHex string) {
	if f.HasColor(yakitColor(rgbHex)) {
		return
	}
	f.AddTag(yakitColor(rgbHex))
}

func (f *HTTPFlow) CalcHash() string {
	return utils.CalcSha1(f.IsHTTPS, f.Url, f.Path, f.Method, f.BodyLength, f.ContentType, f.StatusCode, f.SourceType, f.Tags, f.Request, f.HiddenIndex, f.RuntimeId, f.FromPlugin)
}

func (f *HTTPFlow) CalcCacheHash(full bool) string {
	return utils.CalcSha1(f.ID, f.Hash, full)
}

func (f *HTTPFlow) BeforeSave() error {
	f.fixURL()
	f.Hash = f.CalcHash()
	return nil
}

func (f *HTTPFlow) fixURL() {
	urlIns := utils.ParseStringToUrl(f.Url)
	if f.IsHTTPS {
		urlIns.Scheme = "https"
	}
	if urlIns != nil {
		host, port, _ := utils.ParseStringToHostPort(urlIns.Host)
		if (port == 443 && urlIns.Scheme == "https") || (port == 80 && urlIns.Scheme == "http") {
			urlIns.Host = host
			f.Url = urlIns.String()
		}
	}
}

func (f *HTTPFlow) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call("httpflow", "create")
	return nil
}

func (f *HTTPFlow) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call("httpflow", "update")
	return nil
}

func (f *HTTPFlow) AfterDelete(tx *gorm.DB) (err error) {
	broadcastData.Call("httpflow", "delete")
	return nil
}
