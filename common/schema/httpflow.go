package schema

import (
	"database/sql"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

const COLORPREFIX = "YAKIT_COLOR_"

func yakitColor(i string) string {
	return COLORPREFIX + i
}

type HTTPFlow struct {
	gorm.Model

	HiddenIndex        string
	NoFixContentLength bool   `json:"no_fix_content_length"`
	Hash               string `gorm:"unique_index"`
	IsHTTPS            bool
	Url                string `gorm:"index"`
	Path               string
	Method             string
	BodyLength         int64
	ContentType        string
	StatusCode         int64
	SourceType         string
	Request            string
	Response           string
	Duration           int64
	GetParamsTotal     int
	PostParamsTotal    int
	CookieParamsTotal  int
	IPAddress          string
	RemoteAddr         string
	IPInteger          int
	Tags               string // 用来打标！
	Payload            string

	// Websocket 相关字段
	IsWebsocket bool
	// 用来计算 websocket hash, 每次连接都不一样，一般来说，内部对象 req 指针足够了
	WebsocketHash string

	RuntimeId   string
	FromPlugin  string
	ProcessName sql.NullString

	// friendly for gorm build instance, not for store
	// 这两个字段不参与数据库存储，但是在序列化的时候，会被覆盖
	// 主要用来标记用户的 Request 和 Response 是否超大
	IsRequestOversize  bool `gorm:"-"`
	IsResponseOversize bool `gorm:"-"`

	IsTooLargeResponse         bool
	TooLargeResponseHeaderFile string
	TooLargeResponseBodyFile   string
	// 同步到企业端
	UploadOnline bool `json:"upload_online"`
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

func (f *HTTPFlow) RemoveColor() {
	f.Tags = strings.Join(lo.Filter(utils.PrettifyListFromStringSplited(f.Tags, "|"), func(i string, _ int) bool {
		return !strings.HasPrefix(i, COLORPREFIX)
	}), "|")
}

func (f *HTTPFlow) Red() {
	f.RemoveColor()
	f.AddTag(yakitColor("RED"))
}

func (f *HTTPFlow) Green() {
	f.RemoveColor()
	f.AddTag(yakitColor("GREEN"))
}

func (f *HTTPFlow) Blue() {
	f.RemoveColor()
	f.AddTag(yakitColor("BLUE"))
}

func (f *HTTPFlow) Yellow() {
	f.RemoveColor()
	f.AddTag(yakitColor("YELLOW"))
}

func (f *HTTPFlow) Orange() {
	f.RemoveColor()
	f.AddTag(yakitColor("ORANGE"))
}

func (f *HTTPFlow) Purple() {
	f.RemoveColor()
	f.AddTag(yakitColor("PURPLE"))
}

func (f *HTTPFlow) Cyan() {
	f.RemoveColor()
	f.AddTag(yakitColor("CYAN"))
}

func (f *HTTPFlow) Grey() {
	f.RemoveColor()
	f.AddTag(yakitColor("GREY"))
}

func (f *HTTPFlow) ColorSharp(rgbHex string) {
	f.RemoveColor()
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
