package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

func (s *Server) UpdateFromYakitResource(ctx context.Context, req *ypb.UpdateFromYakitResourceRequest) (*ypb.Empty, error) {
	err := yakit.UpdateYakitStore(s.GetProfileDatabase(), req.GetBaseSourceUrl())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UpdateFromGithub(ctx context.Context, req *ypb.UpdateFromGithubRequest) (*ypb.Empty, error) {
	return nil, utils.Errorf("not implemeted")
}

func (s *Server) GetKey(ctx context.Context, req *ypb.GetKeyRequest) (*ypb.GetKeyResult, error) {
	result := yakit.GetKey(s.GetProfileDatabase(), req.GetKey())
	return &ypb.GetKeyResult{
		Value: utils.EscapeInvalidUTF8Byte([]byte(result)),
	}, nil
}

func (s *Server) SetKey(ctx context.Context, req *ypb.SetKeyRequest) (*ypb.Empty, error) {
	if req.GetTTL() > 0 {
		err := yakit.SetKeyWithTTL(s.GetProfileDatabase(), req.GetKey(), req.GetValue(), int(req.GetTTL()))
		if err != nil {
			return nil, err
		}
	} else {
		err := yakit.SetKey(s.GetProfileDatabase(), req.GetKey(), req.GetValue())
		if err != nil {
			return nil, err
		}
	}
	return &ypb.Empty{}, nil
}

type envBuildin struct {
	Key     string
	Value   string
	Verbose string
}

var processEnv = []*envBuildin{
	{Key: "YAKIT_DINGTALK_WEBHOOK", Verbose: "设置钉钉机器人 Webhook，可用于接受漏洞等信息"},
	{Key: "YAKIT_DINGTALK_SECRET", Verbose: "设置钉钉机器人 Webhook 的密码（SecretKey）"},
	{Key: "YAKIT_WORKWX_WEBHOOK", Verbose: "设置企业微信机器人 Webhook，可用于接受漏洞等信息"},
	{Key: "YAKIT_WORKWX_SECRET", Verbose: "设置企业微信机器人 Webhook 的密码（SecretKey）"},
	{Key: "YAKIT_FEISHU_WEBHOOK", Verbose: "设置飞书 Bot Webhook 地址，可用于接受漏洞等信息"},
	{Key: "YAKIT_FEISHU_SECRET", Verbose: "设置飞书 Bot Webhook 地址的密码（SecretKey）"},
	{Key: "YAK_PROXY", Verbose: "设置 Yaklang 引擎的代理配置"},
	{Key: consts.CONST_YAK_EXTRA_DNS_SERVERS, Verbose: "设置 Yaklang 引擎的额外 DNS 服务器（逗号分隔）"},
	{Key: consts.CONST_YAK_OVERRIDE_DNS_SERVERS, Verbose: "是否使用用户配置 DNS 覆盖原有 DNS？（true/false）"},
}
var onceInitProcessEnv = new(sync.Once)

func (s *Server) GetAllProcessEnvKey(ctx context.Context, req *ypb.Empty) (*ypb.GetProcessEnvKeyResult, error) {
	var result []*ypb.GeneralStorage

	onceInitProcessEnv.Do(func() {
		for _, k := range processEnv {
			yakit.InitKey(s.GetProfileDatabase(), k.Key, k.Verbose, true)
		}
	})

	for _, k := range yakit.GetProcessEnvKey(s.GetProfileDatabase()) {
		if k.Key == "" || k.Key == `""` {
			continue
		}
		result = append(result, k.ToGRPCModel())
	}
	return &ypb.GetProcessEnvKeyResult{Results: result}, nil
}

func (s *Server) SetProcessEnvKey(ctx context.Context, req *ypb.SetKeyRequest) (*ypb.Empty, error) {
	if req.GetKey() == "" {
		return nil, utils.Errorf("empty key")
	}
	_, err := s.SetKey(ctx, req)
	if err != nil {
		return nil, err
	}
	yakit.SetKeyProcessEnv(s.GetProfileDatabase(), req.GetKey(), true)
	yakit.RefreshProcessEnv(s.GetProfileDatabase())
	return &ypb.Empty{}, nil
}

func (s *Server) DelKey(ctx context.Context, req *ypb.GetKeyRequest) (*ypb.Empty, error) {
	key, err := yakit.GetKeyModel(s.GetProfileDatabase(), req.GetKey())
	if err != nil {
		return nil, err
	}

	if key.ProcessEnv {
		s.SetProcessEnvKey(ctx, &ypb.SetKeyRequest{
			Key: req.GetKey(), Value: "",
		})
	}
	yakit.DelKey(s.GetProfileDatabase(), req.GetKey())

	return &ypb.Empty{}, nil
}


func (s *Server) GetProjectKey(ctx context.Context, req *ypb.GetKeyRequest) (*ypb.GetKeyResult, error) {
	result := yakit.GetProjectKey(s.GetProjectDatabase(), req.GetKey())
	return &ypb.GetKeyResult{
		Value: utils.EscapeInvalidUTF8Byte([]byte(result)),
	}, nil
}

func (s *Server) SetProjectKey(ctx context.Context, req *ypb.SetKeyRequest) (*ypb.Empty, error) {
	if req.GetTTL() > 0 {
		err := yakit.SetProjectKeyWithTTL(s.GetProjectDatabase(), req.GetKey(), req.GetValue(), int(req.GetTTL()))
		if err != nil {
			return nil, err
		}
	} else {
		err := yakit.SetProjectKey(s.GetProjectDatabase(), req.GetKey(), req.GetValue())
		if err != nil {
			return nil, err
		}
	}
	return &ypb.Empty{}, nil
}