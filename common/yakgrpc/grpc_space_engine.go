package yakgrpc

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	uuid "github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
	"github.com/yaklang/yaklang/common/utils/spacengine/fofa"
	"github.com/yaklang/yaklang/common/utils/spacengine/go-shodan"
	"github.com/yaklang/yaklang/common/utils/spacengine/hunter"
	"github.com/yaklang/yaklang/common/utils/spacengine/quake"
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

func (s *Server) GetSpaceEngineAccountStatus(ctx context.Context, req *ypb.GetSpaceEngineAccountStatusRequest) (result *ypb.SpaceEngineStatus, err error) {
	key := req.GetKey()
	domain := req.GetDomain()

	result = &ypb.SpaceEngineStatus{
		Type:   req.GetType(),
		Status: SPACE_ENGINE_STATUS_NORMAL,
	}
	var client base.IUserProfile
	switch req.GetType() {
	case SPACE_ENGINE_ZOOMEYE:
		client = zoomeye.NewClientEx(key, domain)
	case SPACE_ENGINE_SHODAN:
		client = shodan.NewClientEx(key, domain)
	case SPACE_ENGINE_HUNTER:
		client = hunter.NewClientEx(key, domain)
	case SPACE_ENGINE_QUAKE:
		client = quake.NewClientEx(key, domain)
	case SPACE_ENGINE_FOFA:
		client = fofa.NewClientEx(req.GetAccount(), key, domain)
	default:
		result.Status = SPACE_ENGINE_STATUS_INVALID_TYPE
		return
	}

	result.Info = "普通账户"
	if key == "" {
		result.Status = SPACE_ENGINE_STATUS_EMPTY_KEY
		result.Info = fmt.Sprintf("%s API Key为空", strings.ToUpper(req.GetType()))
		return result, nil
	}

	bodyRaw, err := client.UserProfile()
	if err != nil {
		result.Status = SPACE_ENGINE_STATUS_ERROR
		result.Info = err.Error()
		return result, nil
	}
	gjsonResult := gjson.ParseBytes(bodyRaw)

	switch req.GetType() {
	case SPACE_ENGINE_ZOOMEYE:
		quota := gjsonResult.Get("quota_info")
		if !quota.Exists() {
			result.Info = "ZoomEye账户信息异常"
			result.Status = SPACE_ENGINE_STATUS_ERROR
		} else {
			result.Remain = quota.Get("remain_free_quota").Int() + quota.Get("remain_pay_quota").Int()
		}
	case SPACE_ENGINE_SHODAN:
		result.Remain = -1
	case SPACE_ENGINE_HUNTER:
		if gjson.ValidBytes(bodyRaw) {
			if gjsonResult.Get("code").Int() == 401 {
				result.Status = SPACE_ENGINE_STATUS_ERROR
				result.Info = "Hunter API Key无效"
				break
			}
			remainStr := gjsonResult.Get("data.rest_quota").String()
			re := regexp.MustCompile(`\d+`)
			match := re.FindStringSubmatch(remainStr)
			if len(match) > 0 {
				remain, err := strconv.ParseInt(match[0], 10, 64)
				if err != nil {
					// 处理转换失败的情况
					result.Status = SPACE_ENGINE_STATUS_ERROR
					result.Info = "解析剩余积分失败"
					break
				} else {
					result.Remain = remain
				}
			} else {
				result.Status = SPACE_ENGINE_STATUS_ERROR
				result.Info = "解析剩余积分失败"
				break
			}
		} else {
			result.Status = SPACE_ENGINE_STATUS_ERROR
			result.Info = "返回值不是有效的JSON"
			break
		}
	case SPACE_ENGINE_QUAKE:
		data := gjsonResult.Get("data")
		result.Remain = data.Get("credit").Int() + data.Get("persistent_credit").Int()
	case SPACE_ENGINE_FOFA:
		email := req.GetAccount()
		if email == "" {
			result.Status = SPACE_ENGINE_STATUS_EMPTY_KEY
			result.Info = "FOFA Email 为空"
			break
		}
		if gjsonResult.Get("isvip").Bool() {
			result.Info = "VIP账户"
		}
		result.Remain = gjsonResult.Get("fofa_point").Int() + gjsonResult.Get("remain_free_point").Int()
	}
	return result, nil
}

func (s *Server) GetSpaceEngineStatus(ctx context.Context, req *ypb.GetSpaceEngineStatusRequest) (*ypb.SpaceEngineStatus, error) {
	config := consts.GetThirdPartyApplicationConfig(req.GetType())
	account, key, domain := config.UserIdentifier, config.APIKey, config.Domain
	return s.GetSpaceEngineAccountStatus(ctx, &ypb.GetSpaceEngineAccountStatusRequest{
		Type:    req.GetType(),
		Account: account,
		Key:     key,
		Domain:  domain,
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
