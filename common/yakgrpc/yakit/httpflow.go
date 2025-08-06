package yakit

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	RegisterPostInitDatabaseFunction(func() error {
		RegisterLowHTTPSaveCallback()
		RegisterLowHTTPLabelingCallback()
		return nil
	})
}

func SaveLowHTTPFlow(r *lowhttp.LowhttpResponse, forceSaveFlowSync bool) {
	var (
		https       = r.Https
		req         = r.RawRequest
		rsp         = r.RawPacket
		url         = r.Url
		remoteAddr  = r.RemoteAddr
		reqSource   = r.Source
		runtimeId   = r.RuntimeId
		fromPlugin  = r.FromPlugin
		hiddenIndex = r.HiddenIndex
		payloads    = r.Payloads
		tags        = r.Tags
		duration    = r.TraceInfo.TotalTime
		reqIns      *http.Request
	)

	// fix some field
	if r.HiddenIndex == "" {
		r.HiddenIndex = uuid.NewString()
		hiddenIndex = r.HiddenIndex
	}
	if r.TooLarge {
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte(`[[response too large(`+utils.ByteSize(uint64(r.TooLargeLimit))+`), truncated]] find more in web fuzzer history!`))
	}
	if rsp == nil || len(rsp) == 0 {
		return
	}
	reqIns = r.RequestInstance

	// db := consts.GetGormProjectDatabase()
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(https, req, rsp, "scan", url, remoteAddr, CreateHTTPFlowWithRequestIns(reqIns), CreateHTTPFlowWithTags(strings.Join(r.Tags, "|")), CreateHTTPFlowWithDuration(duration))
	if err != nil {
		log.Errorf("create httpflow from lowhttp failed: %s", err)
		return
	}
	switch ret := strings.ToLower(reqSource); ret {
	case "mitm":
		flow.SourceType = schema.HTTPFlow_SourceType_MITM
	case "basic-crawler", "crawler", "crawlerx":
		flow.SourceType = schema.HTTPFlow_SourceType_CRAWLER
	case "scan", "port-scan", "plugin":
		flow.SourceType = schema.HTTPFlow_SourceType_SCAN

	}
	flow.FromPlugin = fromPlugin
	flow.RuntimeId = runtimeId
	flow.HiddenIndex = hiddenIndex
	flow.Payload = strings.Join(payloads, ",")
	flow.Tags = strings.Join(tags, "|")
	err = InsertHTTPFlowEx(flow, forceSaveFlowSync)
	if err != nil {
		log.Errorf("insert httpflow failed: %s", err)
	}
}

func RegisterLowHTTPSaveCallback() {
	lowhttp.RegisterSaveHTTPFlowHandler(SaveLowHTTPFlow)
}

type TagAndStatusCode struct {
	Value string
	Count int
}

type CreateHTTPFlowConfig struct {
	isHttps            bool
	reqRaw             []byte
	rspRaw             []byte
	fixRspRaw          []byte // 如果设置了，则不会再修复rspRaw
	source             string
	url                string
	remoteAddr         string
	duration           time.Duration
	reqIns             *http.Request // 如果设置了，则不会再解析reqRaw
	hiddenIndex        string
	runtimeID          string
	tags               string
	tooLargeHeaderFile string
	tooLargeBodyFile   string
	fromPlugin         string
}

type CreateHTTPFlowOptions func(c *CreateHTTPFlowConfig)

func CreateHTTPFlowWithFromPlugin(pluginName string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.fromPlugin = pluginName
	}
}

func CreateHTTPFlowWithTags(tags string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.tags = tags
	}
}

func CreateHTTPFlowWithRuntimeID(runtimeId string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.runtimeID = runtimeId
	}
}

func CreateHTTPFlowWithHTTPS(isHttps bool) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.isHttps = isHttps
	}
}

func CreateHTTPFlowWithRequestRaw(reqRaw []byte) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.reqRaw = reqRaw
	}
}

func CreateHTTPFlowWithResponseRaw(rspRaw []byte) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.rspRaw = rspRaw
	}
}

// 如果传入了fixRspRaw，则不会再修复
func CreateHTTPFlowWithFixResponseRaw(fixRspRaw []byte) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.fixRspRaw = fixRspRaw
	}
}

func CreateHTTPFlowWithSource(source string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.source = source
	}
}

func CreateHTTPFlowWithURL(url string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.url = url
	}
}

func CreateHTTPFlowWithRemoteAddr(remoteAddr string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.remoteAddr = remoteAddr
	}
}

func CreateHTTPFlowWithDuration(d time.Duration) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.duration = d
	}
}

// 如果传入了RequestIns，则优先使用这个作为NewFuzzRequest的参数
func CreateHTTPFlowWithRequestIns(reqIns *http.Request) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.reqIns = reqIns
	}
}

func CreateHTTPFlowWithTooLargeResponseHeaderFile(fp string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.tooLargeHeaderFile = fp
	}
}

func CreateHTTPFlowWithTooLargeResponseBodyFile(fp string) CreateHTTPFlowOptions {
	return func(c *CreateHTTPFlowConfig) {
		c.tooLargeBodyFile = fp
	}
}

func FuzzerResponseToHTTPFlow(db *gorm.DB, rsp *ypb.FuzzerResponse) (*schema.HTTPFlow, error) {
	index := rsp.GetUUID()
	_, flows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{HiddenIndex: []string{index}})
	if err != nil || len(flows) < 1 {
		return SaveFromHTTPFromRaw(db, rsp.GetIsHTTPS(), rsp.GetRequestRaw(), rsp.GetResponseRaw(), "scan", rsp.GetUrl(), rsp.GetHost())
	}
	return flows[0], nil
}

func SaveFromHTTP(db *gorm.DB, isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string) (*schema.HTTPFlow, error) {
	return SaveFromHTTPWithBodySaved(db, isHttps, req, rsp, source, url, remoteAddr)
}

func SaveFromHTTPFromRaw(db *gorm.DB, isHttps bool, req []byte, rsp []byte, source string, url string, remoteAddr string) (*schema.HTTPFlow, error) {
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(isHttps, req, rsp, source, url, remoteAddr)
	if err != nil {
		return nil, utils.Errorf("create httpflow failed: %s", err)
	}
	err = CreateOrUpdateHTTPFlow(db, flow.CalcHash(), flow)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

func SaveFromHTTPWithBodySaved(db *gorm.DB, isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string) (*schema.HTTPFlow, error) {
	flow, err := CreateHTTPFlowFromHTTPWithBodySaved(isHttps, req, rsp, source, url, remoteAddr)
	if err != nil {
		return nil, utils.Errorf("create httpflow failed: %s", err)
	}
	err = CreateOrUpdateHTTPFlow(db, flow.CalcHash(), flow)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

const maxBodyLength = 4 * 1024 * 1024

func CreateHTTPFlow(opts ...CreateHTTPFlowOptions) (*schema.HTTPFlow, error) {
	c := &CreateHTTPFlowConfig{}
	for _, opt := range opts {
		opt(c)
	}

	var (
		isHttps            = c.isHttps
		reqRaw             = c.reqRaw
		rspRaw             = c.rspRaw
		fixRspRaw          = c.fixRspRaw
		source             = c.source
		url                = c.url
		remoteAddr         = c.remoteAddr
		reqIns             = c.reqIns
		runtimeID          = c.runtimeID
		tags               = c.tags
		duration           = int64(c.duration)
		tooLargeHeaderFile = c.tooLargeHeaderFile
		tooLargeBodyFile   = c.tooLargeBodyFile
		fromPlugin         = c.fromPlugin
	)

	var (
		method     string
		requestUri string
		fReq       *mutate.FuzzHTTPRequest
	)

	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketEx(reqRaw, func(m string, r string, proto string) error {
		method = m
		requestUri = r
		return nil
	})

	if false && len(body) > maxBodyLength {
		// Truncated by saver
		reqRaw = lowhttp.ReplaceHTTPPacketBody([]byte(header), body[:maxBodyLength], false)
	}
	requestRaw := strconv.Quote(string(reqRaw))
	if strings.HasPrefix(requestRaw, `"HTTP/1.`) {
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
		log.Errorf("[BUG] requestRaw is invalid: %s", requestRaw)
	}

	// 如果已经修复过响应，则不会再修复
	if len(fixRspRaw) == 0 {
		rawNoGzip, _, _ := lowhttp.FixHTTPResponse(rspRaw)
		if len(rawNoGzip) > 0 {
			rspRaw = rawNoGzip
		}
	} else {
		rspRaw = fixRspRaw
	}
	if rspRaw == nil {
		rspRaw = make([]byte, 0)
	}

	var rspContentType string
	header, body = lowhttp.SplitHTTPHeadersAndBodyFromPacket(rspRaw, func(line string) {
		k, v := lowhttp.SplitHTTPHeader(line)
		if strings.ToLower(k) == "content-type" {
			rspContentType = v
		}
	})
	responseRaw := strconv.Quote(string(rspRaw))

	flow := &schema.HTTPFlow{
		IsHTTPS:                    isHttps,
		Url:                        url,
		Path:                       requestUri,
		Method:                     method,
		BodyLength:                 int64(len(body)),
		RequestLength:              int64(len(requestRaw)),
		ContentType:                rspContentType,
		StatusCode:                 int64(lowhttp.ExtractStatusCodeFromResponse(rspRaw)),
		SourceType:                 source,
		Request:                    requestRaw,
		Response:                   responseRaw,
		RemoteAddr:                 remoteAddr,
		HiddenIndex:                uuid.NewString(),
		RuntimeId:                  runtimeID,
		Tags:                       tags,
		Duration:                   duration,
		TooLargeResponseBodyFile:   tooLargeBodyFile,
		TooLargeResponseHeaderFile: tooLargeHeaderFile,
		FromPlugin:                 fromPlugin,
	}

	// 如果设置了 reqIns，则不会再解析 reqRaw
	if reqIns != nil {
		fReq, _ = mutate.NewFuzzHTTPRequest(reqIns)
		flow.IsTooLargeResponse = httpctx.GetResponseTooLarge(reqIns)
		flow.IsReadTooSlowResponse = httpctx.GetResponseReadTooSlow(reqIns)
		if flow.IsTooLargeResponse || flow.IsReadTooSlowResponse {
			flow.TooLargeResponseHeaderFile = httpctx.GetResponseTooLargeHeaderFile(reqIns)
			flow.TooLargeResponseBodyFile = httpctx.GetResponseTooLargeBodyFile(reqIns)
			flow.BodyLength = httpctx.GetResponseTooLargeSize(reqIns)
		}
	} else {
		fReq, _ = mutate.NewFuzzHTTPRequest(reqRaw)
	}
	host, _, _ := utils.ParseStringToHostPort(url)
	flow.Host = host

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

		postParams := fReq.GetPostJsonParams()
		if len(postParams) <= 0 {
			postParams = fReq.GetPostXMLParams()
		}
		if len(postParams) <= 0 {
			postParams = fReq.GetPostParams()
		}
		flow.PostParamsTotal = len(postParams)

		flow.CookieParamsTotal = len(fReq.GetCookieParams())
	}

	flow.Hash = flow.CalcHash()
	return flow, nil
}

func CreateHTTPFlowFromHTTPWithBodySavedFromRaw(isHttps bool, reqRaw []byte, rspRaw []byte, source string, url string, remoteAddr string, opts ...CreateHTTPFlowOptions) (*schema.HTTPFlow, error) {
	extOpts := []CreateHTTPFlowOptions{
		CreateHTTPFlowWithHTTPS(isHttps), CreateHTTPFlowWithRequestRaw(reqRaw), CreateHTTPFlowWithResponseRaw(rspRaw), CreateHTTPFlowWithSource(source), CreateHTTPFlowWithURL(url), CreateHTTPFlowWithRemoteAddr(remoteAddr),
	}
	extOpts = append(extOpts, opts...)
	flow, err := CreateHTTPFlow(extOpts...)
	if err != nil {
		return nil, err
	}
	return flow, nil
}

func createHTTPFlowFromHTTP(isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string, opts ...CreateHTTPFlowOptions) (*schema.HTTPFlow, error) {
	opts = append(opts, CreateHTTPFlowWithRequestIns(req))

	urlRaw := url
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

	var (
		plainRequest  []byte
		plainResponse []byte
		err           error
	)
	// 为了此处的请求与mitm的请求保持一致，需要重新从httpctx中获取
	if httpctx.GetRequestIsModified(req) {
		plainRequest = httpctx.GetHijackedRequestBytes(req)
	} else {
		plainRequest = httpctx.GetPlainRequestBytes(req)
		if len(plainRequest) <= 0 {
			plainRequest = lowhttp.DeletePacketEncoding(httpctx.GetBareRequestBytes(req))
		}
	}
	if len(plainRequest) <= 0 {
		plainRequest, err = utils.HttpDumpWithBody(req, true)
		if err != nil {
			plainRequest, err = utils.HttpDumpWithBody(req, false)
			if err != nil {
				log.Errorf("dump request failed: %s", err)
			}
		}
	}

	// 为了此处的响应与mitm的响应保持一致，需要重新从httpctx中获取
	if rsp != nil {
		if httpctx.GetResponseIsModified(req) {
			plainResponse = httpctx.GetHijackedResponseBytes(req)
		} else {
			plainResponse = httpctx.GetPlainResponseBytes(req)
			if len(plainResponse) <= 0 {
				plainResponse = lowhttp.DeletePacketEncoding(httpctx.GetBareResponseBytes(req))
			}
		}
		if len(plainResponse) <= 0 {
			plainResponse, err = utils.HttpDumpWithBody(rsp, true)
			if err != nil {
				log.Errorf("dump response failed: %s", err)
			}
		}
	} else {
		plainResponse = make([]byte, 0)
	}

	return CreateHTTPFlowFromHTTPWithBodySavedFromRaw(isHttps, plainRequest, plainResponse, source, urlRaw, remoteAddr, opts...)
}

func CreateHTTPFlowFromHTTPWithNoRspSaved(isHttps bool, req *http.Request, source string, url string, remoteAddr string, opts ...CreateHTTPFlowOptions) (*schema.HTTPFlow, error) {
	return createHTTPFlowFromHTTP(isHttps, req, nil, source, url, remoteAddr, opts...)
}

func CreateHTTPFlowFromHTTPWithBodySaved(isHttps bool, req *http.Request, rsp *http.Response, source string, url string, remoteAddr string, opts ...CreateHTTPFlowOptions) (*schema.HTTPFlow, error) {
	return createHTTPFlowFromHTTP(isHttps, req, rsp, source, url, remoteAddr, opts...)
}

// direct save
func UpdateHTTPFlowTags(db *gorm.DB, i *schema.HTTPFlow) (finErr error) {
	if i == nil {
		return nil
	}
	db = db.Model(&schema.HTTPFlow{})
	id, tags := i.ID, i.Tags
	defer func() {
		if finErr == nil {
			// 需要手动触发广播，因为要拿到id，在AfterSave/AfterUpdate中无法拿到id
			schema.GetBroadCast_Data().Call("httpflow", map[string]any{
				"id":     id,
				"tags":   tags,
				"action": "update",
			})
		}
	}()
	updateData := map[string]interface{}{
		"tags": i.Tags,
	}
	if i.ID > 0 {
		i.Hash = i.CalcHash()
		updateData["hash"] = i.Hash
		if db = db.Where("id = ?", i.ID).UpdateColumns(updateData); db.Error != nil {
			log.Errorf("update tags(by id) failed: %s", db.Error)
			return db.Error
		}
	} else if i.HiddenIndex != "" {
		i.Hash = i.CalcHash()
		updateData["hash"] = i.Hash
		if db = db.Where("hidden_index = ?", i.HiddenIndex).UpdateColumns(updateData); db.Error != nil {
			log.Errorf("update tags(by hidden_index) failed: %s", db.Error)
			return db.Error
		}
	} else if i.Hash != "" {
		if db = db.Where("hash = ?", i.Hash).UpdateColumns(updateData); db.Error != nil {
			log.Errorf("update tags(by hash) failed: %s", db.Error)
			return db.Error
		}
	}
	return nil
}

func InsertHTTPFlow(db *gorm.DB, i *schema.HTTPFlow) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
			debug.PrintStack()
		}
	}()

	i.ID = 0
	if db = db.Model(&schema.HTTPFlow{}).Save(i); db.Error != nil {
		return utils.Errorf("insert HTTPFlow failed: %s", db.Error)
	}

	return nil
}

func CreateOrUpdateHTTPFlow(db *gorm.DB, hash string, i *schema.HTTPFlow) (fErr error) {
	defer func() {
		if err := recover(); err != nil {
			fErr = utils.Errorf("met panic error: %v", err)
		}
	}()

	var flowCopy schema.HTTPFlow
	if db := db.Model(&flowCopy).Where("hash = ?", hash).Assign(i).FirstOrCreate(&flowCopy); db.Error != nil {
		return utils.Errorf("create/update HTTPFlow failed: %s", db.Error)
	}
	i.ID = flowCopy.ID
	return nil
}

func SaveHTTPFlow(db *gorm.DB, i *schema.HTTPFlow) error {
	if db := db.Model(&schema.HTTPFlow{}).Save(i); db.Error != nil {
		return db.Error
	}

	return nil
}

// choose db save mode by const
func UpdateHTTPFlowTagsEx(i *schema.HTTPFlow) error {
	if consts.GLOBAL_DB_SAVE_SYNC.IsSet() {
		return UpdateHTTPFlowTags(consts.GetGormProjectDatabase(), i)
	} else {
		DBSaveAsyncChannel <- func(db *gorm.DB) error {
			return UpdateHTTPFlowTags(db, i)
		}
		return nil
	}
}

func InsertHTTPFlowEx(i *schema.HTTPFlow, forceSync bool, finishHandler ...func()) error {
	if consts.GLOBAL_DB_SAVE_SYNC.IsSet() || forceSync {
		return InsertHTTPFlow(consts.GetGormProjectDatabase(), i)
	} else {
		DBSaveAsyncChannel <- func(db *gorm.DB) error {
			err := InsertHTTPFlow(db, i)
			for _, h := range finishHandler {
				h()
			}
			return err
		}
		return nil
	}
}

func CreateOrUpdateHTTPFlowExg(hash string, i *schema.HTTPFlow) error {
	if consts.GLOBAL_DB_SAVE_SYNC.IsSet() {
		return CreateOrUpdateHTTPFlow(consts.GetGormProjectDatabase(), hash, i)
	} else {
		DBSaveAsyncChannel <- func(db *gorm.DB) error {
			return CreateOrUpdateHTTPFlow(db, hash, i)
		}
		return nil
	}
}

func AppendHTTPFlowTagsByHiddenIndexEx(hiddenIndex string, tags ...string) error {
	if consts.GLOBAL_DB_SAVE_SYNC.IsSet() {
		flow, err := GetHTTPFlowByHiddenIndex(consts.GetGormProjectDatabase(), hiddenIndex)
		if err != nil {
			return err
		} else {
			flow.AddTag(tags...)
		}
		return UpdateHTTPFlowTags(consts.GetGormProjectDatabase(), flow)
	} else {
		DBSaveAsyncChannel <- func(db *gorm.DB) error {
			flow, err := GetHTTPFlowByHiddenIndex(db, hiddenIndex)
			if err != nil {
				return err
			} else {
				flow.AddTag(tags...)
			}
			return UpdateHTTPFlowTags(db, flow)
		}
		return nil
	}
}

func GetHTTPFlow(db *gorm.DB, id int64) (*schema.HTTPFlow, error) {
	var req schema.HTTPFlow
	if db := db.Model(&schema.HTTPFlow{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func GetHTTPFlowByIDOrHash(db *gorm.DB, id int64, hash string) (*schema.HTTPFlow, error) {
	var req schema.HTTPFlow
	if db := db.Model(&schema.HTTPFlow{}).Where("id = ? OR hash = ?", id, hash).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func SaveSetTagForHTTPFlow(db *gorm.DB, id int64, hash string, tags []string) error {
	flow, err := GetHTTPFlowByIDOrHash(db, id, hash)
	if err != nil {
		return err
	}
	// flow.AddTag(tags...)
	extLen := len(tags)
	tagsData := make([]string, extLen)
	if extLen > 0 {
		for i := 0; i < extLen; i++ {
			tagsData[i] = tags[i]
		}
	}
	flow.Tags = strings.Join(utils.RemoveRepeatStringSlice(tagsData), "|")
	err = UpdateHTTPFlowTags(db, flow)
	m, _ := model.ToHTTPFlowGRPCModel(flow, true)
	model.SetHTTPFlowCacheGRPCModel(flow, false, m)
	if err != nil {
		return err
	}
	return nil
}

func GetHttpFlowByRuntimeId(db *gorm.DB, rid string) (*schema.HTTPFlow, error) {
	var req schema.HTTPFlow
	if dbx := db.Model(&schema.HTTPFlow{}).Where("runtime_id=?", rid).First(&req); dbx.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}
	return &req, nil
}

func GetHTTPFlowByHash(db *gorm.DB, hash string) (*schema.HTTPFlow, error) {
	var req schema.HTTPFlow
	if db := db.Model(&schema.HTTPFlow{}).Where("hash = ?", hash).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func GetHTTPFlowByHiddenIndex(db *gorm.DB, index string) (*schema.HTTPFlow, error) {
	var req schema.HTTPFlow
	if db := db.Model(&schema.HTTPFlow{}).Where("hidden_index = ?", index).First(&req); db.Error != nil {
		return nil, utils.Errorf("get HTTPFlow failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteHTTPFlowByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.HTTPFlow{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.HTTPFlow{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteHTTPFlow(db *gorm.DB, req *ypb.DeleteHTTPFlowRequest) error {
	if req.GetDeleteAll() {
		db.DropTableIfExists(&schema.HTTPFlow{})
		if db := db.Exec(`UPDATE SQLITE_SEQUENCE SET SEQ=0 WHERE NAME='http_flows';`); db.Error != nil {
			log.Errorf("update sqlite sequence failed: %s", db.Error)
		}
		db.AutoMigrate(&schema.HTTPFlow{})
		DeleteProjectKeyBareRequestAndResponse(db)
		return nil
	}

	if len(req.GetId()) > 0 {
		db = db.Or("false")
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", req.GetId())
		// for _, id := range req.GetId() {
		// 	db = db.Or("id = ?", id)
		// }
		db.Unscoped().Delete(&schema.HTTPFlow{})
		return nil
	}

	if req.GetFilter() != nil {
		db = FilterHTTPFlow(db, req.GetFilter())
		db.Unscoped().Delete(&schema.HTTPFlow{})
		return nil
	}

	if req.GetURLPrefix() != "" {
		db = db.Model(&schema.HTTPFlow{})
		db = bizhelper.FuzzQueryLike(db, "url", req.GetURLPrefix()).Unscoped().Delete(&schema.HTTPFlow{})
		if db.Error != nil {
			return nil
		}
		return nil
	}

	if req.GetItemHash() != nil {
		db = db.Model(&schema.HTTPFlow{})
		db = bizhelper.ExactQueryStringArrayOr(db, "hash", req.GetItemHash())
		if db := db.Where("true").Unscoped().Delete(&schema.HTTPFlow{}); db.Error != nil {
			return db.Error
		}
		return nil
	}

	if len(req.GetURLPrefixBatch()) > 0 {
		db = db.Model(&schema.HTTPFlow{})
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", req.GetURLPrefixBatch())
		db = db.Unscoped().Delete(&schema.HTTPFlow{})
		if db.Error != nil {
			return db.Error
		}
		return nil
	}
	return nil
}

func FilterHTTPFlow(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) *gorm.DB {
	subQuery := db
	db = db.Model(&schema.HTTPFlow{}) //.Debug()
	if params == nil {
		params = &ypb.QueryHTTPFlowRequest{}
	}

	if params.GetAfterUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", params.GetAfterUpdatedAt(), time.Now().Add(10*time.Minute).Unix())
	}
	keywordType := strings.ToLower(params.GetKeywordType())
	keywordFields := []string{
		"tags", "url", "path", "request",
		"response", "remote_addr",
	}
	if keywordType == "request" {
		keywordFields = []string{"request"}
	} else if keywordType == "response" {
		keywordFields = []string{"response"}
	}
	if params.GetKeyword() != "" {
		params.Keyword = strings.Trim(strconv.Quote(params.Keyword), "\"")
	}
	db = bizhelper.FuzzSearchEx(db, keywordFields, params.GetKeyword(), false)
	if params.GetPayloadKeyword() != "" {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "payload", []string{params.GetPayloadKeyword()})
	}
	if len(params.GetExcludeKeywords()) > 0 {
		for _, keyword := range params.GetExcludeKeywords() {
			db = bizhelper.FuzzSearchNotEx(db, []string{
				"tags", "url", "path", "request",
				"response", "remote_addr",
			}, keyword, false)
		}
	}
	if params.GetAfterId() > 0 {
		db = db.Where("id > ?", params.GetAfterId())
	}
	if params.GetBeforeUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", 0, params.GetBeforeUpdatedAt())
	}
	if params.GetBeforeId() > 0 {
		db = db.Where("id < ?", params.GetBeforeId())
	}

	if params.GetOnlyWebsocket() {
		// log.Info("query websocket request flow")
		db = db.Where("is_websocket = ?", params.OnlyWebsocket)
	}
	switch params.GetIsWebsocket() {
	case "http/https":
		db = db.Where("is_websocket = false")
	case "websocket":
		db = db.Where("is_websocket = true")
	}

	db = bizhelper.ExactQueryStringArrayOr(db, "source_type", utils.PrettifyListFromStringSplited(params.SourceType, ","))
	// 过滤 Methods
	if ms := utils.StringArrayFilterEmpty(utils.PrettifyListFromStringSplited(params.GetMethods(), ",")); ms != nil {
		db = bizhelper.ExactQueryStringArrayOr(db, "method", ms)
	}
	// 搜索 URL
	db = bizhelper.FuzzQueryLike(db, "url", params.GetSearchURL())
	db = bizhelper.FuzzQueryLike(db, "from_plugin", params.GetFromPlugin())
	// status code 这里可以支持范围搜索,支持1-200,300这样的写法
	statusCodeRaw := params.GetStatusCode()
	if strings.HasPrefix(statusCodeRaw, "-") || strings.HasSuffix(statusCodeRaw, "-") {
		// 排除-200,200-这样的写法
		db = db.Where("true = false")
	} else {
		statusCode := utils.ParseStringToPorts(statusCodeRaw)
		if len(statusCode) > 0 {
			db = bizhelper.ExactQueryIntArrayOr(db, "status_code", statusCode)
		} else if len(statusCodeRaw) > 0 {
			// 如果状态码不合理，应该返回空
			db = db.Where("true = false")
		}
	}
	excludeStatusCodeRaw := params.GetExcludeStatusCode()
	if strings.HasPrefix(excludeStatusCodeRaw, "-") || strings.HasSuffix(excludeStatusCodeRaw, "-") {
		// 排除-200,200-这样的写法
		// skip
	} else {
		excludeStatusCode := utils.ParseStringToPorts(excludeStatusCodeRaw)
		if len(excludeStatusCode) > 0 {
			db = bizhelper.ExactExcludeQueryInt64Array(db, "status_code", lo.Map(excludeStatusCode, func(item int, index int) int64 {
				return int64(item)
			}))
		} else if len(excludeStatusCodeRaw) > 0 {
			// 如果状态码不合理，应该返回空
			// skip
		}
	}
	if params.GetHaveBody() {
		db = db.Where("body_length > 0")
	}
	db = bizhelper.ExactQueryString(db, "runtime_id", params.GetRuntimeId())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "runtime_id", params.GetRuntimeIDs())

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
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"tags"}, params.GetColor(), false)
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
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", params.GetIncludeId())
	}

	if len(params.GetExcludeInUrl()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "url", params.GetExcludeInUrl())
	}

	if len(params.GetExcludeInIP()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "ip_address", params.GetExcludeInIP())
	}

	if len(params.GetIncludePath()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "path", params.GetIncludePath())
	}

	if len(params.GetExcludePath()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "path", params.GetExcludePath())
	}

	if len(params.GetIncludeSuffix()) > 0 {
		var suffixes []string
		for _, suffix := range params.GetIncludeSuffix() {
			if !strings.HasPrefix(suffix, ".") {
				suffix = "." + suffix
			}
			suffixes = append(suffixes, suffix)
		}
		db = bizhelper.FuzzQueryStringArrayOrLike(db, "path", suffixes)
	}
	if len(params.GetExcludeSuffix()) > 0 {
		var suffixes []string
		for _, suffix := range params.GetExcludeSuffix() {
			if !strings.HasPrefix(suffix, ".") {
				suffix = "." + suffix
			}
			suffixes = append(suffixes, suffix)
		}
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "path", suffixes)
	}
	if len(params.GetExcludeContentType()) > 0 {
		db = bizhelper.FuzzQueryStringArrayOrLikeExclude(db, "content_type", params.GetExcludeContentType())
	}

	if len(params.GetExcludeId()) > 0 {
		db = bizhelper.ExactExcludeQueryInt64Array(db, "id", params.GetExcludeId())
	}

	if len(params.GetIncludeHash()) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "hash", params.GetIncludeHash())
	}
	if len(params.GetHiddenIndex()) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "hidden_index", params.GetHiddenIndex())
	}
	if len(params.GetAnalyzedIds()) > 0 {
		db = db.Where("id IN (?)",
			subQuery.
				Table("analyzed_http_flows").
				Where("id IN (?)", params.GetAnalyzedIds()).
				Select("http_flow_id").QueryExpr(),
		)
	}
	if params.AfterBodyLength > 0 {
		db = db.Where("body_length >= ?", params.AfterBodyLength)
	}
	if params.BeforeBodyLength > 0 {
		db = db.Where("body_length <= ?", params.BeforeBodyLength)
	}

	if len(params.ProcessName) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "process_name", params.ProcessName)
	}

	return db
}

func QuickSearchHTTPFlowCount(token string) int {
	db := consts.GetGormProjectDatabase()
	var count int
	db.Model(&schema.HTTPFlow{}).Where(
		"(request like ?) OR (response like ?) OR (url like ?)",
		"%"+token+"%",
		"%"+token+"%",
		"%"+token+"%",
	).Count(&count)
	return count
}

func QuickSearchMITMHTTPFlowCount(token string) int {
	db := consts.GetGormProjectDatabase()
	var count int
	db.Model(&schema.HTTPFlow{}).Where(
		"(request like ?) OR (response like ?) OR (url like ?)",
		"%"+token+"%",
		"%"+token+"%",
		"%"+token+"%",
	).Where("source_type = ?", "mitm").Count(&count)
	return count
}

func CountHTTPFlowByRuntimeID(db *gorm.DB, runtimeId string) int {
	var count int
	db.Model(&schema.HTTPFlow{}).Where("runtime_id = ?", runtimeId).Count(&count)
	return count
}

// BuildHTTPFlowQuery 构建带有过滤条件的查询
func BuildHTTPFlowQuery(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) *gorm.DB {
	// 应用所有过滤条件
	if params == nil {
		params = &ypb.QueryHTTPFlowRequest{}
	}

	if !params.GetFull() {
		extraSelectField := ""
		if params.GetWithPayload() {
			extraSelectField = "payload,"
		}
		// 只查询部分字段，主要是为了处理大的 response 和 request 的情况，同时告诉用户
		// max request size is 200K -> 200 * 1024 -> 204800
		// max response size is 500K -> 500 * 1024 -> 512000
		db = db.Select(fmt.Sprintf(`id,created_at,updated_at,hidden_index,%s -- basic gorm fields
body_length, -- handle body length should be careful, if it's big, no return response

-- metainfo
is_http_s, -- legacy
no_fix_content_length, hash,url, path, method,
content_type, status_code, source_type,
get_params_total, post_params_total, cookie_params_total,
ip_address, remote_addr, ip_integer,
tags, is_websocket, websocket_hash, runtime_id, from_plugin,
process_name,
is_read_too_slow_response,

-- request is larger than 200K, return empty string
LENGTH(request) > 204800 as is_request_oversize,
CASE WHEN LENGTH(request) > 204800 THEN '' ELSE request END as request,

-- response is larger than 500K, return empty string
LENGTH(response) > 512000 as is_response_oversize,
CASE WHEN LENGTH(response) > 512000 THEN '' ELSE response END as response,

-- is response too large
is_too_large_response, 
too_large_response_header_file, too_large_response_body_file, duration
`, extraSelectField))
	}

	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	p := params.Pagination
	if p.OrderBy == "" {
		p.OrderBy = "id" // 如果 没有设置 orderby 则以ID排序
	}

	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)

	db = FilterHTTPFlow(db, params)

	return db
}

func QueryHTTPFlow(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) (paging *bizhelper.Paginator, httpflows []*schema.HTTPFlow, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	queryDB := BuildHTTPFlowQuery(db.Model(&schema.HTTPFlow{}), params)

	return SelectHTTPFlowFromDB(queryDB, params)
}

func SelectHTTPFlowFromDB(queryDB *gorm.DB, params *ypb.QueryHTTPFlowRequest) (paging *bizhelper.Paginator, httpflows []*schema.HTTPFlow, err error) {
	var limitFlows, fullFlows []*schema.HTTPFlow

	if params.OffsetId > 0 {
		offsetDB := queryDB
		if params.Pagination.Order == "desc" {
			offsetDB = offsetDB.Where("id < ?", params.OffsetId)
		} else {
			offsetDB = offsetDB.Where("id > ?", params.OffsetId)
		}
		offsetDB.Limit(int(params.Pagination.Limit)).Offset(0).Scan(&limitFlows)
		paging, queryDB = bizhelper.Paging(queryDB, int(params.Pagination.Page), int(params.Pagination.Limit), &fullFlows)
	} else {
		paging, queryDB = bizhelper.Paging(queryDB, int(params.Pagination.Page), int(params.Pagination.Limit), &limitFlows)
	}

	if queryDB.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", queryDB.Error)
	}

	return paging, limitFlows, nil
}

type HTTPFlowUrl struct {
	Url string `json:"url"`
}

func YieldHTTPUrl(db *gorm.DB, ctx context.Context) chan *HTTPFlowUrl {
	return bizhelper.YieldModel[*HTTPFlowUrl](ctx, db)
}

func YieldHTTPFlows(db *gorm.DB, ctx context.Context) chan *schema.HTTPFlow {
	return YieldHTTPFlowsEx(db, ctx, nil)
}

func YieldHTTPFlowsEx(db *gorm.DB, ctx context.Context, countCallback func(int)) chan *schema.HTTPFlow {
	return bizhelper.YieldModel[*schema.HTTPFlow](ctx, db, bizhelper.WithYieldModel_CountCallback(countCallback))
}

const (
	HTTPFLOW_TAG        = "HTTPFLOW_TAG"
	HTTPFLOW_STATUSCODE = "HTTPFLOW_STATUSCODE"
)

/*func HTTPFlowStatusCode(refreshRequest bool) (req []*TagAndStatusCode, err error) {
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
}*/

func HTTPFlowTags(refreshRequest bool) ([]*TagAndStatusCode, error) {
	tagCounts := make(map[string]int)
	for _, v := range model.GlobalHTTPFlowCache.GetAll() {
		for _, tag := range strings.Split(v.Tags, "|") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tagCounts[tag]++
			}
		}
	}
	tags := make([]*TagAndStatusCode, 0)
	for k, v := range tagCounts {
		if !strings.HasPrefix(k, schema.COLORPREFIX) {
			tags = append(tags, &TagAndStatusCode{
				Value: k,
				Count: v,
			})
		}
	}
	return tags, nil
}

func QueryWebsocketFlowsByHTTPFlowHash(db *gorm.DB, req *ypb.DeleteHTTPFlowRequest) *gorm.DB {
	db = db.Model(&schema.HTTPFlow{})

	if len(req.GetId()) > 0 {
		db = db.Or("false")
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", req.GetId())
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

func ExportHTTPFlow(db *gorm.DB, params *ypb.ExportHTTPFlowsRequest) (paging *bizhelper.Paginator, ret []*schema.HTTPFlow, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	queryParams := params.ExportWhere
	queryParams.IncludeId = params.Ids

	db = db.Model(&schema.HTTPFlow{})
	// overwrite Select Field, fix payloads
	for i, field := range params.FieldName {
		if field != "payloads" {
			continue
		}
		queryParams.WithPayload = true
		params.FieldName[i] = "payload"
	}

	queryDB := BuildHTTPFlowQuery(db, queryParams).Select(params.FieldName)
	return SelectHTTPFlowFromDB(queryDB, queryParams)
}

func HTTPFlowToOnline(db *gorm.DB, hash []string) error {
	db = db.Model(&schema.HTTPFlow{})
	db = bizhelper.ExactOrQueryStringArrayOr(db, "hash", hash)
	db = db.Update(map[string]interface{}{"upload_online": true})
	if db.Error != nil {
		return utils.Errorf("HTTPFlowToOnline failed %s", db.Error)
	}
	return nil
}

func QueryHTTPFlowsProcessNames(db *gorm.DB, params *ypb.QueryHTTPFlowRequest) ([]string, error) {
	var processNames []sql.NullString
	db = db.Model(&schema.HTTPFlow{})
	db = FilterHTTPFlow(db, params)
	db = db.Where("process_name != '' and process_name IS NOT NULL")
	if db := db.Pluck("DISTINCT(`process_name`)", &processNames); db.Error != nil {
		return nil, db.Error
	}
	return lo.FilterMap(processNames, func(item sql.NullString, _ int) (string, bool) {
		s := item.String
		return s, s != "" && item.Valid
	}), nil
}
