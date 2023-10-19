package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {

	INIT:
		if yakit.Get(consts.GLOBAL_NETWORK_CONFIG_INIT) == "" {
			log.Info("initialize global network config")
			defaultConfig := GetDefaultNetworkConfig()
			raw, err := json.Marshal(defaultConfig)
			if err != nil {
				return err
			}
			log.Infof("use config: %v", string(raw))
			yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
			ConfigureNetWork(defaultConfig)
			yakit.Set(consts.GLOBAL_NETWORK_CONFIG_INIT, "1")
			return nil
		} else {
			data := yakit.Get(consts.GLOBAL_NETWORK_CONFIG)
			if data == "" {
				yakit.Set(consts.GLOBAL_NETWORK_CONFIG_INIT, "")
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
			log.Debugf("disallow ip address: %v", config.DisallowIPAddress)
			log.Debugf("disallow domain: %v", config.DisallowDomain)
			log.Debugf("global proxy: %v", config.GlobalProxy)
			ConfigureNetWork(&config)
			return nil
		}
	})
}

func GetDefaultNetworkConfig() *ypb.GlobalNetworkConfig {
	defaultConfig := &ypb.GlobalNetworkConfig{
		DisableSystemDNS: false,
		CustomDNSServers: nil,
		DNSFallbackTCP:   false,
		DNSFallbackDoH:   false,
		CustomDoHServers: nil,
		SkipSaveHTTPFlow: false,
	}
	config := netx.NewBackupInitilizedReliableDNSConfig()
	defaultConfig.CustomDoHServers = config.SpecificDoH
	defaultConfig.CustomDNSServers = config.SpecificDNSServers
	defaultConfig.DNSFallbackDoH = config.FallbackDoH
	defaultConfig.DNSFallbackTCP = config.FallbackTCP
	defaultConfig.DisableSystemDNS = config.DisableSystemResolver
	return defaultConfig
}

func ConfigureNetWork(c *ypb.GlobalNetworkConfig) {
	if c == nil {
		return
	}

	consts.GLOBAL_HTTP_FLOW_SAVE.SetTo(!c.GetSkipSaveHTTPFlow())

	netx.SetDefaultDNSOptions(
		netx.WithDNSFallbackDoH(c.DNSFallbackDoH),
		netx.WithDNSFallbackTCP(c.DNSFallbackTCP),
		netx.WithDNSDisableSystemResolver(c.DisableSystemDNS),
		netx.WithDNSSpecificDoH(c.CustomDoHServers...),
		netx.WithDNSServers(c.CustomDNSServers...),
		netx.WithDNSDisabledDomain(c.GetDisallowDomain()...),
	)

	netx.SetDefaultDialXConfig(
		netx.DialX_WithDisallowAddress(c.GetDisallowIPAddress()...),
		netx.DialX_WithProxy(c.GetGlobalProxy()...),
		netx.DialX_WithEnableSystemProxyFromEnv(c.GetEnableSystemProxyFromEnv()),
	)

	for _, certs := range c.GetClientCertificates() {
		if len(certs.GetPkcs12Bytes()) > 0 {
			err := netx.LoadP12Bytes(certs.Pkcs12Bytes, string(certs.GetPkcs12Password()))
			if err != nil {
				log.Errorf("load p12 bytes failed: %s", err)
			}
		} else {
			p12bytes, err := tlsutils.BuildP12(certs.GetCrtPem(), certs.GetKeyPem(), "", certs.GetCaCertificates()...)
			if err != nil {
				log.Errorf("build p12 bytes failed: %s", err)
				continue
			}
			err = netx.LoadP12Bytes(p12bytes, "")
			if err != nil {
				log.Errorf("load p12 bytes failed: %s", err)
			}
		}
	}
}

func (s *Server) GetGlobalNetworkConfig(ctx context.Context, req *ypb.GetGlobalNetworkConfigRequest) (*ypb.GlobalNetworkConfig, error) {
	data := yakit.Get(consts.GLOBAL_NETWORK_CONFIG)
	if data == "" {
		defaultConfig := GetDefaultNetworkConfig()
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
	return &config, nil
}

func (s *Server) SetGlobalNetworkConfig(ctx context.Context, req *ypb.GlobalNetworkConfig) (*ypb.Empty, error) {
	defaultBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ConfigureNetWork(req)
	yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(defaultBytes))
	return &ypb.Empty{}, nil
}

func (s *Server) ResetGlobalNetworkConfig(ctx context.Context, req *ypb.ResetGlobalNetworkConfigRequest) (*ypb.Empty, error) {
	defaultConfig := GetDefaultNetworkConfig()
	raw, err := json.Marshal(defaultConfig)
	if err != nil {
		return nil, err
	}
	yakit.Set(consts.GLOBAL_NETWORK_CONFIG, string(raw))
	ConfigureNetWork(defaultConfig)
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
