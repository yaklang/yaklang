package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const GLOBAL_NETWORK_CONFIG = "GLOBAL_NETWORK_CONFIG"
const GLOBAL_NETWORK_CONFIG_INIT = "GLOBAL_NETWORK_CONFIG_INIT"

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {

	INIT:
		if yakit.Get(GLOBAL_NETWORK_CONFIG_INIT) == "" {
			log.Info("initialize global network config")
			defaultConfig := getDefaultNetworkConfig()
			raw, err := json.Marshal(defaultConfig)
			if err != nil {
				return err
			}
			log.Infof("use config: %v", string(raw))
			yakit.Set(GLOBAL_NETWORK_CONFIG, string(raw))
			loadConfig(defaultConfig)
			yakit.Set(GLOBAL_NETWORK_CONFIG_INIT, "1")
			return nil
		} else {
			data := yakit.Get(GLOBAL_NETWORK_CONFIG)
			if data == "" {
				yakit.Set(GLOBAL_NETWORK_CONFIG_INIT, "")
				goto INIT
			}
			var config ypb.GlobalNetworkConfig
			err := json.Unmarshal([]byte(data), &config)
			if err != nil {
				log.Errorf("unmarshal global network config failed: %s", err)
				return nil
			}

			log.Debugf("load global network config from database user config")
			log.Debugf("disable system dns: %v", config.DisableSystemDNS)
			log.Debugf("dns fallback tcp: %v", config.DNSFallbackTCP)
			log.Debugf("dns fallback doh: %v", config.DNSFallbackDoH)
			log.Debugf("custom dns servers: %v", config.CustomDNSServers)
			log.Debugf("custom doh servers: %v", config.CustomDoHServers)
			loadConfig(&config)
			return nil
		}
	})
}

func getDefaultNetworkConfig() *ypb.GlobalNetworkConfig {
	defaultConfig := &ypb.GlobalNetworkConfig{
		DisableSystemDNS: false,
		CustomDNSServers: nil,
		DNSFallbackTCP:   false,
		DNSFallbackDoH:   false,
		CustomDoHServers: nil,
	}
	config := netx.NewBackupInitilizedReliableDNSConfig()
	defaultConfig.CustomDoHServers = config.SpecificDoH
	defaultConfig.CustomDNSServers = config.SpecificDNSServers
	defaultConfig.DNSFallbackDoH = config.FallbackDoH
	defaultConfig.DNSFallbackTCP = config.FallbackTCP
	defaultConfig.DisableSystemDNS = config.DisableSystemResolver
	return defaultConfig
}

func loadConfig(c *ypb.GlobalNetworkConfig) {
	if c == nil {
		return
	}

	netx.SetDefaultOptions(
		netx.WithDNSFallbackDoH(c.DNSFallbackDoH),
		netx.WithDNSFallbackTCP(c.DNSFallbackTCP),
		netx.WithDNSDisableSystemResolver(c.DisableSystemDNS),
		netx.WithDNSSpecificDoH(c.CustomDoHServers...),
		netx.WithDNSServers(c.CustomDNSServers...),
	)
}

func (s *Server) GetGlobalNetworkConfig(ctx context.Context, req *ypb.GetGlobalNetworkConfigRequest) (*ypb.GlobalNetworkConfig, error) {
	data := yakit.Get(GLOBAL_NETWORK_CONFIG)
	if data == "" {
		defaultConfig := getDefaultNetworkConfig()
		raw, err := json.Marshal(defaultConfig)
		if err != nil {
			return nil, err
		}
		yakit.Set(GLOBAL_NETWORK_CONFIG, string(raw))
		return defaultConfig, nil
	}
	var config ypb.GlobalNetworkConfig
	err := json.Unmarshal([]byte(data), &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *Server) SetGlobalNetworkConfig(ctx context.Context, req *ypb.GlobalNetworkConfig) (*ypb.Empty, error) {
	defaultBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	loadConfig(req)
	yakit.Set(GLOBAL_NETWORK_CONFIG, string(defaultBytes))
	return &ypb.Empty{}, nil
}

func (s *Server) ResetGlobalNetworkConfig(ctx context.Context, req *ypb.ResetGlobalNetworkConfigRequest) (*ypb.Empty, error) {
	defaultConfig := getDefaultNetworkConfig()
	raw, err := json.Marshal(defaultConfig)
	if err != nil {
		return nil, err
	}
	yakit.Set(GLOBAL_NETWORK_CONFIG, string(raw))
	loadConfig(defaultConfig)
	return &ypb.Empty{}, nil
}
