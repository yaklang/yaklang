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
			yakit.EnsureAIBalanceConfig()
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
			yakit.EnsureAIBalanceConfig()
			return nil
		}
	}, "sync-global-config-from-db")
}

func (s *Server) GetGlobalNetworkConfig(ctx context.Context, req *ypb.GetGlobalNetworkConfigRequest) (*ypb.GlobalNetworkConfig, error) {
	var config *ypb.GlobalNetworkConfig
	defer func() {
		raw, err := json.Marshal(config)
		if err != nil {
			log.Errorf("marshal config error: %v", err)
		}
		yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
	}()
	config = yakit.GetNetworkConfig()
	if config == nil {
		config = yakit.GetDefaultNetworkConfig()
		return config, nil
	}
	for _, appConfig := range config.AppConfigs {
		consts.ConvertCompatibleConfig(appConfig)
	}
	total := aispec.RegisteredAIGateways()
	canUseAiApiPriority := make([]string, 0)
	for _, s := range config.AiApiPriority { // remove deprecated ai type
		if utils.StringArrayContains(total, s) {
			canUseAiApiPriority = append(canUseAiApiPriority, s)
		}
	}
	config.AiApiPriority = canUseAiApiPriority
	for _, s := range total { // add new ai type
		if !utils.StringArrayContains(config.AiApiPriority, s) {
			config.AiApiPriority = append(config.AiApiPriority, s)
		}
	}
	return config, nil
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
	templates, err := buildThirdPartyAppConfigTemplates()
	if err != nil {
		return nil, err
	}
	return &ypb.GetThirdPartyAppConfigTemplateResponse{Templates: templates}, nil
}
