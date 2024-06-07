package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
	INIT:
		if yakit.Get(consts.GLOBAL_NETWORK_CONFIG_INIT) == "" {
			log.Info("initialize global network config")
			defaultConfig := yakit.GetDefaultNetworkConfig()
			raw, err := json.Marshal(defaultConfig)
			if err != nil {
				return err
			}
			log.Infof("use config: %v", string(raw))
			yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
			yakit.ConfigureNetWork(defaultConfig)
			yakit.Set(consts.GLOBAL_NETWORK_CONFIG_INIT, "1")
			return nil
		} else {
			config := yakit.GetNetworkConfig()
			if config == nil {
				yakit.Set(consts.GLOBAL_NETWORK_CONFIG_INIT, "")
				goto INIT
			}
			log.Debugf("load global network config from database user config")
			log.Debugf("disable system dns: %v", config.DisableSystemDNS)
			log.Debugf("dns fallback tcp: %v", config.DNSFallbackTCP)
			log.Debugf("dns fallback doh: %v", config.DNSFallbackDoH)
			log.Debugf("custom dns servers: %v", config.CustomDNSServers)
			log.Debugf("custom doh servers: %v", config.CustomDoHServers)
			log.Debugf("disallow ip address: %v", config.DisallowIPAddress)
			log.Debugf("disallow domain: %v", config.DisallowDomain)
			log.Debugf("global proxy: %v", config.GlobalProxy)
			yakit.ConfigureNetWork(config)
			return nil
		}
	})
}

func (s *Server) GetGlobalNetworkConfig(ctx context.Context, req *ypb.GetGlobalNetworkConfigRequest) (*ypb.GlobalNetworkConfig, error) {
	data := yakit.Get(consts.GLOBAL_NETWORK_CONFIG)
	if data == "" {
		defaultConfig := yakit.GetDefaultNetworkConfig()
		raw, err := json.Marshal(defaultConfig)
		if err != nil {
			return nil, err
		}
		yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
		return defaultConfig, nil
	}
	var config ypb.GlobalNetworkConfig
	err := json.Unmarshal([]byte(data), &config)
	if err != nil {
		return nil, err
	}
	if len(config.AiApiPriority) == 0 {
		config.AiApiPriority = aispec.RegisteredAIGateways()
	}
	return &config, nil
}

func (s *Server) SetGlobalNetworkConfig(ctx context.Context, req *ypb.GlobalNetworkConfig) (*ypb.Empty, error) {
	defaultBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	yakit.ConfigureNetWork(req)
	yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(defaultBytes))
	return &ypb.Empty{}, nil
}

func (s *Server) ResetGlobalNetworkConfig(ctx context.Context, req *ypb.ResetGlobalNetworkConfigRequest) (*ypb.Empty, error) {
	defaultConfig := yakit.GetDefaultNetworkConfig()
	raw, err := json.Marshal(defaultConfig)
	if err != nil {
		return nil, err
	}
	yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
	yakit.ConfigureNetWork(defaultConfig)
	return &ypb.Empty{}, nil
}

func (s *Server) ValidP12PassWord(ctx context.Context, req *ypb.ValidP12PassWordRequest) (*ypb.ValidP12PassWordResponse, error) {
	data := true
	if len(req.GetPkcs12Bytes()) > 0 {
		_, _, _, err := tlsutils.LoadP12ToPEM(req.GetPkcs12Bytes(), string(req.GetPkcs12Password()))
		if err != nil {
			data = false
		}
	}
	return &ypb.ValidP12PassWordResponse{IsSetPassWord: data}, nil
}

func (s *Server) GetThirdPartyAppConfigTemplate(ctx context.Context, _ *ypb.Empty) (*ypb.GetThirdPartyAppConfigTemplateResponse, error) {
	newConfigTemplate := func(name, verbose, typeName string, hookOpt func(option *ypb.ThirdPartyAppConfigItemTemplate), opts ...*ypb.ThirdPartyAppConfigItemTemplate) *ypb.GetThirdPartyAppConfigTemplate {
		var copyedOpts []*ypb.ThirdPartyAppConfigItemTemplate
		for _, option := range opts {
			newOpt := &ypb.ThirdPartyAppConfigItemTemplate{
				Name:         option.Name,
				Type:         option.Type,
				Verbose:      option.Verbose,
				Required:     option.Required,
				DefaultValue: option.DefaultValue,
				Desc:         option.Desc,
				Extra:        option.Extra,
			}
			if hookOpt != nil {
				hookOpt(newOpt)
			}
			copyedOpts = append(copyedOpts, newOpt)
		}
		return &ypb.GetThirdPartyAppConfigTemplate{
			Name:    name,
			Verbose: verbose,
			Items:   copyedOpts,
			Type:    typeName,
		}
	}
	opts := make([]*ypb.GetThirdPartyAppConfigTemplate, 0)

	aiOptions, err := utils.LoadAppConfig(&aispec.AIConfig{})
	if err != nil {
		return nil, err
	}

	for _, name := range aispec.RegisteredAIGateways() {
		hook := func(template *ypb.ThirdPartyAppConfigItemTemplate) {}
		verbose := name
		switch name {
		case "openai":

			verbose = "OpenAI"
		case "chatglm":
			verbose = "ChatGLM"
		case "comate":
			verbose = "Comate"
			hook = func(option *ypb.ThirdPartyAppConfigItemTemplate) {
				if option.Name == "APIKey" {
					option.Required = false
				}
			}
		case "moonshot":
			verbose = "Moonshot"
		case "tongyi":
			verbose = "Tongyi"
		}
		opts = append(opts, newConfigTemplate(name, verbose, "ai", hook, aiOptions...))
	}

	return &ypb.GetThirdPartyAppConfigTemplateResponse{Templates: opts}, nil
}
