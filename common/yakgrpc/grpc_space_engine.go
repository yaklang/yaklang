package yakgrpc

import (
	"context"
	_ "embed"
	"encoding/json"
	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/spacengine"
	"github.com/yaklang/yaklang/common/utils/spacengine/fofa"
	"github.com/yaklang/yaklang/common/utils/spacengine/zoomeye"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	SPACE_ENGINE_ZOOMEYE = "zoomeye"
	SPACE_ENGINE_FOFA    = "fofa"
	SPACE_ENGINE_SHODAN  = "shodan"
	SPACE_ENGINE_HUNTER  = "hunter"
	SPACE_ENGINE_QUAKE   = "quake"

	SPACE_ENGINE_STATUS_NORMAL          = "normal"
	SPACE_ENGINE_STATUS_ERROR           = "error"
	SPACE_ENGINE_STATUS_INVALID_ACCOUNT = "invalid_account"
	SPACE_ENGINE_STATUS_EMPTY_KEY       = "empty_key"
	SPACE_ENGINE_STATUS_INVALID_TYPE    = "invalid_type"
)

func (s *Server) GetSpaceEngineAccountStatus(ctx context.Context, req *ypb.GetSpaceEngineAccountStatusRequest) (*ypb.SpaceEngineStatus, error) {
	//var status = SPACE_ENGINE_STATUS_NORMAL
	//info := "ZoomEye额度按月刷新"
	var status = SPACE_ENGINE_STATUS_INVALID_TYPE
	var info = ""
	var raw []byte
	var remain int64
	switch req.GetType() {
	case SPACE_ENGINE_ZOOMEYE:
		status = SPACE_ENGINE_STATUS_NORMAL
		info = "普通账户"
		key := req.GetKey()
		if key == "" {
			status = SPACE_ENGINE_STATUS_EMPTY_KEY
			info = "ZoomEye API Key为空"
			break
		}
		result, err := zoomeye.ZoomeyeUserProfile(key)
		if err != nil {
			status = SPACE_ENGINE_STATUS_ERROR
			info = err.Error()
			break
		}
		raw = []byte(result.Raw)
		quota := result.Get("quota_info")
		if !quota.Exists() {
			info = "ZoomEye账户信息异常"
			status = SPACE_ENGINE_STATUS_ERROR
		} else {
			remain = quota.Get("remain_free_quota").Int() + quota.Get("remain_pay_quota").Int()
		}

	case SPACE_ENGINE_SHODAN:
		status = SPACE_ENGINE_STATUS_NORMAL
		info = "普通账户"
		key := req.GetKey()
		if key == "" {
			status = SPACE_ENGINE_STATUS_EMPTY_KEY
			info = "Shodan API Key为空"
			break
		}
		result, err := spacengine.ShodanUserProfile(key)
		if err != nil {
			status = SPACE_ENGINE_STATUS_ERROR
			info = err.Error()
			break
		}
		_ = result
		remain = -1
	case SPACE_ENGINE_HUNTER:
		status = SPACE_ENGINE_STATUS_NORMAL
		info = "普通账户"
		key := req.GetKey()
		if key == "" {
			status = SPACE_ENGINE_STATUS_EMPTY_KEY
			info = "Hunter API Key为空"
			break
		}
		url := "https://hunter.qianxin.com/openApi/search?api-key=" + key + "&search=apache&page=1&page_size=1&is_web=1&start_time=2021-01-01&end_time=2021-03-01"
		isHttps, reqRaw, err := lowhttp.ParseUrlToHttpRequestRaw("GET", url)
		if err != nil {
			status = SPACE_ENGINE_STATUS_ERROR
			info = err.Error()
			break
		}
		resp, err := lowhttp.HTTP(lowhttp.WithHttps(isHttps), lowhttp.WithRequest(reqRaw))
		if err != nil {
			status = SPACE_ENGINE_STATUS_ERROR
			info = err.Error()
		}
		body := lowhttp.GetHTTPPacketBody(resp.RawPacket)
		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		if err != nil {
			status = SPACE_ENGINE_STATUS_ERROR
			info = err.Error()
			break
		}
		if utils.InterfaceToInt(result["code"]) == 401 {
			status = SPACE_ENGINE_STATUS_ERROR
			info = "Hunter API Key无效"
			break
		}
	case SPACE_ENGINE_QUAKE:
		status = SPACE_ENGINE_STATUS_NORMAL
		info = "普通账户"
		key := req.GetKey()
		if key == "" {
			status = SPACE_ENGINE_STATUS_EMPTY_KEY
			info = "Quake API Key为空"
			break
		}
		client := utils.NewQuake360Client(key)
		userInfo, err := client.UserInfo()
		if err != nil {
			status = SPACE_ENGINE_STATUS_ERROR
			info = err.Error()
			break
		}
		remain = int64(userInfo.MonthRemainingCredit)
	case SPACE_ENGINE_FOFA:
		status = SPACE_ENGINE_STATUS_NORMAL
		info = "普通账户"
		key := req.GetKey()
		if key == "" {
			status = SPACE_ENGINE_STATUS_EMPTY_KEY
			info = "FOFA API Key 为空"
			break
		}
		email := req.GetAccount()
		if email == "" {
			status = SPACE_ENGINE_STATUS_EMPTY_KEY
			info = "FOFA Email 为空"
			break
		}
		client := fofa.NewFofaClient(email, key)
		user, err := client.UserInfo()
		if err != nil {
			status = SPACE_ENGINE_STATUS_ERROR
			info = err.Error()
			break
		}
		if user.Vip {
			info = "VIP账户"
		}
		remain = user.RemainApiQuery
	default:
		status = SPACE_ENGINE_STATUS_INVALID_TYPE
	}
	return &ypb.SpaceEngineStatus{
		Type:   req.GetType(),
		Status: status,
		Info:   info,
		Raw:    raw,
		Remain: remain,
	}, nil
}
func (s *Server) GetSpaceEngineStatus(ctx context.Context, req *ypb.GetSpaceEngineStatusRequest) (*ypb.SpaceEngineStatus, error) {
	account := ""
	key := ""
	switch req.GetType() {
	case SPACE_ENGINE_ZOOMEYE:
		account = consts.GetThirdPartyApplicationConfig("zoomeye").UserIdentifier
		key = consts.GetThirdPartyApplicationConfig("zoomeye").APIKey
	case SPACE_ENGINE_SHODAN:
		account = consts.GetThirdPartyApplicationConfig("shodan").UserIdentifier
		key = consts.GetThirdPartyApplicationConfig("shodan").APIKey
	case SPACE_ENGINE_HUNTER:
		account = consts.GetThirdPartyApplicationConfig("hunter").UserIdentifier
		key = consts.GetThirdPartyApplicationConfig("hunter").APIKey
	case SPACE_ENGINE_QUAKE:
		account = consts.GetThirdPartyApplicationConfig("quake").UserIdentifier
		key = consts.GetThirdPartyApplicationConfig("quake").APIKey
	case SPACE_ENGINE_FOFA:
		account = consts.GetThirdPartyApplicationConfig("fofa").UserIdentifier
		key = consts.GetThirdPartyApplicationConfig("fofa").APIKey
	}
	return s.GetSpaceEngineAccountStatus(ctx, &ypb.GetSpaceEngineAccountStatusRequest{
		Type:    req.GetType(),
		Account: account,
		Key:     key,
	})
}

//go:embed grpc_space_engine.yak
var spaceEngineExecCode string

func (s *Server) FetchPortAssetFromSpaceEngine(req *ypb.FetchPortAssetFromSpaceEngineRequest, stream ypb.Yak_FetchPortAssetFromSpaceEngineServer) error {
	streamCtx, cancel := context.WithCancel(stream.Context())
	runtimeId := uuid.New().String()
	engine := yak.NewYakitVirtualClientScriptEngine(yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		i.RuntimeID = runtimeId
		return stream.Send(i)
	}))
	stream.Send(&ypb.ExecResult{
		RuntimeID: runtimeId,
	})
	if req.PageSize == 0 {
		req.PageSize = 100
	}
	if req.MaxRecord == 0 {
		req.MaxRecord = 1000
	}
	if req.MaxPage == 0 {
		req.MaxPage = 10
	}
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVar("ENGINE_TYPE", req.GetType())
		engine.SetVar("FILTER", req.GetFilter())
		engine.SetVar("MAX_PAGE", req.GetMaxPage())
		engine.SetVar("MAX_RECORD", req.GetMaxRecord())
		engine.SetVar("PAGE_SIZE", req.GetPageSize())
		engine.SetVar("SCAN_VERIFY", req.GetScanBeforeSave())
		engine.SetVar("CONCURRENT", req.GetConcurrent())
		yak.BindYakitPluginContextToEngine(
			engine,
			yak.CreateYakitPluginContext(runtimeId).
				WithPluginName(`space-engine`).
				WithProxy(req.GetProxy()).WithContext(streamCtx).WithContextCancel(cancel),
		)
		return nil
	})
	return engine.ExecuteWithContext(streamCtx, spaceEngineExecCode)
}
