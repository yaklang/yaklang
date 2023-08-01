package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakdns"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const GLOBAL_NETWORK_CONFIG = "GLOBAL_NETWORK_CONFIG"

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		data := yakit.Get(GLOBAL_NETWORK_CONFIG)
		if data == "" {
			log.Info("initialize global network config")
			defaultConfig := getDefaultNetworkConfig()
			raw, err := json.Marshal(defaultConfig)
			if err != nil {
				return err
			}
			yakit.Set(GLOBAL_NETWORK_CONFIG, string(raw))
			loadConfig(defaultConfig)
			return nil
		}

		var config ypb.GlobalNetworkConfig
		err := json.Unmarshal([]byte(data), &config)
		if err != nil {
			log.Errorf("unmarshal global network config failed: %s", err)
			return nil
		}

		log.Info("load global network config from database user config")
		log.Infof("disable system dns: %v", config.DisableSystemDNS)
		log.Infof("dns fallback tcp: %v", config.DNSFallbackTCP)
		log.Infof("dns fallback doh: %v", config.DNSFallbackDoH)
		log.Infof("custom dns servers: %v", config.CustomDNSServers)
		log.Infof("custom doh servers: %v", config.CustomDoHServers)
		loadConfig(&config)
		return nil
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
	config := yakdns.NewBackupInitilizedReliableDNSConfig()
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

	yakdns.SetDefaultOptions(
		yakdns.WithDNSFallbackDoH(c.DNSFallbackDoH),
		yakdns.WithDNSFallbackTCP(c.DNSFallbackTCP),
		yakdns.WithDNSDisableSystemResolver(c.DisableSystemDNS),
		yakdns.WithDNSSpecificDoH(c.CustomDoHServers...),
		yakdns.WithDNSServers(c.CustomDNSServers...),
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
	err := json.Unmarshal([]byte(data), &req)
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
