package yakit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/domainextractor"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const COLORPREFIX = "YAKIT_COLOR_"

func yakitColor(i string) string {
	return COLORPREFIX + i
}

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		lowhttp.RegisterSaveHTTPFlowHandler(func(https bool, req []byte, rsp []byte, url string, remoteAddr string, reqSource string) {
			if rsp == nil || len(rsp) == 0 {
				return
			}
			db := consts.GetGormProjectDatabase()
			flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(db, https, req, rsp, "scan", url, remoteAddr, true, true)
			if err != nil {
				log.Errorf("create httpflow from lowhttp failed: %s", err)
				return
			}
			if reqSource != "" {
				flow.AddTag(reqSource)
			} else {
				if reqSource := os.Getenv(consts.YAKIT_PLUGIN_ID); reqSource != "" {
					flow.AddTag(reqSource)
				}
			}
			err = CreateOrUpdateHTTPFlow(db, flow.Hash, flow)
			if err != nil {
				log.Errorf("save httpflow failed: %s", err)
			}
		})
		return nil
	})
}

type HTTPFlow struct {
	gorm.Model

	HiddenIndex        string
	NoFixContentLength bool   `json:"no_fix_content_length"`
	Hash               string `gorm:"unique_index"`
	IsHTTPS            bool
	Url                string
	Path               string
	Method             string
	BodyLength         int64
	ContentType        string
	StatusCode         int64
	SourceType         string
	Request            string
	Response           string
	GetParamsTotal     int
	PostParamsTotal    int
	CookieParamsTotal  int
	IPAddress          string
	RemoteAddr         string
	IPInteger          int
	Tags               string // 用来打标！

	// Websocket 相关字段
	IsWebsocket bool
	// 用来计算 websocket hash, 每次连接都不一样，一般来说，内部对象 req 指针足够了
	WebsocketHash string
}

func InsertHTTPFlowViaRequest() {

}

type TagAndStatusCode struct {
	Value string
	Count int
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
	f.Tags = strings.Join(funk.Filter(utils.PrettifyListFromStringSplited(f.Tags, "|"), func(i string) bool {
		return !strings.HasPrefix(i, COLORPREFIX)
	}).([]string), "|")
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

func FuzzerResponseToHTTPFlow(db *gorm.DB, rsp *ypb.FuzzerResponse) (*HTTPFlow, error) {
	return SaveFromHTTPFromRaw(db, rsp.IsHTTPS, rsp.RequestRaw, rsp.GetResponseRaw(), "fuzzer", rsp.GetUrl(), rsp.Host)
}

func FuzzParamsToGRPCFuzzableParam(r *mutate.FuzzHTTPRequestParam, isHttps bool) *ypb.FuzzableParam {
	p := &ypb.FuzzableParam{
		Position:  r.PositionVerbose(),
		ParamName: r.Name(),
		IsHTTPS:   isHttps,
	}

	fallback := func() {
		originValueRaw, err := json.Marshal(r.Value())
		if err != nil {
			p.OriginValue = []byte(fmt.Sprintf("%#v", r.Value()))
		} else {
			p.OriginValue = originValueRaw
		}
	}
	switch ret := r.Value().(type) {
	case []string:
		if len(ret) == 1 {
			p.OriginValue = []byte(ret[0])
			break
		}
		fallback()
	default:
		fallback()
	}

	flag := utils.RandStringBytes(40)
	res, err := r.Fuzz(flag).Results()
	if err != nil {
		return p
	}

	var raw []byte
	for _, r := range res {
		raw, err = utils.HttpDumpWithBody(r, true)
		if err != nil {
			continue
		}
	}
	if raw != nil {
		p.AutoTemplate = bytes.ReplaceAll(raw, []byte(flag), []byte("{{randstr(10,10,1)}}"))
	}
	return p
}

func (f *HTTPFlow) ToGRPCModel() (*ypb.HTTPFlow, error) {
	return f.toGRPCModel(false)
}

func (f *HTTPFlow) ToGRPCModelFull() (*ypb.HTTPFlow, error) {
	return f.toGRPCModel(true)
}

func utf8safe(i string) string {
	return utils.EscapeInvalidUTF8Byte([]byte(i))
}

func (f *HTTPFlow) toGRPCModel(full bool) (*ypb.HTTPFlow, error) {
	flow := &ypb.HTTPFlow{
		Id:                 uint64(f.ID),
		IsHTTPS:            f.IsHTTPS,
		Url:                utf8safe(f.Url),
		SourceType:         f.SourceType,
		Path:               utf8safe(f.Path),
		Method:             utf8safe(f.Method),
		BodyLength:         f.BodyLength,
		ContentType:        utf8safe(f.ContentType),
		StatusCode:         f.StatusCode,
		GetParamsTotal:     int64(f.GetParamsTotal),
		PostParamsTotal:    int64(f.PostParamsTotal),
		UpdatedAt:          f.UpdatedAt.Unix(),
		CreatedAt:          f.CreatedAt.Unix(),
		HostPort:           utf8safe(f.RemoteAddr),
		IPAddress:          utf8safe(f.IPAddress),
		HtmlTitle:          "",
		Tags:               f.Tags,
		NoFixContentLength: f.NoFixContentLength,
		IsWebsocket:        f.IsWebsocket,
		WebsocketHash:      f.WebsocketHash,
	}
	// 设置 title
	unquotedResponse, _ := strconv.Unquote(f.Response)
	flow.HtmlTitle = utf8safe(strings.TrimSpace(utils.ExtractTitleFromHTMLTitle(unquotedResponse, "")))

	if f.UpdatedAt.IsZero() {
		flow.UpdatedAt = time.Now().Unix()
		flow.CreatedAt = time.Now().Unix()
	}
	if f.Hash == "" {
		flow.Hash = f.CalcHash()
	} else {
		flow.Hash = f.Hash
	}
	host, port, _ := utils.ParseStringToHostPort(flow.Url)
	flow.HostPort = utils.HostPort(host, port)

	flow.BodySizeVerbose = utils.ByteSize(uint64(flow.BodyLength))

	unquotedReq, err := strconv.Unquote(f.Request)
	if err != nil {
		return nil, utils.Errorf("unquoted failed: %s", err)
	}
	flow.RequestLength = int64(len(unquotedReq))
	flow.RequestSizeVerbose = utils.ByteSize(uint64(len(unquotedReq)))

	if full || len(unquotedReq) <= (maxBodyLength/2) {
		flow.Request = []byte(unquotedReq)
	}

	if full && !flow.NoFixContentLength {
		req, err := lowhttp.ParseStringToHttpRequest(unquotedReq)
		if err != nil {
			return nil, utils.Errorf("[lowhttp.ParseStringToHttpRequest] parse request failed: %s", err)
		}
		for k, vs := range req.Header {
			for _, v := range vs {
				flow.RequestHeader = append(flow.RequestHeader, &ypb.HTTPHeader{
					Header: utf8safe(utils.ParseStringToVisible(k)),
					Value:  utf8safe(utils.ParseStringToVisible(v)),
				})
			}
		}

		if flow.Path == "" {
			flow.Path = utils.EscapeInvalidUTF8Byte([]byte(req.RequestURI))
		}
	}

	// 处理带参数的 Request
	if !flow.NoFixContentLength {
		fReq, _ := mutate.NewFuzzHTTPRequest(flow.Request, mutate.OptHTTPS(flow.IsHTTPS))
		if fReq != nil {
			for _, r := range fReq.GetCommonParams() {
				var fReq = &ypb.FuzzableParam{
					Position:  r.PositionVerbose(),
					ParamName: utf8safe(utils.ParseStringToVisible(r.Name())),
					IsHTTPS:   flow.IsHTTPS,
				}

				if full {
					// 详情模式，这个很耗时。
					fReq = FuzzParamsToGRPCFuzzableParam(r, flow.IsHTTPS)
				}
				fReq.ParamName = utils.EscapeInvalidUTF8Byte([]byte(fReq.ParamName))
				if r.IsGetParams() {
					flow.GetParams = append(flow.GetParams, fReq)
				}
				if r.IsPostParams() {
					flow.PostParams = append(flow.PostParams, fReq)
				}
				if r.IsCookieParams() {
					flow.CookieParams = append(flow.CookieParams, fReq)
				}
			}

			flow.GetParamsTotal = int64(len(flow.GetParams))
			flow.PostParamsTotal = int64(len(flow.PostParams))
			flow.CookieParamsTotal = int64(len(flow.CookieParams))
		}
	}

	if full {
		unquotedRsp := unquotedResponse
		// fixed
		//unquotedRspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(unquotedRsp))
		//if unquotedRspRaw != nil {
		//	unquotedRsp = string(unquotedRspRaw)
		//}
		flow.Response = []byte(unquotedRsp)
		// 显示最多 1M
		if len(flow.Response) > 1000*1000 {
			flow.Response = append(flow.Response[:1000*1000], []byte("...")...)
		}

		if !flow.NoFixContentLength {
			rsp, err := http.ReadResponse(bufio.NewReader(bytes.NewBufferString(unquotedRsp)), nil)
			if err != nil {
				log.Errorf("parse response failed: %s", err)
				return flow, nil
			}
			for k, vs := range rsp.Header {
				for _, v := range vs {
					flow.ResponseHeader = append(flow.ResponseHeader, &ypb.HTTPHeader{
						Header: utf8safe(utils.ParseStringToVisible(k)),
						Value:  utf8safe(utils.ParseStringToVisible(v)),
					})
				}
			}
		}
	}

	// 这里用来标记一下，UTF8 支持情况，要根据情况提供给用户合理 body 建议处理方案
	if full {
		flow.InvalidForUTF8Request = !utf8.ValidString(unquotedReq)
		flow.InvalidForUTF8Response = !utf8.ValidString(unquotedResponse)
		if flow.InvalidForUTF8Request {
			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(flow.Request)
			flow.RawRequestBodyBase64 = codec.EncodeBase64(body)
		}

		if flow.InvalidForUTF8Response {
			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(flow.Response)
			flow.RawResponseBodyBase64 = codec.EncodeBase64(body)
		}

		if flow.InvalidForUTF8Request {
			flow.SafeHTTPRequest = codec.EscapeInvalidUTF8Byte(lowhttp.ConvertHTTPRequestToFuzzTag(flow.Request))
		}
	}

	// 提取数据
	if full {
		domains, rootDomains := domainextractor.ExtractDomainsEx(string(flow.Response))
		jsonObjects := jsonextractor.ExtractStandardJSON(string(flow.Response))
		flow.Domains = make([]string, len(domains))
		for i, d := range domains {
			flow.Domains[i] = utils.EscapeInvalidUTF8Byte([]byte(d))
		}
		flow.RootDomains = make([]string, len(rootDomains))
		for i, d := range rootDomains {
			flow.RootDomains[i] = utils.EscapeInvalidUTF8Byte([]byte(d))
		}
		for _, j := range jsonObjects {
			if j == "" || j == "&&" || j == "{}" {
				continue
			}

			if !utf8.ValidString(j) {
				continue
			}
			if strings.HasPrefix(j, `{"":`) {
				continue
			}
			flow.JsonObjects = append(flow.JsonObjects, utf8safe(j))
		}
	}

	return flow, nil
}

func (f *HTTPFlow) CalcHash() string {
	return utils.CalcSha1(f.IsHTTPS, f.Url, f.Request, f.HiddenIndex)
}

func (f *HTTPFlow) BeforeSave() error {
	f.Hash = f.CalcHash()
	return nil
}

func SaveFromHTTP(db *gorm.DB, isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string) (*HTTPFlow, error) {
	return SaveFromHTTPWithBodySaved(db, isHttps, req, rsp, source, url, remoteAddr, true, true)
}

func SaveFromHTTPFromRaw(db *gorm.DB, isHttps bool, req []byte, rsp []byte, source string, url string, remoteAddr string) (*HTTPFlow, error) {
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(db, isHttps, req, rsp, source, url, remoteAddr, true, true)
	if err != nil {
		return nil, utils.Errorf("create httpflow failed: %s", err)
	}
	err = CreateOrUpdateHTTPFlow(db, flow.CalcHash(), flow)
	if err != nil {
		return nil, err
	}
	return GetHTTPFlowByHash(db, flow.CalcHash())
}

func SaveFromHTTPWithBodySaved(db *gorm.DB, isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string, allowReqBody bool, allowRspBody bool) (*HTTPFlow, error) {
	flow, err := CreateHTTPFlowFromHTTPWithBodySaved(db, isHttps, req, rsp, source, url, remoteAddr, allowReqBody, allowRspBody)
	if err != nil {
		return nil, utils.Errorf("create httpflow failed: %s", err)
	}
	err = CreateOrUpdateHTTPFlow(db, flow.CalcHash(), flow)
	if err != nil {
		return nil, err
	}
	return GetHTTPFlowByHash(db, flow.CalcHash())
}

const maxBodyLength = 4 * 1024 * 1024

func CreateHTTPFlowFromHTTPWithBodySavedFromRaw(db *gorm.DB, isHttps bool, reqRaw []byte, rspRaw []byte, source string, url string, remoteAddr string, allowReqBody bool, allowRspBody bool) (*HTTPFlow, error) {
	var method string
	var requestUri string
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketEx(reqRaw, func(m string, r string, proto string) error {
		method = m
		requestUri = r
		return nil
	})
	if len(body) > maxBodyLength {
		reqRaw = lowhttp.ReplaceHTTPPacketBody([]byte(header), body[:maxBodyLength], false)
	}
	requestRaw := strconv.Quote(string(reqRaw))
	if strings.HasPrefix(requestRaw, `"HTTP/1.`) {
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
	}

	rawNoGzip, _, _ := lowhttp.FixHTTPResponse(rspRaw)
	if len(rawNoGzip) > 0 {
		rspRaw = rawNoGzip
	}

	var rspContentType string
	header, body = lowhttp.SplitHTTPHeadersAndBodyFromPacket(rawNoGzip, func(line string) {
		k, v := lowhttp.SplitHTTPHeader(line)
		if strings.ToLower(k) == "content-type" {
			rspContentType = v
		}
	})
	var rspBody = body
	var bodyLength = int64(len(rspBody))
	if bodyLength > maxBodyLength {
		rspBody = append(rspBody[:maxBodyLength], []byte("(dropped for huge body...)")...)
		rspRaw = lowhttp.ReplaceHTTPPacketBody([]byte(header), rspBody, false)
	}

	var responseRaw = strconv.Quote(string(rspRaw))
	fReq, _ := mutate.NewFuzzHTTPRequest(reqRaw)

	flow := &HTTPFlow{
		IsHTTPS:     isHttps,
		Url:         url,
		Path:        requestUri,
		Method:      method,
		BodyLength:  bodyLength,
		ContentType: rspContentType,
		StatusCode:  int64(lowhttp.ExtractStatusCodeFromResponse(rspRaw)),
		SourceType:  source,
		Request:     requestRaw,
		Response:    responseRaw,
		RemoteAddr:  remoteAddr,
	}
	ip, _, _ := utils.ParseStringToHostPort(remoteAddr)
	if ip != "" {
		flow.IPAddress = ip
		ipInt, _ := utils.IPv4ToUint64(ip)
		if ipInt > 0 {
			flow.IPInteger = int(ipInt)
		}
	}

	if fReq != nil {
		flow.GetParamsTotal = len(fReq.GetGetQueryParams())
		if len(fReq.GetPostJsonParams()) > 0 {
			flow.PostParamsTotal = len(fReq.GetPostJsonParams())
		} else {
			flow.PostParamsTotal = len(fReq.GetPostParams())
		}
		flow.CookieParamsTotal = len(fReq.GetCookieParams())
	}

	flow.Hash = flow.CalcHash()
	return flow, nil
}

func CreateHTTPFlowFromHTTPWithBodySaved(db *gorm.DB, isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string, allowReqBody bool, allowRspBody bool) (*HTTPFlow, error) {
	var urlRaw = url
	if urlRaw == "" {
		u, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
		if err != nil {
			log.Warnf("extract url from request failed: %s", err)
		}
		if u != nil {
			urlRaw = u.String()
		} else {
			if isHttps {
				urlRaw = "https://" + remoteAddr
			} else {
				urlRaw = "http://" + remoteAddr
			}
		}
	}

	reqRaw := httpctx.GetRequestBytes(req)
	if reqRaw == nil {
		var err error
		reqRaw, err = utils.HttpDumpWithBody(req, allowReqBody)
		if err != nil {
			reqRaw, err = utils.HttpDumpWithBody(req, false)
			if err != nil {
				log.Errorf("dump request failed: %s", err)
			}
		}
	}

	rspRaw, err := utils.HttpDumpWithBody(rsp, allowRspBody)
	if err != nil {
		log.Errorf("dump response failed: %s", err)
	}
	return CreateHTTPFlowFromHTTPWithBodySavedFromRaw(db, isHttps, reqRaw, rspRaw, source, urlRaw, remoteAddr, allowReqBody, allowRspBody)
}

func UpdateHTTPFlowTags(db *gorm.DB, i *HTTPFlow) error {
	if i == nil {
		return nil
	}

	if i.ID > 0 {
		if db = db.Model(&HTTPFlow{}).Where("id = ?", i.ID).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by id) failed: %s", db.Error)
			return db.Error
		}
	} else if i.HiddenIndex != "" {
		if db = db.Model(&HTTPFlow{}).Where("hidden_index = ?", i.HiddenIndex).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by hidden_index) failed: %s", db.Error)
			return db.Error
		}
	} else if i.Hash != "" {
		if db = db.Model(&HTTPFlow{}).Where("hash = ?", i.HiddenIndex).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by hash) failed: %s", db.Error)
			return db.Error
		}
	}
	return nil
}

func InsertHTTPFlow(db *gorm.DB, i *HTTPFlow) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
			debug.PrintStack()
		}
	}()

	i.ID = 0
	if db = db.Model(&HTTPFlow{}).Save(i); db.Error != nil {
		return utils.Errorf("insert HTTPFlow failed: %s", db.Error)
	}
	//if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&HTTPFlow{}); db.Error != nil {
	//	return utils.Errorf("create/update HTTPFlow failed: %s", db.Error)
	//}

	return nil
}

func CreateOrUpdateHTTPFlow(db *gorm.DB, hash string, i interface{}) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
		}
	}()

	db = db.Model(&HTTPFlow{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&HTTPFlow{}); db.Error != nil {
		return utils.Errorf("create/update HTTPFlow failed: %s", db.Error)
	}

	return nil
}

func GetHTTPFlow(db *gorm.DB, id int64) (*HTTPFlow, error) {
	var req HTTPFlow
	if db := db.Model(&HTTPFlow{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func GetHTTPFlowByHash(db *gorm.DB, hash string) (*HTTPFlow, error) {
	var req HTTPFlow
	if db := db.Model(&HTTPFlow{}).Where("hash = ?", hash).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteHTTPFlowByID(db *gorm.DB, id int64) error {
	if db := db.Model(&HTTPFlow{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&HTTPFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteHTTPFlow(db *gorm.DB, req *ypb.DeleteHTTPFlowRequest) error {
	if req.GetDeleteAll() {
		db.DropTableIfExists(&HTTPFlow{})
		if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='http_flows';`); db.Error != nil {
			log.Errorf("update sqlite sequence failed: %s", db.Error)
		}
		db.AutoMigrate(&HTTPFlow{})
		//if db := db.Model(&HTTPFlow{}).Where("true").Unscoped().Delete(&HTTPFlow{}); db.Error != nil {
		//	return db.Error
		//}
		return nil
	}

	if len(req.GetId()) > 0 {
		db = db.Or("false")
		for _, id := range req.GetId() {
			db = db.Or("id = ?", id)
		}
		db.Unscoped().Delete(&HTTPFlow{})
		return nil
	}

	if req.GetFilter() != nil {
		db = FilterHTTPFlow(db, req.GetFilter())
		db.Unscoped().Delete(&HTTPFlow{})
		return nil
	}

	if req.GetURLPrefix() != "" {
		db = db.Model(&HTTPFlow{})
		db = bizhelper.FuzzQueryLike(db, "url", req.GetURLPrefix()).Unscoped().Delete(&HTTPFlow{})
		if db.Error != nil {
			return nil
		}
		return nil
	}

	if req.GetItemHash() != nil {
		db = db.Model(&HTTPFlow{})
		db = bizhelper.ExactQueryStringArrayOr(db, "hash", req.GetItemHash())
		if db := db.Where("true").Unscoped().Delete(&HTTPFlow{}); db.Error != nil {
			return db.Error
		}
		return nil
	}

	if len(req.GetURLPrefixBatch()) > 0 {
		db = db.Model(&HTTPFlow{})
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", req.GetURLPrefixBatch())
		db = db.Unscoped().Delete(&HTTPFlow{})
		if db.Error != nil {
			return db.Error
		}
		return nil
	}
	return nil
}

func FilterHTTPFlow(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) *gorm.DB {
	db = db.Model(&HTTPFlow{}) //.Debug()
	if params == nil {
		params = &ypb.QueryHTTPFlowRequest{}
	}

	db = bizhelper.ExactQueryStringArrayOr(db, "source_type", utils.PrettifyListFromStringSplited(params.SourceType, ","))
	// 过滤 Methods
	if ms := utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(params.GetMethods(), ",")); ms != nil {
		db = bizhelper.ExactQueryStringArrayOr(db, "method", ms)
	}
	// 搜索 URL
	db = bizhelper.FuzzQueryLike(db, "url", params.GetSearchURL())
	// status code 这里可以支持范围搜索
	db = bizhelper.QueryBySpecificPorts(db, "status_code", params.GetStatusCode())
	if params.GetHaveBody() {
		db = db.Where("body_length > 0")
	}

	// 搜索是否有对应的参数
	if params.GetHaveCommonParams() {
		db = db.Where("((get_params_total > 0) OR (post_params_total > 0)) OR (cookie_params_total > 0)")
	}

	if params.GetHaveParamsTotal() == "true" {
		db = db.Where("((get_params_total > 0) OR (post_params_total > 0))")
	} else if params.GetHaveParamsTotal() == "false" {
		db = db.Where("((get_params_total = 0) and (post_params_total = 0))")
	}

	if len(params.GetTags()) > 0 {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"tags"}, params.GetTags(), false)
	}

	if len(params.GetColor()) > 0 {
		db = bizhelper.FuzzSearchWithStringArrayOrAf(db, []string{"tags"}, params.GetColor(), false)
	}

	// 搜索 Content-Type
	db = bizhelper.FuzzQueryStringArrayOrLike(db, "content_type",
		utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(params.GetSearchContentType(), ",")))

	if len(params.GetIncludeInUrl()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", params.GetIncludeInUrl())
	}

	if len(params.GetIncludeInIP()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "ip_address", params.GetIncludeInIP())
	}

	if len(params.GetIncludeId()) > 0 {
		db = db.Where("id in (?)", params.GetIncludeId())
	}

	if len(params.GetExcludeInUrl()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "url", params.GetExcludeInUrl())
	}

	if len(params.GetExcludeInIP()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "ip_address", params.GetExcludeInIP())
	}

	if len(params.GetExcludeId()) > 0 {
		for _, id := range params.GetExcludeId() {
			db = db.Where("id <> ?", id)
		}
	}

	if params.AfterBodyLength > 0 {
		db = db.Where("body_length >= ?", params.AfterBodyLength)
	}
	if params.BeforeBodyLength > 0 {
		db = db.Where("body_length <= ?", params.BeforeBodyLength)
	}

	return db
}

func QuickSearchHTTPFlowCount(token string) int {
	db := consts.GetGormProjectDatabase()
	var count int
	db.Model(&HTTPFlow{}).Where(
		"(request like ?) OR (response like ?) OR (url like ?)",
		"%"+token+"%",
		"%"+token+"%",
		"%"+token+"%",
	).Count(&count)
	return count
}

func QueryHTTPFlow(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) (paging *bizhelper.Paginator, ret []*HTTPFlow, err error) {
	if params == nil {
		params = &ypb.QueryHTTPFlowRequest{}
	}

	db = db.Model(&HTTPFlow{}) //.Debug()

	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination

	var needReverse = false
	if params.GetAfterUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", params.GetAfterUpdatedAt(), time.Now().Add(10*time.Minute).Unix())
		p.OrderBy = "updated_at"
		p.Order = "asc"
		needReverse = true
	}
	db = bizhelper.FuzzSearchEx(db, []string{
		"tags", "url", "path", "request",
		"response", "remote_addr",
	}, params.GetKeyword(), false)
	if params.GetAfterId() > 0 {
		db = db.Where("id > ?", params.GetAfterId())
		p.OrderBy = "id"
		p.Order = "asc"
		needReverse = true
	}
	if params.GetBeforeUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", 0, params.GetBeforeUpdatedAt())
		p.OrderBy = "updated_at"
		p.Order = "desc"
	}
	if params.GetBeforeId() > 0 {
		db = db.Where("id < ?", params.GetBeforeId())
		p.OrderBy = "id"
		p.Order = "desc"
	}

	if params.GetOnlyWebsocket() {
		//log.Info("query websocket request flow")
		db = db.Where("(is_websocket = ?) AND (url LIKE 'ws%')", params.OnlyWebsocket)
	}
	switch params.GetIsWebsocket() {
	case "http/https":
		db = db.Where("is_websocket = false")
	case "websocket":
		db = db.Where("(is_websocket = true) AND (url LIKE 'ws%')")
	}

	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)

	db = FilterHTTPFlow(db, params)

	var rets []*HTTPFlow

	if params.OffsetId > 0 {
		db1 := db
		if p.Order == "desc" {
			db1 = db.Where("id < ?", params.OffsetId)
		} else {
			db1 = db.Where("id > ?", params.OffsetId)
		}
		db1.Limit(int(p.Limit)).Offset(0).Scan(&ret)
		paging, db = bizhelper.Paging(db, int(p.Page), int(p.Limit), &rets)
	} else {
		paging, db = bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	}

	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	if needReverse {
		ret = funk.Reverse(ret).([]*HTTPFlow)
	}

	return paging, ret, nil
}

type HTTPFlowUrl struct {
	Url string `json:"url"`
}

func YieldHTTPUrl(db *gorm.DB, ctx context.Context) chan *HTTPFlowUrl {
	outC := make(chan *HTTPFlowUrl)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*HTTPFlowUrl
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func YieldHTTPFlows(db *gorm.DB, ctx context.Context) chan *HTTPFlow {
	outC := make(chan *HTTPFlow)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*HTTPFlow
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

const (
	HTTPFLOW_TAG        = "HTTPFLOW_TAG"
	HTTPFLOW_STATUSCODE = "HTTPFLOW_STATUSCODE"
)

func HTTPFlowStatusCode(refreshRequest bool) (req []*TagAndStatusCode, err error) {

	/*db = db.Raw(`SELECT count(*) as count, status_code as value FROM http_flows GROUP BY status_code order by count desc;`)
	db = db.Scan(&req)
	if db.Error != nil {
		return nil, utils.Errorf("status_code group rows failed: %s", db.Error)
	}*/

	var db = consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("cannot found database config")
		return nil, utils.Error("empty database")
	}
	if !refreshRequest {
		value := GetKey(db, HTTPFLOW_STATUSCODE)
		if value != "" {
			var statusCode []*TagAndStatusCode
			_ = json.Unmarshal([]byte(value), &statusCode)
			if len(statusCode) > 0 {
				return statusCode, nil
			}
		}
	}

	// log.Info("start to execute updating tags")
	db = db.Raw(`SELECT count(*) as count, status_code as value FROM http_flows GROUP BY status_code order by count desc;`)
	rows, err := db.Rows()
	if err != nil {
		return nil, utils.Errorf("rows failed: %s", err)
	}

	var statusCode = make([]*TagAndStatusCode, 0)
	for rows.Next() {
		var codeName string
		var count int
		err = rows.Scan(&count, &codeName)
		if err != nil {
			log.Errorf("scan code stats failed: %s", err)
			continue
		}
		statusCode = append(statusCode, &TagAndStatusCode{
			Value: codeName,
			Count: count,
		})
	}

	raw, _ := json.Marshal(statusCode)
	if len(raw) > 0 {
		log.Infof("start to cache statusCode[%v]", len(raw))
		SetKey(consts.GetGormProfileDatabase(), HTTPFLOW_STATUSCODE, string(raw))
	}
	return statusCode, nil
}

func HTTPFlowTags(refreshRequest bool) ([]*TagAndStatusCode, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("cannot found database config")
		return nil, utils.Error("empty database")
	}
	if !refreshRequest {
		value := GetKey(db, HTTPFLOW_TAG)
		if value != "" {
			var tags []*TagAndStatusCode
			_ = json.Unmarshal([]byte(value), &tags)
			if len(tags) > 0 {
				return tags, nil
			}
		}
	}

	// log.Info("start to execute updating tags")
	db = db.Raw(`SELECT value, count(t.id) as count
from (WITH RECURSIVE split(value, str) AS (
    SELECT null, tags || '|'
    from http_flows WHERE (tags LIKE '%')
    UNION ALL
    SELECT substr(str, 0, instr(str, '|')),
           substr(str, instr(str, '|') + 1)
    FROM split
    WHERE str != ''
)
      SELECT DISTINCT value
      FROM split
      WHERE value is not NULL
        and value != '')
         join http_flows t on ( tags LIKE '%' || value || '%' and value not like 'YAKIT_COLOR_%')
group by value order by count desc;`)
	rows, err := db.Rows()
	if err != nil {
		return nil, utils.Errorf("rows failed: %s", err)
	}

	var tags = make([]*TagAndStatusCode, 0)
	for rows.Next() {
		var tagName string
		var count int
		err = rows.Scan(&tagName, &count)
		if err != nil {
			log.Errorf("scan tag stats failed: %s", err)
			continue
		}
		tags = append(tags, &TagAndStatusCode{
			Value: tagName,
			Count: count,
		})
	}

	raw, _ := json.Marshal(tags)
	if len(raw) > 0 {
		//log.Infof("start to cache tags[%v]", len(raw))
		SetKey(consts.GetGormProfileDatabase(), HTTPFLOW_TAG, string(raw))
	}
	return tags, nil
}

func QueryWebsocketFlowsByHTTPFlowHash(db *gorm.DB, req *ypb.DeleteHTTPFlowRequest) *gorm.DB {
	db = db.Model(&HTTPFlow{})

	if len(req.GetId()) > 0 {
		db = db.Or("false")
		for _, id := range req.GetId() {
			db = db.Or("http_flows.id = ?", id)
		}
	}

	if req.GetFilter() != nil {
		db = FilterHTTPFlow(db, req.GetFilter())
	}

	if req.GetURLPrefix() != "" {
		db = bizhelper.FuzzQueryLike(db, "url", req.GetURLPrefix())
	}
	if len(req.GetURLPrefixBatch()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", req.GetURLPrefixBatch())
	}
	if req.GetItemHash() != nil {
		db = bizhelper.ExactQueryStringArrayOr(db, "hash", req.GetItemHash())
		db = db.Where("true")
	}
	return db
}
